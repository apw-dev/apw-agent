package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/process"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

func checkpointPath(workspace, planID, suffix string) (string, error) {
	if !filepath.IsLocal(planID) || filepath.Base(planID) != planID || planID == "." {
		return "", fmt.Errorf("invalid checkpoint plan ID %q", planID)
	}
	return filepath.Join(workspace, ".apw", "plans", planID+suffix), nil
}

func (e *Executor) saveExecutionCheckpoint(plan *architect.ExecutionPlan, wt *surgeon.Worktree, completed int, files []string) error {
	cp := PlanCheckpoint{
		Pending: e.Pending,
		Mode:    e.Mode, Phase: e.Phase, ReadContext: e.ReadContext, Baseline: e.VerificationBaseline, VerifiedFiles: e.VerifiedFiles, Report: e.CurrentReport,
		PlanID: plan.PlanID, TaskID: plan.TaskID, Timestamp: time.Now(),
		LastSuccessStep: completed, TotalSteps: len(plan.Subtasks),
		WorktreeBranch: wt.Branch, WorktreePath: wt.Path,
		ModifiedFiles: files, Plan: plan, Failover: e.Failover,
	}
	if err := SaveCheckpoint(e.Workspace, cp); err != nil {
		return fmt.Errorf("save checkpoint (worktree retained at %s): %w", wt.Path, err)
	}
	if e.Session != nil {
		e.Session.SetWorktreeCheckpoint(wt.Branch, wt.Path)
	}
	e.emit(ProgressEvent{Stage: "checkpoint_saved", Message: fmt.Sprintf("Saved step %d/%d on %s", completed, len(plan.Subtasks), wt.Branch)})
	return nil
}

func appendModifiedFile(files *[]string, path string) {
	if path == "" {
		return
	}
	for _, existing := range *files {
		if filepath.Clean(existing) == filepath.Clean(path) {
			return
		}
	}
	*files = append(*files, path)
}

func validateResumeWorktree(ctx context.Context, workspace string, cp PlanCheckpoint) error {
	root, err := filepath.Abs(filepath.Join(workspace, ".worktrees"))
	if err != nil {
		return err
	}
	path, err := filepath.Abs(cp.WorktreePath)
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve resume worktree directory: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve resume worktree: %w", err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(rel) || rel == "." {
		return fmt.Errorf("resume worktree must be inside %s", root)
	}
	gitValue := func(dir string, args ...string) (string, error) {
		// Use the bounded runner like every other git call: the daemon runs detached
		// without a console, and a raw exec child gets its own console window, which
		// Windows may close (exit status 0xc000013a) before git finishes.
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		var out, stderr process.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		if err := process.Run(ctx, cmd); err != nil {
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				return "", fmt.Errorf("%w: %s", err, msg)
			}
			return "", err
		}
		return strings.TrimSpace(out.String()), nil
	}
	top, err := gitValue(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("locate resume worktree: %w", err)
	}
	topInfo, err := os.Stat(top)
	if err != nil {
		return err
	}
	pathInfo, err := os.Stat(path)
	if err != nil || !os.SameFile(topInfo, pathInfo) {
		return fmt.Errorf("resume path is not a worktree root")
	}
	branch, err := gitValue(path, "branch", "--show-current")
	if err != nil || branch != cp.WorktreeBranch || branch == "" {
		return fmt.Errorf("resume worktree missing or branch does not match checkpoint")
	}
	var common []os.FileInfo
	for _, dir := range []string{workspace, path} {
		gitDir, err := gitValue(dir, "rev-parse", "--git-common-dir")
		if err != nil {
			return fmt.Errorf("validate resume repository: %w", err)
		}
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(dir, gitDir)
		}
		info, err := os.Stat(gitDir)
		if err != nil {
			return err
		}
		common = append(common, info)
	}
	if !os.SameFile(common[0], common[1]) {
		return fmt.Errorf("resume worktree belongs to a different repository")
	}
	return nil
}

func loadResumePlan(workspace string, cp PlanCheckpoint) (*architect.ExecutionPlan, error) {
	plan := cp.Plan
	if plan == nil {
		path, err := checkpointPath(workspace, cp.PlanID, ".json")
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("load exact plan %s for resume; refusing to substitute another plan: %w", cp.PlanID, err)
		}
		plan = new(architect.ExecutionPlan)
		if err := json.Unmarshal(data, plan); err != nil {
			return nil, fmt.Errorf("parse resume plan: %w", err)
		}
	}
	if plan.PlanID != cp.PlanID || plan.TaskID != cp.TaskID {
		return nil, fmt.Errorf("checkpoint and plan identity mismatch")
	}
	if err := plan.Validate(); err != nil {
		return nil, fmt.Errorf("invalid resume plan: %w", err)
	}
	if cp.TotalSteps != len(plan.Subtasks) || cp.LastSuccessStep < 0 || cp.LastSuccessStep > len(plan.Subtasks) {
		return nil, fmt.Errorf("invalid checkpoint step %d/%d for %d-step plan", cp.LastSuccessStep, cp.TotalSteps, len(plan.Subtasks))
	}
	return plan, nil
}
