package reflex

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/apiwat/apw-agent/internal/process"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"go/format"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Diagnostic represents a parsed compiler or linter issue
type Diagnostic struct {
	Stage    string `json:"stage"` // fmt, vet, build, test
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Col      int    `json:"col,omitempty"`
	Message  string `json:"message"`
	RawError string `json:"raw_error"`
}

// VerificationResult contains outcomes of all verification stages
type VerificationResult struct {
	Passed      bool          `json:"passed"`
	FailedStage string        `json:"failed_stage,omitempty"`
	Diagnostics []Diagnostic  `json:"diagnostics,omitempty"`
	RawOutput   string        `json:"raw_output,omitempty"`
	Duration    time.Duration `json:"duration"`
}

// Verifier manages the multi-stage verification pipeline
type Verifier struct {
	Workspace string
	Timeout   time.Duration
	Packages  []string
}

// NewVerifier creates a new Verifier
func NewVerifier(workspace string, packages ...string) *Verifier {
	pkgs := packages
	if len(pkgs) == 0 {
		pkgs = []string{"./..."}
	}
	return &Verifier{
		Workspace: workspace,
		Timeout:   60 * time.Second,
		Packages:  pkgs,
	}
}

var errRegex = regexp.MustCompile(`(?m)^\s*(?:vet(?:\.exe)?:\s+)?(\S.*?\.go):(\d+):(?:(\d+):)?\s*(.+)$`)

// ParseGoErrors converts raw compiler/vet/test output into structured Diagnostics
func ParseGoErrors(stage, raw string) []Diagnostic {
	var diagnostics []Diagnostic
	matches := errRegex.FindAllStringSubmatch(raw, -1)
	for _, m := range matches {
		if len(m) >= 5 {
			line, _ := strconv.Atoi(m[2])
			col := 0
			if m[3] != "" {
				col, _ = strconv.Atoi(m[3])
			}
			diagnostics = append(diagnostics, Diagnostic{
				Stage:    stage,
				File:     m[1],
				Line:     line,
				Col:      col,
				Message:  strings.TrimSpace(m[4]),
				RawError: m[0],
			})
		}
	}

	if len(diagnostics) == 0 && strings.TrimSpace(raw) != "" {
		diagnostics = append(diagnostics, Diagnostic{
			Stage:    stage,
			Message:  "unstructured failure: " + strings.TrimSpace(raw),
			RawError: raw,
		})
	}

	return diagnostics
}

// RunCommand executes a command inside workspace directory with timeout
func (v *Verifier) RunCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, v.Timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...)
	cmd.Dir = v.Workspace

	var outBuf process.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	err := process.Run(cmdCtx, cmd)
	if err != nil && cmdCtx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("%s %s: timed out after %v: %w", name, strings.Join(args, " "), v.Timeout, err)
	}
	return outBuf.String(), err
}

func failed(stage, out string, err error, start time.Time) *VerificationResult {
	diags := ParseGoErrors(stage, out)
	if len(diags) == 0 && err != nil {
		diags = []Diagnostic{{Stage: stage, Message: err.Error(), RawError: err.Error()}}
	}
	return &VerificationResult{
		Passed:      false,
		FailedStage: stage,
		Diagnostics: diags,
		RawOutput:   out,
		Duration:    time.Since(start),
	}
}

