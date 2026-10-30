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

var resetDurationPattern = regexp.MustCompile(`(?i)(?:resets?|refreshes?|retry[- ]after) in\s+([0-9]+(?:\.[0-9]+)?[hms](?:\s*[0-9]+(?:\.[0-9]+)?[hms])*)`)

func quotaFailure(err error) bool {
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "quota") || strings.Contains(m, "insufficient_quota") || strings.Contains(m, "credit") || strings.Contains(m, "rate_limit_error") || strings.Contains(m, "429")
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

// scaledTimeout lengthens an explicitly configured per-candidate directive timeout with the request size:
// a large file or context takes longer to read and to answer. When base <= 0 (default), it returns 0
// so model requests are event/trigger-driven (completing when the process/stream finishes or the user cancels).
func scaledTimeout(base time.Duration, promptBytes int) time.Duration {
	if base <= 0 {
		return 0
	}
	const step, extra, limit = 16 * 1024, 30 * time.Second, 5 * time.Minute
	if base >= limit || promptBytes <= step {
		return base
	}
	scaled := base + time.Duration((promptBytes-1)/step)*extra
	if scaled > limit {
		scaled = limit
	}
	return scaled
}

// askDirective asks for a patch directive. While the task's delegation budget lasts, the model may
// answer once with a delegation request instead; the subagents' findings are then added to the
// prompt and the directive is requested again without the offer.
func (e *Executor) askDirective(ctx context.Context, prompt string) (*patchDirective, error) {
	offer := e.delegateOffer()
	dir, err := e.askDirectiveOnce(ctx, prompt+offer)
	if err != nil || len(dir.Delegate) == 0 {
		return dir, err
	}
	if !dir.delegationOnly() {
		dir.Delegate = nil // an edit was given as well; use it
		return dir, nil
	}
	if offer == "" {
		return nil, errDelegationUnavailable
	}
	findings := e.runDelegates(ctx, e.delegateRoot(), "Step", dir.Delegate)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err = e.askDirectiveOnce(ctx, prompt+findings+"\nDelegation is no longer available for this request; respond now with the requested JSON directive.\n")
	if err == nil && dir.delegationOnly() {
		return nil, errDelegationUnavailable
	}
	return dir, err
}

func (e *Executor) askDirectiveOnce(ctx context.Context, prompt string) (*patchDirective, error) {
	cachePrefix := "CRITICAL: Do NOT use any tools, shell commands, apply_patch, or file readers. Act purely as an offline JSON generator and output ONLY the requested JSON object." + InstructionsBlock(e.Workspace) + "\nPreviously read source context:\n" + e.ReadContext
	fullPrompt := cachePrefix + "\n" + prompt
	if e.Failover.Models == nil {
		e.Failover.Models = make(map[string]time.Time)
	}
	if e.Failover.Groups == nil {
		e.Failover.Groups = make(map[string]time.Time)
	}

	allCands := e.directiveCandidates()
	var candInfos []core.RouteCandidateInfo
	for idx, c := range allCands {
		r := "fallback"
		if idx == 0 {
			r = "primary"
		}
		candInfos = append(candInfos, core.RouteCandidateInfo{
			ProviderKey: candidateID(c),
			Provider:    c.Provider.Name(),
			Model:       c.Model,
			Priority:    idx + 1,
			Role:        r,
			Status:      "ready",
		})
	}
	if len(candInfos) > 0 {
		e.emit(ProgressEvent{
			Stage:           "route_stack",
			Message:         fmt.Sprintf("Directive route combo ready (%d candidates)", len(candInfos)),
			RouteCandidates: candInfos,
		})
	}

	timeout := scaledTimeout(e.DirectiveTimeout, len(fullPrompt))
	var failures []error
	var modelPaused []FailoverCandidate
	for _, cand := range e.directiveCandidates() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id, group := candidateID(cand), candidateGroup(cand)
		now := time.Now()
		modelUntil := e.Failover.Models[id]
		groupUntil := e.Failover.Groups[group]
		until := modelUntil
		if groupUntil.After(until) {
			until = groupUntil
		}
		if now.Before(until) {
			if !now.Before(groupUntil) {
				modelPaused = append(modelPaused, cand)
			}
			failures = append(failures, fmt.Errorf("%s paused until %s", cand.Model, until.Format(time.RFC3339)))
			e.emit(ProgressEvent{Stage: "step_directive_fallback", Message: fmt.Sprintf("Skipping %s (%s): paused until %s", cand.Model, group, until.Format(time.RFC3339))})
			continue
		}
		directive, fatalErr, attemptErr := e.tryDirectiveCandidate(ctx, cand, cachePrefix, prompt, timeout)
		if directive != nil {
			return directive, nil
		}
		if fatalErr != nil {
			return nil, fatalErr
		}
		if attemptErr != nil {
			failures = append(failures, attemptErr)
		}
	}
	// Never let temporary model-level cooldowns deadlock the pool when quota is still available.
	if len(modelPaused) > 0 {
		failures = nil
		for _, cand := range modelPaused {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			id := candidateID(cand)
			delete(e.Failover.Models, id)
			directive, fatalErr, attemptErr := e.tryDirectiveCandidate(ctx, cand, cachePrefix, prompt, timeout)
			if directive != nil {
				return directive, nil
			}
			if fatalErr != nil {
				return nil, fatalErr
			}
			if attemptErr != nil {
				failures = append(failures, attemptErr)
			}
		}
	}
	if len(failures) == 0 {
		return nil, errors.New("no providers available for directive generation")
	}
	return nil, fmt.Errorf("all configured directive candidates unavailable: %w", errors.Join(failures...))
}

