package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/core"
)

func TestPruneDryRunMatchesAndKeepsProtectedRows(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var mode int
	if err := s.db.QueryRow(`PRAGMA auto_vacuum`).Scan(&mode); err != nil || mode != 2 {
		t.Fatalf("new database auto_vacuum = %d, %v; want incremental", mode, err)
	}
	for id, status := range map[string]core.TaskStatus{"old": core.StatusSucceeded, "ready": core.StatusReady, "kept": core.StatusFailed} {
		if _, _, err := s.CreateJob(ctx, Job{Task: core.Task{ID: id, Workspace: "w", Instruction: "x", Mode: "execute"}, Owner: "local", RequestID: id}); err != nil {
			t.Fatal(err)
		}
		if err := s.SetJob(ctx, id, status, json.RawMessage(`{"diff":"x"}`), ""); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"sent-old", "sent-new", "unsent"} {
		if err := s.QueueDelivery(ctx, "acct", key, 1, key); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-30 * 24 * time.Hour).UnixNano()
	if _, err := s.db.Exec(`UPDATE outbox SET sent=1,next_at=? WHERE dedupe_key='sent-old'`, old); err != nil {
		t.Fatal(err)
	}
	if err := s.DeliveryResult(ctx, 2, true, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE outbox SET next_at=? WHERE dedupe_key='unsent'`, old); err != nil {
		t.Fatal(err)
	}
	for id := int64(1); id <= 5; id++ {
		if err := s.CompleteInbox(ctx, "acct", id, 1, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CompleteInbox(ctx, "other", 9, 1, ""); err != nil {
		t.Fatal(err)
	}
	// "ready" is not terminal, so the statement itself refuses to prune it even if selected.
	req := PruneRequest{TaskIDs: []string{"old", "ready"}, SentBefore: time.Now().Add(-7 * 24 * time.Hour), InboxKeep: 2, DryRun: true}
	want := PruneResult{Events: 1, Reports: 1, Outbox: 1, Inbox: 3}
	got, err := s.Prune(ctx, req)
	if err != nil || got != want {
		t.Fatalf("dry run = %+v, %v; want %+v", got, err, want)
	}
	if events, _ := s.Events(ctx, "old", "local", 0); len(events) != 1 || events[0].Stage != "task_status" {
		t.Fatal("dry run deleted events")
	}
	req.DryRun = false
	if got, err = s.Prune(ctx, req); err != nil || got != want {
		t.Fatalf("prune = %+v, %v; want %+v", got, err, want)
	}
	if events, _ := s.Events(ctx, "old", "local", 0); len(events) != 1 || events[0].Stage != retentionStage {
		t.Fatalf("old task events = %+v", events)
	}
	for _, id := range []string{"ready", "kept"} {
		job, err := s.Job(ctx, id, "local")
		if err != nil || len(job.Report) == 0 {
			t.Fatalf("protected %s lost report: %v", id, err)
		}
		if events, _ := s.Events(ctx, id, "local", 0); len(events) != 1 || events[0].Stage != "task_status" {
			t.Fatalf("protected %s lost events", id)
		}
	}
	if job, _ := s.Job(ctx, "old", "local"); len(job.Report) != 0 || job.Status != core.StatusSucceeded {
		t.Fatal("old report not cleared or status changed")
	}
	if pending, _ := s.Deliveries(ctx, "acct"); len(pending) != 1 || pending[0].Text != "unsent" {
		t.Fatalf("undelivered outbox changed: %+v", pending)
	}
	var outbox int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM outbox`).Scan(&outbox)
	if outbox != 2 {
		t.Fatalf("outbox rows = %d, want 2", outbox)
	}
	if offset, _ := s.TelegramOffset(ctx, "acct"); offset != 6 {
		t.Fatalf("telegram offset = %d, want 6", offset)
	}
	if done, _ := s.InboxDone(ctx, "other", 9); !done {
		t.Fatal("other account inbox trimmed")
	}
	if action, err := s.Compact(ctx, false); err != nil || action != "incremental_vacuum" {
		t.Fatalf("compact = %q, %v", action, err)
	}
	if action, err := s.Compact(ctx, true); err != nil || action != "vacuum" {
		t.Fatalf("full compact = %q, %v", action, err)
	}
}

func TestOpenReadOnlyRejectsWritesAndSkipsMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.db.Exec(`DROP INDEX tasks_updated`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if err := ro.SetPhase(context.Background(), "task-x", core.StatusFailed); err == nil {
		t.Fatal("write through a read-only store succeeded")
	}
	var n int
	if err := ro.ro.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='tasks_updated'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("read-only open ran migrations: n=%d err=%v", n, err)
	}
	if _, err := OpenReadOnly(filepath.Join(t.TempDir(), "missing.db")); err == nil {
		t.Fatal("read-only open created a missing database")
	}
}
