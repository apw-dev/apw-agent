package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/project"
	"github.com/apiwat/apw-agent/internal/provider/mock"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

// completedFileCheckFixture returns a resume checkpoint whose steps are all done and whose only
// check is file-level (no "verification" check), so acceptance alone passes.
func completedFileCheckFixture(t *testing.T) (string, PlanCheckpoint) {
	t.Helper()
	ws, _, cp := resumeFixture(t)
	cp.LastSuccessStep = len(cp.Plan.Subtasks)
	cp.Plan.AcceptanceCriteria = []string{"Greet uses hello"}
	cp.Plan.Checks = []architect.AcceptanceCheck{
		{Criterion: "Greet uses hello", Kind: "contains", File: "main.go", Value: `"hello"`},
	}
	if err := SaveCheckpoint(ws, cp); err != nil {
		t.Fatal(err)
	}
	return ws, cp
}

func TestFailedVerificationIsNotOverriddenByFileChecks(t *testing.T) {
	ws, cp := completedFileCheckFixture(t)
	broken := "package main\n\nfunc Greet() string {\n\treturn \"hello\"\n}\n\nfunc main() { undefinedCall() }\n"
	if err := os.WriteFile(filepath.Join(cp.WorktreePath, "main.go"), []byte(broken), 0644); err != nil {
		t.Fatal(err)
	}
	// The repair applies cleanly but leaves the build broken.
	mp := mock.New(`{"target_file":"helper.go","old_content":"return \"old\"","new_content":"return \"still old\""}`)
	ex := New(ws, mp, mp, "arch", "reflex", 1)
	ex.Trusted = true
	report, err := ex.Resume(context.Background(), cp)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if report.Passed {
		t.Fatalf("task passed although go build failed: %s", report.Summary)
	}
	last := report.ReflexAttempts[len(report.ReflexAttempts)-1].Result
	if last.Passed || last.FailedStage == "" {
		t.Fatalf("expected a failed verification attempt, got %+v", last)
	}
}

func TestDependencyInstallFailureFailsTask(t *testing.T) {
	ws, cp := completedFileCheckFixture(t)
	mp := mock.New()
	ex := New(ws, mp, mp, "arch", "reflex", 1)
	ex.Trusted = true
	ex.VerifyOverride = []project.Stage{
		{Name: "install", Command: []string{"go", "apw-no-such-subcommand"}, Once: true},
		{Name: "test", Command: []string{"go", "version"}},
	}
	report, err := ex.Resume(context.Background(), cp)
	if err == nil || !strings.Contains(err.Error(), "dependency install") {
		t.Fatalf("expected dependency install error, got err=%v", err)
	}
	if report != nil && report.Passed {
		t.Fatal("task passed although dependency install failed")
	}
}

func TestNoVerificationPipelineDoesNotPass(t *testing.T) {
	// A notes-only repository has no detectable build or test pipeline.
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "notes.txt"), []byte("hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	initExecutorRepo(t, ws)
	wt, err := surgeon.CreateWorktree(context.Background(), ws, surgeon.WorktreeOptions{TaskID: "no-pipeline"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wt.Cleanup(context.Background()) })
	if err := os.WriteFile(filepath.Join(wt.Path, "notes.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	plan := &architect.ExecutionPlan{
		PlanID: "plan-no-pipeline", TaskID: "no-pipeline", Objective: "Update notes",
		Subtasks:           []architect.Subtask{{ID: "one", Action: architect.ActionPatch, TargetFile: "notes.txt", Instruction: "Say hello"}},
		AcceptanceCriteria: []string{"notes say hello"},
		Checks:             []architect.AcceptanceCheck{{Criterion: "notes say hello", Kind: "contains", File: "notes.txt", Value: "hello"}},
	}
	cp := PlanCheckpoint{
		PlanID: plan.PlanID, TaskID: plan.TaskID, LastSuccessStep: 1, TotalSteps: 1,
		WorktreeBranch: wt.Branch, WorktreePath: wt.Path, ModifiedFiles: []string{"notes.txt"}, Plan: plan,
	}
	if err := SaveCheckpoint(ws, cp); err != nil {
		t.Fatal(err)
	}
	mp := mock.New()
	ex := New(ws, mp, mp, "arch", "reflex", 3)
	ex.Trusted = true
	ex.MaxReplans = 2
	report, err := ex.Resume(context.Background(), cp)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if report.Passed {
		t.Fatalf("task passed without any verification command: %s", report.Summary)
	}
	if n := len(report.ReflexAttempts); n != 1 {
		t.Fatalf("expected one verification attempt without repairs, got %d", n)
	}
	if !strings.Contains(report.ReflexAttempts[0].Result.RawOutput, "no verification pipeline") {
		t.Fatalf("expected missing-pipeline diagnostic, got %q", report.ReflexAttempts[0].Result.RawOutput)
	}
	if len(mp.Requests) != 0 {
		t.Fatalf("expected no model requests for a missing pipeline, got %d", len(mp.Requests))
	}
}
