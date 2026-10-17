package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/apiwat/apw-agent/internal/core"
)

const (
	codexFixtureEnv       = "APW_CLI_TEST_CODEX_FIXTURE"
	codexFixtureReportEnv = "APW_CLI_TEST_CODEX_REPORT"
	codexFixtureAnswer    = "คำตอบจากไฟล์ fixture"
)

func TestMain(m *testing.M) {
	if os.Getenv("APW_CLI_TEST_BLOCK") == "1" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	if os.Getenv(codexFixtureEnv) == "1" {
		if err := runCodexFixture(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestAskReportsParentDeadlineWithoutClaimingLongerProviderTimeout(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("APW_CLI_TEST_BLOCK", "1")
	p := New(BackendCodex, "fixture", 5*time.Minute)
	p.binary = bin
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = p.Ask(ctx, core.ModelRequest{Prompt: "test"})
	if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "after 5m") {
		t.Fatalf("misleading timeout: %v", err)
	}
}

func codexFixturePrompt() string {
	return " \t\r\n" + strings.Repeat("ข้อความภาษาไทย 🌏\nบรรทัดถัดไป\n", 2048) + " \t\r\n"
}

func runCodexFixture() error {
	args := os.Args[1:]
	if len(args) != 11 {
		return fmt.Errorf("fixture: expected exactly 11 arguments without prompt, got %q", args)
	}
	wantArgs := []string{"exec", "--ephemeral", "--skip-git-repo-check", "--sandbox", "read-only", "--ignore-user-config", "-o", args[7], "--model", "gpt-6.1-sol", "-"}
	for i, want := range wantArgs {
		if args[i] != want {
			return fmt.Errorf("fixture: unexpected arg[%d]: got %q, want %q", i, args[i], want)
		}
	}
	if args[7] == "" {
		return fmt.Errorf("fixture: missing output path after -o")
	}

	// ReadAll only succeeds after EOF; fail explicitly if stdin stays open.
	timer := time.AfterFunc(5*time.Second, func() {
		fmt.Fprintln(os.Stderr, "fixture: timed out reading stdin through EOF")
		os.Exit(2)
	})
	input, err := io.ReadAll(os.Stdin)
	timer.Stop()
	if err != nil {
		return fmt.Errorf("fixture: read stdin: %w", err)
	}
	wantPrompt := strings.TrimSpace(codexFixturePrompt())
	if string(input) != wantPrompt {
		return fmt.Errorf("fixture: stdin differs from trimmed prompt (got %d bytes, want %d)", len(input), len(wantPrompt))
	}
	if err := os.WriteFile(args[7], []byte(codexFixtureAnswer), 0600); err != nil {
		return fmt.Errorf("fixture: write output: %w", err)
	}
	if err := os.WriteFile(os.Getenv(codexFixtureReportEnv), []byte(args[7]), 0600); err != nil {
		return fmt.Errorf("fixture: report output path: %w", err)
	}
	_, err = fmt.Fprintln(os.Stdout, "different fixture stdout response")
	return err
}

func TestAskCodexLongPromptViaStdin(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatalf("locate current test executable: %v", err)
	}
	reportPath := filepath.Join(t.TempDir(), "output-path.txt")
	t.Setenv(codexFixtureEnv, "1")
	t.Setenv(codexFixtureReportEnv, reportPath)

	req := core.ModelRequest{Prompt: codexFixturePrompt()}
	if units := len(utf16.Encode([]rune(strings.TrimSpace(req.Prompt)))); units <= 32768 {
		t.Fatalf("fixture prompt must exceed 32768 UTF-16 code units, got %d", units)
	}

	p := New(BackendCodex, "gpt-6.1-sol", 15*time.Second)
	p.binary = binary
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	response, err := p.Ask(ctx, req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatalf("Ask timed out: subprocess must receive the prompt and stdin EOF: %v", err)
		}
		t.Fatalf("Ask with subprocess fixture: %v", err)
	}
	if response == nil {
		t.Fatal("Ask returned a nil response")
	}
	if response.Content != codexFixtureAnswer {
		t.Errorf("expected answer from output file %q, got %q", codexFixtureAnswer, response.Content)
	}

	outputPath, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read fixture output path: %v", err)
	}
	if len(outputPath) == 0 {
		t.Fatal("fixture reported an empty output path")
	}
	if _, err := os.Stat(string(outputPath)); !os.IsNotExist(err) {
		t.Fatalf("expected Ask to remove output file %q, stat error: %v", outputPath, err)
	}
}

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name         string
		backend      BackendType
		model        string
		prompt       string
		tmpFile      string
		expectedArgs []string
	}{
		{
			name:         "codex backend empty model",
			backend:      BackendCodex,
			model:        "",
			prompt:       "test prompt",
			tmpFile:      "out.tmp",
			expectedArgs: []string{"exec", "--ephemeral", "--skip-git-repo-check", "--sandbox", "read-only", "--ignore-user-config", "-o", "out.tmp", "-"},
		},
		{
			name:         "codex backend default model",
			backend:      BackendCodex,
			model:        "gpt-6.1-sol",
			prompt:       "test prompt",
			tmpFile:      "out.tmp",
			expectedArgs: []string{"exec", "--ephemeral", "--skip-git-repo-check", "--sandbox", "read-only", "--ignore-user-config", "-o", "out.tmp", "--model", "gpt-6.1-sol", "-"},
		},
		{
			name:         "codex backend custom model",
			backend:      BackendCodex,
			model:        "o3-mini",
			prompt:       "test prompt",
			tmpFile:      "output dir/out.tmp",
			expectedArgs: []string{"exec", "--ephemeral", "--skip-git-repo-check", "--sandbox", "read-only", "--ignore-user-config", "-o", "output dir/out.tmp", "--model", "o3-mini", "-"},
		},
		{
			name:         "codex backend without output file",
			backend:      BackendCodex,
			model:        "o3-mini",
			prompt:       "test prompt",
			expectedArgs: []string{"exec", "--ephemeral", "--skip-git-repo-check", "--sandbox", "read-only", "--ignore-user-config", "--model", "o3-mini", "-"},
		},
		{
			name:         "agy backend empty model",
			backend:      BackendAGY,
			model:        "",
			prompt:       "hello world",
			expectedArgs: []string{"--output-format", "text", "--disable-slash-commands", "--mode", "plan", "--sandbox"},
		},
		{
			name:         "agy backend with model",
			backend:      BackendAGY,
			model:        "gemini-3.8-flash-high",
			prompt:       "hello world",
			expectedArgs: []string{"--output-format", "text", "--disable-slash-commands", "--mode", "plan", "--sandbox", "--model", "gemini-3.8-flash-high"},
		},
		{
			name:         "claude backend empty model",
			backend:      BackendClaude,
			model:        "",
			prompt:       "hello claude",
			expectedArgs: []string{"-p", "--output-format", "text", "--tools", ""},
		},
		{
			name:         "claude backend with model",
			backend:      BackendClaude,
			model:        "claude-sonnet-4-6",
			prompt:       "hello claude",
			expectedArgs: []string{"-p", "--output-format", "text", "--tools", "", "--model", "claude-sonnet-4-6"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := New(tc.backend, tc.model, 30*time.Second)
			args := p.buildArgs(tc.model, tc.prompt, tc.tmpFile)

			if len(args) != len(tc.expectedArgs) {
				t.Fatalf("expected args %q, got %q", tc.expectedArgs, args)
			}
			for i, want := range tc.expectedArgs {
				if args[i] != want {
					t.Errorf("arg[%d]: expected %q, got %q; full args: %q", i, want, args[i], args)
				}
			}

			if tc.backend == BackendCodex {
				if args[len(args)-1] != "-" {
					t.Errorf("expected last arg to be %q, got %q", "-", args[len(args)-1])
				}
				for i, arg := range args {
					if strings.Contains(arg, tc.prompt) {
						t.Errorf("expected prompt to be absent from arguments, found in arg[%d]: %q", i, arg)
					}
				}
			}
		})
	}
}

func TestAskEmptyPrompt(t *testing.T) {
	p := New(BackendCodex, "gpt-6.1-sol", 5*time.Second)
	_, err := p.Ask(context.Background(), core.ModelRequest{Prompt: "   "})
	if err == nil {
		t.Fatalf("expected error for empty prompt, got nil")
	}
}

func TestLiveAskAGY(t *testing.T) {
	if os.Getenv("APW_TEST_LIVE_AGY") != "1" {
		t.Skip("skipping live AGY test; set APW_TEST_LIVE_AGY=1 to run")
	}
	p := New(BackendAGY, "gemini-3.8-flash-high", 45*time.Second)
	res, err := p.Ask(context.Background(), core.ModelRequest{
		Prompt: "Provide surgical patch for hello.go.\nRespond ONLY with JSON: {\"old_content\": \"a\", \"new_content\": \"b\"}",
	})
	if err != nil {
		t.Fatalf("Ask AGY failed: %v", err)
	}
	t.Logf("AGY Content: %s", res.Content)
}
