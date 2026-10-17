package executor

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/project"
	"github.com/apiwat/apw-agent/internal/research"
)

// strongPlanJSON has an executable check for every criterion, including scope and verification.
const strongPlanJSON = `{"objective":"strong","affected_files":["main.go"],"subtasks":[{"id":"one","action":"patch","target_file":"main.go","instruction":"return hello"}],"acceptance_criteria":["updated","tests","scope"],"checks":[{"criterion":"updated","kind":"contains","file":"main.go","value":"hello"},{"criterion":"tests","kind":"verification","value":"all"},{"criterion":"scope","kind":"only_changes","files":["main.go"]}]}`

// weakPlanJSON leaves a criterion unchecked and edits a file it does not declare.
const weakPlanJSON = `{"objective":"weak","affected_files":["main.go"],"subtasks":[{"id":"one","action":"patch","target_file":"main.go","instruction":"x"},{"id":"two","action":"patch","target_file":"other.go","instruction":"y"}],"acceptance_criteria":["updated","works"],"checks":[{"criterion":"updated","kind":"contains","file":"main.go","value":"hello"}]}`

// scripted answers requests in order; it is safe for the parallel proposal calls.
type scripted struct {
	name    string
	mu      sync.Mutex
	replies []func(core.ModelRequest) (*core.ModelResponse, error)
	prompts []string
}

func (s *scripted) Name() string { return s.name }
func (s *scripted) Close() error { return nil }
func (s *scripted) Ask(_ context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, req.Prompt)
	if len(s.replies) == 0 {
		return nil, errors.New("no reply scripted")
	}
	next := s.replies[0]
	s.replies = s.replies[1:]
	return next(req)
}

func reply(content string) func(core.ModelRequest) (*core.ModelResponse, error) {
	return func(core.ModelRequest) (*core.ModelResponse, error) {
		return &core.ModelResponse{Completed: true, Content: content}, nil
	}
}

func proposalExecutor(t *testing.T, a, b *scripted) *Executor {
	t.Helper()
	ex := New(t.TempDir(), a, a, "a-model", "a-model", 1)
	ex.ArchitectPool = []FailoverCandidate{
		{Provider: a, Model: "a-model", ProviderKey: "a"},
		{Provider: b, Model: "b-model", ProviderKey: "b"},
	}
	ex.Proposals = 2
	return ex
}

