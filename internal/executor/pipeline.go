package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/reflex"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"github.com/apiwat/apw-agent/internal/workspace"
)

func (e *Executor) Execute(ctx context.Context, instruction, overview string) (*ExecutionReport, error) {
	if e.Mode == "" {
		e.Mode = core.ModeExecute
	}
	if !e.Mode.Valid() {
		return nil, errors.New("invalid execution mode")
	}
	if e.TaskID == "" {
		e.TaskID = fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	if e.Mode == core.ModeReview {
		return e.review(ctx, instruction, overview)
	}
	if (e.Mode == core.ModeExecute || e.Mode == core.ModeVerify) && !e.Trusted {
		return nil, errors.New("workspace is not trusted for build/test; use preview or explicitly trust this workspace")
	}
	lock, err := workspace.Acquire(e.Workspace, "write")
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if !surgeon.IsGitRepo(e.Workspace) {
		return nil, errors.New("preview/execute requires a Git repository with an initial commit")
	}
	contextText := overview + "\n" + CodeContext(e.Workspace)
	if e.Session != nil {
		contextText = e.Session.BuildContextSummary() + "\n" + contextText
	}
	e.emit(ProgressEvent{Stage: "planning", Message: "Building plan from source context"})
	var plan *architect.ExecutionPlan
	if e.Mode == core.ModeVerify {
		plan = &architect.ExecutionPlan{PlanID: "plan-" + e.TaskID, TaskID: e.TaskID, Objective: "Verify project", Subtasks: []architect.Subtask{{ID: "verify", Action: architect.ActionVerify}}, AcceptanceCriteria: []string{"builds cleanly"}}
	} else {
		plan, err = e.Architect.Plan(ctx, core.Task{ID: e.TaskID, Workspace: e.Workspace, Instruction: instruction, Mode: string(e.Mode)}, contextText)
	}

	if err != nil {
		return nil, err
	}
	for _, st := range plan.Subtasks {
		if st.Action != architect.ActionVerify {
			if err := surgeon.AllowedPath(st.TargetFile); err != nil {
				return nil, err
			}
		}
	}
	e.emit(ProgressEvent{Stage: "plan_ready", Message: plan.Objective, TotalSteps: len(plan.Subtasks)})
	e.emit(ProgressEvent{Stage: "worktree_start", Message: "Snapshotting workspace"})
	wt, err := surgeon.CreateWorktree(ctx, e.Workspace, surgeon.WorktreeOptions{TaskID: plan.TaskID, SyncDirtyFiles: true})
	if err != nil {
		return nil, err
	}
	e.emit(ProgressEvent{Stage: "worktree_ready", Message: wt.Path})
	e.Phase = "working"
	e.Pending = nil
	e.RecoveredSteps = nil
	e.VerificationBaseline = nil
	if err := e.saveExecutionCheckpoint(plan, wt, 0, nil); err != nil {
		return nil, err
	}
	if e.Mode == core.ModeExecute {
		e.emit(ProgressEvent{Stage: "baseline", Message: "Checking baseline before editing"})
		e.VerificationBaseline = reflex.NewVerifier(wt.Path).Verify(ctx)
	}
	return e.runPreparedPlan(ctx, plan, wt, 0, nil, instruction)
}

func (e *Executor) review(ctx context.Context, instruction, overview string) (*ExecutionReport, error) {
	if e.Architect == nil || e.Architect.Provider == nil {
		return nil, errors.New("review provider is not configured")
	}
	if e.Session != nil {
		overview = e.Session.BuildContextSummary() + "\n" + overview
	}
	e.emit(ProgressEvent{Stage: "reviewing", Message: "Reading source without executing repository code"})
	response, err := e.Architect.Provider.Ask(ctx, core.ModelRequest{
		RequestID: e.TaskID, Model: e.Architect.Model,
		Prompt: "Review the supplied source only. Do not run tools, commands, tests, or modify files. Cite file paths and evidence. Distinguish confirmed bugs from uncertainty.\nInstruction: " + instruction + "\n" + overview + "\n" + CodeContext(e.Workspace),
	})
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if response == nil || !response.Completed || strings.TrimSpace(response.Content) == "" {
		return nil, errors.New("review response is incomplete")
	}
	return &ExecutionReport{TaskID: e.TaskID, Mode: core.ModeReview, Passed: true, Summary: response.Content}, nil
}

func (e *Executor) Resume(ctx context.Context, cp PlanCheckpoint) (*ExecutionReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e.Mode == "" {
		e.Mode = cp.Mode
	}
	if e.Mode == "" {
		e.Mode = core.ModeExecute
	}
	if e.Mode != core.ModeExecute && e.Mode != core.ModePreview {
		return nil, errors.New("resume requires execute or preview mode")
	}
	if (e.Mode == core.ModeExecute || e.Mode == core.ModeVerify) && !e.Trusted {
		return nil, errors.New("workspace is not trusted for build/test")
	}
	lock, err := workspace.Acquire(e.Workspace, "write")
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	plan, err := loadResumePlan(e.Workspace, cp)
	if err != nil {
		return nil, err
	}
	if err = validateResumeWorktree(ctx, e.Workspace, cp); err != nil {
		return nil, err
	}
	wt, err := surgeon.LoadWorktree(cp.WorktreePath)
	if err != nil {
		return nil, err
	}
	if wt.TaskID != cp.TaskID || wt.Branch != cp.WorktreeBranch {
		return nil, errors.New("checkpoint does not match snapshot manifest")
	}
	cp.Plan = plan
	if err := recoverMutation(&cp); err != nil {
		return nil, err
	}
	e.Pending = nil
	e.RecoveredSteps = nil
	if cp.Report != nil {
		for _, step := range cp.Report.Steps {
			if step.Success {
				e.RecoveredSteps = append(e.RecoveredSteps, step)
			}
		}
	}
	e.TaskID = cp.TaskID
	e.Failover = cp.Failover
	e.ReadContext = cp.ReadContext
	e.VerificationBaseline = cp.Baseline
	e.Phase = "working"
	return e.runPreparedPlan(ctx, plan, wt, cp.LastSuccessStep, cp.ModifiedFiles, "Resume "+cp.PlanID)
}

func (e *Executor) runPreparedPlan(ctx context.Context, plan *architect.ExecutionPlan, wt *surgeon.Worktree, completed int, files []string, instruction string) (*ExecutionReport, error) {
	start := time.Now()
	modified := append([]string{}, files...)
	report := &ExecutionReport{TaskID: plan.TaskID, Plan: plan, Mode: e.Mode, WorktreePath: wt.Path, Baseline: e.VerificationBaseline, Steps: append([]StepReport{}, e.RecoveredSteps...)}
	save := func() error {
		report.ModifiedFiles = append([]string{}, modified...)
		report.Duration = time.Since(start)
		e.CurrentReport = report
		return e.saveExecutionCheckpoint(plan, wt, completed, modified)
	}
	e.mutation = func(path string, before, after surgeon.FileState) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		e.Pending = &MutationIntent{Step: completed, Path: path, Before: before, After: after, Repair: completed == len(plan.Subtasks)}
		return save()
	}
	defer func() { e.mutation = nil }()
	if err := save(); err != nil {
		return report, err
	}
	surg := surgeon.NewSurgeon()
	for i := completed; i < len(plan.Subtasks); i++ {
		if err := ctx.Err(); err != nil {
			return report, errors.Join(err, save())
		}
		st := plan.Subtasks[i]
		e.emit(ProgressEvent{Stage: "step_start", StepIndex: i + 1, TotalSteps: len(plan.Subtasks), Message: fmt.Sprintf("[%s] %s — %s", st.Action, st.ID, st.TargetFile)})
		sr, path, err := e.executeSubtask(ctx, wt.Path, surg, st)
		if err != nil {
			sr.Message = err.Error()
		}
		report.Steps = append(report.Steps, sr)
		e.emit(ProgressEvent{Stage: "step_done", StepIndex: i + 1, TotalSteps: len(plan.Subtasks), Step: &sr, Message: sr.Message})
		if err != nil {
			return report, errors.Join(err, save())
		}
		completed = i + 1
		e.Pending = nil
		appendModifiedFile(&modified, path)
		if err = save(); err != nil {
			return report, err
		}
	}
	if e.Mode == core.ModePreview {
		e.Phase = "preview"
		report.Summary = "Preview prepared in isolated worktree. No repository commands were executed; verification and apply are still required."
	} else {
		e.emit(ProgressEvent{Stage: "verifying", Message: "Verifying isolated changes"})
		engine := reflex.NewReflexEngine(wt.Path, e.Reflex.MaxRetries)
		attempts, err := engine.Run(ctx, func(ctx context.Context, diagnostics string) error {
			if e.Mode == core.ModeVerify {
				return errors.New("verification failed; verification mode does not repair files")
			}
			err := e.repairWithReflex(ctx, wt.Path, surg, diagnostics, &modified)
			if err == nil {
				e.Pending = nil
			}
			return errors.Join(err, save())
		})
		report.ReflexAttempts = attempts
		if len(attempts) > 0 {
			report.Diagnostics = compareDiagnostics(report.Baseline, attempts[len(attempts)-1].Result)
		}
		if err != nil {
			return report, errors.Join(err, save())
		}
		verified := len(attempts) > 0 && attempts[len(attempts)-1].Result.Passed
		var accepted bool
		report.Acceptance, accepted = acceptanceSnapshot(wt.Path, plan, verified, wt)
		report.Passed = verified && accepted
		if report.Passed {
			e.Phase = "ready"
		} else {
			e.Phase = "failed"
		}
		e.emit(ProgressEvent{Stage: "verified", Passed: report.Passed, Message: "Verification and acceptance checks complete", Duration: time.Since(start)})
		report.Summary = e.formatReportSummary(instruction, plan, report.Steps, verified, time.Since(start))
		for _, check := range report.Acceptance {
			label := "PASS"
			if !check.Passed {
				label = "FAIL"
			}
			report.Summary += "\nAcceptance " + label + ": " + check.Criterion + " — " + check.Evidence
		}
		if report.Diagnostics != nil {
			d := report.Diagnostics
			report.Summary += fmt.Sprintf("\nDiagnostics: %d observed before, %d not observed before, %d no longer observed. Baseline stopped at: %s.\n", len(d.ObservedBefore), len(d.NotObservedBefore), len(d.NoLongerObserved), d.BaselineFailedStage)
		}
		if report.Passed {
			report.Summary += "\nChanges are retained in the worktree. Explicit apply is required."
		}
	}
	changed, scanErr := wt.ChangedFiles(ctx)
	if scanErr != nil {
		report.Passed = false
		e.Phase = "failed"
		return report, errors.Join(scanErr, save())
	}
	allowed := map[string]bool{}
	for _, path := range modified {
		allowed[path] = true
	}
	for _, path := range changed {
		if !allowed[path] {
			report.Passed = false
			e.Phase = "failed"
			return report, errors.Join(fmt.Errorf("unplanned worktree change: %s; refusing to mark task verified", path), save())
		}
	}
	var err error
	report.Diff, err = wt.DiffFiles(ctx, modified)
	if err != nil {
		return report, errors.Join(err, save())
	}
	if report.Passed {
		e.VerifiedFiles = map[string]surgeon.FileState{}
		for _, path := range modified {
			state, err := surgeon.StateInRoot(wt.Path, path)
			if err != nil {
				return report, err
			}
			e.VerifiedFiles[path] = state
		}
	}
	if err = save(); err != nil {
		return report, err
	}
	return report, nil
}

