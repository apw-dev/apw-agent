package executor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/provider/mock"
)

func TestUnverifiedModeRunsNoCommandsAndCanApply(t *testing.T) {
	ws, wt, _ := resumeFixture(t)
	if err := wt.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	plan := architect.ExecutionPlan{
		Objective: "Say hello", AffectedFiles: []string{"main.go"},
		Subtasks:           []architect.Subtask{{ID: "patch", Action: architect.ActionPatch, TargetFile: "main.go", Instruction: "return hello"}},
		AcceptanceCriteria: []string{"greets", "tests", "page"},
		Checks: []architect.AcceptanceCheck{
			{Criterion: "greets", Kind: "contains", File: "main.go", Value: `"hello"`},
			{Criterion: "tests", Kind: "verification", Value: "test"},
			{Criterion: "page", Kind: "browser", Value: "Hello"},
		},
	}
	planBytes, _ := json.Marshal(plan)
	mp := mock.New(string(planBytes), `{"old_content":"return \"hi\"","new_content":"return \"hello\""}`)
	ex := New(ws, mp, mp, "arch", "reflex", 1)
	ex.Mode = core.ModeUnverified // untrusted: unverified mode needs no trust
	var stages []string
	ex.OnProgress = func(ev ProgressEvent) { stages = append(stages, ev.Stage) }
	report, err := ex.Execute(context.Background(), "Say hello", "File: main.go")
	if err != nil || !report.Passed || !report.Unverified || len(report.ReflexAttempts) != 0 {
		t.Fatalf("unverified execution: %+v %v", report, err)
	}
	for _, stage := range stages {
		if stage == "verifying" || stage == "baseline" {
			t.Fatalf("unverified mode ran verification (%s)", stage)
		}
	}
	if !strings.Contains(report.Summary, "UNVERIFIED") || !strings.Contains(report.Acceptance[1].Evidence, "SKIPPED") || !strings.Contains(report.Acceptance[2].Evidence, "SKIPPED") {
		t.Fatalf("summary/evidence do not say what was skipped:\n%s\n%+v", report.Summary, report.Acceptance)
	}
	applyPreparedTest(t, ex, report)
	if data, _ := os.ReadFile(filepath.Join(ws, "main.go")); !strings.Contains(string(data), `"hello"`) {
		t.Fatalf("unverified change not applied: %s", data)
	}
}

func TestUnverifiedModeStillEnforcesFileChecks(t *testing.T) {
	ws, wt, _ := resumeFixture(t)
	if err := wt.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	plan := architect.ExecutionPlan{
		Objective: "Say hello", AffectedFiles: []string{"main.go"},
		Subtasks:           []architect.Subtask{{ID: "patch", Action: architect.ActionPatch, TargetFile: "main.go", Instruction: "return hello"}},
		AcceptanceCriteria: []string{"greets"},
		Checks:             []architect.AcceptanceCheck{{Criterion: "greets", Kind: "contains", File: "main.go", Value: "Goodbye"}},
	}
	planBytes, _ := json.Marshal(plan)
	mp := mock.New(string(planBytes), `{"old_content":"return \"hi\"","new_content":"return \"hello\""}`)
	ex := New(ws, mp, mp, "arch", "reflex", 1)
	ex.Mode = core.ModeUnverified
	report, err := ex.Execute(context.Background(), "Say hello", "File: main.go")
	if err != nil || report.Passed {
		t.Fatalf("failed file check accepted: %+v %v", report, err)
	}
	cp, err := LoadCheckpoint(ws, report.Plan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ex.Apply(context.Background(), *cp); err == nil {
		t.Fatal("failed unverified task was applied")
	}
}
