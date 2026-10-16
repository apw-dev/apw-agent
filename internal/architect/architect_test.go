package architect

import (
	"testing"
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
