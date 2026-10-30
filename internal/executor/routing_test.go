package executor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
)

type routingProvider struct {
	name  string
	calls int
	ask   func(context.Context, core.ModelRequest) (*core.ModelResponse, error)
}

func (p *routingProvider) Name() string { return p.name }
func (p *routingProvider) Close() error { return nil }
func (p *routingProvider) Ask(ctx context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
	p.calls++
	return p.ask(ctx, req)
}

func routingSuccess(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
	return &core.ModelResponse{Completed: true, Content: `{"old_content":"before","new_content":"after"}`}, nil
}

func TestConfigureFailoverDoesNotEnrollDiscoveredModels(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.ProviderConfig{
		"flash": {Type: config.ProviderAGY, Model: "gemini-3.8-flash-high"},
		"gpt":   {Type: config.ProviderCodexCLI, Model: "gpt-6.1-sol"},
		"argon": {Type: config.ProviderAGY, Model: "gemini-4-argon-high"},
		"haiku": {Type: config.ProviderClaudeCLI, Model: "claude-haiku-5-5"},
	}}
	cfg.Roles.Reflex, cfg.Roles.Architect = "flash", "gpt"
	for i := 0; i < 20; i++ {
		ex := New(t.TempDir(), &routingProvider{name: "codex-cli"}, &routingProvider{name: "agy"}, "gpt-6.1-sol", "gemini-3.8-flash-high", 1)
		closeExtras, err := ex.ConfigureFailover(cfg)
		if err != nil {
			t.Fatal(err)
		}
		candidates := ex.directiveCandidates()
		closeExtras()
		if len(candidates) != 2 || candidates[0].ProviderKey != "flash" || candidates[1].ProviderKey != "gpt" {
			t.Fatalf("discovery changed routing order: %+v", candidates)
		}
		if ex.DirectiveTimeout != 0 {
			t.Fatalf("unexpected default timeout: %v", ex.DirectiveTimeout)
		}
	}
	cfg.Failover.Reflex = []string{"argon", "argon"}
	cfg.Failover.DirectiveTimeoutSec = 17
	ex := New(t.TempDir(), &routingProvider{name: "codex-cli"}, &routingProvider{name: "agy"}, "gpt-6.1-sol", "gemini-3.8-flash-high", 1)
	closeExtras, err := ex.ConfigureFailover(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer closeExtras()
	candidates := ex.directiveCandidates()
	if len(candidates) != 3 || candidates[1].ProviderKey != "argon" || ex.DirectiveTimeout != 17*time.Second {
		t.Fatalf("explicit fallback configuration not honored: %+v", candidates)
	}
}

func TestArgonQuotaDoesNotExhaustGeminiOrOtherTransports(t *testing.T) {
	agy := &routingProvider{name: "agy", ask: func(_ context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
		switch req.Model {
		case "gemini-3.8-flash-high":
			return nil, context.DeadlineExceeded
		case "gemini-4-argon-high":
			return nil, errors.New(`AGY_ERROR: {"status":"RESOURCE_EXHAUSTED","error_code":429,"short_error":"Individual quota reached. Resets in 153h22m49s."}`)
		case "gemini-4-argon-low":
			t.Fatal("a second Argon variant was called after shared quota exhaustion")
		case "gemini-pro":
			return routingSuccess(context.Background(), req)
		}
		return nil, errors.New("unexpected model")
	}}
	ex := New(t.TempDir(), nil, agy, "", "gemini-3.8-flash-high", 1)
	for _, model := range []string{"gemini-4-argon-high", "gemini-4-argon-low", "gemini-pro"} {
		ex.ReflexPool = append(ex.ReflexPool, FailoverCandidate{Provider: agy, Model: model})
	}
	if _, err := ex.askDirective(context.Background(), "patch"); err != nil {
		t.Fatal(err)
	}
	if agy.calls != 3 {
		t.Fatalf("want Flash, one Argon, then Pro; got %d calls", agy.calls)
	}
	until := ex.Failover.Groups["agy:argon"]
	if remaining := time.Until(until); remaining < 153*time.Hour || remaining > 154*time.Hour {
		t.Fatalf("quota reset not preserved: %v", remaining)
	}
	if !ex.Failover.Groups["agy:gemini"].IsZero() {
		t.Fatal("timeout or Argon quota incorrectly exhausted Gemini group")
	}
	if _, err := ex.askDirective(context.Background(), "next patch"); err != nil {
		t.Fatal(err)
	}
	if agy.calls != 4 {
		t.Fatal("next step did not prefer the successful model")
	}
	groups := map[string]string{
		"gemini-3.8-flash-low": "agy:gemini", "gemini-pro": "agy:gemini",
		"gemini-4-argon-medium": "agy:argon",
		"claude-opus-5-5":       "agy:claude-gpt-oss", "gpt-oss-120b": "agy:claude-gpt-oss",
	}
	for model, want := range groups {
		if got := agyQuotaGroup(model); got != want {
			t.Errorf("%s group = %q, want %q", model, got, want)
		}
	}
	if candidateGroup(FailoverCandidate{Provider: &routingProvider{name: "claude-cli"}, Model: "claude-opus-5-5"}) == agyQuotaGroup("claude-opus-5-5") {
		t.Fatal("AGY cooldown leaked to independent Claude subscription")
	}
}