func (e *Executor) tryDirectiveCandidate(ctx context.Context, cand FailoverCandidate, cachePrefix, prompt string, timeout time.Duration) (*patchDirective, error, error) {
	id, group := candidateID(cand), candidateGroup(cand)
	reqID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	fullPrompt := cachePrefix + "\n" + prompt
	modeLabel := "event-triggered"
	if timeout > 0 {
		modeLabel = fmt.Sprintf("timeout %v", timeout)
	}
	e.emit(ProgressEvent{
		Stage:   "directive_attempt",
		Message: fmt.Sprintf("Requesting patch from %s (%s; %s)", cand.Model, cand.Provider.Name(), modeLabel),
		Trace: &core.PromptTrace{
			RequestID:   reqID,
			Stage:       "directive",
			Provider:    cand.Provider.Name(),
			Model:       cand.Model,
			Prompt:      fullPrompt,
			PromptBytes: len(fullPrompt),
			Status:      "running",
			Timestamp:   time.Now(),
		},
	})
	started := time.Now()
	attemptCtx := ctx
	var cancel context.CancelFunc = func() {}
	if timeout > 0 {
		attemptCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	resp, err := cand.Provider.Ask(attemptCtx, core.ModelRequest{
		RequestID:   reqID,
		CachePrefix: cachePrefix,
		Prompt:      fullPrompt,
		Model:       cand.Model,
		Options:     map[string]string{"cache_ttl": "5m"},
	})
	if attemptCtx.Err() != nil {
		err = attemptCtx.Err()
	}
	cancel()
	if ctx.Err() != nil {
		return nil, ctx.Err(), nil
	}
	if errors.Is(err, context.Canceled) {
		return nil, err, nil
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
				e.emit(ProgressEvent{
					Stage:    "directive_done",
					Message:  fmt.Sprintf("Patch directive received from %s (%s; %v)", cand.Model, cand.Provider.Name(), time.Since(started).Round(time.Millisecond)),
					Duration: time.Since(started),
					Trace: &core.PromptTrace{
						RequestID:     reqID,
						Stage:         "directive",
						Provider:      cand.Provider.Name(),
						Model:         cand.Model,
						Prompt:        fullPrompt,
						PromptBytes:   len(fullPrompt),
						Response:      resp.Content,
						ResponseBytes: len(resp.Content),
						Status:        "completed",
						DurationMs:    time.Since(started).Milliseconds(),
						Timestamp:     time.Now(),
					},
				})
				return directive, nil, nil
			}
			parseFailure = true
		}
	}
	attemptErr := fmt.Errorf("%s: %w", cand.Model, err)
	if !parseFailure && !IsFailoverError(err) {
		return nil, fmt.Errorf("directive provider %s failed: %w", cand.Model, err), nil
	}
	reason := "invalid directive"
	if !parseFailure && quotaFailure(err) {
		until := time.Now().Add(quotaCooldown(err))
		e.Failover.Groups[group] = until
		reason = fmt.Sprintf("quota exhausted for %s; paused until %s", group, until.Format(time.RFC3339))
	} else if !parseFailure {
		e.Failover.Models[id] = time.Now().Add(45 * time.Second)
		reason = err.Error()
	}
	e.emit(ProgressEvent{
		Stage:    "step_directive_fallback",
		Duration: time.Since(started),
		Message:  fmt.Sprintf("%s failed after %v: %s; trying next eligible configured provider", cand.Model, time.Since(started).Round(time.Millisecond), reason),
		Trace: &core.PromptTrace{
			RequestID:   reqID,
			Stage:       "directive",
			Provider:    cand.Provider.Name(),
			Model:       cand.Model,
			Prompt:      fullPrompt,
			PromptBytes: len(fullPrompt),
			Status:      "fallback",
			Error:       reason,
			DurationMs:  time.Since(started).Milliseconds(),
			Timestamp:   time.Now(),
		},
	})
	return nil, nil, attemptErr
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
	var candInfos []core.RouteCandidateInfo
	for idx, c := range candidates {
		r := "fallback"
		if idx == 0 {
			r = "primary"
		}
		candInfos = append(candInfos, core.RouteCandidateInfo{
			ProviderKey: candidateID(c),
			Provider:    c.Provider.Name(),
			Model:       c.Model,
			Priority:    idx + 1,
			Role:        r,
			Status:      "ready",
		})
	}
	if len(candInfos) > 0 {
		e.emit(ProgressEvent{
			Stage:           "route_stack",
			Message:         fmt.Sprintf("Architect route combo ready (%d candidates)", len(candInfos)),
			RouteCandidates: candInfos,
		})
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
	timeout := e.DirectiveTimeout
	var failures []error
	for i, cand := range eligible {
		if err := ctx.Err(); err != nil {
			return err
		}
		e.emit(ProgressEvent{
			Stage:   stage + "_attempt",
			Message: fmt.Sprintf("Requesting %s from %s (%s)...", stage, cand.Model, cand.Provider.Name()),
		})
		started := time.Now()
		attemptCtx := ctx
		var cancel context.CancelFunc = func() {}
		if timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		err := attempt(attemptCtx, cand)
		if attemptCtx.Err() != nil && ctx.Err() == nil {
			err = attemptCtx.Err()
		}
		cancel()
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
			e.Failover.Models[id] = time.Now().Add(45 * time.Second)
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
// With research enabled the Architect may first read documentation; with planning.proposals > 1
// and several architect candidates, plans are proposed in parallel and ranked.
func (e *Executor) PlanTask(ctx context.Context, task core.Task, overview string) (*architect.ExecutionPlan, error) {
	docs, err := e.gatherContext(ctx, task, overview)
	if err != nil {
		return nil, err
	}
	overview += docs
	if e.Proposals > 1 && len(e.architectCandidates()) > 1 {
		return e.planProposals(ctx, task, overview)
	}
	var plan *architect.ExecutionPlan
	err = e.withArchitectFailover(ctx, "planning", func(ctx context.Context, c FailoverCandidate) error {
		a := architect.NewArchitect(c.Provider)
		a.Model = c.Model
		a.OnTrace = func(trace *core.PromptTrace) {
			st := "planning_attempt"
			msg := fmt.Sprintf("Planning request to %s (%s)...", c.Model, c.Provider.Name())
			if trace.Status == "completed" {
				st = "planning_done"
				msg = fmt.Sprintf("Planning response received from %s (%d bytes)", c.Model, trace.ResponseBytes)
			} else if trace.Status == "failed" {
				st = "planning_failed"
				msg = fmt.Sprintf("Planning failed for %s: %s", c.Model, trace.Error)
			}
			e.emit(ProgressEvent{
				Stage:   st,
				Message: msg,
				Trace:   trace,
			})
		}
		p, err := a.Plan(ctx, task, overview)
		if err == nil {
			plan = p
		}
		return err
	})
	return plan, err
}
