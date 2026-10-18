package executor

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/provider/mock"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func readyTask(t *testing.T) (string, *ExecutionReport, *Executor) {
	t.Helper()
	ws, wt, _ := resumeFixture(t)
	if err := wt.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	plan := architect.ExecutionPlan{Objective: "Say hello", AffectedFiles: []string{"main.go"},
		Subtasks: []architect.Subtask{{ID: "patch", Action: architect.ActionPatch, TargetFile: "main.go", Instruction: "return hello"}}}
	planBytes, _ := json.Marshal(plan)
	mp := mock.New(string(planBytes), `{"old_content":"return \"hi\"","new_content":"return \"hello\""}`)
	ex := New(ws, mp, mp, "arch", "reflex", 1)
	ex.Trusted = true
	report, err := ex.Execute(context.Background(), "Say hello", "File: main.go")
	if err != nil || !report.Passed {
		t.Fatalf("execute: %+v %v", report, err)
	}
	return ws, report, ex
}

func TestCommitVerifiedAndPushLeaveWorkspaceUntouched(t *testing.T) {
	ws, report, _ := readyTask(t)
	before, _ := os.ReadFile(filepath.Join(ws, "main.go"))
	origin := filepath.Join(t.TempDir(), "origin.git")
	gitOut(t, ws, "init", "-q", "--bare", origin)
	gitOut(t, ws, "remote", "add", "origin", origin)

	pub, err := CommitVerified(context.Background(), ws, "", report.TaskID, "Fix #7: say hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(pub.Files) != 1 || pub.Files[0] != "main.go" || !strings.HasPrefix(pub.Branch, "apw/") {
		t.Fatalf("published = %+v", pub)
	}
	if after, _ := os.ReadFile(filepath.Join(ws, "main.go")); string(after) != string(before) {
		t.Fatal("source workspace changed")
	}
	if files := gitOut(t, ws, "show", "--name-only", "--format=", pub.Commit); files != "main.go" {
		t.Fatalf("commit contains %q", files)
	}
	if err := surgeon.PushBranch(context.Background(), ws, "origin", pub.Branch, "apw/issue-7-say-hello"); err != nil {
		t.Fatal(err)
	}
	if remote := gitOut(t, origin, "rev-parse", "refs/heads/apw/issue-7-say-hello"); remote != pub.Commit {
		t.Fatalf("remote branch at %s, want %s", remote, pub.Commit)
	}
	if _, err := CommitVerified(context.Background(), ws, "", report.TaskID, "again"); err == nil {
		t.Fatal("published twice")
	}
	for _, bad := range [][3]string{{"--upload-pack=x", pub.Branch, "b"}, {"origin", pub.Branch, "../escape"}, {"origin", "-f", "b"}} {
		if err := surgeon.PushBranch(context.Background(), ws, bad[0], bad[1], bad[2]); err == nil {
			t.Fatalf("PushBranch accepted %v", bad)
		}
	}
}

func TestCommitVerifiedRefusesChangedWorktree(t *testing.T) {
	ws, report, _ := readyTask(t)
	if err := os.WriteFile(filepath.Join(report.WorktreePath, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := CommitVerified(context.Background(), ws, "", report.TaskID, "Fix"); err == nil || !strings.Contains(err.Error(), "verified worktree changed") {
		t.Fatalf("changed worktree published: %v", err)
	}
}
