//go:build windows

package surgeon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func mklinkJunction(t *testing.T, link, target string) {
	t.Helper()
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("create test junction: %s %v", out, err)
	}
}

func TestExternalRootIsUserOnly(t *testing.T) {
	repo := initTestGitRepo(t)
	base := filepath.Join(t.TempDir(), "wt")
	wt, err := CreateWorktree(context.Background(), repo, WorktreeOptions{TaskID: "task-1", Root: base})
	if err != nil {
		t.Fatal(err)
	}
	defer wt.Cleanup(context.Background())
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{base, filepath.Dir(wt.Path)} {
		sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		sddl := sd.String()
		dacl := sddl[strings.Index(sddl, "D:"):]
		// Protected (no inherited entries) and granting only the current user and SYSTEM.
		if !strings.HasPrefix(dacl, "D:P") || strings.Count(dacl, "(A;") != 2 || !strings.Contains(dacl, sid.String()) || !strings.Contains(dacl, ";;;SY)") {
			t.Fatalf("%s DACL = %s, want protected user+SYSTEM only", dir, dacl)
		}
	}
}

func TestExternalRootRefusesJunctions(t *testing.T) {
	repo := initTestGitRepo(t)
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(t.TempDir(), "wt")
	wt, err := CreateWorktree(context.Background(), repo, WorktreeOptions{TaskID: "task-1", Root: base})
	if err != nil {
		t.Fatal(err)
	}
	defer wt.Cleanup(context.Background())
	dir := filepath.Dir(wt.Path)

	// A junction planted as a task directory is never located, removed or treated as a worktree.
	planted := filepath.Join(dir, "planted")
	mklinkJunction(t, planted, outside)
	if _, _, err := LocateWorktree(repo, base, planted); err == nil {
		t.Fatal("junction task directory accepted")
	}
	fake := &Worktree{Workspace: repo, Path: planted, Branch: "apw/planted", BaseCommit: wt.BaseCommit}
	if err := RemoveRetainedWorktree(context.Background(), repo, base, fake); err == nil {
		t.Fatal("worktree removed through a junction")
	}

	// A junction replacing the per-workspace directory is refused for create, locate and listing.
	moved := t.TempDir()
	gitIn(t, repo, "worktree", "move", wt.Path, filepath.Join(moved, "w"))
	wt.Path = filepath.Join(moved, "w")
	if err := os.Remove(planted); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	mklinkJunction(t, dir, outside)
	if _, err := CreateWorktree(context.Background(), repo, WorktreeOptions{TaskID: "task-2", Root: base}); err == nil {
		t.Fatal("worktree created through a junction workspace directory")
	}
	if _, _, err := LocateWorktree(repo, base, filepath.Join(dir, "keep.txt")); err == nil {
		t.Fatal("path under a junction workspace directory accepted")
	}
	for _, listed := range WorktreeDirs(repo, base) {
		if listed == dir {
			t.Fatal("junction workspace directory listed for cleanup")
		}
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep" {
		t.Fatalf("outside file changed: %q %v", data, err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 1 {
		t.Fatalf("outside directory modified: %v", entries)
	}
	_ = os.Remove(dir)
}
