package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/project"
	"github.com/apiwat/apw-agent/internal/research"
)

// researchContext lets the Architect request documentation before planning (apw.json research).
// Research is best effort: a failed request or fetch only removes that context, except that
// cancellation is returned.
func (e *Executor) researchContext(ctx context.Context, task core.Task, overview string) (string, error) {
	e.ResearchSources = nil
	if e.Research == nil {
		return "", nil
	}
	e.emit(ProgressEvent{Stage: "research", Message: "Asking the architect which documentation to read"})
	var req research.Request
	err := e.withArchitectFailover(ctx, "research", func(ctx context.Context, c FailoverCandidate) error {
		resp, err := c.Provider.Ask(ctx, core.ModelRequest{RequestID: task.ID + "-research", Model: c.Model, Prompt: e.Research.Prompt(task.Instruction, overview)})
		if err != nil {
			return err
		}
		if resp == nil || !resp.Completed || strings.TrimSpace(resp.Content) == "" {
			return errReviewIncomplete
		}
		req, err = research.ParseRequest(resp.Content)
		return err
	})
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		e.emit(ProgressEvent{Stage: "research_skipped", Message: "Planning without external documentation: " + err.Error()})
		return "", nil
	}
	if len(req.Fetch)+len(req.Search) == 0 {
		e.emit(ProgressEvent{Stage: "research_done", Message: "No external documentation requested"})
		return "", nil
	}
	e.ResearchSources = e.Research.Run(ctx, req)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	read := 0
	for _, s := range e.ResearchSources {
		label := s.URL
		if s.Query != "" {
			label = "search: " + s.Query
		}
		if s.Error != "" {
			e.emit(ProgressEvent{Stage: "research_source", Message: label + " — " + s.Error})
			continue
		}
		read++
		e.emit(ProgressEvent{Stage: "research_source", Message: fmt.Sprintf("%s — %d bytes", label, s.Bytes)})
	}
	e.emit(ProgressEvent{Stage: "research_done", Message: fmt.Sprintf("Read %d of %d requested documentation sources", read, len(e.ResearchSources))})
	return research.Context(e.ResearchSources), nil
}

type proposal struct {
	candidate FailoverCandidate
	plan      *architect.ExecutionPlan
	err       error
	summary   architect.ProposalSummary
}

