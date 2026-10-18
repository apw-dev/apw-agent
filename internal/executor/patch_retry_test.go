package executor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/provider/mock"
)

func retryExecutor(t *testing.T, directives ...string) (string, *Executor, *mock.Provider, *[]string) {
	t.Helper()
	ws, wt, _ := resumeFixture(t)
	if err := wt.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	plan := architect.ExecutionPlan{Objective: "Say hello", AffectedFiles: []string{"main.go"},
		Subtasks: []architect.Subtask{{ID: "patch", Action: architect.ActionPatch, TargetFile: "main.go", Instruction: "return hello"}}}
	planBytes, _ := json.Marshal(plan)
	mp := mock.New(append([]string{string(planBytes)}, directives...)...)
	ex := New(ws, mp, mp, "arch", "reflex", 1)
	ex.Trusted = true
	var stages []string
	ex.OnProgress = func(ev ProgressEvent) { stages = append(stages, ev.Stage+": "+ev.Message) }
	return ws, ex, mp, &stages
}

func TestPatchRetriesWithSurgeonFeedback(t *testing.T) {
	for name, first := range map[string]string{
		"not found": `{"old_content":"return \"hey\"","new_content":"return \"hello\""}`,
		"multiple":  `{"old_content":"\"","new_content":"'"}`,
		"empty":     `{"old_content":"","new_content":"x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, ex, mp, stages := retryExecutor(t, first, `{"old_content":"return \"hi\"","new_content":"return \"hello\""}`)
			report, err := ex.Execute(context.Background(), "Say hello", "File: main.go")
			if err != nil || !report.Passed {
				t.Fatalf("retry did not recover: %+v %v", report, err)
			}
			if len(mp.Requests) != 3 {
				t.Fatalf("model asked %d times, want plan + 2 patch attempts", len(mp.Requests))
			}
			retry := mp.Requests[2].Prompt
			if !strings.Contains(retry, "YOUR PREVIOUS PATCH FOR main.go WAS REJECTED") || !strings.Contains(retry, "func Greet() string") {
				t.Fatalf("retry prompt lacks feedback or current content:\n%s", retry)
			}
			if !strings.Contains(strings.Join(*stages, "\n"), "patch_retry: main.go") {
				t.Fatalf("no patch_retry event: %v", *stages)
			}
			data, _ := os.ReadFile(filepath.Join(report.WorktreePath, "main.go"))
			if !strings.Contains(string(data), `"hello"`) || strings.Contains(string(data), "'") {
				t.Fatalf("worktree file = %s", data)
			}
		})
	}
}

func TestPatchRetryIsBounded(t *testing.T) {
	bad := `{"old_content":"no such text","new_content":"x"}`
	_, ex, mp, _ := retryExecutor(t, bad, bad, bad, bad)
	if _, err := ex.Execute(context.Background(), "Say hello", "File: main.go"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a not-found failure, got %v", err)
	}
	if got := len(mp.Requests); got != 1+patchMatchAttempts {
		t.Fatalf("model asked %d times, want plan + %d patch attempts", got, patchMatchAttempts)
	}
}
