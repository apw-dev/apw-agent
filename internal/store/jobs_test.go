package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/apiwat/apw-agent/internal/core"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func newJob(t *testing.T, st *Store, owner, req string) *Job {
	t.Helper()
	j, _, err := st.CreateJob(context.Background(), Job{Task: core.Task{ID: "task-" + req, Workspace: ".", Instruction: "x", Mode: "preview"}, Owner: owner, RequestID: req})
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestReadPoolSeesWriterImmediately(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	j := newJob(t, st, "o", "r1")
	got, err := st.Job(ctx, j.ID, "o")
	if err != nil || got.Status != core.StatusQueued {
		t.Fatalf("ro read after write: %v %+v", err, got)
	}
	if err := st.SetPhase(ctx, j.ID, core.StatusPlanning); err != nil {
		t.Fatal(err)
	}
	got, err = st.Job(ctx, j.ID, "o")
	if err != nil || got.Status != core.StatusPlanning {
		t.Fatalf("ro did not see update: %v %+v", err, got)
	}
	if _, err := st.ro.ExecContext(ctx, "DELETE FROM tasks"); err == nil {
		t.Fatal("read pool must be read-only")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEventsOwnership(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	j := newJob(t, st, "o", "r1")
	if _, err := st.Events(ctx, "missing", "o", 0); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing task: %v", err)
	}
	if _, err := st.Events(ctx, j.ID, "other", 0); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign owner: %v", err)
	}
	ev, err := st.Events(ctx, j.ID, "o", 0)
	if err != nil || len(ev) != 0 {
		t.Fatalf("owned, no events: %v %v", ev, err)
	}
	for i := 0; i < 3; i++ {
		if err := st.AddEvent(ctx, j.ID, "s", "m", map[string]int{"i": i}); err != nil {
			t.Fatal(err)
		}
	}
	ev, err = st.Events(ctx, j.ID, "o", 0)
	if err != nil || len(ev) != 3 || ev[0].Stage != "s" || string(ev[0].Data) != `{"i":0}` || ev[0].TaskID != j.ID {
		t.Fatalf("events: %+v %v", ev, err)
	}
	ev, err = st.Events(ctx, j.ID, "o", ev[1].Seq)
	if err != nil || len(ev) != 1 {
		t.Fatalf("after cursor: %+v %v", ev, err)
	}
	ev, err = st.Events(ctx, j.ID, "o", ev[0].Seq)
	if err != nil || len(ev) != 0 {
		t.Fatalf("after last: %+v %v", ev, err)
	}
}

