package executor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
	e.emit(ProgressEvent{Stage: "analyzing", Message: "Scanning workspace structure and building AST context..."})
	if pinned := expandPromptMentions(e.Workspace, instruction, nil); pinned != "" && !strings.Contains(e.ReadContext, pinned) {
		e.ReadContext += pinned
	}
	contextText := overview + "\n" + ProjectOverviewForTask(e.Workspace, instruction, e.Profile(e.Workspace))
	if e.EpisodicContext != "" {
		contextText = e.EpisodicContext + "\n" + contextText
	}
	if e.Session != nil {
		contextText = e.Session.BuildContextSummary() + "\n" + contextText
	}
	if hookCtx, hookErr := e.runLifecycleHooks(ctx, HookPrePlan, "", string(e.Mode)); hookErr != nil {
		return nil, hookErr
	} else if hookCtx != "" {
		contextText = hookCtx + "\n" + contextText
	}
	if e.Mode != core.ModeVerify {
		if auditCtx := e.runDomainAuditSubagents(ctx); auditCtx != "" {
			contextText += "\n" + auditCtx
		}
	}
	e.emit(ProgressEvent{Stage: "planning", Message: "Building plan from source context"})
	var plan *architect.ExecutionPlan
	if e.Mode == core.ModeVerify {
		plan = &architect.ExecutionPlan{PlanID: "plan-" + e.TaskID, TaskID: e.TaskID, Objective: "Verify project", Subtasks: []architect.Subtask{{ID: "verify", Action: architect.ActionVerify}}, AcceptanceCriteria: []string{"builds cleanly"}}
	} else {
		plan, err = e.PlanTask(ctx, core.Task{ID: e.TaskID, Workspace: e.Workspace, Instruction: instruction, Mode: string(e.Mode)}, contextText)
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
	wt, err := surgeon.CreateWorktree(ctx, e.Workspace, surgeon.WorktreeOptions{TaskID: plan.TaskID, SyncDirtyFiles: true, Root: e.WorktreeRoot})
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
		baseline := reflex.NewVerifierForProfile(wt.Path, e.Profile(wt.Path))
		baseline.Sandbox = e.Sandbox
		baseline.OnlyFiles = plannedTargetFiles(plan)
		e.VerificationBaseline = baseline.Verify(ctx)
	}
	return e.runPreparedPlan(ctx, plan, wt, 0, nil, instruction)
}

