package architect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/provider"
)

// SubtaskAction represents the operational type for an atomic step
type SubtaskAction string

const (
	ActionRead   SubtaskAction = "read"
	ActionPatch  SubtaskAction = "patch"
	ActionCreate SubtaskAction = "create"
	ActionDelete SubtaskAction = "delete"
	ActionVerify SubtaskAction = "verify"
)

// Subtask is an atomic unit of execution devised by the Architect
type Subtask struct {
	ID           string        `json:"id"`
	Description  string        `json:"description"`
	Action       SubtaskAction `json:"action"`
	TargetFile   string        `json:"target_file,omitempty"`
	TargetSymbol string        `json:"target_symbol,omitempty"`
	Instruction  string        `json:"instruction"`
}

// ExecutionPlan represents the architectural roadmap before surgery
type ExecutionPlan struct {
	PlanID             string            `json:"plan_id"`
	TaskID             string            `json:"task_id"`
	Objective          string            `json:"objective"`
	AffectedFiles      []string          `json:"affected_files"`
	AffectedSymbols    []string          `json:"affected_symbols,omitempty"`
	Subtasks           []Subtask         `json:"subtasks"`
	AcceptanceCriteria []string          `json:"acceptance_criteria"`
	Checks             []AcceptanceCheck `json:"checks,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
}

type AcceptanceCheck struct {
	Criterion string   `json:"criterion"`
	Kind      string   `json:"kind"` // file_exists, file_absent, contains, not_contains, go_symbol
	File      string   `json:"file,omitempty"`
	Files     []string `json:"files,omitempty"`
	Value     string   `json:"value,omitempty"`
}

// Validate checks that the ExecutionPlan is well-formed and safe to execute
func (p *ExecutionPlan) Validate() error {
	if len(p.Subtasks) == 0 {
		return errors.New("plan has no subtasks")
	}
	for _, st := range p.Subtasks {
		switch st.Action {
		case ActionRead, ActionPatch, ActionCreate, ActionDelete, ActionVerify:
		default:
			return fmt.Errorf("subtask %s: unknown action %q", st.ID, st.Action)
		}
		if st.TargetFile == "" {
			if st.Action != ActionVerify {
				return fmt.Errorf("subtask %s: action %s requires target_file", st.ID, st.Action)
			}
			continue
		}
		if st.TargetFile == "." || st.TargetFile == "./" || st.TargetFile == ".\\" {
			if st.Action != ActionRead && st.Action != ActionVerify {
				return fmt.Errorf("subtask %s: action %s cannot target directory %q (must specify a target file)", st.ID, st.Action, st.TargetFile)
			}
		}
		if !filepath.IsLocal(st.TargetFile) {
			return fmt.Errorf("subtask %s: target_file %q escapes workspace", st.ID, st.TargetFile)
		}
	}
	for _, c := range p.Checks {
		if c.Kind != "verification" && c.Kind != "only_changes" && c.Kind != "browser" && (!filepath.IsLocal(c.File) || c.File == ".") {
			return errors.New("acceptance check requires a workspace file")
		}
		switch c.Kind {
		case "file_exists", "file_absent", "unchanged":
		case "verification":
			if !validStageValue(c.Value) {
				return errors.New(`verification check requires "all" or a verification stage name (e.g. test, build, lint, typecheck)`)
			}
		case "only_changes":
			if len(c.Files) == 0 {
				return errors.New("only_changes check requires files")
			}
			for _, file := range c.Files {
				if !filepath.IsLocal(file) || filepath.Clean(file) == "." {
					return errors.New("only_changes requires local file paths")
				}
			}
		case "browser":
			if strings.TrimSpace(c.Value) == "" {
				return errors.New("browser check requires value (text the rendered page must show)")
			}
			if c.File != "" && (!strings.HasPrefix(c.File, "/") || strings.Contains(c.File, "..") || strings.Contains(c.File, "://") || len(c.File) > 200) {
				return errors.New(`browser check file must be a page path such as "/" or "/about"`)
			}
		case "contains", "not_contains", "go_symbol":
			if c.Value == "" {
				return errors.New("acceptance check requires value")
			}
		default:
			return fmt.Errorf("unknown acceptance check %q", c.Kind)
		}
		found := false
		for _, criterion := range p.AcceptanceCriteria {
			if criterion == c.Criterion {
				found = true
			}
		}
		if !found {
			return errors.New("check must reference an acceptance criterion")
		}
	}
	return nil
}

// Architect plans and decomposes tasks
type Architect struct {
	Provider provider.Provider
	Model    string
}

// NewArchitect creates a new Architect
func NewArchitect(p provider.Provider) *Architect {
	return &Architect{
		Provider: p,
	}
}

// BuildPlanningPrompt creates a structured prompt for the Architect model
func BuildPlanningPrompt(task core.Task, codebaseOverview string) string {
	return fmt.Sprintf(`You are the ARCHITECT in the APW Triad Agent Engine.
Your mission is to analyze the user task and formulate a structured ExecutionPlan.

User Task:
Instruction: %s
Workspace: %s

Codebase Overview:
%s

Rules:
1. Adhere to the Claude Code Surgeon philosophy: Plan minimal, surgical, targeted edits. Avoid rewriting entire files.
2. Decompose into atomic subtasks (actions: read, patch, create, delete, verify). For read/patch/create/delete, target_file must be a specific relative file path (e.g. "src/components/Button.tsx" or "internal/surgeon/worktree.go"), never a directory or ".".
   Follow the project's language, framework and conventions shown in the Project profile and source. Files listed as not included exist; add a read step before patching one.
3. Explicitly list all affected files and acceptance criteria.
   For EVERY acceptance criterion, including verification and scope restrictions, include a matching entry in checks. Do not leave any criterion without an executable check.
   Supported check kinds:
   - file_exists/file_absent/contains/not_contains: use file and value as applicable. go_symbol (Go files only): value is a top-level Go identifier.
   - unchanged: use file; APW compares its state with the original snapshot.
   - only_changes: use files (an array of allowed relative paths); APW compares all snapshot changes.
   - verification: use value "all" or a stage name from the Verification pipeline in the Project profile; APW runs only that detected or configured pipeline, never commands you propose.
   - browser (only when the Project profile lists a Browser check): file is the page path ("/" by default), value is text the rendered page must visibly show; APW renders the built page headlessly.
   Example scope check: {"criterion":"Only src/App.tsx changes","kind":"only_changes","files":["src/App.tsx"]}.
   Example test check: {"criterion":"Tests pass","kind":"verification","value":"test"}.
   Checks must use source evidence; do not substitute a generic build-pass criterion for requested behavior.
4. Output valid JSON adhering to the ExecutionPlan schema:
{
  "objective": "...",
  "affected_files": ["..."],
  "affected_symbols": ["..."],
  "subtasks": [
    {
      "id": "step-1",
      "description": "...",
      "action": "read|patch|create|delete|verify",
      "target_file": "...",
      "target_symbol": "...",
      "instruction": "..."
    }
  ],
  "acceptance_criteria": ["..."],
  "checks": [{"criterion":"...","kind":"contains","file":"relative/path/to/file","value":"expected behavior in source"}]
}
Respond with ONLY the JSON object, wrapped in a single `+"```json ... ```"+` block.`, task.Instruction, task.Workspace, codebaseOverview)
}

// ParsePlan extracts ExecutionPlan from raw model response
// planCandidates lists ways to extract the plan JSON from a model response, most
// literal first. Models often wrap JSON in a ```json fence while string values
// (for example a check expecting a ```bash block) contain fences too, so cutting
// at the first closing fence alone can truncate the plan.
func planCandidates(raw string) []string {
	var out []string
	add := func(c string) {
		c = strings.TrimSpace(c)
		for _, seen := range out {
			if seen == c {
				return
			}
		}
		if c != "" {
			out = append(out, c)
		}
	}
	if t := strings.TrimSpace(raw); strings.HasPrefix(t, "{") {
		add(t)
	}
	if idx := strings.Index(raw, "```"); idx != -1 {
		rest := raw[idx+3:]
		if nl := strings.IndexByte(rest, '\n'); nl != -1 && !strings.ContainsAny(strings.TrimSpace(rest[:nl]), "{}\" ") {
			rest = rest[nl+1:] // drop the language tag line, e.g. "json"
		}
		if last := strings.LastIndex(rest, "```"); last != -1 {
			add(rest[:last])
		}
		if first := strings.Index(rest, "```"); first != -1 {
			add(rest[:first])
		}
	}
	if startIdx := strings.Index(raw, "{"); startIdx != -1 {
		if endIdx := strings.LastIndex(raw, "}"); endIdx > startIdx {
			add(raw[startIdx : endIdx+1])
		}
	}
	return out
}

// decodePlan strictly decodes one candidate: known fields only, no trailing content.
func decodePlan(jsonStr string) (*ExecutionPlan, error) {
	var plan ExecutionPlan
	decoder := json.NewDecoder(bytes.NewReader([]byte(jsonStr)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("trailing plan content")
	}
	return &plan, nil
}

func ParsePlan(taskID, raw string) (*ExecutionPlan, error) {
	candidates := planCandidates(raw)
	if len(candidates) == 0 {
		return nil, errors.New("empty plan response")
	}
	var plan *ExecutionPlan
	var firstErr error
	for _, c := range candidates {
		p, err := decodePlan(c)
		if err == nil {
			plan = p
			break
		}
		if firstErr == nil {
			if err.Error() == "trailing plan content" {
				firstErr = err
			} else {
				firstErr = fmt.Errorf("failed to unmarshal execution plan: %w\nRaw: %s", err, c)
			}
		}
	}
	if plan == nil {
		return nil, firstErr
	}
	if err := plan.Validate(); err != nil {
		return nil, fmt.Errorf("invalid execution plan: %w", err)
	}

	plan.PlanID = fmt.Sprintf("plan-%d", time.Now().UnixNano())
	plan.TaskID = taskID
	plan.CreatedAt = time.Now()
	return plan, nil
}

// Plan coordinates with the model provider to formulate an ExecutionPlan
func (a *Architect) Plan(ctx context.Context, task core.Task, codebaseOverview string) (*ExecutionPlan, error) {
	if a.Provider == nil {
		return nil, errors.New("architect: model provider not configured")
	}

	prompt := BuildPlanningPrompt(task, codebaseOverview)
	req := core.ModelRequest{
		RequestID: fmt.Sprintf("req-%d", time.Now().UnixNano()),
		Prompt:    prompt,
		Model:     a.Model,
	}

	res, err := a.Provider.Ask(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("architect planning query failed: %w", err)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if res == nil || !res.Completed {
		return nil, errors.New("architect returned incomplete response")
	}

	return ParsePlan(task.ID, res.Content)
}

// validStageValue accepts "all" or a verification stage name (same rule as
// apw.json verify.stages names).
func validStageValue(v string) bool {
	if v == "" || len(v) > 32 || v[0] < 'a' || v[0] > 'z' {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == ':') {
			return false
		}
	}
	return true
}