func TestDirectiveAttemptTimeoutAndStickyFallback(t *testing.T) {
	primary := &routingProvider{name: "agy", ask: func(ctx context.Context, _ core.ModelRequest) (*core.ModelResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	backup := &routingProvider{name: "codex-cli", ask: routingSuccess}
	ex := New(t.TempDir(), backup, primary, "gpt-6.1-sol", "gemini-3.8-flash-high", 1)
	ex.DirectiveTimeout = 10 * time.Millisecond
	for i := 0; i < 2; i++ {
		if _, err := ex.askDirective(context.Background(), "patch"); err != nil {
			t.Fatal(err)
		}
	}
	if primary.calls != 1 || backup.calls != 2 {
		t.Fatalf("unexpected call counts: primary=%d backup=%d", primary.calls, backup.calls)
	}
	if len(ex.Failover.Groups) != 0 {
		t.Fatal("timeout must not set a group quota cooldown")
	}
}

func TestDirectiveStopsOnCancellationAndFatalErrors(t *testing.T) {
	for _, cause := range []error{context.Canceled, errors.New("401 unauthorized")} {
		t.Run(cause.Error(), func(t *testing.T) {
			primary := &routingProvider{name: "agy", ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) { return nil, cause }}
			backup := &routingProvider{name: "codex-cli", ask: routingSuccess}
			ex := New(t.TempDir(), backup, primary, "gpt", "flash", 1)
			_, err := ex.askDirective(context.Background(), "patch")
			if !errors.Is(err, cause) || backup.calls != 0 {
				t.Fatalf("fatal/cancellation fell through: err=%v backup=%d", err, backup.calls)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	primary := &routingProvider{name: "agy", ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
		cancel()
		return nil, context.DeadlineExceeded
	}}
	backup := &routingProvider{name: "codex-cli", ask: routingSuccess}
	ex := New(t.TempDir(), backup, primary, "gpt", "flash", 1)
	if _, err := ex.askDirective(ctx, "patch"); !errors.Is(err, context.Canceled) || backup.calls != 0 {
		t.Fatalf("parent cancellation did not stop failover: %v", err)
	}
}

func TestDirectiveInvalidOrEmptyResponseFallsBack(t *testing.T) {
	for _, content := range []string{"", "not json", "invalid directive mentioning quota"} {
		primary := &routingProvider{name: "agy", ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
			return &core.ModelResponse{Completed: true, Content: content}, nil
		}}
		backup := &routingProvider{name: "codex-cli", ask: routingSuccess}
		ex := New(t.TempDir(), backup, primary, "gpt", "flash", 1)
		if _, err := ex.askDirective(context.Background(), "patch"); err != nil || backup.calls != 1 {
			t.Fatalf("invalid response did not fall back: %v", err)
		}
		if len(ex.Failover.Groups) != 0 {
			t.Fatal("invalid JSON was mistaken for a provider quota error")
		}
	}
}

func TestQuotaCooldownCheckpointAndExpiry(t *testing.T) {
	ws := t.TempDir()
	ex := New(ws, nil, nil, "", "", 1)
	ex.Failover = FailoverSnapshot{Groups: map[string]time.Time{"agy:argon": time.Now().Add(time.Hour)}, Preferred: "working-model"}
	cp := PlanCheckpoint{PlanID: "plan-cooldown", Failover: ex.Failover}
	if err := SaveCheckpoint(ws, cp); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCheckpoint(ws, cp.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	agy := &routingProvider{name: "agy", ask: routingSuccess}
	ex = New(ws, nil, agy, "", "gemini-4-argon-high", 1)
	ex.Failover = loaded.Failover
	if _, err := ex.askDirective(context.Background(), "patch"); err == nil || !strings.Contains(err.Error(), "paused until") || agy.calls != 0 {
		t.Fatalf("checkpoint did not preserve quota cooldown: %v", err)
	}
	ex.Failover.Groups["agy:argon"] = time.Now().Add(-time.Second)
	if _, err := ex.askDirective(context.Background(), "patch"); err != nil || agy.calls != 1 {
		t.Fatalf("expired quota did not become eligible: %v", err)
	}
}

func TestDirectiveModelCooldownUnblocksWhenPoolExhausted(t *testing.T) {
	codex := &routingProvider{name: "codex-cli", ask: routingSuccess}
	argon := &routingProvider{name: "agy", ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
		return nil, context.DeadlineExceeded
	}}
	ex := New(t.TempDir(), codex, argon, "gpt-6.1-sol", "gemini-4-argon-high", 1)
	ex.Failover.Models = map[string]time.Time{
		"codex-cli:gpt-6.1-sol": time.Now().Add(5 * time.Minute),
	}
	if _, err := ex.askDirective(context.Background(), "patch"); err != nil {
		t.Fatalf("expected model-paused candidate to recover when active candidate timed out: %v", err)
	}
	if codex.calls != 1 {
		t.Fatalf("expected codex fallback to be retried once, got %d", codex.calls)
	}
}
