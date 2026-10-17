package reflex

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/apiwat/apw-agent/internal/project"
	"github.com/apiwat/apw-agent/internal/sandbox"
)

// maxPromptOutput bounds the raw tool output sent back to the repair model.
const maxPromptOutput = 12 * 1024

const stampName = ".apw-verify-stamp"

// verifyStages runs a detected or configured pipeline in order and stops at the
// first failing stage.
func (v *Verifier) verifyStages(ctx context.Context, p project.Profile) *VerificationResult {
	start := time.Now()
	if len(p.Stages) == 0 {
		msg := "no verification pipeline for " + p.Summary + "; add verify.stages to apw.json to tell APW how to check this project"
		return &VerificationResult{FailedStage: "verify", Diagnostics: []Diagnostic{{Stage: "verify", Message: msg, RawError: msg}}, RawOutput: msg, Duration: time.Since(start)}
	}
	for _, s := range p.Stages {
		if err := ctx.Err(); err != nil {
			return failed(s.Name, "", err, start)
		}
		switch s.Builtin {
		case project.BuiltinGoFmt:
			if res := v.goFmtResult(ctx, start); res != nil {
				res.FailedStage = s.Name
				return res
			}
			continue
		case project.BuiltinHTMLLinks:
			diags, err := checkHTMLLinks(ctx, v.Workspace, s.Name)
			if err != nil {
				return failed(s.Name, err.Error(), err, start)
			}
			if len(diags) > 0 {
				var raw strings.Builder
				for _, d := range diags {
					raw.WriteString(d.RawError + "\n")
				}
				return &VerificationResult{FailedStage: s.Name, Diagnostics: diags, RawOutput: raw.String(), Duration: time.Since(start)}
			}
			continue
		case "":
		default:
			err := fmt.Errorf("unknown builtin verification stage %q", s.Builtin)
			return failed(s.Name, err.Error(), err, start)
		}
		if s.SkipIfMissing && v.Sandbox.Mode != sandbox.ModeDocker && !project.Available(s) {
			continue
		}
		stamp := ""
		if s.Once {
			stamp = v.stampValue(s)
			if stamp != "" && v.stampMatches(s, stamp) {
				continue
			}
		}
		out, err := v.runStage(ctx, s)
		if err != nil {
			res := failed(s.Name, out, err, start)
			if hint := longPathHint(v.Workspace, out); hint != "" {
				res.Diagnostics = append([]Diagnostic{{Stage: s.Name, Message: hint, RawError: hint}}, res.Diagnostics...)
				res.RawOutput = hint + "\n" + res.RawOutput
			}
			return res
		}
		if s.Once && stamp != "" {
			v.writeStamp(s, stamp)
		}
	}
	return &VerificationResult{Passed: true, Duration: time.Since(start)}
}

// runStage runs one command stage with its environment and timeout, bounded by
// the process runner (job object / process group, output limits) and the sandbox.
func (v *Verifier) runStage(ctx context.Context, s project.Stage) (string, error) {
	if len(s.Command) == 0 {
		return "", fmt.Errorf("stage %s has no command", s.Name)
	}
	timeout := project.TimeoutOf(s)
	out, err := v.run(ctx, timeout, sandbox.Spec{Args: s.Command, Env: s.Env, Install: s.Once, Optional: s.SkipIfMissing})
	var notFound *exec.Error
	if err != nil && errors.As(err, &notFound) {
		err = fmt.Errorf("%s is not installed or not on PATH: %w", s.Command[0], err)
	}
	return out, err
}