// planProposals asks up to e.Proposals architect candidates for plans in parallel, scores the valid
// ones and returns the best, with the ranking recorded on the plan. Candidates in a quota cooldown
// are skipped; failures update cooldowns like the sequential failover does.
func (e *Executor) planProposals(ctx context.Context, task core.Task, overview string) (*architect.ExecutionPlan, error) {
	candidates := e.eligibleArchitects()
	if len(candidates) > e.Proposals {
		candidates = candidates[:e.Proposals]
	}
	e.emit(ProgressEvent{Stage: "proposals", Message: fmt.Sprintf("Asking %d architect models for plans", len(candidates))})
	results := make([]proposal, len(candidates))
	var wg sync.WaitGroup
	for i, c := range candidates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := architect.NewArchitect(c.Provider)
			a.Model = c.Model
			plan, err := a.Plan(ctx, task, overview)
			results[i] = proposal{candidate: c, plan: plan, err: err}
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	profile := e.Profile(e.Workspace)
	var failures []error
	for i := range results {
		r := &results[i]
		r.summary.Model = r.candidate.Model
		if r.err != nil {
			r.summary.Error = r.err.Error()
			failures = append(failures, fmt.Errorf("%s: %w", r.candidate.Model, r.err))
			e.coolDown(r.candidate, r.err)
			continue
		}
		r.summary.PlanID = r.plan.PlanID
		r.summary.Objective = r.plan.Objective
		r.summary.Steps = len(r.plan.Subtasks)
		r.summary.Files = append([]string{}, r.plan.AffectedFiles...)
		r.summary.Static, r.summary.Reasons = scorePlan(r.plan, profile)
		r.summary.Score = r.summary.Static
	}
	var valid []int
	for i, r := range results {
		if r.plan != nil {
			valid = append(valid, i)
		}
	}
	if len(valid) == 0 {
		if len(failures) == 1 {
			return nil, failures[0]
		}
		return nil, fmt.Errorf("all %d plan proposals failed: %w", len(failures), errors.Join(failures...))
	}
	if e.JudgeProposals && len(valid) > 1 {
		e.judge(ctx, task, results, valid)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	best := valid[0]
	for _, i := range valid[1:] {
		if results[i].summary.Score > results[best].summary.Score {
			best = i
		}
	}
	ranking := &architect.PlanRanking{Chosen: best}
	for _, r := range results {
		ranking.Proposals = append(ranking.Proposals, r.summary)
		if r.plan != nil {
			e.emit(ProgressEvent{Stage: "proposal", Message: fmt.Sprintf("%s: score %d, %d steps — %s", r.summary.Model, r.summary.Score, r.summary.Steps, r.summary.Objective)})
		} else {
			e.emit(ProgressEvent{Stage: "proposal", Message: fmt.Sprintf("%s: failed — %s", r.summary.Model, r.summary.Error)})
		}
	}
	plan := results[best].plan
	plan.Ranking = ranking
	e.emit(ProgressEvent{Stage: "proposal_chosen", Message: fmt.Sprintf("Chose the plan from %s (score %d of %d valid proposals)", results[best].summary.Model, results[best].summary.Score, len(valid))})
	return plan, nil
}

// eligibleArchitects returns the architect pool without candidates that are cooling down (or the
// whole pool when every candidate is paused).
func (e *Executor) eligibleArchitects() []FailoverCandidate {
	candidates := e.architectCandidates()
	var eligible []FailoverCandidate
	now := time.Now()
	for _, c := range candidates {
		until := e.Failover.Models[candidateID(c)]
		if g := e.Failover.Groups[candidateGroup(c)]; g.After(until) {
			until = g
		}
		if now.Before(until) {
			continue
		}
		eligible = append(eligible, c)
	}
	if len(eligible) == 0 {
		return candidates
	}
	return eligible
}

func (e *Executor) coolDown(c FailoverCandidate, err error) {
	if !IsFailoverError(err) {
		return
	}
	if e.Failover.Models == nil {
		e.Failover.Models = make(map[string]time.Time)
	}
	if e.Failover.Groups == nil {
		e.Failover.Groups = make(map[string]time.Time)
	}
	if quotaFailure(err) {
		e.Failover.Groups[candidateGroup(c)] = time.Now().Add(quotaCooldown(err))
	} else {
		e.Failover.Models[candidateID(c)] = time.Now().Add(5 * time.Minute)
	}
}

// scorePlan rates a valid plan from 0 to 100 by rules that predict whether APW can execute and
// prove it: every acceptance criterion has an executable check, checks reference stages that exist,
// the plan states its scope, edits stay within its declared files, and it stays small.
func scorePlan(plan *architect.ExecutionPlan, profile project.Profile) (int, []string) {
	score := 100
	var reasons []string
	penalize := func(points int, reason string) {
		score -= points
		reasons = append(reasons, fmt.Sprintf("-%d %s", points, reason))
	}
	checked := map[string]bool{}
	hasVerification, hasScope := false, false
	stages := map[string]bool{"all": true}
	for _, name := range profile.StageNames() {
		stages[name] = true
	}
	_, hasBrowser := profile.BrowserStage()
	for _, c := range plan.Checks {
		checked[c.Criterion] = true
		switch c.Kind {
		case "verification":
			hasVerification = true
			if !stages[c.Value] {
				penalize(15, fmt.Sprintf("verification check names stage %q, which is not in the pipeline", c.Value))
			}
		case "only_changes":
			hasScope = true
		case "browser":
			if !hasBrowser {
				penalize(15, "browser check without a browser stage")
			}
		}
	}
	for _, criterion := range plan.AcceptanceCriteria {
		if !checked[criterion] {
			penalize(15, fmt.Sprintf("criterion %q has no executable check", criterion))
		}
	}
	if len(plan.AcceptanceCriteria) == 0 {
		penalize(20, "no acceptance criteria")
	}
	if !hasVerification {
		penalize(10, "no verification check")
	}
	if !hasScope {
		penalize(5, "no only_changes scope check")
	}
	declared := map[string]bool{}
	for _, f := range plan.AffectedFiles {
		declared[filepath.Clean(f)] = true
	}
	undeclared := map[string]bool{}
	edits := 0
	for _, st := range plan.Subtasks {
		switch st.Action {
		case architect.ActionPatch, architect.ActionCreate, architect.ActionDelete:
			edits++
			if f := filepath.Clean(st.TargetFile); !declared[f] && !undeclared[f] {
				undeclared[f] = true
				penalize(5, fmt.Sprintf("edits %s, which is not in affected_files", st.TargetFile))
			}
		}
	}
	if edits == 0 {
		penalize(30, "plan makes no edits")
	}
	if extra := len(plan.Subtasks) - 4; extra > 0 {
		points := 2 * extra
		if points > 20 {
			points = 20
		}
		penalize(points, fmt.Sprintf("%d steps (smaller plans are preferred)", len(plan.Subtasks)))
	}
	if score < 0 {
		score = 0
	}
	if len(reasons) == 0 {
		reasons = []string{"all criteria have executable checks; scope and verification stated"}
	}
	return score, reasons
}

// judge asks the primary architect to rate the valid proposals 0-10; the combined score weighs the
// judge and the rule score equally. Judge failures keep the rule scores.
func (e *Executor) judge(ctx context.Context, task core.Task, results []proposal, valid []int) {
	type brief struct {
		Index              int                         `json:"index"`
		Objective          string                      `json:"objective"`
		Subtasks           []architect.Subtask         `json:"subtasks"`
		AcceptanceCriteria []string                    `json:"acceptance_criteria"`
		Checks             []architect.AcceptanceCheck `json:"checks,omitempty"`
	}
	var briefs []brief
	for n, i := range valid {
		p := results[i].plan
		briefs = append(briefs, brief{Index: n + 1, Objective: p.Objective, Subtasks: p.Subtasks, AcceptanceCriteria: p.AcceptanceCriteria, Checks: p.Checks})
	}
	data, err := json.MarshalIndent(briefs, "", " ")
	if err != nil {
		return
	}
	prompt := fmt.Sprintf(`You are reviewing alternative execution plans for the same code change. Rate each plan from 0 to 10 for how likely it is to implement the task correctly with minimal, safe edits and checks that prove the requested behavior. Do not favor a plan because of its position.

Task: %s

Plans:
%s

Respond with ONLY JSON: {"scores":[{"index":1,"score":7,"reason":"..."}]}`, task.Instruction, data)
	var scores struct {
		Scores []struct {
			Index  int    `json:"index"`
			Score  int    `json:"score"`
			Reason string `json:"reason"`
		} `json:"scores"`
	}
	e.emit(ProgressEvent{Stage: "proposal_judge", Message: fmt.Sprintf("Asking the architect to rate %d plans", len(valid))})
	err = e.withArchitectFailover(ctx, "judge", func(ctx context.Context, c FailoverCandidate) error {
		resp, err := c.Provider.Ask(ctx, core.ModelRequest{RequestID: task.ID + "-judge", Model: c.Model, Prompt: prompt})
		if err != nil {
			return err
		}
		if resp == nil || !resp.Completed {
			return errReviewIncomplete
		}
		raw := resp.Content
		start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
		if start < 0 || end <= start {
			return errors.New("judge returned no JSON")
		}
		return json.Unmarshal([]byte(raw[start:end+1]), &scores)
	})
	if err != nil {
		e.emit(ProgressEvent{Stage: "proposal_judge", Message: "Judge unavailable; using rule scores: " + err.Error()})
		return
	}
	seen := map[int]bool{}
	for _, s := range scores.Scores {
		if s.Index < 1 || s.Index > len(valid) || seen[s.Index] {
			continue
		}
		seen[s.Index] = true
		score := s.Score
		if score < 0 {
			score = 0
		}
		if score > 10 {
			score = 10
		}
		sum := &results[valid[s.Index-1]].summary
		sum.Judge = &score
		sum.Score = (sum.Static + score*10) / 2
		if reason := strings.TrimSpace(s.Reason); reason != "" {
			if len(reason) > 300 {
				reason = reason[:300] + "..."
			}
			sum.Reasons = append(sum.Reasons, "judge: "+reason)
		}
	}
}
