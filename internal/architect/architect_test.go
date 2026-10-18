package architect

import (
	"context"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/provider/mock"
)

func TestParsePlan(t *testing.T) {
	rawResponse := `Here is the architectural plan:

` + "```json" + `
{
  "objective": "Implement user authentication",
  "affected_files": ["internal/auth/auth.go", "cmd/main.go"],
  "affected_symbols": ["AuthService", "LoginHandler"],
  "subtasks": [
    {
      "id": "step-1",
      "description": "Read current auth service",
      "action": "read",
      "target_file": "internal/auth/auth.go",
      "instruction": "Inspect existing struct"
    },
    {
      "id": "step-2",
      "description": "Surgically add Login method",
      "action": "patch",
      "target_file": "internal/auth/auth.go",
      "target_symbol": "AuthService",
      "instruction": "Add Login(username, password) method"
    }
  ],
  "acceptance_criteria": ["go test ./internal/auth passes", "no compiler errors"]
}
` + "```" + `
Hope this helps!`

	plan, err := ParsePlan("task-123", rawResponse)
	if err != nil {
		t.Fatalf("ParsePlan failed: %v", err)
	}

	if plan.TaskID != "task-123" {
		t.Errorf("expected TaskID task-123, got %s", plan.TaskID)
	}
	if plan.Objective != "Implement user authentication" {
		t.Errorf("expected objective 'Implement user authentication', got '%s'", plan.Objective)
	}
	if len(plan.AffectedFiles) != 2 {
		t.Errorf("expected 2 affected files, got %d", len(plan.AffectedFiles))
	}
	if len(plan.Subtasks) != 2 {
		t.Errorf("expected 2 subtasks, got %d", len(plan.Subtasks))
	}
	if plan.Subtasks[1].Action != ActionPatch {
		t.Errorf("expected subtask 2 action to be patch, got %s", plan.Subtasks[1].Action)
	}
}

func TestParsePlan_Validation(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{
			name:    "empty subtasks",
			raw:     `{"objective":"x","subtasks":[]}`,
			wantErr: true,
		},
		{
			name:    "unknown action",
			raw:     `{"objective":"x","subtasks":[{"id":"1","action":"exec","target_file":"a.go"}]}`,
			wantErr: true,
		},
		{
			name:    "missing target_file for patch",
			raw:     `{"objective":"x","subtasks":[{"id":"1","action":"patch"}]}`,
			wantErr: true,
		},
		{
			name:    "verify without target_file allowed",
			raw:     `{"objective":"x","subtasks":[{"id":"1","action":"verify"}]}`,
			wantErr: false,
		},
		{
			name:    "path traversal rejected",
			raw:     `{"objective":"x","subtasks":[{"id":"1","action":"read","target_file":"../../.ssh/config"}]}`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParsePlan("task-1", tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParsePlan(%s) err = %v, wantErr = %v", tc.name, err, tc.wantErr)
			}
		})
	}
}

func TestArchitect_Plan_MockProvider(t *testing.T) {
	validJSON := "```json\n" + `{
		"objective": "Add healthcheck",
		"affected_files": ["internal/server/health.go"],
		"subtasks": [
			{
				"id": "step-1",
				"description": "Create health handler",
				"action": "create",
				"target_file": "internal/server/health.go",
				"instruction": "Write HealthHandler"
			},
			{
				"id": "step-2",
				"description": "Verify build and tests",
				"action": "verify",
				"instruction": "Run verification suite"
			}
		],
		"acceptance_criteria": ["all tests pass"]
	}` + "\n```"

	invalidTraversalJSON := "```json\n" + `{
		"objective": "Escape workspace",
		"affected_files": ["../x"],
		"subtasks": [
			{
				"id": "step-1",
				"description": "Bad path",
				"action": "patch",
				"target_file": "../x",
				"instruction": "Attempt traversal"
			}
		]
	}` + "\n```"

	mp := &mock.Provider{
		Responses: []string{validJSON, invalidTraversalJSON},
	}

	arch := NewArchitect(mp)
	arch.Model = "claude-sonnet-test"

	task := core.Task{
		ID:          "task-mock-1",
		Workspace:   ".",
		Instruction: "Add healthcheck endpoint",
	}

	// 1. First call returns valid plan
	plan, err := arch.Plan(context.Background(), task, "package server")
	if err != nil {
		t.Fatalf("Architect.Plan failed on valid mock response: %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("plan.Validate() failed: %v", err)
	}
	if len(mp.Requests) != 1 {
		t.Fatalf("expected 1 recorded request, got %d", len(mp.Requests))
	}
	if mp.Requests[0].Model != "claude-sonnet-test" {
		t.Errorf("request model = %q, want %q", mp.Requests[0].Model, "claude-sonnet-test")
	}
	if !strings.Contains(mp.Requests[0].Prompt, "Add healthcheck endpoint") {
		t.Errorf("request prompt did not contain task instruction")
	}

	// 2. Second call returns "../x" target_file -> must fail validation
	_, err = arch.Plan(context.Background(), task, "package server")
	if err == nil {
		t.Fatal("expected error when mock returns target_file '../x', got nil")
	}
	if !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("expected escapes workspace error, got: %v", err)
	}
}

