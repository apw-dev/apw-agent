//go:build windows

package surgeon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRetentionRefusesJunctions(t *testing.T) {
	root := initTestGitRepo(t)
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "plans"), 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "plans", "p.json")
	if err := os.WriteFile(sentinel, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	junction := func(name, target string) {
		t.Helper()
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(root, name), target).CombinedOutput(); err != nil {
			t.Fatalf("create test junction: %s %v", out, err)
		}
	}
	junction(".apw", outside)
	if _, err := RemoveStateFile(root, filepath.Join(".apw", "plans"), "p.json"); err == nil {
		t.Fatal("state file removed through a junction")
	}
	if err := os.MkdirAll(filepath.Join(outside, "task-x"), 0700); err != nil {
		t.Fatal(err)
	}
	junction(".worktrees", outside)
	wt := &Worktree{Workspace: root, Path: filepath.Join(root, ".worktrees", "task-x"), Branch: "apw/task-x", BaseCommit: "0"}
	if err := RemoveRetainedWorktree(context.Background(), root, wt); err == nil {
		t.Fatal("worktree removed through a junction")
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep" {
		t.Fatalf("outside file changed: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "task-x")); err != nil {
		t.Fatal("outside directory removed")
	}
}