func TestProposalsPickTheBestScoredPlan(t *testing.T) {
	weak := &scripted{name: "weak", replies: []func(core.ModelRequest) (*core.ModelResponse, error){reply(weakPlanJSON)}}
	strong := &scripted{name: "strong", replies: []func(core.ModelRequest) (*core.ModelResponse, error){reply(strongPlanJSON)}}
	ex := proposalExecutor(t, weak, strong)
	plan, err := ex.PlanTask(context.Background(), core.Task{ID: "task-1", Instruction: "greet"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Objective != "strong" || plan.Ranking == nil || plan.Ranking.Chosen != 1 || len(plan.Ranking.Proposals) != 2 {
		t.Fatalf("plan = %+v ranking = %+v", plan.Objective, plan.Ranking)
	}
	w, s := plan.Ranking.Proposals[0], plan.Ranking.Proposals[1]
	if w.Model != "a-model" || s.Score != 100 || w.Score >= s.Score || !strings.Contains(strings.Join(w.Reasons, ";"), `"works" has no executable check`) || !strings.Contains(strings.Join(w.Reasons, ";"), "other.go") {
		t.Fatalf("scores: weak=%+v strong=%+v", w, s)
	}
}

func TestProposalsSurviveAFailedCandidateAndCoolItDown(t *testing.T) {
	failing := &scripted{name: "agy", replies: []func(core.ModelRequest) (*core.ModelResponse, error){
		func(core.ModelRequest) (*core.ModelResponse, error) {
			return nil, errors.New("429 quota exceeded; resets in 2m")
		},
	}}
	ok := &scripted{name: "codex", replies: []func(core.ModelRequest) (*core.ModelResponse, error){reply(weakPlanJSON)}}
	ex := proposalExecutor(t, failing, ok)
	plan, err := ex.PlanTask(context.Background(), core.Task{ID: "task-1", Instruction: "greet"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Objective != "weak" || plan.Ranking.Chosen != 1 || !strings.Contains(plan.Ranking.Proposals[0].Error, "quota") || len(ex.Failover.Groups) != 1 {
		t.Fatalf("ranking = %+v groups = %v", plan.Ranking, ex.Failover.Groups)
	}
	both := &scripted{name: "x", replies: []func(core.ModelRequest) (*core.ModelResponse, error){reply("not json")}}
	none := &scripted{name: "y", replies: []func(core.ModelRequest) (*core.ModelResponse, error){reply("still not json")}}
	if _, err := proposalExecutor(t, both, none).PlanTask(context.Background(), core.Task{ID: "task-2"}, ""); err == nil || !strings.Contains(err.Error(), "all 2 plan proposals failed") {
		t.Fatalf("expected all proposals to fail: %v", err)
	}
}

func TestJudgeCombinesWithRuleScores(t *testing.T) {
	// The primary also judges: it rates the weak plan 10 and the strong plan 0.
	weak := &scripted{name: "weak", replies: []func(core.ModelRequest) (*core.ModelResponse, error){
		reply(weakPlanJSON),
		reply(`{"scores":[{"index":1,"score":10,"reason":"touches both files"},{"index":2,"score":0,"reason":"too narrow"}]}`),
	}}
	strong := &scripted{name: "strong", replies: []func(core.ModelRequest) (*core.ModelResponse, error){reply(strongPlanJSON)}}
	ex := proposalExecutor(t, weak, strong)
	ex.JudgeProposals = true
	plan, err := ex.PlanTask(context.Background(), core.Task{ID: "task-1", Instruction: "greet"}, "")
	if err != nil {
		t.Fatal(err)
	}
	w, s := plan.Ranking.Proposals[0], plan.Ranking.Proposals[1]
	if w.Judge == nil || *w.Judge != 10 || s.Judge == nil || *s.Judge != 0 || w.Score != (w.Static+100)/2 || s.Score != 50 {
		t.Fatalf("combined scores: weak=%+v strong=%+v", w, s)
	}
	if want := 0; (w.Score > s.Score) != (plan.Ranking.Chosen == want) {
		t.Fatalf("chosen %d does not match scores %d/%d", plan.Ranking.Chosen, w.Score, s.Score)
	}
	if !strings.Contains(weak.prompts[1], "Rate each plan") {
		t.Fatal("judge prompt not sent to the primary architect")
	}
}

func TestSingleArchitectIgnoresProposals(t *testing.T) {
	only := &scripted{name: "only", replies: []func(core.ModelRequest) (*core.ModelResponse, error){reply(validPlanJSON)}}
	ex := New(t.TempDir(), only, only, "m", "m", 1)
	ex.Proposals = 3
	plan, err := ex.PlanTask(context.Background(), core.Task{ID: "task-1"}, "")
	if err != nil || plan.Ranking != nil {
		t.Fatalf("plan = %+v err = %v", plan, err)
	}
}

func TestScorePlanPenalizesUnknownStagesAndBrowserChecks(t *testing.T) {
	plan := &architect.ExecutionPlan{
		AffectedFiles:      []string{"main.go"},
		Subtasks:           []architect.Subtask{{ID: "1", Action: architect.ActionPatch, TargetFile: "main.go"}},
		AcceptanceCriteria: []string{"tests", "page"},
		Checks:             []architect.AcceptanceCheck{{Criterion: "tests", Kind: "verification", Value: "e2e"}, {Criterion: "page", Kind: "browser", Value: "Hi"}},
	}
	score, reasons := scorePlan(plan, project.Profile{Stages: []project.Stage{{Name: "test"}}})
	text := strings.Join(reasons, ";")
	if score != 100-15-15-5 || !strings.Contains(text, `stage "e2e"`) || !strings.Contains(text, "browser check without a browser stage") {
		t.Fatalf("score %d reasons %s", score, text)
	}
}

func TestResearchRunsBeforePlanning(t *testing.T) {
	arch := &scripted{name: "arch", replies: []func(core.ModelRequest) (*core.ModelResponse, error){
		reply(`{"fetch":["https://evil.example/collect","http://pkg.go.dev/net/http"],"search":["ignored without search"]}`),
		reply(validPlanJSON),
	}}
	ex := New(t.TempDir(), arch, arch, "m", "m", 1)
	ex.Research = research.New(config.ResearchConfig{Enabled: true})
	var stages []string
	ex.OnProgress = func(ev ProgressEvent) { stages = append(stages, ev.Stage+": "+ev.Message) }
	plan, err := ex.PlanTask(context.Background(), core.Task{ID: "task-1", Instruction: "use net/http"}, "overview")
	if err != nil || plan.Objective != "change greeting" {
		t.Fatalf("plan = %+v err = %v", plan, err)
	}
	if !strings.Contains(arch.prompts[0], "Documentation (external") || len(ex.ResearchSources) != 2 {
		t.Fatalf("research prompt/sources: %q %+v", arch.prompts[0], ex.ResearchSources)
	}
	for _, s := range ex.ResearchSources {
		if s.Error == "" {
			t.Fatalf("disallowed URL fetched: %+v", s)
		}
	}
	if strings.Contains(arch.prompts[1], "External documentation") {
		t.Fatal("failed sources added documentation context")
	}
	if !strings.Contains(strings.Join(stages, "\n"), "research_done: Gathered 0 of 2") {
		t.Fatalf("progress = %v", stages)
	}

	// A malformed research reply never blocks planning.
	bad := &scripted{name: "arch", replies: []func(core.ModelRequest) (*core.ModelResponse, error){reply("I need nothing"), reply(validPlanJSON)}}
	ex = New(t.TempDir(), bad, bad, "m", "m", 1)
	ex.Research = research.New(config.ResearchConfig{Enabled: true})
	if _, err := ex.PlanTask(context.Background(), core.Task{ID: "task-2"}, ""); err != nil {
		t.Fatalf("malformed research reply blocked planning: %v", err)
	}
}
