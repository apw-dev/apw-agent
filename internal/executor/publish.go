package executor

import (
	"context"
	"errors"
	"fmt"

	"github.com/apiwat/apw-agent/internal/surgeon"
)

// Published describes a ready task committed on its worktree branch.
type Published struct {
	TaskID string   `json:"task_id"`
	Branch string   `json:"branch"`
	Commit string   `json:"commit"`
	Files  []string `json:"files"`
}

// CommitVerified commits a ready task's verified files on its isolated worktree branch, leaving the
// source workspace untouched. It applies the same checks as apply: the checkpoint passed
// verification and acceptance, the worktree belongs to this repository, and every verified file
// is still exactly as verified.
func CommitVerified(ctx context.Context, workspace, worktreeRoot, taskID, message string) (*Published, error) {
	cp, err := CheckpointForTask(workspace, taskID)
	if err != nil {
		return nil, err
	}
	if cp.Phase != "ready" || cp.Report == nil || !cp.Report.Passed {
		return nil, errors.New("task has not passed verification and acceptance")
	}
	if len(cp.ModifiedFiles) == 0 {
		return nil, errors.New("task changed no files; nothing to publish")
	}
	if _, err := loadResumePlan(workspace, *cp); err != nil {
		return nil, err
	}
	if err := validateResumeWorktree(ctx, workspace, worktreeRoot, *cp); err != nil {
		return nil, err
	}
	wt, err := surgeon.LoadWorktree(cp.WorktreePath)
	if err != nil {
		return nil, err
	}
	if wt.TaskID != cp.TaskID || wt.Branch != cp.WorktreeBranch {
		return nil, errors.New("checkpoint does not match snapshot manifest")
	}
	if len(cp.VerifiedFiles) != len(cp.ModifiedFiles) {
		return nil, errors.New("verified file manifest is incomplete")
	}
	for _, path := range cp.ModifiedFiles {
		expected, ok := cp.VerifiedFiles[path]
		current, err := surgeon.StateInRoot(wt.Path, path)
		if !ok || err != nil || current != expected {
			return nil, fmt.Errorf("verified worktree changed: %s; reverify before publishing", path)
		}
	}
	commit, err := wt.CommitFiles(ctx, cp.ModifiedFiles, message)
	if err != nil {
		return nil, err
	}
	return &Published{TaskID: taskID, Branch: wt.Branch, Commit: commit, Files: append([]string{}, cp.ModifiedFiles...)}, nil
}
