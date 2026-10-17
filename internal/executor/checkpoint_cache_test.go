package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/apiwat/apw-agent/internal/architect"
)

func cacheTestCheckpoint(plan *architect.ExecutionPlan) PlanCheckpoint {
	return PlanCheckpoint{PlanID: plan.PlanID, TaskID: plan.TaskID, Plan: plan, TotalSteps: 2}
}

func TestSaveCheckpointSkipsUnchangedPlanRewrite(t *testing.T) {
	ws := t.TempDir()
	plan := &architect.ExecutionPlan{PlanID: "plan-cache-1", TaskID: "task-cache-1", Objective: "x"}
	planPath := filepath.Join(ws, ".apw", "plans", plan.PlanID+".json")

	cp := cacheTestCheckpoint(plan)
	if err := SaveCheckpoint(ws, cp); err != nil {
		t.Fatal(err)
	}
	// Tamper with the plan file; an unchanged plan must not be rewritten.
	if err := os.WriteFile(planPath, []byte("sentinel"), 0644); err != nil {
		t.Fatal(err)
	}
	cp.LastSuccessStep = 1
	if err := SaveCheckpoint(ws, cp); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "sentinel" {
		t.Fatalf("plan file was rewritten although plan was unchanged: %q", got)
	}
	loaded, err := LoadCheckpoint(ws, plan.PlanID)
	if err != nil || loaded.LastSuccessStep != 1 {
		t.Fatalf("checkpoint not updated: %+v err=%v", loaded, err)
	}

	// A changed plan is rewritten.
	changed := *plan
	changed.Objective = "y"
	cp = cacheTestCheckpoint(&changed)
	if err := SaveCheckpoint(ws, cp); err != nil {
		t.Fatal(err)
	}
	if got, _ = os.ReadFile(planPath); string(got) == "sentinel" {
		t.Fatal("changed plan was not rewritten")
	}

	// A deleted plan file is recreated even though the hash is cached.
	if err := os.Remove(planPath); err != nil {
		t.Fatal(err)
	}
	if err := SaveCheckpoint(ws, cp); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(planPath); err != nil {
		t.Fatalf("deleted plan file was not recreated: %v", err)
	}
}

func TestSaveCheckpointSamePlanIDDifferentWorkspaces(t *testing.T) {
	plan := &architect.ExecutionPlan{PlanID: "plan-cache-shared", TaskID: "task-cache-shared"}
	cp := cacheTestCheckpoint(plan)
	for i := 0; i < 2; i++ {
		ws := t.TempDir()
		if err := SaveCheckpoint(ws, cp); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(ws, ".apw", "plans", plan.PlanID+".json")); err != nil {
			t.Fatalf("workspace %d missing plan file: %v", i, err)
		}
	}
}

func TestSaveCheckpointIdentityMismatch(t *testing.T) {
	plan := &architect.ExecutionPlan{PlanID: "plan-a", TaskID: "task-a"}
	cp := cacheTestCheckpoint(plan)
	cp.TaskID = "other"
	if err := SaveCheckpoint(t.TempDir(), cp); err == nil {
		t.Fatal("expected identity mismatch error")
	}
}