// stampValue hashes the stage command and its input files; "" disables stamping.
func (v *Verifier) stampValue(s project.Stage) string {
	if s.StampDir == "" {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(strings.Join(s.Command, "\x00")))
	for _, in := range s.OnceInputs {
		data, err := os.ReadFile(filepath.Join(v.Workspace, in))
		if err != nil {
			return ""
		}
		h.Write([]byte{0})
		h.Write([]byte(in))
		h.Write([]byte{0})
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (v *Verifier) stampMatches(s project.Stage, value string) bool {
	data, err := os.ReadFile(filepath.Join(v.Workspace, s.StampDir, stampName))
	return err == nil && strings.TrimSpace(string(data)) == value
}

// writeStamp records a successful install inside StampDir (ignored by git, so it
// is never reported as a worktree change). Failure only costs a reinstall.
func (v *Verifier) writeStamp(s project.Stage, value string) {
	dir := filepath.Join(v.Workspace, s.StampDir)
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, stampName), []byte(value+"\n"), 0644)
}

var htmlRefPattern = regexp.MustCompile(`(?i)\b(?:src|href)\s*=\s*["']([^"']+)["']`)

// checkHTMLLinks reports local src/href references in .html files that do not exist.
func checkHTMLLinks(ctx context.Context, root, stage string) ([]Diagnostic, error) {
	var diags []Diagnostic
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if path != root {
				switch d.Name() {
				case ".git", ".apw", ".worktrees", "node_modules", "vendor", "dist", "build":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(path), ".html") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		rel, _ := filepath.Rel(root, path)
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for line := 1; scanner.Scan(); line++ {
			for _, m := range htmlRefPattern.FindAllStringSubmatch(scanner.Text(), -1) {
				ref := strings.TrimSpace(m[1])
				if skipRef(ref) {
					continue
				}
				if i := strings.IndexAny(ref, "?#"); i >= 0 {
					ref = ref[:i]
				}
				if ref == "" {
					continue
				}
				target := filepath.Join(filepath.Dir(path), filepath.FromSlash(ref))
				if strings.HasPrefix(ref, "/") {
					target = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(ref, "/")))
				}
				if _, err := os.Stat(target); err != nil {
					msg := "missing local reference " + m[1]
					diags = append(diags, Diagnostic{Stage: stage, File: filepath.ToSlash(rel), Line: line, Message: msg, RawError: fmt.Sprintf("%s:%d: %s", filepath.ToSlash(rel), line, msg)})
				}
			}
		}
		return scanner.Err()
	})
	return diags, err
}