// Regression: a live preview plan was wrapped in a ```json fence and a check value
// held a ```bash block; cutting at the first inner fence truncated the JSON.
func TestParsePlanKeepsFencesInsideStrings(t *testing.T) {
	body := "{\"objective\":\"doc\",\"affected_files\":[\"README.md\"],\"subtasks\":[{\"id\":\"1\",\"action\":\"patch\",\"target_file\":\"README.md\",\"instruction\":\"add a ```bash block\"}]," +
		"\"acceptance_criteria\":[\"has block\"],\"checks\":[{\"criterion\":\"has block\",\"kind\":\"contains\",\"file\":\"README.md\",\"value\":\"```bash\\ngit clone x\\n```\"}]}"
	for name, raw := range map[string]string{
		"fenced json":       "```json\n" + body + "\n```",
		"fenced untagged":   "```\n" + body + "\n```",
		"bare json":         body,
		"prose and fence":   "Here is the plan:\n```json\n" + body + "\n```\nDone.",
		"prose bare braces": "Plan follows " + body + " end",
	} {
		t.Run(name, func(t *testing.T) {
			plan, err := ParsePlan("task-1", raw)
			if err != nil {
				t.Fatalf("ParsePlan: %v", err)
			}
			if len(plan.Subtasks) != 1 || plan.Subtasks[0].Instruction != "add a ```bash block" {
				t.Fatalf("unexpected plan: %+v", plan.Subtasks)
			}
		})
	}
	if _, err := ParsePlan("task-1", "```json\n{\"objective\": \"x\",\n```"); err == nil || !strings.Contains(err.Error(), "failed to unmarshal execution plan") {
		t.Fatalf("truncated plan must still fail clearly, got %v", err)
	}
}

func TestVerificationCheckAcceptsProjectStageNames(t *testing.T) {
	base := `{"objective":"x","subtasks":[{"id":"1","action":"verify"}],"acceptance_criteria":["ok"],"checks":[{"criterion":"ok","kind":"verification","value":"%s"}]}`
	for value, ok := range map[string]bool{"all": true, "test": true, "typecheck": true, "lint:ci": true, "e2e_smoke": true, "": false, "Test": false, "rm -rf": false, "1test": false} {
		_, err := ParsePlan("task-1", strings.Replace(base, "%s", value, 1))
		if (err == nil) != ok {
			t.Fatalf("value %q: err=%v want ok=%v", value, err, ok)
		}
	}
}

func TestBrowserCheckValidation(t *testing.T) {
	base := `{"objective":"x","subtasks":[{"id":"1","action":"verify"}],"acceptance_criteria":["ok"],"checks":[{"criterion":"ok","kind":"browser"%s}]}`
	for extra, ok := range map[string]bool{
		`,"value":"Count is 10"`:                   true,
		`,"file":"/about","value":"About us"`:      true,
		`,"file":"/"`:                              false,
		`,"file":"about","value":"x"`:              false,
		`,"file":"https://evil.test/","value":"x"`: false,
		`,"file":"/../secret","value":"x"`:         false,
	} {
		_, err := ParsePlan("task-1", strings.Replace(base, "%s", extra, 1))
		if (err == nil) != ok {
			t.Fatalf("%s: err=%v want ok=%v", extra, err, ok)
		}
	}
}

func TestPlanningPromptWarnsAgainstLiteralCodeChecks(t *testing.T) {
	prompt := BuildPlanningPrompt(core.Task{Instruction: "x"}, "")
	for _, want := range []string{"cannot be predicted", "variable or constant", "never use contains on test files", "Prove behavior with tests"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("planning prompt lacks %q", want)
		}
	}
}