func (e *Executor) review(ctx context.Context, instruction, overview string) (*ExecutionReport, error) {
	if len(e.architectCandidates()) == 0 {
		return nil, errors.New("review provider is not configured")
	}
	if e.Session != nil {
		overview = e.Session.BuildContextSummary() + "\n" + overview
	}
	if auditCtx := e.runDomainAuditSubagents(ctx); auditCtx != "" {
		overview += "\n" + auditCtx
	}
	e.emit(ProgressEvent{Stage: "reviewing", Message: "Reading source without executing repository code"})
	prompt := "Review the supplied source only. Do not run tools, commands, tests, or modify files. Cite file paths and evidence. Distinguish confirmed bugs from uncertainty.\nInstruction: " + instruction + "\n" + overview + "\n" + ProjectOverviewForTask(e.Workspace, instruction, e.Profile(e.Workspace))
	var summary string
	err := e.withArchitectFailover(ctx, "review", func(ctx context.Context, c FailoverCandidate) error {
		response, err := c.Provider.Ask(ctx, core.ModelRequest{RequestID: e.TaskID, Model: c.Model, Prompt: prompt})
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if response == nil || !response.Completed || strings.TrimSpace(response.Content) == "" {
			return errReviewIncomplete
		}
		summary = response.Content
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &ExecutionReport{TaskID: e.TaskID, Mode: core.ModeReview, Passed: true, Summary: summary, Subagents: e.snapshotSubagents()}, nil
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
	if e.Mode != core.ModeExecute && e.Mode != core.ModePreview && e.Mode != core.ModeUnverified {
		return nil, errors.New("resume requires execute, preview or unverified mode")
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
	if err = validateResumeWorktree(ctx, e.Workspace, e.WorktreeRoot, cp); err != nil {
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
	e.Failover.Models = make(map[string]time.Time)
	e.ReadContext = cp.ReadContext
	e.VerificationBaseline = cp.Baseline
	e.Phase = "working"
	return e.runPreparedPlan(ctx, plan, wt, cp.LastSuccessStep, cp.ModifiedFiles, "Resume "+cp.PlanID)
}

func (e *Executor) runPreparedPlan(ctx context.Context, plan *architect.ExecutionPlan, wt *surgeon.Worktree, completed int, files []string, instruction string) (*ExecutionReport, error) {
	start := time.Now()
	e.CurrentPlan = plan
	modified := append([]string{}, files...)
	report := &ExecutionReport{TaskID: plan.TaskID, Plan: plan, Mode: e.Mode, WorktreePath: wt.Path, Baseline: e.VerificationBaseline, Steps: append([]StepReport{}, e.RecoveredSteps...), Subagents: e.snapshotSubagents(), Research: e.ResearchSources}
	save := func() error {
		report.ModifiedFiles = append([]string{}, modified...)
		report.Subagents = e.snapshotSubagents()
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
		if readWave := collectDisjointReadWave(plan.Subtasks, i, e.SubagentConcurrency); len(readWave) >= 2 {
			waveErr := e.runReadSubagentWave(ctx, wt.Path, surg, plan.Subtasks, readWave, func(stepIdx int, sr StepReport, _ string) error {
				report.Steps = append(report.Steps, sr)
				e.emit(ProgressEvent{Stage: "step_done", StepIndex: stepIdx + 1, TotalSteps: len(plan.Subtasks), Step: &sr, Message: sr.Message})
				if !sr.Success {
					return save()
				}
				completed = stepIdx + 1
				e.Pending = nil
				return save()
			})
			if waveErr != nil {
				return report, errors.Join(waveErr, save())
			}
			i = readWave[len(readWave)-1]
			continue
		}
		if wave := collectDisjointMutationWave(plan.Subtasks, i, e.SubagentConcurrency); len(wave) >= 2 && e.Mode != core.ModeReview {
			waveErr := e.runSubagentWave(ctx, wt.Path, surg, plan.Subtasks, wave, func(stepIdx int, sr StepReport, path string) error {
				report.Steps = append(report.Steps, sr)
				e.emit(ProgressEvent{Stage: "step_done", StepIndex: stepIdx + 1, TotalSteps: len(plan.Subtasks), Step: &sr, Message: sr.Message})
				if !sr.Success {
					return save()
				}
				completed = stepIdx + 1
				e.Pending = nil
				appendModifiedFile(&modified, path)
				return save()
			})
			if waveErr != nil {
				return report, errors.Join(waveErr, save())
			}
			i = wave[len(wave)-1]
			continue
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
	} else if e.Mode == core.ModeUnverified {
		// No install, build, test or browser check runs; only file-level acceptance checks decide.
		var accepted bool
		report.Unverified = true
		report.Acceptance, accepted = acceptanceSnapshot(wt.Path, plan, false, wt, nil, true)
		report.Passed = accepted
		e.Phase = "failed"
		if accepted {
			e.Phase = "ready"
		}
		report.Summary = e.formatReportSummary(instruction, plan, report.Steps, false, time.Since(start))
		report.Summary += "\nUNVERIFIED: no install, build, test or browser check was run. Review the diff before applying."
		for _, check := range report.Acceptance {
			label := "PASS"
			if !check.Passed {
				label = "FAIL"
			}
			report.Summary += "\nAcceptance " + label + ": " + check.Criterion + " — " + check.Evidence
		}
		if report.Passed {
			report.Summary += "\nChanges are retained in the worktree. Explicit apply is required."
		}
	} else {
		if e.Mode == core.ModeExecute {
			e.repairUnmetAcceptance(ctx, wt.Path, surg, plan, wt, &modified, &report.Steps, save)
		}
		e.emit(ProgressEvent{Stage: "verifying", Message: "Verifying isolated changes"})
		prof := e.Profile(wt.Path)
		engine := reflex.NewReflexEngineForProfile(wt.Path, e.Reflex.MaxRetries, prof)
		engine.Verifier.Sandbox = e.Sandbox
		engine.Verifier.OnlyFiles = modified
		// Without a pipeline nothing can verify the change, and no source repair can add one:
		// report the missing pipeline once instead of asking models to repair or re-plan.
		// File-level checks alone never mark a task verified; unverified mode is for that.
		noPipeline := len(prof.Stages) == 0
		var attempts []reflex.Attempt
		var err error
		if noPipeline {
			attempts = []reflex.Attempt{{Number: 1, Result: engine.Verifier.Verify(ctx)}}
		} else {
			attempts, err = engine.Run(ctx, func(ctx context.Context, diagnostics string) error {
				if e.Mode == core.ModeVerify {
					return errors.New("verification failed; verification mode does not repair files")
				}
				err := e.repairWithReflex(ctx, wt.Path, surg, diagnostics, &modified)
				engine.Verifier.OnlyFiles = modified
				if err == nil {
					e.Pending = nil
				}
				return errors.Join(err, save())
			})
		}
		report.ReflexAttempts = attempts
		if len(attempts) > 0 {
			report.Diagnostics = compareDiagnostics(report.Baseline, attempts[len(attempts)-1].Result)
		}
		for replanRound := 1; replanRound <= e.MaxReplans && e.Mode == core.ModeExecute && !noPipeline && (err != nil || len(attempts) == 0 || !attempts[len(attempts)-1].Result.Passed); replanRound++ {
			diagText := ""
			if len(attempts) > 0 && attempts[len(attempts)-1].Result != nil {
				diagText = reflex.FormatDiagnostics(attempts[len(attempts)-1].Result)
			} else if err != nil {
				diagText = err.Error()
			}
			revPlan, replanErr := e.replanWithArchitect(ctx, wt.Path, instruction, plan, diagText, modified)
			if replanErr != nil {
				break
			}
			for j, st := range revPlan.Subtasks {
				if st.Action == architect.ActionVerify {
					continue
				}
				e.emit(ProgressEvent{Stage: "step_start", StepIndex: j + 1, TotalSteps: len(revPlan.Subtasks), Message: fmt.Sprintf("[REPLAN %d · %s] %s — %s", replanRound, st.Action, st.ID, st.TargetFile)})
				sr, path, stepErr := e.executeSubtask(ctx, wt.Path, surg, st)
				if stepErr != nil {
					sr.Message = stepErr.Error()
				}
				report.Steps = append(report.Steps, sr)
				e.emit(ProgressEvent{Stage: "step_done", StepIndex: j + 1, TotalSteps: len(revPlan.Subtasks), Step: &sr, Message: sr.Message})
				if stepErr != nil {
					err = stepErr
					break
				}
				e.Pending = nil
				appendModifiedFile(&modified, path)
			}
			engine.Verifier.OnlyFiles = modified
			moreAttempts, verifyErr := engine.Run(ctx, func(ctx context.Context, diagnostics string) error {
				rErr := e.repairWithReflex(ctx, wt.Path, surg, diagnostics, &modified)
				engine.Verifier.OnlyFiles = modified
				if rErr == nil {
					e.Pending = nil
				}
				return errors.Join(rErr, save())
			})
			attempts = append(attempts, moreAttempts...)
			report.ReflexAttempts = attempts
			if len(attempts) > 0 {
				report.Diagnostics = compareDiagnostics(report.Baseline, attempts[len(attempts)-1].Result)
			}
			err = verifyErr
		}
		if err != nil {
			return report, errors.Join(err, save())
		}
		verified := len(attempts) > 0 && attempts[len(attempts)-1].Result.Passed
		var accepted bool
		report.Acceptance, accepted = acceptanceSnapshot(wt.Path, plan, verified, wt, e.pageVisitor(ctx, wt.Path), false)
		if !accepted && e.Mode == core.ModeExecute {
			e.repairUnmetAcceptance(ctx, wt.Path, surg, plan, wt, &modified, &report.Steps, save)
			report.Acceptance, accepted = acceptanceSnapshot(wt.Path, plan, verified, wt, e.pageVisitor(ctx, wt.Path), false)
		}
		report.Passed = verified && accepted
		if hookCtx, hookErr := e.runLifecycleHooks(ctx, HookPostVerify, "", instruction); hookErr != nil {
			report.Passed = false
			report.Summary += "\nPostVerify Hook Denied: " + hookErr.Error()
		} else if hookCtx != "" {
			report.Summary += "\nPostVerify Hook: " + hookCtx
		}
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
		if last := attempts[len(attempts)-1].Result; last != nil && last.Sandbox != "" {
			report.Summary += "\nSandbox: " + last.Sandbox
		}
		if report.Diagnostics != nil {
			d := report.Diagnostics
			report.Summary += fmt.Sprintf("\nDiagnostics: %d observed before, %d not observed before, %d no longer observed. Baseline stopped at: %s.\n", len(d.ObservedBefore), len(d.NotObservedBefore), len(d.NoLongerObserved), d.BaselineFailedStage)
		}
		if report.Passed {
			report.Summary += "\nChanges are retained in the worktree. Explicit apply is required."
		}
	}
	if report.Warnings = mirrorWarnings(instruction, wt.Path, modified); len(report.Warnings) > 0 {
		for _, w := range report.Warnings {
			report.Summary += "\nWarning: " + w
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
		// Plans use forward slashes; the worktree scan uses the OS separator.
		allowed[filepath.Clean(filepath.FromSlash(path))] = true
	}
	for _, path := range changed {
		if !allowed[filepath.Clean(filepath.FromSlash(path))] {
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
	if err := validateResumeWorktree(ctx, e.Workspace, e.WorktreeRoot, cp); err != nil {
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

func plannedTargetFiles(plan *architect.ExecutionPlan) []string {
	if plan == nil {
		return nil
	}
	var out []string
	for _, st := range plan.Subtasks {
		if st.Action == architect.ActionCreate || st.Action == architect.ActionPatch || st.Action == architect.ActionDelete {
			appendModifiedFile(&out, st.TargetFile)
		}
	}
	return out
}