func (e *Executor) Apply(ctx context.Context, cp PlanCheckpoint) (*ExecutionReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cp.Phase == "applied" && surgeon.Applied(e.Workspace, cp.TaskID) {
		return cp.Report, nil
	}
	if cp.Phase != "ready" || cp.Report == nil || !cp.Report.Passed {
		return nil, errors.New("task has not passed verification and acceptance; execute/resume it first")
	}
	if _, err := loadResumePlan(e.Workspace, cp); err != nil {
		return nil, err
	}
	if err := validateResumeWorktree(ctx, e.Workspace, cp); err != nil {
		return nil, err
	}
	wt, err := surgeon.LoadWorktree(cp.WorktreePath)
	if err != nil {
		return nil, err
	}
	if len(cp.VerifiedFiles) != len(cp.ModifiedFiles) {
		return nil, errors.New("verified file manifest is incomplete")
	}
	for _, path := range cp.ModifiedFiles {
		expected, ok := cp.VerifiedFiles[path]
		current, err := surgeon.StateInRoot(wt.Path, path)
		if !ok || err != nil || current != expected {
			return nil, fmt.Errorf("verified worktree changed: %s; reverify before apply", path)
		}
	}
	e.emit(ProgressEvent{Stage: "applying", Message: "Checking workspace and applying verified set"})
	if err = wt.ApplyToWorkspaceContext(ctx, cp.ModifiedFiles); err != nil {
		return cp.Report, err
	}
	cp.Report.Applied = true
	cp.Report.Summary = strings.ReplaceAll(cp.Report.Summary, "Changes are retained in the worktree. Explicit apply is required.", "Verified changes were applied to the workspace.")
	cp.Phase = "applied"
	if err = SaveCheckpoint(e.Workspace, cp); err != nil {
		return cp.Report, fmt.Errorf("apply committed; durable receipt retained: %w", err)
	}
	e.emit(ProgressEvent{Stage: "applied", Message: "Verified set applied"})
	// Keep the checkpoint/report and worktree for inspection; cleanup is a separate retention policy.
	return cp.Report, nil
}
