package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/apiwat/apw-agent/internal/provider/mock"
)

func TestExecutor_EndToEndWithSelfHeal(t *testing.T) {
	ws := t.TempDir()

	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module example.com/e2e\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	initialMain := "package main\n\nfunc Greet() string {\n\treturn \"hi\"\n}\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(ws, "main.go"), []byte(initialMain), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Architect returns plan with patch + verify
	planResp := "```json\n" + `{
		"objective": "Update Greet return value",
		"affected_files": ["main.go"],
		"subtasks": [
			{
				"id": "step-1",
				"description": "Patch Greet",
				"action": "patch",
				"target_file": "main.go",
				"instruction": "Change return to hello"
			},
			{
				"id": "step-2",
				"description": "Verify",
				"action": "verify",
				"instruction": "Run verification"
			}
		],
		"acceptance_criteria": ["builds cleanly"]
	}` + "\n```"

	// 2. First patch introduces a typo (undefinedVar) so Reflex must heal it
	firstPatchResp := `{"old_content": "return \"hi\"", "new_content": "return undefinedVar"}`

	// 3. Reflex repair fixes undefinedVar -> "hello"
	reflexFixResp := `{"target_file": "main.go", "old_content": "return undefinedVar", "new_content": "return \"hello\""}`

	mp := mock.New(planResp, firstPatchResp, reflexFixResp)
	exec := New(ws, mp, mp, "mock-arch", "mock-reflex", 3)

	report, err := exec.Execute(context.Background(), "Change Greet to return hello", "File: main.go")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !report.Passed {
		t.Fatalf("expected report.Passed = true after self-heal, got false")
	}
	if len(report.ReflexAttempts) != 2 {
		t.Fatalf("expected 2 reflex attempts (1 fail + 1 healed pass), got %d", len(report.ReflexAttempts))
	}

	finalBytes, err := os.ReadFile(filepath.Join(ws, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(finalBytes) != "package main\n\nfunc Greet() string {\n\treturn \"hello\"\n}\n\nfunc main() {}\n" {
		t.Errorf("unexpected final main.go content:\n%s", string(finalBytes))
	}
}
