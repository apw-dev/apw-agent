package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/provider/mock"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

func TestReviewReadsSourceWithoutMutationsOrVerification(t *testing.T) {
	ws := t.TempDir()
	source := []byte("package broken\n// source-evidence-sentinel\nfunc invalid syntax\n")
	if err := os.WriteFile(filepath.Join(ws, "main.go"), source, 0644); err != nil {
		t.Fatal(err)
	}
	provider := mock.New("Found invalid Go syntax in main.go")
	e := New(ws, provider, provider, "review", "reflex", 1)
	e.Mode = core.ModeReview
	report, err := e.Execute(context.Background(), "review only, do not modify", "")
	if err != nil || !report.Passed {
		t.Fatalf("review: %+v %v", report, err)
	}
	actual, _ := os.ReadFile(filepath.Join(ws, "main.go"))
	if string(actual) != string(source) {
		t.Fatal("review modified source")
	}
	if _, err := os.Stat(filepath.Join(ws, ".apw")); !os.IsNotExist(err) {
		t.Fatal("standalone review created state or worktree")
	}
	if len(provider.Requests) != 1 || !strings.Contains(provider.Requests[0].Prompt, "source-evidence-sentinel") {
		t.Fatal("review did not receive actual source")
	}
}

func TestPreviewDoesNotRunTestsAndExecuteRequiresTrust(t *testing.T) {
	ws, wt, cp := resumeFixture(t)
	if err := wt.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	plan := *cp.Plan
	plan.Subtasks = plan.Subtasks[:1]
	planText := `{"objective":"preview","subtasks":[{"id":"one","action":"patch","target_file":"main.go","instruction":"update greeting"}]}`
	provider := mock.New(planText, `{"old_content":"return \"hi\"","new_content":"return \"hello\""}`)
	e := New(ws, provider, provider, "model", "model", 1)
	e.Mode = core.ModePreview
	report, err := e.Execute(context.Background(), "preview", "")
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || report.Baseline != nil || len(report.ReflexAttempts) != 0 {
		t.Fatal("preview ran or claimed verification")
	}
	data, _ := os.ReadFile(filepath.Join(ws, "main.go"))
	if strings.Contains(string(data), "hello") {
		t.Fatal("preview wrote to main workspace")
	}
	checkpoint, err := LoadCheckpoint(ws, report.Plan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Apply(context.Background(), *checkpoint); err == nil {
		t.Fatal("unverified preview was applied")
	}
	e.Mode = core.ModeExecute
	if _, err = e.Execute(context.Background(), "run", ""); err == nil || !strings.Contains(err.Error(), "trusted") {
		t.Fatalf("execution bypassed trust: %v", err)
	}
}

func TestResumeRecognizesMutationCommittedBeforeStepCheckpoint(t *testing.T) {
	ws, wt, cp := resumeFixture(t)
	after, err := surgeon.StateInRoot(wt.Path, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	cp.LastSuccessStep = 0
	cp.ModifiedFiles = nil
	cp.Pending = &MutationIntent{Step: 0, Path: "main.go", Before: wt.Baseline["main.go"], After: after}
	if err = SaveCheckpoint(ws, cp); err != nil {
		t.Fatal(err)
	}
	provider := mock.New(`{"old_content":"return \"old\"","new_content":"return \"new\""}`)
	e := New(ws, provider, provider, "model", "model", 1)
	e.Trusted = true
	report, err := e.Resume(context.Background(), cp)
	if err != nil || !report.Passed {
		t.Fatalf("resume: %+v %v", report, err)
	}
	if len(provider.Requests) != 1 || !strings.Contains(provider.Requests[0].Prompt, "helper.go") {
		t.Fatal("resume repeated a committed step")
	}
	if len(report.ModifiedFiles) != 2 {
		t.Fatalf("recovered mutation missing from apply set: %v", report.ModifiedFiles)
	}
}

func TestAcceptanceRequiresBehaviorEvidence(t *testing.T) {
	_, _, cp := resumeFixture(t)
	plan := *cp.Plan
	plan.AcceptanceCriteria = []string{"returns the requested greeting"}
	results, ok := acceptance(t.TempDir(), &plan, true)
	if ok || len(results) != 1 || results[0].Passed {
		t.Fatal("build success was substituted for a functional requirement")
	}
}

func TestVerificationRejectsUnexpectedRepositoryWrites(t *testing.T) {
	ws, wt, cp := resumeFixture(t)
	testSource := "package main\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestWrites(t *testing.T) {\n\tif err := os.WriteFile(\"unexpected.txt\", []byte(\"side effect\"), 0644); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n"
	if err := os.WriteFile(filepath.Join(wt.Path, "side_effect_test.go"), []byte(testSource), 0644); err != nil {
		t.Fatal(err)
	}
	// The test itself belongs to the prepared source snapshot; its output does not.
	state, err := surgeon.StateInRoot(wt.Path, "side_effect_test.go")
	if err != nil {
		t.Fatal(err)
	}
	wt.Baseline["side_effect_test.go"] = state
	wt.BaseContents["side_effect_test.go"] = []byte(testSource)
	if err = wt.SaveManifest(); err != nil {
		t.Fatal(err)
	}
	provider := mock.New("{\"old_content\":\"return \\\"old\\\"\",\"new_content\":\"return \\\"new\\\"\"}")
	e := New(ws, provider, provider, "model", "model", 1)
	e.Trusted = true
	report, err := e.Resume(context.Background(), cp)
	if err == nil || !strings.Contains(err.Error(), "unplanned worktree change: unexpected.txt") || report.Passed {
		t.Fatalf("repository test wrote outside the planned set without rejection: %v", err)
	}
}
