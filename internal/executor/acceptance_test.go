package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/provider/mock"
)

func TestAcceptanceUsesSnapshotScopeAndFixedVerification(t *testing.T) {
	_, wt, _ := resumeFixture(t)
	plan := &architect.ExecutionPlan{AcceptanceCriteria: []string{"scope", "preserved", "tests"}, Checks: []architect.AcceptanceCheck{
		{Criterion: "scope", Kind: "only_changes", Files: []string{"main.go"}},
		{Criterion: "preserved", Kind: "unchanged", File: "helper.go"},
		{Criterion: "tests", Kind: "verification", Value: "test"},
	}}
	if _, ok := acceptanceSnapshot(wt.Path, plan, true, wt, nil, false); !ok {
		t.Fatal("valid scope/verification failed")
	}
	if _, ok := acceptanceSnapshot(wt.Path, plan, false, wt, nil, false); ok {
		t.Fatal("failed tests accepted")
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "helper.go"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	results, ok := acceptanceSnapshot(wt.Path, plan, true, wt, nil, false)
	if ok || results[0].Passed || results[1].Passed {
		t.Fatal("unexpected or changed file was accepted")
	}
}

func TestAcceptanceHandlesUncheckedProseEnvAliasAndQuoteNormalization(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "deploy"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deploy", "example.env"), []byte("CROWDSEC_LAPI_URL=http://localhost:8080\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token.ts"), []byte("throw new Error(\"Machine credentials rejected\");\n"), 0644); err != nil {
		t.Fatal(err)
	}
	plan := &architect.ExecutionPlan{
		AcceptanceCriteria: []string{
			"The secret .env is in .gitignore and no real secret appears in any file",
			"The root .env.example is not created; deploy/example.env is used instead and reported as a deviation",
			"MachineToken retries once after a 401 and then fails with 'Machine credentials rejected'",
			"env template exists",
			"no secret env file",
			"no dangerouslySetInnerHTML",
		},
		Checks: []architect.AcceptanceCheck{
			{Criterion: "env template exists", Kind: "contains", File: ".env.example", Value: "CROWDSEC_LAPI_URL"},
			{Criterion: "no secret env file", Kind: "file_absent", File: ".env"},
			{Criterion: "MachineToken retries once after a 401 and then fails with 'Machine credentials rejected'", Kind: "contains", File: "token.ts", Value: "'Machine credentials rejected'"},
			{Criterion: "no dangerouslySetInnerHTML", Kind: "not_contains", File: "missing-table.tsx", Value: "dangerouslySetInnerHTML"},
		},
	}
	results, ok := acceptanceSnapshot(dir, plan, true, nil, nil, false)
	if !ok {
		t.Fatalf("expected acceptanceSnapshot to pass, got results: %+v", results)
	}
	for _, r := range results {
		if !r.Passed {
			t.Fatalf("criterion %q failed: %s", r.Criterion, r.Evidence)
		}
	}
}

func TestResumeRepairsUnmetAcceptanceChecksWhenStepsAlreadyCompleted(t *testing.T) {
	ws, wt, cp := resumeFixture(t)
	cp.LastSuccessStep = len(cp.Plan.Subtasks)
	cp.Plan.AcceptanceCriteria = []string{
		"Prose summary criterion without direct check",
		"Greet uses hello",
		"Caddy default site address is :8080",
	}
	cp.Plan.Checks = []architect.AcceptanceCheck{
		{Criterion: "Greet uses hello", Kind: "contains", File: "main.go", Value: `"hello"`},
		{Criterion: "Caddy default site address is :8080", Kind: "contains", File: "deploy/Caddyfile", Value: ":8080"},
	}
	if err := SaveCheckpoint(ws, cp); err != nil {
		t.Fatal(err)
	}
	_ = wt
	mp := mock.New(`{"file_content":":8080 {\n\treverse_proxy /api/* api:3000\n}\n"}`)
	ex := New(ws, mp, mp, "arch", "reflex", 1)
	ex.Trusted = true
	report, err := ex.Resume(context.Background(), cp)
	if err != nil || !report.Passed {
		t.Fatalf("expected resume to repair unmet acceptance checks and pass, got report=%+v err=%v", report, err)
	}
}


