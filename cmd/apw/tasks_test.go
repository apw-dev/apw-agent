package main

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/store"
)

type countingBackend struct {
	app.API
	gets   int
	polls  int
	events func(poll int) []store.Event
	job    *store.Job
}

func (b *countingBackend) Events(_ context.Context, _, _ string, _ int64) ([]store.Event, error) {
	b.polls++
	return b.events(b.polls), nil
}
func (b *countingBackend) Get(context.Context, string, string) (*store.Job, error) {
	b.gets++
	return b.job, nil
}

func TestFollowTaskFetchesJobOnlyOnStatusEventsOrFallback(t *testing.T) {
	running := &store.Job{}
	running.ID = "task-poll"
	running.Status = core.StatusPatching
	backend := &countingBackend{job: running}
	backend.events = func(poll int) []store.Event {
		if poll == 6 {
			// A status change arrives; the job becomes terminal.
			backend.job = &store.Job{}
			backend.job.ID = "task-poll"
			backend.job.Status = core.StatusSucceeded
			return []store.Event{{Seq: 1, Stage: "task_status", Message: "succeeded"}}
		}
		return nil
	}
	start := &store.Job{}
	start.ID = "task-poll"
	var out bytes.Buffer
	if code := followTaskWithCancel(context.Background(), backend, start, &out, io.Discard, false); code != 0 {
		t.Fatalf("exit=%d out=%s", code, out.String())
	}
	if backend.polls != 6 {
		t.Fatalf("polls=%d", backend.polls)
	}
	// First tick (fallback) + the tick that carried the task_status event.
	if backend.gets != 2 {
		t.Fatalf("Get called %d times over %d polls, want 2", backend.gets, backend.polls)
	}
}

func TestFollowTaskTerminalJobFinishesOnFirstTick(t *testing.T) {
	done := &store.Job{}
	done.ID = "task-done"
	done.Status = core.StatusSucceeded
	backend := &countingBackend{job: done, events: func(int) []store.Event { return nil }}
	if code := followTaskWithCancel(context.Background(), backend, done, io.Discard, io.Discard, false); code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if backend.polls != 1 || backend.gets != 1 {
		t.Fatalf("polls=%d gets=%d", backend.polls, backend.gets)
	}
}