// checkFormat inspects source files without traversing state, dependencies, or test fixtures.
func (v *Verifier) checkFormat(ctx context.Context) (string, error) {
	var files []string
	err := filepath.WalkDir(v.Workspace, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if path != v.Workspace {
				switch entry.Name() {
				case ".git", ".apw", ".worktrees", "vendor", "node_modules", "testdata":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(file)
		generated := false
		for line := 0; line < 20 && scanner.Scan(); line++ {
			text := scanner.Text()
			if strings.HasPrefix(text, "// Code generated ") && strings.HasSuffix(text, " DO NOT EDIT.") {
				generated = true
				break
			}
		}
		readErr := scanner.Err()
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if !generated {
			rel, err := filepath.Rel(v.Workspace, path)
			if err != nil {
				return err
			}
			files = append(files, "."+string(filepath.Separator)+rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	var output strings.Builder
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return output.String(), err
		}
		rel := strings.TrimPrefix(path, "."+string(filepath.Separator))
		data, _, err := surgeon.ReadInRoot(v.Workspace, rel)
		if err != nil {
			return output.String(), err
		}
		normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		formatted, err := format.Source(normalized)
		if err != nil {
			return fmt.Sprintf("%s:%v\n", path, err), err
		}
		if !bytes.Equal(normalized, formatted) {
			output.WriteString(path + "\n")
		}
	}
	return output.String(), nil
}

// Verify runs closed-loop verification: gofmt -> go vet -> go build -> go test
func (v *Verifier) Verify(ctx context.Context) *VerificationResult {
	start := time.Now()

	// Stage 0: gofmt
	out, err := v.checkFormat(ctx)
	if err != nil {
		return failed("fmt", out, err, start)
	}
	if files := strings.TrimSpace(out); files != "" {
		var diags []Diagnostic
		for _, f := range strings.Split(files, "\n") {
			f = strings.TrimSpace(f)
			if f != "" {
				diags = append(diags, Diagnostic{
					Stage:    "fmt",
					File:     f,
					Message:  "file is not gofmt-formatted",
					RawError: f,
				})
			}
		}
		return &VerificationResult{
			Passed:      false,
			FailedStage: "fmt",
			Diagnostics: diags,
			RawOutput:   out,
			Duration:    time.Since(start),
		}
	}

	// Stage 1: go vet
	out, err = v.RunCommand(ctx, "go", append([]string{"vet", "-mod=readonly"}, v.Packages...)...)
	if err != nil {
		return failed("vet", out, err, start)
	}

	// Stage 2: go build
	out, err = v.RunCommand(ctx, "go", append([]string{"build", "-mod=readonly", "-o", os.DevNull}, v.Packages...)...)
	if err != nil {
		return failed("build", out, err, start)
	}

	// Stage 3: go test
	out, err = v.RunCommand(ctx, "go", append([]string{"test", "-mod=readonly"}, v.Packages...)...)
	if err != nil {
		return failed("test", out, err, start)
	}

	return &VerificationResult{
		Passed:   true,
		Duration: time.Since(start),
	}
}

// Fixer receives formatted diagnostics and applies a surgical fix (typically via Surgeon)
type Fixer func(ctx context.Context, diagnostics string) error

// Attempt records the outcome of a single verification round in the repair loop
type Attempt struct {
	Number int
	Result *VerificationResult
}

// ReflexEngine orchestrates the self-correction loop
type ReflexEngine struct {
	Verifier   *Verifier
	MaxRetries int
}

// NewReflexEngine creates a new ReflexEngine
func NewReflexEngine(workspace string, maxRetries int) *ReflexEngine {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &ReflexEngine{
		Verifier:   NewVerifier(workspace),
		MaxRetries: maxRetries,
	}
}

// Run executes verify -> if failed, invokes fix and re-verifies up to MaxRetries times.
// It returns all attempts for reporting and stops immediately if ctx is cancelled or fix returns an error.
func (e *ReflexEngine) Run(ctx context.Context, fix Fixer) ([]Attempt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var attempts []Attempt
	res := e.Verifier.Verify(ctx)
	attempts = append(attempts, Attempt{
		Number: 1,
		Result: res,
	})
	if res.Passed {
		return attempts, nil
	}

	for retry := 1; retry <= e.MaxRetries; retry++ {
		if err := ctx.Err(); err != nil {
			return attempts, err
		}
		if err := fix(ctx, FormatDiagnostics(res)); err != nil {
			return attempts, err
		}
		if err := ctx.Err(); err != nil {
			return attempts, err
		}

		res = e.Verifier.Verify(ctx)
		attempts = append(attempts, Attempt{
			Number: retry + 1,
			Result: res,
		})
		if res.Passed {
			return attempts, nil
		}
	}

	return attempts, nil
}

// FormatDiagnostics formats error list into actionable markdown prompt for model reflection
func FormatDiagnostics(res *VerificationResult) string {
	if res.Passed {
		return "All checks passed."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Verification failed at stage: **%s**\n\n", res.FailedStage))
	sb.WriteString("### Diagnostics:\n")
	for _, d := range res.Diagnostics {
		if d.File != "" {
			if d.Col == 0 {
				sb.WriteString(fmt.Sprintf("- `%s:%d`: %s\n", d.File, d.Line, d.Message))
			} else {
				sb.WriteString(fmt.Sprintf("- `%s:%d:%d`: %s\n", d.File, d.Line, d.Col, d.Message))
			}
		} else {
			sb.WriteString(fmt.Sprintf("- %s\n", d.Message))
		}
	}
	sb.WriteString("\n### Raw Compiler Output:\n```\n")
	sb.WriteString(res.RawOutput)
	sb.WriteString("\n```\n")
	sb.WriteString("\n**Action required:** Apply minimal surgical correction to fix the errors above.")
	return sb.String()
}
