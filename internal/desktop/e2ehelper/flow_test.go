package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/session"
)

// The completion gate walks review → preview → execute → cancel → resume → apply. Each step
// must produce the statuses and reports the real service produces.
func TestFlowBackendCompletionGate(t *testing.T) {
	ctx := context.Background()
	ws := t.TempDir()
	b := newFlowBackend(ws)

	review, err := b.Submit(ctx, "local", app.Request{SessionID: "conv-1", Instruction: "ตรวจสอบ", Mode: core.ModeReview})
	if err != nil || review.Status != core.StatusSucceeded {
		t.Fatalf("review = %+v, %v", review, err)
	}
	preview, _ := b.Submit(ctx, "local", app.Request{SessionID: "conv-1", Instruction: "เตรียมแก้", Mode: core.ModePreview})
	var rep executor.ExecutionReport
	if err := json.Unmarshal(preview.Report, &rep); err != nil || preview.Status != core.StatusPreview || rep.Diff == "" {
		t.Fatalf("preview = %s %s, %v", preview.Status, preview.Report, err)
	}
	exec, _ := b.Submit(ctx, "local", app.Request{SessionID: "conv-1", Instruction: "แก้ไข", Mode: core.ModeExecute})
	if exec.Status != core.StatusPatching {
		t.Fatalf("execute starts %s, want patching", exec.Status)
	}
	if err := b.Cancel(ctx, exec.ID, "local"); err != nil {
		t.Fatal(err)
	}
	if j, _ := b.Get(ctx, exec.ID, "local"); j.Status != core.StatusCancelled {
		t.Fatalf("after cancel %s", j.Status)
	}
	if _, err := b.Apply(ctx, exec.ID, "local"); err == nil {
		t.Fatal("apply of a cancelled task must fail like the real service")
	}
	resumed, err := b.Resume(ctx, exec.ID, "local")
	if err != nil || resumed.Status != core.StatusReady {
		t.Fatalf("resume = %+v, %v", resumed, err)
	}
	applied, err := b.Apply(ctx, exec.ID, "local")
	if err != nil || applied.Status != core.StatusSucceeded {
		t.Fatalf("apply = %+v, %v", applied, err)
	}
	if err := json.Unmarshal(applied.Report, &rep); err != nil || !rep.Applied {
		t.Fatalf("applied report = %s", applied.Report)
	}

	list, _ := b.List(ctx, "local")
	if len(list) != 3 || list[0].ID != exec.ID {
		t.Fatalf("List must be newest first: %+v", list)
	}
	sess, err := session.Load(ws, app.SessionFileID("local", "conv-1"))
	if err != nil || len(sess.History) != 3 || !sess.History[2].Applied {
		t.Fatalf("session turns = %+v, %v", sess, err)
	}
}