func TestAddEventPhase(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	j := newJob(t, st, "o", "r1")
	if err := st.AddEventPhase(ctx, j.ID, "planning", "go", map[string]string{"a": "b"}, core.StatusPlanning); err != nil {
		t.Fatal(err)
	}
	got, _ := st.Job(ctx, j.ID, "o")
	ev, _ := st.Events(ctx, j.ID, "o", 0)
	if got.Status != core.StatusPlanning || len(ev) != 1 || ev[0].Stage != "planning" {
		t.Fatalf("status=%s events=%+v", got.Status, ev)
	}
	if err := st.AddEventPhase(ctx, j.ID, "note", "x", nil, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Job(ctx, j.ID, "o")
	ev, _ = st.Events(ctx, j.ID, "o", 0)
	if got.Status != core.StatusPlanning || len(ev) != 2 {
		t.Fatalf("empty phase must not change status: %s %d", got.Status, len(ev))
	}
	// Unmarshalable data fails before touching anything.
	if err := st.AddEventPhase(ctx, j.ID, "bad", "x", make(chan int), core.StatusFailed); err == nil {
		t.Fatal("expected marshal error")
	}
	got, _ = st.Job(ctx, j.ID, "o")
	if got.Status != core.StatusPlanning {
		t.Fatalf("status changed on error: %s", got.Status)
	}
}

func TestRequestCancel(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	j := newJob(t, st, "o", "r1")
	if err := st.RequestCancel(ctx, "missing", "o"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing: %v", err)
	}
	if err := st.RequestCancel(ctx, j.ID, "other"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign: %v", err)
	}
	if got, _ := st.Job(ctx, j.ID, "o"); got.CancelRequested {
		t.Fatal("foreign owner must not flag cancel")
	}
	if err := st.RequestCancel(ctx, j.ID, "o"); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Job(ctx, j.ID, "o"); !got.CancelRequested {
		t.Fatal("cancel not flagged")
	}
	for i, status := range []core.TaskStatus{core.StatusSucceeded, core.StatusCancelled} {
		term := newJob(t, st, "o", "t"+string(rune('a'+i)))
		if err := st.SetJob(ctx, term.ID, status, nil, ""); err != nil {
			t.Fatal(err)
		}
		if err := st.RequestCancel(ctx, term.ID, "o"); err != nil {
			t.Fatalf("terminal %s: %v", status, err)
		}
		if got, _ := st.Job(ctx, term.ID, "o"); got.CancelRequested {
			t.Fatalf("terminal %s flagged", status)
		}
	}
}

func TestTerminalJobsSince(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	a := newJob(t, st, "o", "a")
	b := newJob(t, st, "o", "b")
	c := newJob(t, st, "other", "c")
	_ = newJob(t, st, "o", "queued")
	for _, j := range []*Job{a, b, c} {
		if err := st.SetJob(ctx, j.ID, core.StatusSucceeded, nil, ""); err != nil {
			t.Fatal(err)
		}
	}
	jobs, err := st.TerminalJobsSince(ctx, "o", 0)
	if err != nil || len(jobs) != 2 || jobs[0].ID != a.ID || jobs[1].ID != b.ID {
		t.Fatalf("jobs: %+v %v", jobs, err)
	}
	mark := jobs[1].UpdatedAt.UnixNano()
	jobs, err = st.TerminalJobsSince(ctx, "o", mark)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("watermark: %+v %v", jobs, err)
	}
	if err := st.SetJob(ctx, a.ID, core.StatusFailed, nil, "boom"); err != nil {
		t.Fatal(err)
	}
	jobs, err = st.TerminalJobsSince(ctx, "o", mark)
	if err != nil || len(jobs) != 1 || jobs[0].ID != a.ID || jobs[0].Status != core.StatusFailed {
		t.Fatalf("after change: %+v %v", jobs, err)
	}
	for _, status := range []core.TaskStatus{core.StatusReady, core.StatusPreview, core.StatusProviderUnavailable} {
		mark = jobs[len(jobs)-1].UpdatedAt.UnixNano()
		if err := st.SetJob(ctx, b.ID, status, nil, ""); err != nil {
			t.Fatal(err)
		}
		jobs, err = st.TerminalJobsSince(ctx, "o", mark)
		if err != nil || len(jobs) != 1 || jobs[0].ID != b.ID || jobs[0].Status != status {
			t.Fatalf("deliverable status %s not returned: %+v %v", status, jobs, err)
		}
	}
}

func TestQueueDeliveriesDedupe(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	items := []Delivery{{ChatID: 1, Text: "a"}, {ChatID: 2, Text: "b"}}
	keys := []string{"k1", "k2"}
	if err := st.QueueDeliveries(ctx, "acc", items, keys); err != nil {
		t.Fatal(err)
	}
	if err := st.QueueDeliveries(ctx, "acc", append(items, Delivery{ChatID: 3, Text: "c"}), append(keys, "k3")); err != nil {
		t.Fatal(err)
	}
	if err := st.QueueDelivery(ctx, "acc", "k1", 9, "dup"); err != nil {
		t.Fatal(err)
	}
	got, err := st.Deliveries(ctx, "acc")
	if err != nil || len(got) != 3 || got[0].Text != "a" || got[2].Text != "c" {
		t.Fatalf("deliveries: %+v %v", got, err)
	}
	if err := st.QueueDeliveries(ctx, "acc", items, keys[:1]); err == nil {
		t.Fatal("expected length mismatch error")
	}
	if err := st.QueueDeliveries(ctx, "acc", nil, nil); err != nil {
		t.Fatal(err)
	}
}
