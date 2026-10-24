package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/provider/mock"
	"github.com/apiwat/apw-agent/internal/session"
	"github.com/apiwat/apw-agent/internal/surgeon"
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
	initExecutorRepo(t, ws)
	exec := New(ws, mp, mp, "mock-arch", "mock-reflex", 3)
	exec.Trusted = true

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

	applyPreparedTest(t, exec, report)
	finalBytes, err := os.ReadFile(filepath.Join(ws, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(finalBytes) != "package main\n\nfunc Greet() string {\n\treturn \"hello\"\n}\n\nfunc main() {}\n" {
		t.Errorf("unexpected final main.go content:\n%s", string(finalBytes))
	}
}

func TestExecutor_WithGitWorktreeIsolation(t *testing.T) {
	ws := t.TempDir()

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = ws
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}

	runGit("init")
	runGit("config", "user.name", "APW Agent Test")
	runGit("config", "user.email", "test@apw.local")
	runGit("config", "commit.gpgsign", "false")
	runGit("config", "core.autocrlf", "false")

	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module example.com/wt\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	initialMain := "package main\n\nfunc Version() string {\n\treturn \"v1.0\"\n}\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(ws, "main.go"), []byte(initialMain), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "go.mod", "main.go")
	runGit("commit", "-m", "Initial commit")

	planResp := "```json\n" + `{
		"objective": "Bump Version to v2.0",
		"affected_files": ["main.go"],
		"subtasks": [
			{
				"id": "step-1",
				"description": "Patch Version",
				"action": "patch",
				"target_file": "main.go",
				"instruction": "Change return to v2.0"
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

	patchResp := `{"old_content": "\treturn \"v1.0\"", "new_content": "\treturn \"v2.0\""}`
	summaryResp := "สรุปผลการทำงาน: อัปเดตเวอร์ชัน v2.0 เรียบร้อยแล้วใน worktree"

	mp := mock.New(planResp, patchResp, summaryResp)
	initExecutorRepo(t, ws)
	executor := New(ws, mp, mp, "mock-arch", "mock-reflex", 3)
	executor.Trusted = true

	var stages []string
	executor.OnProgress = func(ev ProgressEvent) {
		stages = append(stages, ev.Stage)
	}

	report, err := executor.Execute(context.Background(), "Bump version to v2.0", "File: main.go")
	if err != nil {
		t.Fatalf("Execute with worktree failed: %v", err)
	}
	if !report.Passed {
		t.Fatalf("expected report.Passed = true, got false")
	}

	// Verify progress recorded worktree stages
	foundWorktree := false
	foundApplied := false
	for _, s := range stages {
		if s == "worktree_ready" {
			foundWorktree = true
		}
		if s == "applied" {
			foundApplied = true
		}
	}
	if !foundWorktree {
		t.Errorf("expected worktree_ready progress stage in %v", stages)
	}
	if foundApplied {
		t.Fatal("execute applied changes without an explicit apply request")
	}
	applyPreparedTest(t, executor, report)

	// Verify final content in main workspace
	finalBytes, err := os.ReadFile(filepath.Join(ws, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(finalBytes), "v2.0") {
		t.Errorf("expected updated version in workspace, got:\n%s", string(finalBytes))
	}
}

func TestExecutor_WithSessionContinuity(t *testing.T) {
	ws := t.TempDir()

	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module example.com/sess\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	planResp := "```json\n" + `{
		"objective": "Add helper func",
		"affected_files": ["main.go"],
		"subtasks": [
			{
				"id": "step-1",
				"description": "Add Helper",
				"action": "patch",
				"target_file": "main.go",
				"instruction": "Add func Helper() string { return \"ok\" }"
			}
		],
		"acceptance_criteria": ["builds cleanly"]
	}` + "\n```"
	patchResp := `{"old_content": "func main() {}\n", "new_content": "func Helper() string { return \"ok\" }\n\nfunc main() {}\n"}`
	summaryResp := "Added helper function successfully"

	mp := mock.New(planResp, patchResp, summaryResp)
	initExecutorRepo(t, ws)
	ex := New(ws, mp, mp, "mock-arch", "mock-reflex", 2)
	ex.Trusted = true
	sess := session.New(ws)
	sess.RecordTurn(session.Turn{Instruction: "Previous task context", ModifiedFiles: []string{"main.go"}})
	ex.Session = sess

	report, err := ex.Execute(context.Background(), "Add helper func", "Context info")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !report.Passed {
		t.Fatalf("expected report.Passed = true")
	}

	if len(mp.Requests) == 0 || !strings.Contains(mp.Requests[0].Prompt, "Previous task context") {
		t.Fatal("planning lost supplied session context")
	}
	if !strings.Contains(ex.ReadContext, "func main()") {
		t.Fatal("patch read context did not retain actual source")
	}

}

