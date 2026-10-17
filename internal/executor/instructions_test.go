package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/project"
)

func TestProjectInstructionsPreferAPWAndAreBounded(t *testing.T) {
	root := t.TempDir()
	if name, _ := ProjectInstructions(root); name != "" || InstructionsBlock(root) != "" {
		t.Fatal("instructions found in an empty workspace")
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Use tabs."), 0644); err != nil {
		t.Fatal(err)
	}
	if name, text := ProjectInstructions(root); name != "AGENTS.md" || text != "Use tabs." {
		t.Fatalf("AGENTS.md fallback = %q %q", name, text)
	}
	if err := os.WriteFile(filepath.Join(root, "APW.md"), []byte("ใช้ภาษาไทยในข้อความ UI "+strings.Repeat("ก", 6000)), 0644); err != nil {
		t.Fatal(err)
	}
	name, text := ProjectInstructions(root)
	if name != "APW.md" || !strings.HasPrefix(text, "ใช้ภาษาไทย") || !strings.HasSuffix(text, "[instructions truncated]") || len(text) > maxInstructions+40 {
		t.Fatalf("APW.md = %q (%d bytes)", name, len(text))
	}
	if !strings.Contains(strings.TrimSuffix(text, "\n[instructions truncated]"), "ก") || !utf8Valid(text) {
		t.Fatal("truncation split a multi-byte character")
	}
	overview := ProjectOverview(root, project.Detect(root, nil))
	if !strings.Contains(overview, "Project instructions (from APW.md") || !strings.Contains(overview, "never override APW's rules") {
		t.Fatalf("overview lacks instructions:\n%.400s", overview)
	}
}

func TestDirectivePromptCarriesInstructions(t *testing.T) {
	var prompt string
	p := &routingProvider{name: "codex-cli", ask: func(_ context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
		prompt = req.Prompt
		return &core.ModelResponse{Completed: true, Content: `{"old_content":"a","new_content":"b"}`}, nil
	}}
	ex := New(t.TempDir(), p, p, "m", "m", 1)
	if err := os.WriteFile(filepath.Join(ex.Workspace, "APW.md"), []byte("Never use any."), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ex.askDirective(context.Background(), "patch"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Never use any.") {
		t.Fatalf("directive prompt lacks project instructions: %q", prompt)
	}
}

func utf8Valid(s string) bool { return strings.ToValidUTF8(s, "\uFFFD") == s }
