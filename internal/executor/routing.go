package executor

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
)

// FailoverSnapshot preserves cooldowns across checkpoints without storing credentials.
type FailoverSnapshot struct {
	Models    map[string]time.Time `json:"models,omitempty"`
	Groups    map[string]time.Time `json:"groups,omitempty"`
	Preferred string               `json:"preferred,omitempty"`
}

func agyQuotaGroup(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "argon"):
		return "agy:argon"
	case strings.Contains(m, "gemini"):
		return "agy:gemini"
	case strings.Contains(m, "claude"), strings.Contains(m, "gpt-oss"):
		return "agy:claude-gpt-oss"
	default:
		return "agy:" + m
	}
}

func candidateID(c FailoverCandidate) string {
	if c.ProviderKey != "" {
		return c.ProviderKey
	}
	return c.Provider.Name() + ":" + c.Model
}

func candidateGroup(c FailoverCandidate) string {
	if c.QuotaGroup != "" {
		return c.QuotaGroup
	}
	if c.Provider.Name() == "agy" {
		return agyQuotaGroup(c.Model)
	}
	return c.Provider.Name()
}

var resetDurationPattern = regexp.MustCompile(`(?i)(?:resets?|refreshes?) in\s+([0-9]+(?:\.[0-9]+)?[hms](?:\s*[0-9]+(?:\.[0-9]+)?[hms])*)`)

func quotaFailure(err error) bool {
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "quota") || strings.Contains(m, "insufficient_quota") || strings.Contains(m, "credit")
}

func quotaCooldown(err error) time.Duration {
	if match := resetDurationPattern.FindStringSubmatch(err.Error()); len(match) > 1 {
		if d, parseErr := time.ParseDuration(strings.ReplaceAll(match[1], " ", "")); parseErr == nil && d > 0 {
			return d
		}
	}
	return 5 * time.Minute
}

func (e *Executor) directiveCandidates() []FailoverCandidate {
	var all []FailoverCandidate
	if len(e.ReflexPool) > 0 {
		all = append(all, e.ReflexPool...)
	} else if e.ReflexProvider != nil {
		all = append(all, FailoverCandidate{Provider: e.ReflexProvider, Model: e.ReflexModel})
	}
	if len(e.ArchitectPool) > 0 {
		all = append(all, e.ArchitectPool...)
	} else if e.Architect != nil && e.Architect.Provider != nil {
		all = append(all, FailoverCandidate{Provider: e.Architect.Provider, Model: e.Architect.Model})
	}
	var result []FailoverCandidate
	seen := make(map[string]bool)
	for _, c := range all {
		if c.Provider == nil || seen[candidateID(c)] {
			continue
		}
		seen[candidateID(c)] = true
		result = append(result, c)
	}
	for i, c := range result {
		if candidateID(c) == e.Failover.Preferred {
			copy(result[1:i+1], result[:i])
			result[0] = c
			break
		}
	}
	return result
}

func (e *Executor) askDirective(ctx context.Context, prompt string) (*patchDirective, error) {
	prompt += "\nPreviously read source context:\n" + e.ReadContext
	if e.Failover.Models == nil {
		e.Failover.Models = make(map[string]time.Time)
	}
	if e.Failover.Groups == nil {
		e.Failover.Groups = make(map[string]time.Time)
	}
	timeout := e.DirectiveTimeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	var failures []error
	for _, cand := range e.directiveCandidates() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id, group := candidateID(cand), candidateGroup(cand)
		now := time.Now()
		until := e.Failover.Models[id]
		if e.Failover.Groups[group].After(until) {
			until = e.Failover.Groups[group]
		}
		if now.Before(until) {
			failures = append(failures, fmt.Errorf("%s paused until %s", cand.Model, until.Format(time.RFC3339)))
			e.emit(ProgressEvent{Stage: "step_directive_fallback", Message: fmt.Sprintf("Skipping %s (%s): paused until %s", cand.Model, group, until.Format(time.RFC3339))})
			continue
		}
		e.emit(ProgressEvent{Stage: "directive_attempt", Message: fmt.Sprintf("Requesting patch from %s (%s; timeout %v)", cand.Model, cand.Provider.Name(), timeout)})
		started := time.Now()
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		resp, err := cand.Provider.Ask(attemptCtx, core.ModelRequest{
			RequestID: fmt.Sprintf("req-%d", time.Now().UnixNano()), Prompt: prompt, Model: cand.Model,
		})
		if attemptCtx.Err() != nil {
			err = attemptCtx.Err()
		}
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		parseFailure := false
		if err == nil {
			if resp != nil && !resp.Completed {
				err = errors.New("model returned incomplete response")
			}
		}
		if err == nil {
			if resp == nil || strings.TrimSpace(resp.Content) == "" {
				err = errors.New("model returned empty response")
			} else {
				var directive *patchDirective
				directive, err = parsePatchDirective(resp.Content)
				if err == nil {
					e.Failover.Preferred = id
					return directive, nil
				}
				parseFailure = true
			}
		}
		failures = append(failures, fmt.Errorf("%s: %w", cand.Model, err))
		if !parseFailure && !IsFailoverError(err) {
			return nil, fmt.Errorf("directive provider %s failed: %w", cand.Model, err)
		}
		reason := "invalid directive"
		if !parseFailure && quotaFailure(err) {
			until = time.Now().Add(quotaCooldown(err))
			e.Failover.Groups[group] = until
			reason = fmt.Sprintf("quota exhausted for %s; paused until %s", group, until.Format(time.RFC3339))
		} else {
			e.Failover.Models[id] = time.Now().Add(5 * time.Minute)
			if !parseFailure {
				reason = err.Error()
			}
		}
		e.emit(ProgressEvent{Stage: "step_directive_fallback", Duration: time.Since(started), Message: fmt.Sprintf("%s failed after %v: %s; trying next eligible configured provider", cand.Model, time.Since(started).Round(time.Millisecond), reason)})
	}
	if len(failures) == 0 {
		return nil, errors.New("no providers available for directive generation")
	}
	return nil, fmt.Errorf("all configured directive candidates unavailable: %w", errors.Join(failures...))
}

