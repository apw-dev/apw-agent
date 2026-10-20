package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/desktop/devfixture"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/session"
	"github.com/apiwat/apw-agent/internal/store"
)

// flowBackend is a deterministic, in-memory app.API. Every job and report is built from the real
// Go types (via devfixture), and conversation turns go through the real session package, so the
// real desktop.Server serves exactly the shapes it serves in production.
type flowBackend struct {
	ws   string
	mu   sync.Mutex
	jobs []*store.Job // newest first, like store.List
	seq  int
}

func newFlowBackend(ws string) *flowBackend { return &flowBackend{ws: ws} }

var errNotFound = errors.New("sql: no rows in result set")

func (b *flowBackend) Plan(_ context.Context, _ string, instruction string) (*architect.ExecutionPlan, error) {
	return devfixture.PreviewReport().Plan, nil
}

func (b *flowBackend) Submit(_ context.Context, owner string, r app.Request) (*store.Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	id := fmt.Sprintf("task-%d", b.seq)
	var (
		status core.TaskStatus
		rep    executor.ExecutionReport
	)
	switch r.Mode {
	case core.ModeReview:
		status = core.StatusSucceeded
		rep = executor.ExecutionReport{Mode: core.ModeReview, Passed: true, Summary: "Review: governor limits are reset on pause. See [go docs](https://pkg.go.dev/context)."}
	case core.ModePreview:
		status, rep = core.StatusPreview, devfixture.PreviewReport()
	default:
		// Execute stays in flight until the user cancels it, so Cancel/Resume are exercised.
		status = core.StatusPatching
		rep = executor.ExecutionReport{Mode: r.Mode, Plan: devfixture.PreviewReport().Plan}
	}
	j := devfixture.Job(id, r.Mode, status, r.Instruction, &rep)
	j.Owner, j.RequestID, j.SessionID = owner, r.RequestID, r.SessionID
	now := time.Now()
	j.CreatedAt, j.UpdatedAt = now, now
	b.jobs = append([]*store.Job{&j}, b.jobs...)
	if err := b.recordTurn(&j, rep, false); err != nil {
		return nil, err
	}
	c := j
	return &c, nil
}

func (b *flowBackend) find(id, owner string) (*store.Job, error) {
	for _, j := range b.jobs {
		if j.ID == id && j.Owner == owner {
			return j, nil
		}
	}
	return nil, errNotFound
}

func (b *flowBackend) Get(_ context.Context, id, owner string) (*store.Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	j, err := b.find(id, owner)
	if err != nil {
		return nil, err
	}
	c := *j
	return &c, nil
}

func (b *flowBackend) List(_ context.Context, owner string) ([]store.Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []store.Job{}
	for _, j := range b.jobs {
		if j.Owner == owner {
			out = append(out, *j)
		}
	}
	return out, nil
}

func (b *flowBackend) Events(_ context.Context, id, owner string, after int64) ([]store.Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, err := b.find(id, owner); err != nil {
		return nil, err
	}
	all := []store.Event{
		{Seq: 1, TaskID: id, Stage: "planning", Message: "Planned 1 change"},
		{Seq: 2, TaskID: id, Stage: "step_start", Message: "Patching " + devfixture.GovernorFile},
	}
	out := []store.Event{}
	for _, e := range all {
		if e.Seq > after {
			out = append(out, e)
		}
	}
	return out, nil
}

func (b *flowBackend) Cancel(_ context.Context, id, owner string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	j, err := b.find(id, owner)
	if err != nil {
		return err
	}
	j.Status, j.CancelRequested, j.UpdatedAt = core.StatusCancelled, true, time.Now()
	return nil
}

// Resume finishes the task: verification and acceptance pass, so it is ready to apply.
func (b *flowBackend) Resume(_ context.Context, id, owner string) (*store.Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	j, err := b.find(id, owner)
	if err != nil {
		return nil, err
	}
	switch j.Status {
	case core.StatusInterrupted, core.StatusFailed, core.StatusCancelled, core.StatusPreview:
	default:
		return nil, fmt.Errorf("task %s is %s and cannot be resumed", id, j.Status)
	}
	rep := devfixture.ReadyReport()
	rep.TaskID = id
	j.Status, j.CancelRequested, j.UpdatedAt = core.StatusReady, false, time.Now()
	j.Report, _ = json.Marshal(rep)
	c := *j
	return &c, b.recordTurn(j, rep, false)
}

func (b *flowBackend) Apply(_ context.Context, id, owner string) (*store.Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	j, err := b.find(id, owner)
	if err != nil {
		return nil, err
	}
	if j.Status != core.StatusReady {
		return nil, fmt.Errorf("task %s is %s, not ready to apply", id, j.Status)
	}
	var rep executor.ExecutionReport
	_ = json.Unmarshal(j.Report, &rep)
	rep.Applied = true
	rep.Summary = "Verified changes were applied to the workspace."
	j.Status, j.UpdatedAt = core.StatusSucceeded, time.Now()
	j.Report, _ = json.Marshal(rep)
	c := *j
	return &c, b.recordTurn(j, rep, true)
}

// recordTurn writes the conversation turn the way app.Service does after a run.
func (b *flowBackend) recordTurn(j *store.Job, rep executor.ExecutionReport, applied bool) error {
	if j.SessionID == "" {
		return nil
	}
	id := app.SessionFileID(j.Owner, j.SessionID)
	sess, err := session.Load(b.ws, id)
	if errors.Is(err, os.ErrNotExist) {
		sess, err = session.New(b.ws), nil
		sess.ID = id
	}
	if err != nil {
		return err
	}
	sess.Owner, sess.Conversation = j.Owner, j.SessionID
	sess.RecordTurn(session.Turn{
		TaskID:        j.ID,
		Instruction:   j.Instruction,
		ModifiedFiles: rep.ModifiedFiles,
		Summary:       rep.Summary,
		Success:       j.Status != core.StatusFailed && j.Status != core.StatusCancelled,
		Applied:       applied,
	})
	return sess.Save()
}