func skipRef(ref string) bool {
	lower := strings.ToLower(ref)
	for _, prefix := range []string{"#", "//", "mailto:", "tel:", "data:", "javascript:", "{{", "{%", "${"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return strings.Contains(lower, "://")
}

var (
	ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	// src/App.tsx(12,5): error TS2322: ...
	tscParenPattern = regexp.MustCompile(`^\s*(\S[^():]*?\.[A-Za-z0-9]+)\((\d+),(\d+)\):\s*(.+)$`)
	// path/file.ext:12:5 - error ... | path/file.ext:12:5: msg | --> src/main.rs:2:5 | ❯ src/a.test.ts:10:5
	fileLineColPattern = regexp.MustCompile(`^\s*(?:-->\s*|❯\s*|at\s+)?(\S+\.[A-Za-z0-9]+):(\d+):(\d+)\b[\s:-]*(.*)$`)
	// path/file.ext:12: msg (no column)
	fileLinePattern = regexp.MustCompile(`^\s*(\S+\.[A-Za-z0-9]+):(\d+):\s+(.+)$`)
	// Python: File "app/main.py", line 3
	pythonPattern = regexp.MustCompile(`^\s*File "([^"]+)", line (\d+)`)
	// ESLint stylish row under a file header: "  12:5  error  message  rule"
	eslintRowPattern = regexp.MustCompile(`^\s+(\d+):(\d+)\s+(error|warning)\s+(.+)$`)
	// Vitest/Jest failing test: " FAIL  src/App.test.tsx > renders"
	testFailPattern = regexp.MustCompile(`^\s*(?:FAIL|×|✗)\s+(\S+\.[A-Za-z0-9]+)(.*)$`)
)

// ParseDiagnostics extracts file/line diagnostics from Go, TypeScript, ESLint,
// Vitest/Jest, Rust, Python and generic "file:line:col" tool output.
func ParseDiagnostics(stage, raw string) []Diagnostic {
	clean := ansiPattern.ReplaceAllString(raw, "")
	// ParseGoErrors falls back to one "unstructured failure" entry when it finds no
	// Go positions; keep that only if no other tool format matches either.
	var diags, unstructured []Diagnostic
	for _, d := range ParseGoErrors(stage, clean) {
		if d.File == "" {
			unstructured = append(unstructured, d)
		} else {
			diags = append(diags, d)
		}
	}
	seen := map[string]bool{}
	for _, d := range diags {
		seen[fmt.Sprintf("%s:%d:%d", d.File, d.Line, d.Col)] = true
	}
	add := func(file string, line, col int, msg, rawLine string) {
		key := fmt.Sprintf("%s:%d:%d", file, line, col)
		if seen[key] || len(diags) >= 50 {
			return
		}
		seen[key] = true
		diags = append(diags, Diagnostic{Stage: stage, File: file, Line: line, Col: col, Message: strings.TrimSpace(msg), RawError: strings.TrimSpace(rawLine)})
	}
	eslintFile := ""
	for _, line := range strings.Split(clean, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := tscParenPattern.FindStringSubmatch(line); m != nil {
			add(m[1], atoi(m[2]), atoi(m[3]), m[4], line)
			continue
		}
		if m := fileLineColPattern.FindStringSubmatch(line); m != nil && !strings.HasSuffix(m[1], ".go") {
			add(m[1], atoi(m[2]), atoi(m[3]), m[4], line)
			continue
		}
		if m := pythonPattern.FindStringSubmatch(line); m != nil {
			add(m[1], atoi(m[2]), 0, line, line)
			continue
		}
		if m := eslintRowPattern.FindStringSubmatch(line); m != nil && eslintFile != "" {
			add(eslintFile, atoi(m[1]), atoi(m[2]), m[3]+": "+m[4], line)
			continue
		}
		if m := testFailPattern.FindStringSubmatch(line); m != nil {
			add(m[1], 0, 0, "failing test"+m[2], line)
			continue
		}
		if m := fileLinePattern.FindStringSubmatch(line); m != nil && !strings.HasSuffix(m[1], ".go") {
			add(m[1], atoi(m[2]), 0, m[3], line)
			continue
		}
		// ESLint stylish prints the file path alone on a line before its rows.
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.ContainsAny(trimmed, " \t") && filepath.Ext(trimmed) != "" && (strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\")) {
			eslintFile = trimmed
		} else if trimmed == "" {
			eslintFile = ""
		}
	}
	if len(diags) == 0 {
		return unstructured
	}
	return diags
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// tailOutput keeps the last max bytes of s (tool output ends with the failure
// summary) without splitting a UTF-8 rune.
func tailOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	start := len(s) - max
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return "[earlier output omitted]\n" + s[start:]
}

// isInstallStage reports whether name is the profile's dependency-install stage.
func (v *Verifier) isInstallStage(name string) bool {
	if v.Profile == nil || name == "" {
		return false
	}
	for _, s := range v.Profile.Stages {
		if s.Name == name {
			return s.Once
		}
	}
	return false
}

// windowsPathBudget approximates how much of the 260-character Windows path limit
// a deep dependency tree (for example pnpm's node_modules/.pnpm layout) needs.
const windowsPathBudget = 260 - 120

// longPathHint explains a "file not found" failure that is most likely caused by
// the Windows path length limit rather than by the project.
func longPathHint(workspace, out string) string {
	if runtime.GOOS != "windows" || len(workspace) <= windowsPathBudget || !strings.Contains(out, "ENOENT") {
		return ""
	}
	return fmt.Sprintf("likely Windows path length limit: the isolated worktree path is %d characters and dependency paths below it exceed 260; set a short worktree_root in apw.json (for example D:\\apw-wt) or enable Windows long paths", len(workspace))
}
