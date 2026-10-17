package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/core"
)

type failProvider struct {
	failCount int
	calls     int
	respText  string
	errType   error
}

func (f *failProvider) Name() string { return "test-fail" }
func (f *failProvider) Close() error { return nil }
func (f *failProvider) Ask(ctx context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
	f.calls++
	if f.calls <= f.failCount {
		return nil, f.errType
	}
	return &core.ModelResponse{
		Content: f.respText,
	}, nil
}

func TestAskWithFailover_SuccessOnSecondCandidate(t *testing.T) {
	p1 := &failProvider{failCount: 1, errType: errors.New("429: rate limit exceeded")}
	p2 := &failProvider{failCount: 0, respText: `{"status": "ok"}`}

	candidates := []FailoverCandidate{
		{Provider: p1, Model: "primary-model"},
		{Provider: p2, Model: "secondary-model"},
	}

	var failovers []string
	onFailover := func(from, to FailoverCandidate, err error) {
		failovers = append(failovers, from.Model+"->"+to.Model)
	}

	resp, winner, err := AskWithFailover(context.Background(), candidates, core.ModelRequest{Prompt: "test"}, onFailover)
	if err != nil {
		t.Fatalf("expected failover to succeed, got error: %v", err)
	}

	if winner.Model != "secondary-model" {
		t.Errorf("expected winner to be secondary-model, got %s", winner.Model)
	}
	if resp.Content != `{"status": "ok"}` {
		t.Errorf("unexpected content: %s", resp.Content)
	}
	if len(failovers) != 1 || failovers[0] != "primary-model->secondary-model" {
		t.Errorf("expected failover record primary-model->secondary-model, got %v", failovers)
	}
}

func TestPlanCheckpoint_SaveAndLoad(t *testing.T) {
	ws := t.TempDir()

	cp := PlanCheckpoint{
		PlanID:          "plan-test-1",
		TaskID:          "task-123",
		Timestamp:       time.Now(),
		LastSuccessStep: 4,
		TotalSteps:      10,
		WorktreeBranch:  "apw/task-123",
		WorktreePath:    "/tmp/wt-123",
		ModifiedFiles:   []string{"a.go", "b.go"},
	}

	if err := SaveCheckpoint(ws, cp); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	loaded, err := LoadCheckpoint(ws, "plan-test-1")
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}

	if loaded.LastSuccessStep != 4 || loaded.TotalSteps != 10 {
		t.Errorf("unexpected loaded checkpoint values: %+v", loaded)
	}

	latest, err := FindLatestCheckpoint(ws)
	if err != nil {
		t.Fatalf("FindLatestCheckpoint failed: %v", err)
	}
	if latest.PlanID != "plan-test-1" {
		t.Errorf("expected plan-test-1, got %s", latest.PlanID)
	}

	if err := ClearCheckpoint(ws, "plan-test-1"); err != nil {
		t.Fatalf("ClearCheckpoint failed: %v", err)
	}

	_, err = LoadCheckpoint(ws, "plan-test-1")
	if err == nil {
		t.Fatalf("expected error loading deleted checkpoint, got nil")
	}
}
