package surgeon

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// CommitFiles commits exactly files (workspace-relative, including deletions) on the worktree's
// branch. Repository hooks do not run. It refuses when the worktree HEAD moved past the snapshot
// base, when nothing is staged, or when anything besides files would be committed.
func (w *Worktree) CommitFiles(ctx context.Context, files []string, message string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cleanedUp {
		return "", errors.New("worktree already cleaned up")
	}
	if len(files) == 0 {
		return "", errors.New("no changed files to commit")
	}
	if strings.TrimSpace(message) == "" {
		return "", errors.New("commit message required")
	}
	head, err := GetHeadCommit(ctx, w.Path)
	if err != nil {
		return "", err
	}
	if head != w.BaseCommit {
		return "", errors.New("worktree branch already has commits beyond the snapshot base")
	}
	want := map[string]bool{}
	args := []string{"add", "-A", "--"}
	for _, f := range files {
		if err := AllowedPath(f); err != nil {
			return "", err
		}
		rel := filepath.ToSlash(filepath.Clean(f))
		want[rel] = true
		args = append(args, rel)
	}
	if _, err := gitOutput(ctx, w.Path, args...); err != nil {
		return "", err
	}
	staged, err := gitOutput(ctx, w.Path, "diff", "--cached", "--name-only", "-z")
	if err != nil {
		return "", err
	}
	var extra []string
	count := 0
	for _, p := range strings.Split(string(staged), "\x00") {
		if p == "" {
			continue
		}
		count++
		if !want[p] {
			extra = append(extra, p)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		_, _ = gitOutput(ctx, w.Path, "reset", "-q")
		return "", fmt.Errorf("refusing to commit files outside the verified set: %s", strings.Join(extra, ", "))
	}
	if count == 0 {
		return "", errors.New("no changes to commit")
	}
	if _, err := gitOutput(ctx, w.Path, "commit", "-q", "--no-verify", "-m", message); err != nil {
		return "", err
	}
	return GetHeadCommit(ctx, w.Path)
}

var remoteBranchPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

// PushBranch pushes a local branch of the workspace repository to remote as remoteBranch, without
// running repository hooks and without force.
func PushBranch(ctx context.Context, workspace, remote, localBranch, remoteBranch string) error {
	for _, name := range []string{remote, localBranch, remoteBranch} {
		if !remoteBranchPattern.MatchString(name) || strings.Contains(name, "..") {
			return fmt.Errorf("invalid git name %q", name)
		}
	}
	_, err := gitOutput(ctx, workspace, "push", "--no-verify", remote, "refs/heads/"+localBranch+":refs/heads/"+remoteBranch)
	return err
}
