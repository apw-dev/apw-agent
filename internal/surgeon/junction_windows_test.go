//go:build windows

package surgeon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/apiwat/apw-agent/internal/workspace"
)

func TestWindowsJunctionsCannotRedirectSourceOrState(t *testing.T) {
	root := initTestGitRepo(t)
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "main.go")
	if err := os.WriteFile(sentinel, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	junction := func(name string) {
		t.Helper()
		cmd := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(root, name), outside)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("create test junction: %s %v", out, err)
		}
	}
	junction("escape")
	if _, _, err := ReadInRoot(root, "escape/main.go"); err == nil {
		t.Fatal("junction read escaped the source root")
	}
	if err := WriteInRoot(root, "escape/main.go", []byte("changed"), 0644, false); err == nil {
		t.Fatal("junction write escaped the source root")
	}
	if err := RemoveInRoot(root, "escape/main.go"); err == nil {
		t.Fatal("junction delete escaped the source root")
	}
	external := filepath.Join(t.TempDir(), "wt")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", external, outside).CombinedOutput(); err != nil {
		t.Fatalf("create test junction: %s %v", out, err)
	}
	if _, err := CreateWorktree(context.Background(), root, WorktreeOptions{TaskID: "junction", Root: external}); err == nil {
		t.Fatal("worktree was created through a junction")
	}
	junction(".apw")
	if lock, err := workspace.Acquire(root, "write"); err == nil {
		lock.Close()
		t.Fatal("state directory junction was accepted")
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "keep" {
		t.Fatalf("outside file changed: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "write.lock")); !os.IsNotExist(err) {
		t.Fatal("lock was created outside the workspace")
	}
}