func TestParsePatchDirective_SanitizesInvalidEscapes(t *testing.T) {
	// 1. Exact case from user log: invalid escape "\ " in string
	rawWithInvalidEscape := `{"old_content": "import (\n\t\"bytes\"\ \n)", "new_content": "import (\n\t\"bytes\"\n)"}`
	dir, err := parsePatchDirective(rawWithInvalidEscape)
	if err != nil {
		t.Fatalf("expected parsePatchDirective to succeed with sanitized escape, got: %v", err)
	}
	if !strings.Contains(dir.OldContent, "bytes") {
		t.Errorf("expected OldContent to contain bytes, got: %q", dir.OldContent)
	}

	// 2. Raw unescaped newlines inside JSON string literal
	rawWithRawNewline := "{\n\"old_content\": \"line 1\nline 2\",\n\"new_content\": \"line 1\nline 2 edited\"\n}"
	dir2, err := parsePatchDirective(rawWithRawNewline)
	if err != nil {
		t.Fatalf("expected parsePatchDirective to succeed with raw newlines, got: %v", err)
	}
	if !strings.Contains(dir2.NewContent, "edited") {
		t.Errorf("expected NewContent to contain edited, got: %q", dir2.NewContent)
	}

	// 3. Conversational wrapper with markdown block and backslash escapes
	rawMarkdown := "Here is the surgical patch:\n```json\n{\"old_content\": \"foo\\ bar\", \"new_content\": \"foo bar\"}\n```\nHope this helps!"
	dir3, err := parsePatchDirective(rawMarkdown)
	if err != nil {
		t.Fatalf("expected parsePatchDirective to extract from markdown, got: %v", err)
	}
	if dir3.NewContent != "foo bar" {
		t.Errorf("expected NewContent to be 'foo bar', got: %q", dir3.NewContent)
	}
}

func initExecutorRepo(t *testing.T, ws string) {
	t.Helper()
	if surgeon.IsGitRepo(ws) {
		return
	}
	for _, args := range [][]string{{"init"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"}, {"config", "commit.gpgsign", "false"}, {"config", "core.autocrlf", "false"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = ws
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
	}
}
func applyPreparedTest(t *testing.T, e *Executor, r *ExecutionReport) {
	t.Helper()
	cp, err := LoadCheckpoint(e.Workspace, r.Plan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Apply(context.Background(), *cp); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteSubtask_PatchMissingFileCreatesAndBarePromptPathPinned(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	specDir := filepath.Join(ws, "docs", "superpowers", "specs")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "design.md"), []byte("# CrowdSec Spec\nTask 0: setup .gitignore\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctxStr := CodeContextForInstruction(ws, "อ่าน spec เต็มก่อนลงมือ: docs/superpowers/specs/design.md")
	if !strings.Contains(ctxStr, "# CrowdSec Spec") {
		t.Fatalf("expected bare path docs/superpowers/specs/design.md to be pinned in context, got:\n%s", ctxStr)
	}

	planResp := "```json\n" + `{
		"objective": "Create .gitignore via patch action",
		"affected_files": [".gitignore"],
		"subtasks": [
			{
				"id": "step-0",
				"description": "Read missing .gitignore",
				"action": "read",
				"target_file": ".gitignore",
				"instruction": "Check existing .gitignore"
			},
			{
				"id": "step-1",
				"description": "Add .env to .gitignore",
				"action": "patch",
				"target_file": ".gitignore",
				"instruction": "Ignore .env"
			}
		],
		"acceptance_criteria": [".gitignore exists"],
		"checks": [{"criterion": ".gitignore exists", "kind": "file_exists", "file": ".gitignore"}]
	}` + "\n```"
	createResp := `{"file_content": ".env\n__pycache__/\n"}`
	mp := mock.New(planResp, createResp)
	initExecutorRepo(t, ws)
	ex := New(ws, mp, mp, "mock-arch", "mock-reflex", 1)
	ex.Trusted = true

	report, err := ex.Execute(context.Background(), "Setup .gitignore", "")
	if err != nil {
		t.Fatalf("Execute failed when patching missing .gitignore: %v", err)
	}
	if !report.Passed {
		t.Fatalf("expected report.Passed = true")
	}
}

