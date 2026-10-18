package eval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/provider/mock"
)

const hiddenTwo = "package x\n\nimport \"testing\"\n\nfunc TestTwo(t *testing.T) {\n\tif Two() != 2 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n"

func writeTask(t *testing.T, suite, name string) {
	t.Helper()
	files := map[string]string{
		"task.json":          `{"instruction":"Add func Two() int returning 2 to package x.","check":["go","test","./..."]}`,
		"repo/go.mod":        "module example.com/x\n\ngo 1.24\n",
		"repo/x/x.go":        "package x\n\n// One returns 1.\nfunc One() int { return 1 }\n",
		"hidden/x/h_test.go": hiddenTwo,
	}
	for rel, body := range files {
		p := filepath.Join(suite, name, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// scriptedFactory answers the plan with one patch of x/x.go that appends body.
func scriptedFactory(t *testing.T, body string) func(*config.Config) (*executor.Executor, func(), error) {
	t.Helper()
	plan := architect.ExecutionPlan{
		Objective: "add Two", AffectedFiles: []string{"x/x.go"},
		Subtasks:           []architect.Subtask{{ID: "one", Action: architect.ActionPatch, TargetFile: "x/x.go", Instruction: "add Two"}},
		AcceptanceCriteria: []string{"builds"},
		Checks:             []architect.AcceptanceCheck{{Criterion: "builds", Kind: "verification", Value: "all"}},
	}
	planJSON, _ := json.Marshal(plan)
	patch, _ := json.Marshal(map[string]string{"old_content": "func One() int { return 1 }", "new_content": "func One() int { return 1 }\n\n" + body})
	return func(cfg *config.Config) (*executor.Executor, func(), error) {
		p := mock.New(string(planJSON), string(patch))
		return executor.New(cfg.Workspace, p, p, "scripted", "scripted", 1), func() {}, nil
	}
}

// TestRunClassifiesOutcomes drives the real service and worktree pipeline with scripted models.
func TestRunClassifiesOutcomes(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test inside task worktrees")
	}
	suite := t.TempDir()
	writeTask(t, suite, "two")
	tasks, err := LoadSuite(suite)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("LoadSuite = %v, %v", tasks, err)
	}
	base := &config.Config{Providers: map[string]config.ProviderConfig{"m": {Type: config.ProviderMock, Model: "offline"}}}
	base.Roles.Architect, base.Roles.Reflex = "m", "m"
	base.Budget.MaxRepairs, base.Budget.TaskTimeoutSec = 1, 600
	run := func(factory func(*config.Config) (*executor.Executor, func(), error)) Result {
		t.Helper()
		report, err := Run(context.Background(), tasks, Options{Base: base, Factory: factory})
		if err != nil || len(report.Results) != 1 {
			t.Fatalf("Run = %+v, %v", report, err)
		}
		return report.Results[0]
	}
	if r := run(scriptedFactory(t, "// Two returns 2.\nfunc Two() int { return 2 }")); r.Outcome != Solved || !r.Applied || !r.HiddenPassed || len(r.Files) != 1 {
		t.Fatalf("correct edit = %+v", r)
	}
	if r := run(scriptedFactory(t, "// Two returns 2.\nfunc Two() int { return 3 }")); r.Outcome != FalsePositive || !r.Applied || r.HiddenPassed || !strings.Contains(r.CheckOutput, "bad") {
		t.Fatalf("wrong but compiling edit = %+v", r)
	}
	// The built-in offline engine plans a read and a verification but edits nothing.
	if r := run(nil); r.Outcome != Unsolved || r.Applied || !strings.Contains(r.Error, "without changing") {
		t.Fatalf("no edit = %+v", r)
	}
}

func TestMarkdownAndBaseline(t *testing.T) {
	report := &Report{Results: []Result{{Task: "a", Outcome: Solved}, {Task: "b", Outcome: FalsePositive}}, Solved: 1, FalsePositive: 1}
	md := report.Markdown(&Report{Results: []Result{{Task: "b", Outcome: Solved}}, Solved: 1})
	for _, want := range []string{"| b | false_positive |", "solved → false_positive", "Solved 1/2 (50%) · false positives 1", "Baseline: solved 1/1"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown lacks %q:\n%s", want, md)
		}
	}
}

func TestLoadSuiteRejectsIncompleteTasks(t *testing.T) {
	suite := t.TempDir()
	if err := os.MkdirAll(filepath.Join(suite, "bad"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(suite, "bad", "task.json"), []byte(`{"instruction":"x"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSuite(suite); err == nil {
		t.Fatal("task without check accepted")
	}
}
