package executor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/provider/mock"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

func resumeFixture(t *testing.T) (string, *surgeon.Worktree, PlanCheckpoint) {
	t.Helper()
	ws := t.TempDir()
	files := map[string]string{
		"go.mod":    "module example.com/resume\n\ngo 1.24\n",
		"main.go":   "package main\n\nfunc Greet() string {\n\treturn \"hi\"\n}\n\nfunc main() {}\n",
		"helper.go": "package main\n\nfunc Helper() string {\n\treturn \"old\"\n}\n",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(ws, path), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"},
		{"config", "commit.gpgsign", "false"}, {"config", "core.autocrlf", "false"},
		{"add", "."}, {"commit", "-m", "fixture"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = ws
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	wt, err := surgeon.CreateWorktree(context.Background(), ws, surgeon.WorktreeOptions{TaskID: "resume-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wt.Cleanup(context.Background()) })
	plan := &architect.ExecutionPlan{
		PlanID: "plan-resume", TaskID: "resume-test", Objective: "Update greetings",
		Subtasks: []architect.Subtask{
			{ID: "one", Action: architect.ActionPatch, TargetFile: "main.go", Instruction: "Update Greet"},
			{ID: "two", Action: architect.ActionPatch, TargetFile: "helper.go", Instruction: "Update Helper"},
		},
	}
	// The first step already completed in the isolated worktree.
	if err := os.WriteFile(filepath.Join(wt.Path, "main.go"), []byte(strings.ReplaceAll(files["main.go"], `"hi"`, `"hello"`)), 0644); err != nil {
		t.Fatal(err)
	}
	cp := PlanCheckpoint{
		PlanID: plan.PlanID, TaskID: plan.TaskID, LastSuccessStep: 1, TotalSteps: 2,
		WorktreeBranch: wt.Branch, WorktreePath: wt.Path, ModifiedFiles: []string{"main.go"}, Plan: plan,
	}
	if err := SaveCheckpoint(ws, cp); err != nil {
		t.Fatal(err)
	}
	return ws, wt, cp
}

func TestResumeRetainsCheckpointOnQuotaThenContinuesExactStep(t *testing.T) {
	ws, wt, cp := resumeFixture(t)
	failed := &routingProvider{name: "agy", ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
		return nil, errors.New("Individual quota reached. Resets in 153h17m")
	}}
	ex := New(ws, failed, failed, "gemini-4-argon-high", "gemini-4-argon-high", 1)
	ex.Trusted = true
	if _, err := ex.Resume(context.Background(), cp); err == nil {
		t.Fatal("expected quota failure")
	}
	loaded, err := LoadCheckpoint(ws, cp.PlanID)
	if err != nil || loaded.LastSuccessStep != 1 || loaded.Plan == nil {
		t.Fatalf("checkpoint lost completed step/plan: %+v %v", loaded, err)
	}
	if loaded.Failover.Groups["agy:argon"].IsZero() {
		t.Fatal("quota cooldown was not checkpointed")
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatal("quota failure removed resumable worktree")
	}
	mp := mock.New(`{"old_content":"return \"old\"","new_content":"return \"new\""}`, "done")
	ex = New(ws, mp, mp, "working", "working", 1)
	ex.Trusted = true
	checkpointBeforeVerify := false
	ex.OnProgress = func(ev ProgressEvent) {
		if ev.Stage == "verifying" {
			latest, err := LoadCheckpoint(ws, cp.PlanID)
			checkpointBeforeVerify = err == nil && latest.LastSuccessStep == 2
		}
	}
	report, err := ex.Resume(context.Background(), *loaded)
	if err != nil || !report.Passed || !checkpointBeforeVerify {
		t.Fatalf("resume failed or progress not saved: %+v %v", report, err)
	}
	if len(report.Steps) != 1 || report.Steps[0].SubtaskID != "two" {
		t.Fatalf("resume repeated completed steps: %+v", report.Steps)
	}
	applyPreparedTest(t, ex, report)
	for path, want := range map[string]string{"main.go": `"hello"`, "helper.go": `"new"`} {
		data, err := os.ReadFile(filepath.Join(ws, path))
		if err != nil || !strings.Contains(string(data), want) {
			t.Fatalf("%s missing resumed changes: %s %v", path, data, err)
		}
	}
	if applied, err := LoadCheckpoint(ws, cp.PlanID); err != nil || applied.Phase != "applied" {
		t.Fatalf("apply receipt/checkpoint missing: %v", err)
	}
}

func TestResumeRetainsWorktreeOnVerificationAndApplyFailures(t *testing.T) {
	for _, stage := range []string{"verification", "apply"} {
		t.Run(stage, func(t *testing.T) {
			ws, wt, cp := resumeFixture(t)
			cp.LastSuccessStep = 2
			if stage == "verification" {
				if err := os.WriteFile(filepath.Join(wt.Path, "helper.go"), []byte("package main\n\nfunc Helper() string {\n\treturn missing\n}\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			failed := &routingProvider{name: "agy", ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
				return nil, errors.New("Individual quota reached. Resets in 1h")
			}}
			ex := New(ws, failed, failed, "flash", "flash", 1)
			ex.Trusted = true
			ex.OnProgress = func(ev ProgressEvent) {
				if stage == "apply" && ev.Stage == "applying" {
					if err := os.Remove(filepath.Join(ws, "main.go")); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(filepath.Join(ws, "main.go"), 0755); err != nil {
						t.Fatal(err)
					}
				}
			}
			report, runErr := ex.Resume(context.Background(), cp)
			if stage == "apply" && runErr == nil {
				prepared, err := LoadCheckpoint(ws, cp.PlanID)
				if err != nil {
					t.Fatal(err)
				}
				_, runErr = ex.Apply(context.Background(), *prepared)
			}
			if runErr == nil {
				t.Fatalf("expected %s failure: %+v", stage, report)
			}

			if _, err := os.Stat(wt.Path); err != nil {
				t.Fatalf("%s failure destroyed worktree: %v", stage, err)
			}
			loaded, err := LoadCheckpoint(ws, cp.PlanID)
			if err != nil || loaded.LastSuccessStep != 2 {
				t.Fatalf("%s failure lost checkpoint: %+v %v", stage, loaded, err)
			}
		})
	}
}

func TestResumeRefusesUnrelatedPlanAndInvalidStep(t *testing.T) {
	ws := t.TempDir()
	unrelated := &architect.ExecutionPlan{PlanID: "plan-other", TaskID: "other", Subtasks: []architect.Subtask{{Action: architect.ActionVerify}}}
	if err := SaveCheckpoint(ws, PlanCheckpoint{PlanID: unrelated.PlanID, TaskID: unrelated.TaskID, Plan: unrelated}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadResumePlan(ws, PlanCheckpoint{PlanID: "plan-missing", TaskID: "missing"}); err == nil {
		t.Fatal("resume substituted an unrelated plan")
	}
	for _, step := range []int{-1, 2} {
		if _, err := loadResumePlan(ws, PlanCheckpoint{PlanID: unrelated.PlanID, TaskID: unrelated.TaskID, Plan: unrelated, LastSuccessStep: step, TotalSteps: 1}); err == nil {
			t.Fatalf("accepted invalid step %d", step)
		}
	}
	if _, err := LoadCheckpoint(ws, "../escape"); err == nil {
		t.Fatal("accepted escaping plan ID")
	}
}

func TestExecuteAppliesAdditionalReflexRepairFile(t *testing.T) {
	ws, wt, _ := resumeFixture(t)
	if err := wt.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	plan := architect.ExecutionPlan{Objective: "Use a new helper", Subtasks: []architect.Subtask{{ID: "patch", Action: architect.ActionPatch, TargetFile: "main.go", Instruction: "Call Missing"}}}
	planBytes, _ := json.Marshal(plan)
	mp := mock.New(string(planBytes),
		`{"old_content":"func main() {}","new_content":"func main() { _ = Missing() }"}`,
		`{"target_file":"helper.go","old_content":"func Helper() string","new_content":"func Missing() string"}`, "done")
	ex := New(ws, mp, mp, "arch", "reflex", 1)
	ex.Trusted = true
	report, err := ex.Execute(context.Background(), "Use a new helper", "File: main.go\nFile: helper.go")
	if err != nil || !report.Passed {
		t.Fatalf("execution failed: %+v %v", report, err)
	}
	applyPreparedTest(t, ex, report)
	data, err := os.ReadFile(filepath.Join(ws, "helper.go"))
	if err != nil || !strings.Contains(string(data), "func Missing()") {
		t.Fatalf("verified repair was dropped during apply: %s %v", data, err)
	}
}

// moveToLegacyWorktree relocates the fixture worktree to <workspace>/.worktrees/<task-id>, where
// APW kept worktrees before the external root, and rewrites manifest and checkpoint to match.
func moveToLegacyWorktree(t *testing.T, ws string, wt *surgeon.Worktree, cp *PlanCheckpoint) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(ws, ".git", "info", "exclude"), []byte(".worktrees/\n.apw/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(ws, surgeon.LegacyWorktreeDir, wt.TaskID)
	if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", ws, "worktree", "move", wt.Path, legacy).CombinedOutput(); err != nil {
		t.Fatalf("git worktree move: %v (%s)", err, out)
	}
	wt.Path = legacy
	if err := wt.SaveManifest(); err != nil {
		t.Fatal(err)
	}
	cp.WorktreePath = legacy
	if err := SaveCheckpoint(ws, *cp); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyWorktreeStillResumesAndApplies(t *testing.T) {
	ws, wt, cp := resumeFixture(t)
	moveToLegacyWorktree(t, ws, wt, &cp)
	mp := mock.New(`{"old_content":"return \"old\"","new_content":"return \"new\""}`, "done")
	ex := New(ws, mp, mp, "working", "working", 1)
	ex.Trusted = true
	report, err := ex.Resume(context.Background(), cp)
	if err != nil || !report.Passed || report.WorktreePath != wt.Path {
		t.Fatalf("legacy resume failed: %+v %v", report, err)
	}
	applyPreparedTest(t, ex, report)
	for path, want := range map[string]string{"main.go": `"hello"`, "helper.go": `"new"`} {
		data, err := os.ReadFile(filepath.Join(ws, path))
		if err != nil || !strings.Contains(string(data), want) {
			t.Fatalf("%s missing legacy changes: %s %v", path, data, err)
		}
	}
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("legacy worktree was moved or removed: %v", err)
	}
}

func TestResumeRefusesWorktreeOutsideRootsOrFromAnotherRepository(t *testing.T) {
	ws, wt, cp := resumeFixture(t)
	if err := validateResumeWorktree(context.Background(), ws, "", cp); err != nil {
		t.Fatalf("external-root worktree rejected: %v", err)
	}
	// The same repository's worktree outside every worktree root.
	stray := filepath.Join(t.TempDir(), "stray")
	if out, err := exec.Command("git", "-C", ws, "worktree", "add", "-b", "apw/stray", stray, "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v (%s)", err, out)
	}
	outside := cp
	outside.WorktreePath, outside.WorktreeBranch = stray, "apw/stray"
	if err := validateResumeWorktree(context.Background(), ws, "", outside); err == nil || !strings.Contains(err.Error(), "not directly inside") {
		t.Fatalf("worktree outside roots accepted: %v", err)
	}
	// Another repository's worktree placed inside this workspace's worktree directory.
	other, _, _ := resumeFixture(t)
	foreign := filepath.Join(filepath.Dir(wt.Path), "foreign")
	if out, err := exec.Command("git", "-C", other, "worktree", "add", "-b", "apw/foreign", foreign, "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v (%s)", err, out)
	}
	defer exec.Command("git", "-C", other, "worktree", "remove", "--force", foreign).Run()
	alien := cp
	alien.WorktreePath, alien.WorktreeBranch = foreign, "apw/foreign"
	if err := validateResumeWorktree(context.Background(), ws, "", alien); err == nil || !strings.Contains(err.Error(), "different repository") {
		t.Fatalf("foreign repository worktree accepted: %v", err)
	}
}
