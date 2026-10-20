// Package devfixture builds Desktop API task data from the real Go types (store.Job,
// executor.ExecutionReport). The Playwright e2e helper serves these jobs through the real
// desktop.Server, and the frontend unit tests read them as JSON fixtures, so neither can drift
// from the contract the way hand-written mock data can.
package devfixture

import (
	"encoding/json"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/reflex"
	"github.com/apiwat/apw-agent/internal/store"
)

// GovernorFile is the file every fixture task changes.
const GovernorFile = "internal/store/autonomy.go"

// GovernorDiff is the unified diff every fixture task reports for GovernorFile.
const GovernorDiff = "--- a/internal/store/autonomy.go\n+++ b/internal/store/autonomy.go\n@@ -1,2 +1,3 @@\n package store\n-var oldLimit = 0\n+var defaultLimit = 6\n+var keepExisting = true\n"

// fixedTime keeps the generated fixtures byte-stable.
var fixedTime = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

func plan(objective string) *architect.ExecutionPlan {
	return &architect.ExecutionPlan{
		PlanID:             "plan-governor",
		Objective:          objective,
		AffectedFiles:      []string{GovernorFile},
		AcceptanceCriteria: []string{"pause keeps configured limits"},
		CreatedAt:          fixedTime,
	}
}

// PreviewReport is a preview that produced a diff in an isolated worktree.
func PreviewReport() executor.ExecutionReport {
	return executor.ExecutionReport{
		Mode:          core.ModePreview,
		Plan:          plan("Keep governor limits when pausing"),
		Passed:        true,
		Summary:       "Preview ready: governor keeps configured limits.",
		ModifiedFiles: []string{GovernorFile},
		Diff:          GovernorDiff,
	}
}

// ReadyReport is an execute run whose verification and acceptance checks passed.
func ReadyReport() executor.ExecutionReport {
	r := PreviewReport()
	r.Mode = core.ModeExecute
	r.Summary = "Verified: governor keeps configured limits."
	r.ReflexAttempts = []reflex.Attempt{{Number: 1, Result: &reflex.VerificationResult{Passed: true}}}
	r.Acceptance = []executor.CheckResult{{Criterion: "pause keeps configured limits", Passed: true, Evidence: "ok  internal/store 0.42s"}}
	return r
}

// FailedReport is an execute run whose tests failed.
func FailedReport() executor.ExecutionReport {
	r := ReadyReport()
	r.Passed = false
	r.Summary = "go test failed"
	r.ReflexAttempts = []reflex.Attempt{{Number: 1, Result: &reflex.VerificationResult{
		Passed:      false,
		FailedStage: "test",
		RawOutput:   "--- FAIL: TestGovernorPause (0.01s)\n    autonomy_test.go:42: limit reset to 10",
		Diagnostics: []reflex.Diagnostic{{Stage: "test", File: "internal/store/autonomy_test.go", Line: 42, Message: "limit reset to 10"}},
	}}}
	r.Acceptance = []executor.CheckResult{{Criterion: "pause keeps configured limits", Passed: false, Evidence: "max_tasks_per_hour reset to 10"}}
	return r
}

// Job wraps a report in the store.Job the Desktop API returns.
func Job(id string, mode core.TaskMode, status core.TaskStatus, instruction string, rep *executor.ExecutionReport) store.Job {
	j := store.Job{
		Task: core.Task{
			ID:          id,
			Workspace:   "/workspace",
			Instruction: instruction,
			Mode:        string(mode),
			Status:      status,
			CreatedAt:   fixedTime,
			UpdatedAt:   fixedTime,
		},
		Owner:     "local",
		RequestID: "req-" + id,
	}
	if rep != nil {
		rep.TaskID = id
		j.Report, _ = json.Marshal(rep)
	}
	return j
}

func PreviewJob() store.Job {
	r := PreviewReport()
	return Job("task-preview", core.ModePreview, core.StatusPreview, "เตรียมแก้ limit ของ governor", &r)
}

func ReadyJob() store.Job {
	r := ReadyReport()
	return Job("task-ready", core.ModeExecute, core.StatusReady, "แก้ไขและรันทดสอบ governor", &r)
}

func FailedJob() store.Job {
	r := FailedReport()
	j := Job("task-failed", core.ModeExecute, core.StatusFailed, "แก้ไขและรันทดสอบ governor", &r)
	j.Error = "go test failed"
	return j
}

// TaskStatuses groups core statuses the way the UI needs them. Resumable mirrors the statuses
// app.Service.Resume requeues; in-flight are the non-terminal ones the UI must keep polling.
func TaskStatuses() map[string][]core.TaskStatus {
	return map[string][]core.TaskStatus{
		"in_flight": {core.StatusQueued, core.StatusAnalyzing, core.StatusPlanning, core.StatusPatching, core.StatusVerifying, core.StatusApplying},
		"resumable": {core.StatusInterrupted, core.StatusFailed, core.StatusCancelled, core.StatusPreview},
		"terminal":  {core.StatusSucceeded, core.StatusFailed, core.StatusCancelled, core.StatusProviderUnavailable, core.StatusReady, core.StatusPreview, core.StatusInterrupted},
	}
}

// FrontendFixtures are written to internal/desktop/web/src/fixtures/<name>.json.
func FrontendFixtures() map[string]any {
	return map[string]any{
		"preview-job":   PreviewJob(),
		"ready-job":     ReadyJob(),
		"failed-job":    FailedJob(),
		"task-statuses": TaskStatuses(),
	}
}