// errReviewIncomplete keeps the review error text stable while letting the
// architect failover treat an unusable review as a reason to try a fallback.
var errReviewIncomplete = errors.New("review response is incomplete")

// architectCandidates returns the architect role followed by its explicit
// fallbacks. Planning and review never borrow reflex-only providers.
func (e *Executor) architectCandidates() []FailoverCandidate {
	pool := e.ArchitectPool
	if len(pool) == 0 && e.Architect != nil && e.Architect.Provider != nil {
		pool = []FailoverCandidate{{Provider: e.Architect.Provider, Model: e.Architect.Model}}
	}
	var result []FailoverCandidate
	seen := make(map[string]bool)
	for _, c := range pool {
		if c.Provider == nil || seen[candidateID(c)] {
			continue
		}
		seen[candidateID(c)] = true
		result = append(result, c)
	}
	return result
}

// withArchitectFailover runs attempt against the architect pool in order. It
// moves to the next candidate only for failover-eligible errors (quota, rate
// limit, timeout, empty or incomplete output) and shares cooldowns with
// directive routing. Cancellation and fatal errors stop immediately.
func (e *Executor) withArchitectFailover(ctx context.Context, stage string, attempt func(context.Context, FailoverCandidate) error) error {
	candidates := e.architectCandidates()
	if len(candidates) == 0 {
		return errors.New("architect: model provider not configured")
	}
	if e.Failover.Models == nil {
		e.Failover.Models = make(map[string]time.Time)
	}
	if e.Failover.Groups == nil {
		e.Failover.Groups = make(map[string]time.Time)
	}
	// Skip cooled-down candidates, but never skip every candidate: a lone or
	// fully paused pool still gets its attempt, matching single-provider behavior.
	var eligible []FailoverCandidate
	now := time.Now()
	for _, c := range candidates {
		until := e.Failover.Models[candidateID(c)]
		if g := e.Failover.Groups[candidateGroup(c)]; g.After(until) {
			until = g
		}
		if now.Before(until) {
			e.emit(ProgressEvent{Stage: stage + "_fallback", Message: fmt.Sprintf("Skipping %s: paused until %s", c.Model, until.Format(time.RFC3339))})
			continue
		}
		eligible = append(eligible, c)
	}
	if len(eligible) == 0 {
		eligible = candidates
	}
	var failures []error
	for i, cand := range eligible {
		if err := ctx.Err(); err != nil {
			return err
		}
		started := time.Now()
		err := attempt(ctx, cand)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.Canceled) || !(IsFailoverError(err) || errors.Is(err, errReviewIncomplete)) {
			return err
		}
		failures = append(failures, err)
		id, group := candidateID(cand), candidateGroup(cand)
		if quotaFailure(err) {
			e.Failover.Groups[group] = time.Now().Add(quotaCooldown(err))
		} else {
			e.Failover.Models[id] = time.Now().Add(5 * time.Minute)
		}
		if i < len(eligible)-1 {
			e.emit(ProgressEvent{Stage: stage + "_fallback", Duration: time.Since(started), Message: fmt.Sprintf("%s failed after %v: %v; trying next configured architect", cand.Model, time.Since(started).Round(time.Millisecond), err)})
		}
	}
	if len(failures) == 1 {
		return failures[0]
	}
	return fmt.Errorf("all %d architect candidates failed: %w", len(failures), errors.Join(failures...))
}

// PlanTask asks the architect pool for an execution plan, failing over to the
// explicitly configured architect fallbacks.
func (e *Executor) PlanTask(ctx context.Context, task core.Task, overview string) (*architect.ExecutionPlan, error) {
	var plan *architect.ExecutionPlan
	err := e.withArchitectFailover(ctx, "planning", func(ctx context.Context, c FailoverCandidate) error {
		a := architect.NewArchitect(c.Provider)
		a.Model = c.Model
		p, err := a.Plan(ctx, task, overview)
		if err == nil {
			plan = p
		}
		return err
	})
	return plan, err
}
