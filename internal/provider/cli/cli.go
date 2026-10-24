package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/process"
	"github.com/apiwat/apw-agent/internal/provider"
)

// BackendType identifies which authenticated CLI subscription tool to invoke.
type BackendType string

const (
	BackendAGY    BackendType = "agy"
	BackendClaude BackendType = "claude-cli"
	BackendCodex  BackendType = "codex-cli"
)

// Provider implements provider.Provider by bridging to an authenticated local CLI subscription (agy, claude, or codex).
type Provider struct {
	backend      BackendType
	binary       string
	defaultModel string
	timeout      time.Duration
}

var _ provider.Provider = (*Provider)(nil)

// New creates a new CLI-bridged provider for agy, claude-cli, or codex-cli.
func New(backend BackendType, defaultModel string, timeout time.Duration) *Provider {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	bin := "agy"
	switch backend {
	case BackendClaude:
		bin = "claude"
	case BackendCodex:
		bin = "codex"
	}
	return &Provider{
		backend:      backend,
		binary:       bin,
		defaultModel: strings.TrimSpace(defaultModel),
		timeout:      timeout,
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return string(p.backend)
}

// Ask executes a single non-interactive prompt via the authenticated CLI binary.
func (p *Provider) Ask(ctx context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.defaultModel
	}

	fullPrompt := strings.TrimSpace(req.Prompt)
	if fullPrompt == "" {
		return nil, errors.New("cli provider: prompt cannot be empty")
	}
	if p.backend == BackendAGY && !strings.Contains(fullPrompt, "CRITICAL: Do NOT use any tools") {
		fullPrompt = "CRITICAL: Do NOT use any tools, commands, or file readers. Act purely as an offline text generator.\n" + fullPrompt
	}

	runCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	scratch, err := os.MkdirTemp("", "apw-model-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)
	var tmpOutputFile string
	if p.backend == BackendCodex {
		tmpOutputFile = filepath.Join(scratch, "answer.txt")
		defer os.Remove(tmpOutputFile)
	}

	args := p.buildArgs(model, fullPrompt, tmpOutputFile)
	cmd := exec.CommandContext(runCtx, p.binary, args...)
	cmd.Dir = scratch
	cmd.Stdin = strings.NewReader(fullPrompt)

	var stdout, stderr process.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := process.Run(runCtx, cmd); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s CLI parent request stopped (model %s): %w", p.binary, model, ctx.Err())
		}
		if runCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("%s CLI timed out after %v (model %s): %w", p.binary, p.timeout, model, runCtx.Err())
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			stdoutStr := strings.TrimSpace(stdout.String())
			if stdoutStr != "" && !strings.HasPrefix(stdoutStr, "{") && !strings.HasPrefix(stdoutStr, "```") {
				errMsg = stdoutStr
			} else {
				errMsg = err.Error()
			}
		}
		return nil, fmt.Errorf("%s CLI execution failed: %s", p.binary, errMsg)
	}

	var content string
	if tmpOutputFile != "" {
		if data, err := os.ReadFile(tmpOutputFile); err == nil && len(bytes.TrimSpace(data)) > 0 {
			content = string(bytes.TrimSpace(data))
		}
	}
	if content == "" {
		content = strings.TrimSpace(stdout.String())
	}
	content = strings.TrimSpace(strings.TrimPrefix(content, "warning: --mode plan has no effect while slash command expansion is disabled."))
	if content == "" {
		errExtra := ""
		if stderrStr := strings.TrimSpace(stderr.String()); stderrStr != "" {
			errExtra = fmt.Sprintf(" (stderr: %s)", stderrStr)
		}
		return nil, fmt.Errorf("%s CLI returned empty response%s", p.binary, errExtra)
	}

	reqID := req.RequestID
	if reqID == "" {
		reqID = fmt.Sprintf("%s-%d", p.backend, time.Now().UnixNano())
	}

	return &core.ModelResponse{
		ModelRequested: model,
		ModelEvidence:  modelEvidence(stderr.String()),
		RequestID:      reqID,
		Provider:       string(p.backend),
		ModelObserved:  observedModel(stderr.String()),
		Content:        content,
		Completed:      true,
		RawResponse:    content,
		Timestamp:      time.Now(),
	}, nil
}

var modelHeader = regexp.MustCompile(`(?m)^\s*model:\s*([^\s]+)\s*$`)

func observedModel(streams ...string) string {
	for _, stream := range streams {
		if match := modelHeader.FindStringSubmatch(stream); len(match) > 1 {
			return match[1]
		}
	}
	return ""
}
func modelEvidence(streams ...string) string {
	if observedModel(streams...) != "" {
		return "CLI model header (not an independent server attestation)"
	}
	return "explicit --model argument; CLI emitted no model identity metadata"
}

func (p *Provider) buildArgs(model, prompt, tmpOutputFile string) []string {
	switch p.backend {
	case BackendClaude:
		args := []string{"-p", "--output-format", "text", "--tools", ""}
		if model != "" {
			args = append(args, "--model", model)
		}
		return args
	case BackendCodex:
		args := []string{"exec", "--ephemeral", "--skip-git-repo-check", "--sandbox", "read-only", "--ignore-user-config"}
		if tmpOutputFile != "" {
			args = append(args, "-o", tmpOutputFile)
		}
		// In ChatGPT account mode, codex uses gpt-6.1-sol by default
		if model != "" {
			args = append(args, "--model", model)
		}
		// Instruct codex to read instructions from stdin via '-' to avoid Windows CLI length limit (32KB)
		args = append(args, "-")
		return args
	default: // BackendAGY
		// Prompts are streamed through Stdin to completely eliminate Windows 32KB command line limits and quote escaping corruption
		args := []string{"--output-format", "text", "--disable-slash-commands", "--mode", "plan", "--sandbox"}
		if model != "" {
			args = append(args, "--model", model)
		}
		return args
	}
}

// Close releases any provider resources.
func (p *Provider) Close() error {
	return nil
}
