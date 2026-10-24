package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

type concurrentSubagentProvider struct {
	inFlight    atomic.Int32
	maxInFlight atomic.Int32
}

func (p *concurrentSubagentProvider) Name() string { return "subagent-mock" }
func (p *concurrentSubagentProvider) Close() error { return nil }
func (p *concurrentSubagentProvider) Ask(_ context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
	cur := p.inFlight.Add(1)
	for {
		prev := p.maxInFlight.Load()
		if cur <= prev || p.maxInFlight.CompareAndSwap(prev, cur) {
			break
		}
	}
	time.Sleep(40 * time.Millisecond)
	p.inFlight.Add(-1)

	switch {
	case strings.Contains(req.Prompt, "Create file alpha.go"):
		return &core.ModelResponse{Completed: true, Content: `{"file_content":"package main\nfunc Alpha() int { return 1 }\n"}`}, nil
	case strings.Contains(req.Prompt, "Create file beta.go"):
		return &core.ModelResponse{Completed: true, Content: `{"file_content":"package main\nfunc Beta() int { return 2 }\n"}`}, nil
	case strings.Contains(req.Prompt, "Create file gamma.go"):
		return &core.ModelResponse{Completed: true, Content: `{"file_content":"package main\nfunc Gamma() int { return 3 }\n"}`}, nil
	default:
		return &core.ModelResponse{Completed: true, Content: `{"file_content":"package main\n"}`}, nil
	}
}

func TestParallelSurgeonSubagentWave(t *testing.T) {
	subtasks := []architect.Subtask{
		{ID: "s1", Action: architect.ActionCreate, TargetFile: "alpha.go", Instruction: "create alpha"},
		{ID: "s2", Action: architect.ActionCreate, TargetFile: "beta.go", Instruction: "create beta"},
		{ID: "s3", Action: architect.ActionCreate, TargetFile: "gamma.go", Instruction: "create gamma"},
		// Same file as s3 stops the first wave so waves remain strictly conflict-free
		{ID: "s4", Action: architect.ActionPatch, TargetFile: "gamma.go", Instruction: "patch gamma"},
	}

	wave := collectDisjointMutationWave(subtasks, 0, 4)
	if len(wave) != 3 || wave[0] != 0 || wave[1] != 1 || wave[2] != 2 {
		t.Fatalf("expected disjoint wave [0 1 2], got %v", wave)
	}

	dir := t.TempDir()
	prov := &concurrentSubagentProvider{}
	e := New(dir, prov, prov, "subagent-mock", "subagent-mock", 1)
	e.SubagentConcurrency = 4

	var committed []string
	err := e.runSubagentWave(context.Background(), dir, surgeon.NewSurgeon(), subtasks, wave, func(_ int, sr StepReport, path string) error {
		if !sr.Success {
			t.Errorf("subtask %s failed: %s", sr.SubtaskID, sr.Message)
		}
		committed = append(committed, path)
		return nil
	})
	if err != nil {
		t.Fatalf("runSubagentWave: %v", err)
	}
	if prov.maxInFlight.Load() < 2 {
		t.Fatalf("expected parallel subagent execution (maxInFlight >= 2), got %d", prov.maxInFlight.Load())
	}
	if len(committed) != 3 {
		t.Fatalf("expected 3 committed files in order, got %v", committed)
	}
	for _, f := range []string{"alpha.go", "beta.go", "gamma.go"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("expected %s to be written by subagent wave: %v", f, err)
		}
	}
	if len(e.Subagents) != 3 {
		t.Fatalf("expected 3 recorded Surgeon subagents, got %+v", e.Subagents)
	}
}

func TestParallelAuditorSubagentWaves(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "frontend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backend", "api.go"), []byte("package backend\nfunc Serve() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "frontend", "App.svelte"), []byte("<h1>App</h1>\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prov := &concurrentSubagentProvider{}
	e := New(dir, prov, prov, "subagent-mock", "subagent-mock", 1)
	e.SubagentConcurrency = 4

	var stages []string
	e.OnProgress = func(ev ProgressEvent) {
		stages = append(stages, ev.Stage)
	}

	auditOut := e.runDomainAuditSubagents(context.Background())
	if !strings.Contains(auditOut, "Backend Codebase Auditor") || !strings.Contains(auditOut, "Frontend Codebase Auditor") {
		t.Fatalf("expected both Backend and Frontend Codebase Auditors in summary, got:\n%s", auditOut)
	}
	if len(e.Subagents) < 2 {
		t.Fatalf("expected >= 2 recorded domain auditor subagents, got %+v", e.Subagents)
	}
	stageJoined := strings.Join(stages, ",")
	if !strings.Contains(stageJoined, "subagent_start") || !strings.Contains(stageJoined, "subagent_done") {
		t.Fatalf("expected subagent_start and subagent_done events, got %v", stages)
	}

	readSubtasks := []architect.Subtask{
		{ID: "r1", Action: architect.ActionRead, TargetFile: "backend/api.go"},
		{ID: "r2", Action: architect.ActionRead, TargetFile: "frontend/App.svelte"},
	}
	readWave := collectDisjointReadWave(readSubtasks, 0, 4)
	if len(readWave) != 2 {
		t.Fatalf("expected readWave len 2, got %v", readWave)
	}
	err := e.runReadSubagentWave(context.Background(), dir, surgeon.NewSurgeon(), readSubtasks, readWave, func(_ int, sr StepReport, _ string) error {
		if !sr.Success {
			t.Errorf("read subtask %s failed: %s", sr.SubtaskID, sr.Message)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("runReadSubagentWave: %v", err)
	}
}
