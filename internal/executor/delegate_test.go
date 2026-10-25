package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/apiwat/apw-agent/internal/core"
)

// routedProvider answers by the first rule whose marker the prompt contains and records prompts.
type routedProvider struct {
	mu      sync.Mutex
	prompts []string
	route   func(prompt string) string
}

func (p *routedProvider) Name() string { return "routed" }
func (p *routedProvider) Close() error { return nil }
func (p *routedProvider) Ask(_ context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
	p.mu.Lock()
	p.prompts = append(p.prompts, req.Prompt)
	p.mu.Unlock()
	return &core.ModelResponse{Completed: true, Content: p.route(req.Prompt)}, nil
}

func (p *routedProvider) promptsWith(marker string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, s := range p.prompts {
		if strings.Contains(s, marker) {
			out = append(out, s)
		}
	}
	return out
}

func delegateWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api", "server.go"), []byte("package api\n\nfunc Serve() error {\n\treturn listen(\":8080\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// subagentRoute searches on its first turn and summarizes once it has seen results.
func subagentRoute(prompt string) string {
	if strings.Contains(prompt, "Your lookup results so far") {
		return `{"summary":"Serve listens on :8080 (api/server.go:4)"}`
	}
	return `{"grep":[{"pattern":"listen\\(","glob":"**/*.go"}]}`
}

func TestDelegateRunsItsOwnReadOnlyLoop(t *testing.T) {
	ws := delegateWorkspace(t)
	prov := &routedProvider{route: subagentRoute}
	e := New(ws, prov, prov, "arch", "reflex", 1)
	e.ReadContext = "CALLER-CONTEXT-MUST-NOT-LEAK"

	out := e.runDelegates(context.Background(), ws, "Test", []delegateRequest{{Role: "Port Tracer", Goal: "Which port does Serve listen on?"}})
	if !strings.Contains(out, "Serve listens on :8080 (api/server.go:4)") {
		t.Fatalf("summary not returned:\n%s", out)
	}
	sub := prov.promptsWith("read-only Port Tracer subagent")
	if len(sub) != 2 {
		t.Fatalf("expected 2 subagent turns, got %d", len(sub))
	}
	if !strings.Contains(sub[1], `api/server.go:4: 	return listen(":8080")`) {
		t.Fatalf("second turn lacks the lookup result:\n%s", sub[1])
	}
	for _, p := range sub {
		if strings.Contains(p, "CALLER-CONTEXT-MUST-NOT-LEAK") {
			t.Fatal("subagent prompt contains the caller's context")
		}
	}
	if len(e.Subagents) != 1 || e.Subagents[0].Action != "delegate" || e.Subagents[0].Status != "done" {
		t.Fatalf("subagent report: %+v", e.Subagents)
	}
}

func TestDelegateLastTurnForcesSummary(t *testing.T) {
	ws := delegateWorkspace(t)
	prov := &routedProvider{route: func(p string) string {
		if strings.Contains(p, "this is your last turn") {
			return `{"summary":"could not confirm"}`
		}
		return `{"glob":["**/*.go"]}`
	}}
	e := New(ws, prov, prov, "arch", "reflex", 1)
	summary, turns, err := e.runDelegate(context.Background(), ws, delegateRequest{Role: "Looper", Goal: "x"})
	if err != nil || turns != delegateMaxTurns || summary != "could not confirm" {
		t.Fatalf("summary=%q turns=%d err=%v", summary, turns, err)
	}
}

func TestDelegateBudgetAndOff(t *testing.T) {
	ws := delegateWorkspace(t)
	prov := &routedProvider{route: func(string) string { return `{"summary":"ok"}` }}
	e := New(ws, prov, prov, "arch", "reflex", 1)
	e.DelegateBudget = 1
	reqs := []delegateRequest{{Goal: "a"}, {Goal: "b"}, {Goal: "c"}}
	if out := e.runDelegates(context.Background(), ws, "Test", reqs); strings.Count(out, "### ") != 1 {
		t.Fatalf("budget of 1 started more subagents:\n%s", out)
	}
	if out := e.runDelegates(context.Background(), ws, "Test", reqs); out != "" || e.delegateOffer() != "" {
		t.Fatalf("spent budget still delegated: %q", out)
	}

	off := New(ws, prov, prov, "arch", "reflex", 1)
	off.DelegateBudget = -1
	if off.delegateOffer() != "" || strings.Contains(off.gatherPrompt("x", "overview", true), `"delegate"`) {
		t.Fatal("delegation offered while disabled")
	}
	if !strings.Contains(New(ws, prov, prov, "a", "r", 1).gatherPrompt("x", "overview", true), `"delegate":[]`) {
		t.Fatal("planner is not offered delegation")
	}
}

func TestStepDirectiveCanDelegateOnce(t *testing.T) {
	ws := delegateWorkspace(t)
	prov := &routedProvider{route: func(p string) string {
		switch {
		case strings.Contains(p, "read-only Port Tracer subagent"):
			return subagentRoute(p)
		case strings.Contains(p, "Delegated findings"):
			return `{"old_content":"\":8080\"","new_content":"\":9090\""}`
		default:
			return `{"delegate":[{"role":"Port Tracer","goal":"Which port does Serve listen on?"}]}`
		}
	}}
	e := New(ws, prov, prov, "arch", "reflex", 1)
	dir, err := e.askDirective(context.Background(), "Provide surgical patch for api/server.go.")
	if err != nil {
		t.Fatal(err)
	}
	if dir.NewContent != `":9090"` || len(dir.Delegate) != 0 {
		t.Fatalf("directive = %+v", dir)
	}
	final := prov.promptsWith("Delegated findings")
	if len(final) != 1 || !strings.Contains(final[0], "Serve listens on :8080") || strings.Contains(final[0], `respond once with ONLY {"delegate"`) {
		t.Fatalf("second directive prompt must carry the findings and no new offer:\n%v", final)
	}
}

func TestParallelWaveRejectsDelegationOnlyDirective(t *testing.T) {
	ws := delegateWorkspace(t)
	prov := &routedProvider{route: func(string) string { return `{"delegate":[{"role":"r","goal":"g"}]}` }}
	e := New(ws, prov, prov, "arch", "reflex", 1)
	if _, err := e.askDirectiveSafe(context.Background(), "Create file x.go.", ""); err == nil || !strings.Contains(err.Error(), "delegation") {
		t.Fatalf("wave accepted a delegation-only directive: %v", err)
	}
}

func TestPlannerDelegatesBeforePlanning(t *testing.T) {
	ws := delegateWorkspace(t)
	prov := &routedProvider{route: func(p string) string {
		switch {
		case strings.Contains(p, "read-only Port Tracer subagent"):
			return subagentRoute(p)
		case strings.Contains(p, "preparing to plan"):
			return `{"delegate":[{"role":"Port Tracer","goal":"Which port does Serve listen on?"}]}`
		}
		return "{}"
	}}
	e := New(ws, prov, prov, "arch", "reflex", 1)
	e.Lookup = "always"
	out, err := e.gatherContext(context.Background(), core.Task{ID: "t1", Instruction: "change the port"}, "overview")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Serve listens on :8080 (api/server.go:4)") {
		t.Fatalf("planner context lacks delegated findings:\n%s", out)
	}
}
