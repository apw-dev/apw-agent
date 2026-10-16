package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/core"
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

	runCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	var tmpOutputFile string
	if p.backend == BackendCodex {
		tmpOutputFile = filepath.Join(os.TempDir(), fmt.Sprintf("apw_codex_%d.txt", time.Now().UnixNano()))
		defer os.Remove(tmpOutputFile)
	}

	args := p.buildArgs(model, fullPrompt, tmpOutputFile)
	cmd := exec.CommandContext(runCtx, p.binary, args...)
	if p.backend == BackendCodex {
		cmd.Stdin = strings.NewReader(fullPrompt)
	} else {
		cmd.Stdin = strings.NewReader("") // Crucial: close stdin so CLI does not hang waiting for input
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout.String())
		}
		if errMsg == "" {
			errMsg = err.Error()
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
	if content == "" {
		return nil, fmt.Errorf("%s CLI returned empty response", p.binary)
	}

	reqID := req.RequestID
	if reqID == "" {
		reqID = fmt.Sprintf("%s-%d", p.backend, time.Now().UnixNano())
	}

	return &core.ModelResponse{
		RequestID:     reqID,
		Provider:      string(p.backend),
		ModelObserved: model,
		Content:       content,
		Completed:     true,
		RawResponse:   content,
		Timestamp:     time.Now(),
	}, nil
}

func (p *Provider) buildArgs(model, prompt, tmpOutputFile string) []string {
	switch p.backend {
	case BackendClaude:
		args := []string{"-p", prompt, "--output-format", "text", "--tools", ""}
		if model != "" {
			args = append(args, "--model", model)
		}
		return args
	case BackendCodex:
		args := []string{"exec", "--ephemeral", "--skip-git-repo-check"}
		if tmpOutputFile != "" {
			args = append(args, "-o", tmpOutputFile)
		}
		// In ChatGPT account mode, codex uses gpt-6.1-sol by default
		if model != "" && model != "gpt-6.1-sol" {
			args = append(args, "--model", model)
		}
		// Instruct codex to read instructions from stdin via '-' to avoid Windows CLI length limit (32KB)
		args = append(args, "-")
		return args
	default: // BackendAGY
		args := []string{"-p", prompt, "--output-format", "text", "--disable-slash-commands"}
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
