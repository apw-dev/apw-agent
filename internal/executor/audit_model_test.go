package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/apiwat/apw-agent/internal/core"
)

// auditProvider answers each auditor by role and records the prompts it receives.
type auditProvider struct {
	mu      sync.Mutex
	prompts []string
	fail    bool
}

func (p *auditProvider) Name() string { return "audit-mock" }
func (p *auditProvider) Close() error { return nil }
func (p *auditProvider) Ask(_ context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
	p.mu.Lock()
	p.prompts = append(p.prompts, req.Prompt)
	p.mu.Unlock()
	if p.fail {
		return nil, errors.New("provider unavailable")
	}
	switch {
	case strings.Contains(req.Prompt, "Backend Codebase Auditor"):
		return &core.ModelResponse{Completed: true, Content: "- backend/api.go: Serve is the entry point to change\n- keep the signature"}, nil
	case strings.Contains(req.Prompt, "Frontend Codebase Auditor"):
		return &core.ModelResponse{Completed: true, Content: "- frontend/App.svelte renders the title"}, nil
	}
	return &core.ModelResponse{Completed: true, Content: "- nothing relevant"}, nil
}

func auditWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for path, content := range map[string]string{
		"backend/api.go":      "package backend\nfunc Serve() {}\n",
		"frontend/App.svelte": "<h1>App</h1>\n",
	} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestModelAuditorsReviewFilesForTheInstruction(t *testing.T) {
	dir := auditWorkspace(t)
	prov := &auditProvider{}
	e := New(dir, prov, prov, "arch", "reflex", 1)
	e.Audit = AuditModel

	out := e.runDomainAuditSubagents(context.Background(), "rename Serve to Start")
	if len(prov.prompts) != 2 {
		t.Fatalf("expected one model request per domain auditor, got %d", len(prov.prompts))
	}
	for _, p := range prov.prompts {
		if !strings.Contains(p, "Task: rename Serve to Start") {
			t.Fatalf("auditor prompt lacks the instruction:\n%s", p)
		}
	}
	joined := strings.Join(prov.prompts, "\n")
	if !strings.Contains(joined, "func Serve() {}") || !strings.Contains(joined, "<h1>App</h1>") {
		t.Fatalf("auditor prompts lack file contents:\n%s", joined)
	}
	if !strings.Contains(out, "Serve is the entry point to change") || !strings.Contains(out, "renders the title") {
		t.Fatalf("audit summary lacks model findings:\n%s", out)
	}
	for _, sr := range e.Subagents {
		if sr.Status != "done" || sr.Detail == "" {
			t.Fatalf("auditor report: %+v", sr)
		}
	}
}

func TestModelAuditorFailureIsReportedAndFallsBack(t *testing.T) {
	dir := auditWorkspace(t)
	prov := &auditProvider{fail: true}
	e := New(dir, prov, prov, "arch", "reflex", 1)
	e.Audit = AuditModel

	out := e.runDomainAuditSubagents(context.Background(), "x")
	if !strings.Contains(out, "model audit failed; structure only") {
		t.Fatalf("expected structure-only fallback, got:\n%s", out)
	}
	for _, sr := range e.Subagents {
		if sr.Status != "failed" || !strings.Contains(sr.Detail, "provider unavailable") {
			t.Fatalf("failed auditor must be reported as failed: %+v", sr)
		}
	}
}

func TestStaticAndOffAuditorsMakeNoModelRequests(t *testing.T) {
	dir := auditWorkspace(t)
	prov := &auditProvider{}
	e := New(dir, prov, prov, "arch", "reflex", 1)
	if out := e.runDomainAuditSubagents(context.Background(), "x"); !strings.Contains(out, "AST symbols") {
		t.Fatalf("static audit summary: %s", out)
	}
	e.Audit = AuditOff
	if out := e.runDomainAuditSubagents(context.Background(), "x"); out != "" {
		t.Fatalf("audit off returned %q", out)
	}
	if len(prov.prompts) != 0 {
		t.Fatalf("static/off auditors asked the model %d times", len(prov.prompts))
	}
}

func TestFirstLineSkipsPreamble(t *testing.T) {
	for in, want := range map[string]string{
		"Here is the codebase audit report:\n\n- calc.go: add Sub\n- keep Add": "calc.go: add Sub",
		"* only bullet":        "only bullet",
		"plain answer\nsecond": "plain answer",
	} {
		if got := firstLine(in); got != want {
			t.Fatalf("firstLine(%q) = %q, want %q", in, got, want)
		}
	}
}
