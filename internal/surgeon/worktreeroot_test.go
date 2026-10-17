package surgeon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
	return string(out)
}

// moveToLegacy relocates a worktree to <workspace>/.worktrees/<task-id>, the layout used before the
// external root, and rewrites its manifest as an older APW would have.
func moveToLegacy(t *testing.T, workspace string, wt *Worktree) {
	t.Helper()
	legacy := filepath.Join(workspace, LegacyWorktreeDir, wt.TaskID)
	if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
		t.Fatal(err)
	}
	gitIn(t, workspace, "worktree", "move", wt.Path, legacy)
	wt.Path = legacy
	if err := wt.SaveManifest(); err != nil {
		t.Fatal(err)
	}
}

func TestTaskWorktreeName(t *testing.T) {
	for id, want := range map[string]string{
		"task-0123456789abcdef0123456789abcdef": "0123456789ab",
		"task-1":                                "task-1",
		"resume-test":                           "resume-test",
		"weird/../id":                           "weird----id",
	} {
		if got := TaskWorktreeName(id); got != want {
			t.Errorf("TaskWorktreeName(%q) = %q, want %q", id, got, want)
		}
	}
	long := TaskWorktreeName("task-1792166521189239500")
	if len(long) != 12 || long != TaskWorktreeName("task-1792166521189239500") {
		t.Fatalf("long ID name = %q, want stable 12 characters", long)
	}
}

func TestWorktreeLivesOutsideWorkspaceWithShortName(t *testing.T) {
	repo := initTestGitRepo(t)
	id := "task-0123456789abcdef0123456789abcdef"
	wt, err := CreateWorktree(context.Background(), repo, WorktreeOptions{TaskID: id, SyncDirtyFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	defer wt.Cleanup(context.Background())
	dir, err := WorktreeDir(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if wt.Path != filepath.Join(dir, "0123456789ab") || wt.Branch != "apw/"+id {
		t.Fatalf("worktree %s on %s, want %s on apw/%s", wt.Path, wt.Branch, filepath.Join(dir, "0123456789ab"), id)
	}
	if rel, err := filepath.Rel(repo, wt.Path); err == nil && filepath.IsLocal(rel) {
		t.Fatalf("worktree %s is inside the workspace", wt.Path)
	}
	if !strings.HasPrefix(dir, os.Getenv(WorktreeRootEnv)) {
		t.Fatalf("worktree dir %s ignores %s", dir, WorktreeRootEnv)
	}
	if _, err := os.Lstat(filepath.Join(repo, LegacyWorktreeDir)); !os.IsNotExist(err) {
		t.Fatal("new task created the legacy .worktrees directory")
	}
	if status := gitIn(t, repo, "status", "--porcelain", "--untracked-files=all"); strings.TrimSpace(status) != "" {
		t.Fatalf("workspace shows APW state: %q", status)
	}
	if gotDir, name, err := LocateWorktree(repo, "", wt.Path); err != nil || gotDir != dir || name != "0123456789ab" {
		t.Fatalf("LocateWorktree = %q %q %v", gotDir, name, err)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(dir)
		if err != nil || fi.Mode().Perm() != 0700 {
			t.Fatalf("worktree directory mode = %v %v, want 0700", fi.Mode().Perm(), err)
		}
	}
	// The same task cannot be created twice.
	if _, err := CreateWorktree(context.Background(), repo, WorktreeOptions{TaskID: id}); err == nil {
		t.Fatal("duplicate worktree created")
	}
}

func TestWorktreeRootOverride(t *testing.T) {
	repo := initTestGitRepo(t)
	override := filepath.Join(t.TempDir(), "apw-wt")
	wt, err := CreateWorktree(context.Background(), repo, WorktreeOptions{TaskID: "task-1", Root: override})
	if err != nil {
		t.Fatal(err)
	}
	defer wt.Cleanup(context.Background())
	if filepath.Dir(filepath.Dir(wt.Path)) != override {
		t.Fatalf("worktree %s not under override %s", wt.Path, override)
	}
	if _, _, err := LocateWorktree(repo, override, wt.Path); err != nil {
		t.Fatal(err)
	}
	// Without the override the worktree is not in any known root.
	if _, _, err := LocateWorktree(repo, "", wt.Path); err == nil {
		t.Fatal("worktree located without its configured root")
	}
	for _, bad := range []string{filepath.Join(repo, "wt"), "relative/wt", repo} {
		if _, err := CreateWorktree(context.Background(), repo, WorktreeOptions{TaskID: "task-2", Root: bad}); err == nil {
			t.Fatalf("accepted worktree root %q", bad)
		}
	}
}

func TestLocateWorktreeRejectsPathsOutsideRoots(t *testing.T) {
	repo := initTestGitRepo(t)
	wt, err := CreateWorktree(context.Background(), repo, WorktreeOptions{TaskID: "task-1"})
	if err != nil {
		t.Fatal(err)
	}
	defer wt.Cleanup(context.Background())
	for _, path := range []string{t.TempDir(), filepath.Join(wt.Path, "nested"), filepath.Dir(wt.Path), repo} {
		if _, _, err := LocateWorktree(repo, "", path); err == nil {
			t.Fatalf("located %s as a task worktree", path)
		}
	}
	other := initTestGitRepo(t)
	if _, _, err := LocateWorktree(other, "", wt.Path); err == nil {
		t.Fatal("another workspace claimed this worktree")
	}
}

func TestLegacyWorktreeStillLoadsAppliesAndCleansUp(t *testing.T) {
	repo := initTestGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".git", "info", "exclude"), []byte(".worktrees/\n.apw/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	wt, err := CreateWorktree(context.Background(), repo, WorktreeOptions{TaskID: "task-legacy", SyncDirtyFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	moveToLegacy(t, repo, wt)
	loaded, err := LoadWorktree(wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if dir, name, err := LocateWorktree(repo, "", loaded.Path); err != nil || dir != filepath.Join(repo, LegacyWorktreeDir) || name != "task-legacy" {
		t.Fatalf("legacy worktree not located: %q %q %v", dir, name, err)
	}
	found := false
	for _, dir := range WorktreeDirs(repo, "") {
		found = found || dir == filepath.Join(repo, LegacyWorktreeDir)
	}
	if !found {
		t.Fatal("legacy directory missing from WorktreeDirs")
	}
	if err := os.WriteFile(filepath.Join(loaded.Path, "main.go"), []byte("package main\n\nfunc main() { println() }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := loaded.ApplyToWorkspaceContext(context.Background(), []string{"main.go"}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "main.go")); !strings.Contains(string(data), "println") {
		t.Fatal("legacy worktree change not applied")
	}
	gitIn(t, loaded.Path, "checkout", "--", "main.go")
	if err := RemoveRetainedWorktree(context.Background(), repo, "", loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(loaded.Path); !os.IsNotExist(err) {
		t.Fatal("legacy worktree not removed")
	}
}

func TestExternalRootRejectsSymlinks(t *testing.T) {
	repo := initTestGitRepo(t)
	outside := t.TempDir()
	base := filepath.Join(t.TempDir(), "wt")
	if err := os.Symlink(outside, base); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := CreateWorktree(context.Background(), repo, WorktreeOptions{TaskID: "task-1", Root: base}); err == nil {
		t.Fatal("worktree created through a symlinked root")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatalf("symlink target modified: %v", entries)
	}
}
