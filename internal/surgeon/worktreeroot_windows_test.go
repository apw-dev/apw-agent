//go:build windows

package surgeon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

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
		// Compare ACE SIDs rather than SDDL text: SDDL prints some accounts as aliases
		// (the built-in Administrator on CI runners renders as "LA", not its SID).
		if !strings.HasPrefix(dacl, "D:P") || !daclGrantsOnly(t, sd, sid) {
			t.Fatalf("%s DACL = %s, want protected user+SYSTEM only", dir, dacl)
		}
	}
}

// daclGrantsOnly reports whether the DACL holds exactly two allow entries: one for user and one
// for SYSTEM.
func daclGrantsOnly(t *testing.T, sd *windows.SECURITY_DESCRIPTOR, user *windows.SID) bool {
	t.Helper()
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatal(err)
	}
	if acl == nil || acl.AceCount != 2 {
		return false
	}
	var sawUser, sawSystem bool
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return false
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		switch {
		case sid.Equals(user):
			sawUser = true
		case sid.Equals(system):
			sawSystem = true
		default:
			return false
		}
	}
	return sawUser && sawSystem
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
