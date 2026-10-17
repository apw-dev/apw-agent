package surgeon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// openStateDir opens the workspace root and checks that every component of dir is a plain directory.
// Links and Windows junctions (reported as irregular files) are refused.
func openStateDir(workspace, dir string) (*os.Root, error) {
	if !filepath.IsLocal(dir) || filepath.Clean(dir) == "." {
		return nil, fmt.Errorf("%w: %q", ErrPathEscape, dir)
	}
	r, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, err
	}
	cur := ""
	for _, part := range strings.Split(filepath.Clean(dir), string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		fi, err := r.Lstat(cur)
		if err != nil {
			r.Close()
			return nil, err
		}
		if !fi.IsDir() || fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			r.Close()
			return nil, fmt.Errorf("state directory %s must not be a link", cur)
		}
	}
	return r, nil
}

func validStateName(name string) error {
	if name == "" || name == "." || filepath.Base(name) != name || !filepath.IsLocal(name) {
		return fmt.Errorf("%w: %q", ErrPathEscape, name)
	}
	return nil
}

// RemoveStateFile removes one regular file name inside the workspace state directory dir (for example
// ".apw/plans"). It reports false when the file or directory is already absent.
func RemoveStateFile(workspace, dir, name string) (bool, error) {
	if err := validStateName(name); err != nil {
		return false, err
	}
	r, err := openStateDir(workspace, dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer r.Close()
	rel := filepath.Join(dir, name)
	fi, err := r.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !fi.Mode().IsRegular() {
		return false, fmt.Errorf("refusing to remove non-file %s", rel)
	}
	return true, r.Remove(rel)
}

// JournalState returns the state of a task's apply journal ("prepared", "committed", "rolled_back"),
// or "" when the task has none.
func JournalState(root, id string) (string, error) {
	if _, err := journalPath(root, id); err != nil {
		return "", err
	}
	r, err := openStateDir(root, filepath.Join(".apw", "transactions"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer r.Close()
	data, err := r.ReadFile(filepath.Join(".apw", "transactions", id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var j applyJournal
	if err = json.Unmarshal(data, &j); err != nil {
		return "", fmt.Errorf("unreadable apply journal for %s: %w", id, err)
	}
	if j.TaskID != id {
		return "", fmt.Errorf("apply journal identity mismatch for %s", id)
	}
	return j.State, nil
}

// RemoveRolledBackJournal deletes a task's rolled-back apply journal, which still holds full file contents.
// Committed receipts and unfinished journals are kept.
func RemoveRolledBackJournal(root, id string) (bool, error) {
	state, err := JournalState(root, id)
	if err != nil || state != "rolled_back" {
		return false, err
	}
	return RemoveStateFile(root, filepath.Join(".apw", "transactions"), id+".json")
}

// RemoveRetainedWorktree removes a task worktree and its apw/ branch once retention has selected it.
// It refuses paths outside the workspace's worktree directories (the external root or the legacy
// .worktrees), links/junctions, worktrees Git does not have registered on the recorded branch, and
// worktrees or branches that moved past the snapshot base commit. override is apw.json worktree_root.
func RemoveRetainedWorktree(ctx context.Context, workspace, override string, w *Worktree) error {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	if _, _, err := LocateWorktree(abs, override, w.Path); err != nil {
		return err
	}
	recorded, e1 := os.Stat(w.Workspace)
	current, e2 := os.Stat(abs)
	if e1 != nil || e2 != nil || !os.SameFile(recorded, current) {
		return errors.New("worktree manifest belongs to another workspace")
	}
	if !strings.HasPrefix(w.Branch, "apw/") || w.BaseCommit == "" {
		return errors.New("worktree manifest has no apw branch or base commit")
	}
	list, err := gitOutput(ctx, abs, "worktree", "list", "--porcelain")
	if err != nil {
		return err
	}
	if !registeredWorktree(string(list), w.Path, w.Branch) {
		return fmt.Errorf("worktree %s is not registered with Git on branch %s", w.Path, w.Branch)
	}
	head, err := GetHeadCommit(ctx, w.Path)
	if err != nil {
		return err
	}
	tip, err := gitOutput(ctx, abs, "rev-parse", "--verify", "refs/heads/"+w.Branch)
	if err != nil {
		return err
	}
	if head != w.BaseCommit || strings.TrimSpace(string(tip)) != w.BaseCommit {
		return errors.New("worktree branch has commits beyond the snapshot base; remove it manually after review")
	}
	return w.Cleanup(ctx)
}

func registeredWorktree(list, path, branch string) bool {
	want, err := os.Stat(path)
	if err != nil {
		return false
	}
	matched := false
	scanner := bufio.NewScanner(strings.NewReader(list))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			matched = false
		case strings.HasPrefix(line, "worktree "):
			info, err := os.Stat(filepath.FromSlash(strings.TrimPrefix(line, "worktree ")))
			matched = err == nil && os.SameFile(info, want)
		case matched && line == "branch refs/heads/"+branch:
			return true
		}
	}
	return false
}
