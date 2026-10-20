package surgeon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v, output: %s", strings.Join(args, " "), err, string(out))
		}
	}

	run("init")
	run("config", "user.name", "APW Agent Test")
	run("config", "user.email", "agent@apw.local")
	run("config", "commit.gpgsign", "false")

	initialFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(initialFile, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	run("add", "main.go")
	run("commit", "-m", "Initial commit")

	return dir
}

func TestIsGitRepo(t *testing.T) {
	tempNonGit := t.TempDir()
	if IsGitRepo(tempNonGit) {
		t.Errorf("expected empty temp dir to not be a git repo")
	}

	gitDir := initTestGitRepo(t)
	if !IsGitRepo(gitDir) {
		t.Errorf("expected initialized git repo to return true")
	}
}

func TestWorktree_CreateAndCleanup(t *testing.T) {
	repoDir := initTestGitRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := WorktreeOptions{
		TaskID:         "test-task-1",
		BranchPrefix:   "apw-test/",
		SyncDirtyFiles: false,
	}

	wt, err := CreateWorktree(ctx, repoDir, opts)
	if err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}
	defer wt.Cleanup(ctx)

	// Verify worktree directory exists
	if fi, err := os.Stat(wt.Path); err != nil || !fi.IsDir() {
		t.Fatalf("expected worktree directory %q to exist", wt.Path)
	}

	// Verify base commit is populated
	if wt.BaseCommit == "" {
		t.Fatalf("expected non-empty BaseCommit")
	}

	// Make an edit inside the worktree
	wtMain := filepath.Join(wt.Path, "main.go")
	newCode := "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"isolated\") }\n"
	if err := os.WriteFile(wtMain, []byte(newCode), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify main workspace is STILL pristine
	origBytes, err := os.ReadFile(filepath.Join(repoDir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(origBytes), "isolated") {
		t.Fatalf("original workspace file was unexpectedly modified before apply")
	}

	// Test Diff
	diff, err := wt.Diff(ctx)
	if err != nil {
		t.Fatalf("wt.Diff failed: %v", err)
	}
	if !strings.Contains(diff, "isolated") {
		t.Fatalf("expected diff to contain 'isolated', got:\n%s", diff)
	}

	// Apply verified changes back to original workspace
	if err := wt.ApplyToWorkspace([]string{"main.go"}); err != nil {
		t.Fatalf("ApplyToWorkspace failed: %v", err)
	}

	// Verify main workspace now has the new content
	updatedBytes, err := os.ReadFile(filepath.Join(repoDir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(updatedBytes) != newCode {
		t.Fatalf("expected main workspace to be updated, got:\n%s", string(updatedBytes))
	}

	// Test Cleanup
	if err := wt.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Worktree directory should be gone
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("expected worktree directory to be removed after cleanup")
	}
}

func TestWorktree_SyncDirtyFiles(t *testing.T) {
	repoDir := initTestGitRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dirtyFile := filepath.Join(repoDir, "dirty.go")
	if err := os.WriteFile(dirtyFile, []byte("package main\n// uncommitted file\n"), 0644); err != nil {
		t.Fatal(err)
	}

	opts := WorktreeOptions{
		TaskID:         "test-task-dirty",
		SyncDirtyFiles: true,
	}

	wt, err := CreateWorktree(ctx, repoDir, opts)
	if err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}
	defer wt.Cleanup(ctx)

	// Verify dirty.go was synced into worktree
	wtDirty := filepath.Join(wt.Path, "dirty.go")
	data, err := os.ReadFile(wtDirty)
	if err != nil {
		t.Fatalf("expected dirty.go to be present in worktree: %v", err)
	}
	if !strings.Contains(string(data), "uncommitted file") {
		t.Fatalf("expected dirty file content in worktree")
	}
}
