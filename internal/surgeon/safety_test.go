package surgeon

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotIncludesRenamesDeletesAndUntrackedDirectories(t *testing.T) {
	ws := initTestGitRepo(t)
	cmd := exec.Command("git", "mv", "main.go", "renamed file.go")
	cmd.Dir = ws
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rename: %s %v", out, err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "new dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "new dir", "ภาษาไทย.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	wt, err := CreateWorktree(context.Background(), ws, WorktreeOptions{TaskID: "snapshot", SyncDirtyFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	defer wt.Cleanup(context.Background())
	if _, err := os.Stat(filepath.Join(wt.Path, "main.go")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("deleted/renamed source reappeared")
	}
	for _, path := range []string{"renamed file.go", filepath.Join("new dir", "ภาษาไทย.go")} {
		if _, _, err := ReadInRoot(wt.Path, path); err != nil {
			t.Fatal(err)
		}
	}
	dirty, err := GetDirtyFiles(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(dirty, "|")
	if !strings.Contains(joined, "main.go") || !strings.Contains(joined, "renamed file.go") || !strings.Contains(joined, "ภาษาไทย.go") {
		t.Fatalf("incomplete dirty paths: %v", dirty)
	}
}

func TestApplyPreflightsWholeSetAndPreservesConcurrentChanges(t *testing.T) {
	ws := initTestGitRepo(t)
	if err := os.WriteFile(filepath.Join(ws, "second.go"), []byte("package main\n// old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	wt, err := CreateWorktree(context.Background(), ws, WorktreeOptions{TaskID: "conflict", SyncDirtyFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	defer wt.Cleanup(context.Background())
	if err = WriteInRoot(wt.Path, "main.go", []byte("package main\n// generated\nfunc main() {}\n"), 0644, false); err != nil {
		t.Fatal(err)
	}
	if err = WriteInRoot(wt.Path, "second.go", []byte("package main\n// generated\n"), 0644, false); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(filepath.Join(ws, "main.go"))
	userEdit := []byte("package main\n// edited by user\n")
	if err = os.WriteFile(filepath.Join(ws, "second.go"), userEdit, 0644); err != nil {
		t.Fatal(err)
	}
	if err = wt.ApplyToWorkspace([]string{"main.go", "second.go"}); !errors.Is(err, ErrWorkspaceConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	a, _ := os.ReadFile(filepath.Join(ws, "main.go"))
	b, _ := os.ReadFile(filepath.Join(ws, "second.go"))
	if string(a) != string(original) || string(b) != string(userEdit) {
		t.Fatal("preflight conflict changed workspace files")
	}
}

func TestRecoveryRollsBackAnInterruptedSetAndRetainsUserEdits(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		ws := initTestGitRepo(t)
		before, data, err := readState(ws, "main.go")
		if err != nil {
			t.Fatal(err)
		}
		afterData := []byte("package main\n// applied\nfunc main() {}\n")
		after := FileState{Exists: true, Hash: CalculateHash(string(afterData)), Mode: 0644}
		j := applyJournal{TaskID: "interrupted", State: "prepared", Entries: []applyEntry{{Path: "main.go", Before: before, BeforeData: data, After: after, AfterData: afterData}}}
		path, _ := journalPath(ws, j.TaskID)
		if err = saveJournal(path, j); err != nil {
			t.Fatal(err)
		}
		if err = WriteInRoot(ws, "main.go", afterData, 0644, false); err != nil {
			t.Fatal(err)
		}
		if conflict {
			if err = WriteInRoot(ws, "main.go", []byte("user edit"), 0644, false); err != nil {
				t.Fatal(err)
			}
		}
		err = RecoverTransactions(ws)
		actual, _ := os.ReadFile(filepath.Join(ws, "main.go"))
		if conflict {
			if !errors.Is(err, ErrWorkspaceConflict) || string(actual) != "user edit" {
				t.Fatalf("overwrote later edit: %s %v", actual, err)
			}
		} else if err != nil || string(actual) != string(data) {
			t.Fatalf("rollback failed: %s %v", actual, err)
		}
	}
}

func TestRootOperationsRejectLinksAndProtectedPaths(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.go")
	if err := os.WriteFile(secret, []byte("do not change"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, _, err := ReadInRoot(ws, "escape/secret.go"); err == nil {
		t.Fatal("read escaped root")
	}
	if err := WriteInRoot(ws, "escape/new.go", []byte("bad"), 0644, false); err == nil {
		t.Fatal("write escaped root")
	}
	if err := RemoveInRoot(ws, "escape/secret.go"); err == nil {
		t.Fatal("delete escaped root")
	}
	for _, path := range []string{"../escape.go", ".git/config", ".apw/auth.json", ".env"} {
		if err := WriteInRoot(ws, path, nil, 0644, false); err == nil {
			t.Fatalf("allowed protected path %s", path)
		}
	}
	data, _ := os.ReadFile(secret)
	if string(data) != "do not change" {
		t.Fatal("outside file changed")
	}
}

func TestExistingWorktreeIsNeverDestructivelyReplaced(t *testing.T) {
	ws := initTestGitRepo(t)
	opts := WorktreeOptions{TaskID: "same"}
	wt, err := CreateWorktree(context.Background(), ws, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.Cleanup(context.Background())
	if err = os.WriteFile(filepath.Join(wt.Path, "keep.txt"), []byte("work in progress"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = CreateWorktree(context.Background(), ws, opts); !errors.Is(err, ErrWorktreeExists) {
		t.Fatalf("expected collision, got %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(wt.Path, "keep.txt")); string(data) != "work in progress" {
		t.Fatal("existing worktree was removed")
	}
}
