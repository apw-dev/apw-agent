package executor

import (
	"github.com/apiwat/apw-agent/internal/architect"
	"os"
	"path/filepath"
	"testing"
)

func TestAcceptanceUsesSnapshotScopeAndFixedVerification(t *testing.T) {
	_, wt, _ := resumeFixture(t)
	plan := &architect.ExecutionPlan{AcceptanceCriteria: []string{"scope", "preserved", "tests"}, Checks: []architect.AcceptanceCheck{
		{Criterion: "scope", Kind: "only_changes", Files: []string{"main.go"}},
		{Criterion: "preserved", Kind: "unchanged", File: "helper.go"},
		{Criterion: "tests", Kind: "verification", Value: "test"},
	}}
	if _, ok := acceptanceSnapshot(wt.Path, plan, true, wt); !ok {
		t.Fatal("valid scope/verification failed")
	}
	if _, ok := acceptanceSnapshot(wt.Path, plan, false, wt); ok {
		t.Fatal("failed tests accepted")
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "helper.go"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	results, ok := acceptanceSnapshot(wt.Path, plan, true, wt)
	if ok || results[0].Passed || results[1].Passed {
		t.Fatal("unexpected or changed file was accepted")
	}
}
