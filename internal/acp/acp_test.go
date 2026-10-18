package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/store"
)

// fakeAPI runs every task straight to ready_to_apply with one changed file.
type fakeAPI struct {
	app.API
	mu        sync.Mutex
	submitted []app.Request
	applied   []string
	cancelled []string
	block     bool // keep tasks running until cancelled
}

func job(id string, status core.TaskStatus, mode core.TaskMode, report any) *store.Job {
	j := &store.Job{}
	j.ID, j.Status, j.Mode = id, status, string(mode)
	if report != nil {
		j.Report, _ = json.Marshal(report)
	}
	return j
}

func (f *fakeAPI) Plan(_ context.Context, _, instruction string) (*architect.ExecutionPlan, error) {
	return &architect.ExecutionPlan{PlanID: "plan-1", Objective: "do " + instruction, AcceptanceCriteria: []string{"works"},
		Subtasks: []architect.Subtask{{Action: architect.ActionPatch, TargetFile: "main.go", Instruction: "edit"}}}, nil
}
func (f *fakeAPI) Submit(_ context.Context, _ string, r app.Request) (*store.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitted = append(f.submitted, r)
	return job("task-1", core.StatusQueued, r.Mode, nil), nil
}
func (f *fakeAPI) Events(_ context.Context, _, _ string, after int64) ([]store.Event, error) {
	if after > 0 {
		return nil, nil
	}
	return []store.Event{{Seq: 1, Stage: "plan_ready", Message: "Say hello"}}, nil
}
func (f *fakeAPI) Get(_ context.Context, id, _ string) (*store.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.applied) > 0 {
		return job(id, core.StatusSucceeded, core.ModeExecute, nil), nil
	}
	if f.block && len(f.cancelled) == 0 {
		return job(id, core.StatusPatching, core.ModeExecute, nil), nil
	}
	mode := f.submitted[len(f.submitted)-1].Mode
	return job(id, core.StatusReady, mode, executor.ExecutionReport{Summary: "Verified.", ModifiedFiles: []string{"main.go"}, Diff: "--- a/main.go\n+++ b/main.go\n", Unverified: mode == core.ModeUnverified}), nil
}
func (f *fakeAPI) Apply(_ context.Context, id, _ string) (*store.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, id)
	return job(id, core.StatusApplying, core.ModeExecute, nil), nil
}
func (f *fakeAPI) Cancel(_ context.Context, id, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelled = append(f.cancelled, id)
	return nil
}

// client is the editor side of the connection.
type client struct {
	t    *testing.T
	in   *io.PipeWriter
	msgs chan map[string]any
}

func start(t *testing.T, api *fakeAPI, trusted bool) (*client, string) {
	t.Helper()
	ws := t.TempDir()
	cfgPath := filepath.Join(ws, "apw.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := &Server{Version: "test", Poll: 5 * time.Millisecond,
		Connect: func(context.Context, string) (app.API, error) { return api, nil },
		LoadConfig: func(path string) (*config.Config, string, error) {
			c := &config.Config{Workspace: ws}
			c.Policy.Trusted = trusted
			return c, path, nil
		}}
	done := make(chan struct{})
	go func() {
		_ = s.Serve(context.Background(), inR, outW)
		outW.Close()
		close(done)
	}()
	c := &client{t: t, in: inW, msgs: make(chan map[string]any, 100)}
	go func() {
		scanner := bufio.NewScanner(outR)
		scanner.Buffer(make([]byte, 64*1024), 1<<20)
		for scanner.Scan() {
			var m map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
				t.Errorf("agent wrote non-JSON line %q", scanner.Text())
				continue
			}
			c.msgs <- m
		}
		close(c.msgs)
	}()
	t.Cleanup(func() {
		inW.Close()
		<-done
	})
	return c, ws
}

func (c *client) send(v map[string]any) {
	v["jsonrpc"] = "2.0"
	data, _ := json.Marshal(v)
	if _, err := c.in.Write(append(data, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

// until reads messages until match returns true, returning everything read.
func (c *client) until(match func(map[string]any) bool) []map[string]any {
	c.t.Helper()
	var seen []map[string]any
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m, ok := <-c.msgs:
			if !ok {
				c.t.Fatalf("connection closed; saw %v", seen)
			}
			seen = append(seen, m)
			if match(m) {
				return seen
			}
		case <-timeout:
			c.t.Fatalf("timed out; saw %v", seen)
		}
	}
}

func isResponse(id float64) func(map[string]any) bool {
	return func(m map[string]any) bool { v, ok := m["id"].(float64); return ok && v == id && m["method"] == nil }
}

func setup(t *testing.T, api *fakeAPI, trusted bool) (*client, string) {
	c, ws := start(t, api, trusted)
	c.send(map[string]any{"id": 0, "method": "initialize", "params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}}})
	init := c.until(isResponse(0))
	res := init[len(init)-1]["result"].(map[string]any)
	if res["protocolVersion"].(float64) != 1 || res["agentInfo"].(map[string]any)["name"] != "apw" {
		t.Fatalf("initialize = %v", res)
	}
	c.send(map[string]any{"id": 1, "method": "session/new", "params": map[string]any{"cwd": ws, "mcpServers": []any{}}})
	msgs := c.until(isResponse(1))
	result := msgs[len(msgs)-1]["result"].(map[string]any)
	if result["modes"].(map[string]any)["currentModeId"] != "ask" {
		t.Fatalf("session/new = %v", result)
	}
	return c, result["sessionId"].(string)
}

func updates(msgs []map[string]any, kind string) []map[string]any {
	var out []map[string]any
	for _, m := range msgs {
		if m["method"] == "session/update" {
			u := m["params"].(map[string]any)["update"].(map[string]any)
			if u["sessionUpdate"] == kind {
				out = append(out, u)
			}
		}
	}
	return out
}

func TestPromptAsksBeforeApplying(t *testing.T) {
	api := &fakeAPI{}
	c, sid := setup(t, api, true)
	c.send(map[string]any{"id": 2, "method": "session/prompt", "params": map[string]any{"sessionId": sid, "prompt": []any{
		map[string]any{"type": "text", "text": "Say hello"},
		map[string]any{"type": "resource", "resource": map[string]any{"uri": "file:///x/main.go", "text": "package main"}},
	}}})
	msgs := c.until(func(m map[string]any) bool { return m["method"] == "session/request_permission" })
	perm := msgs[len(msgs)-1]
	if title := perm["params"].(map[string]any)["toolCall"].(map[string]any)["title"].(string); !strings.Contains(title, "main.go") {
		t.Fatalf("permission title = %q", title)
	}
	if len(updates(msgs, "tool_call")) != 1 || len(updates(msgs, "tool_call_update")) == 0 || len(updates(msgs, "agent_message_chunk")) == 0 {
		t.Fatalf("missing progress updates: %v", msgs)
	}
	if r := api.submitted[0]; r.Mode != core.ModeExecute || r.SessionID != sid || !strings.Contains(r.Instruction, "Attached file:///x/main.go") {
		t.Fatalf("submitted = %+v", r)
	}
	c.send(map[string]any{"id": perm["id"], "result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "apply"}}})
	done := c.until(isResponse(2))
	if stop := done[len(done)-1]["result"].(map[string]any)["stopReason"]; stop != "end_turn" || len(api.applied) != 1 {
		t.Fatalf("stop = %v applied = %v", stop, api.applied)
	}
}

func TestDeclinedApplyAndModes(t *testing.T) {
	api := &fakeAPI{}
	c, sid := setup(t, api, true)
	c.send(map[string]any{"id": 2, "method": "session/set_mode", "params": map[string]any{"sessionId": sid, "modeId": "unverified"}})
	c.until(isResponse(2))
	c.send(map[string]any{"id": 3, "method": "session/prompt", "params": map[string]any{"sessionId": sid, "prompt": []any{map[string]any{"type": "text", "text": "x"}}}})
	msgs := c.until(func(m map[string]any) bool { return m["method"] == "session/request_permission" })
	perm := msgs[len(msgs)-1]
	if title := perm["params"].(map[string]any)["toolCall"].(map[string]any)["title"].(string); !strings.Contains(title, "UNVERIFIED") {
		t.Fatalf("title = %q", title)
	}
	c.send(map[string]any{"id": perm["id"], "result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "keep"}}})
	c.until(isResponse(3))
	if api.submitted[0].Mode != core.ModeUnverified || len(api.applied) != 0 {
		t.Fatalf("unverified run: %+v applied=%v", api.submitted, api.applied)
	}
	// auto applies verified work without asking
	c.send(map[string]any{"id": 4, "method": "session/set_mode", "params": map[string]any{"sessionId": sid, "modeId": "auto"}})
	c.until(isResponse(4))
	c.send(map[string]any{"id": 5, "method": "session/prompt", "params": map[string]any{"sessionId": sid, "prompt": []any{map[string]any{"type": "text", "text": "y"}}}})
	msgs = c.until(isResponse(5))
	for _, m := range msgs {
		if m["method"] == "session/request_permission" {
			t.Fatal("auto mode asked for permission")
		}
	}
	if len(api.applied) != 1 {
		t.Fatalf("auto did not apply: %v", api.applied)
	}
	// plan mode only plans
	c.send(map[string]any{"id": 6, "method": "session/set_mode", "params": map[string]any{"sessionId": sid, "modeId": "plan"}})
	c.until(isResponse(6))
	c.send(map[string]any{"id": 7, "method": "session/prompt", "params": map[string]any{"sessionId": sid, "prompt": []any{map[string]any{"type": "text", "text": "z"}}}})
	msgs = c.until(isResponse(7))
	if len(updates(msgs, "plan")) != 1 || len(api.submitted) != 2 {
		t.Fatalf("plan mode: updates=%v submitted=%d", msgs, len(api.submitted))
	}
	c.send(map[string]any{"id": 8, "method": "session/set_mode", "params": map[string]any{"sessionId": sid, "modeId": "yolo"}})
	if m := c.until(isResponse(8)); m[len(m)-1]["error"] == nil {
		t.Fatal("unknown mode accepted")
	}
}

func TestUntrustedAndCancelAndErrors(t *testing.T) {
	api := &fakeAPI{}
	c, sid := setup(t, api, false)
	c.send(map[string]any{"id": 2, "method": "session/prompt", "params": map[string]any{"sessionId": sid, "prompt": []any{map[string]any{"type": "text", "text": "x"}}}})
	msgs := c.until(isResponse(2))
	if len(api.submitted) != 0 || !strings.Contains(textOf(updates(msgs, "agent_message_chunk")), "not trusted") {
		t.Fatalf("untrusted prompt ran: %v", msgs)
	}

	blocking := &fakeAPI{block: true}
	c2, sid2 := setup(t, blocking, true)
	c2.send(map[string]any{"id": 2, "method": "session/prompt", "params": map[string]any{"sessionId": sid2, "prompt": []any{map[string]any{"type": "text", "text": "x"}}}})
	c2.until(func(m map[string]any) bool { return len(updates([]map[string]any{m}, "tool_call")) == 1 })
	c2.send(map[string]any{"method": "session/cancel", "params": map[string]any{"sessionId": sid2}})
	done := c2.until(isResponse(2))
	if stop := done[len(done)-1]["result"].(map[string]any)["stopReason"]; stop != "cancelled" || len(blocking.cancelled) != 1 {
		t.Fatalf("cancel: stop=%v cancelled=%v", stop, blocking.cancelled)
	}

	c.send(map[string]any{"id": 9, "method": "nope/unknown", "params": map[string]any{}})
	if m := c.until(isResponse(9)); m[len(m)-1]["error"].(map[string]any)["code"].(float64) != codeMethodNotFound {
		t.Fatalf("unknown method: %v", m)
	}
	c.send(map[string]any{"id": 10, "method": "session/new", "params": map[string]any{"cwd": t.TempDir()}})
	if m := c.until(isResponse(10)); m[len(m)-1]["error"] == nil {
		t.Fatal("session/new accepted a cwd outside the workspace")
	}
}

func textOf(us []map[string]any) string {
	var b strings.Builder
	for _, u := range us {
		b.WriteString(u["content"].(map[string]any)["text"].(string))
	}
	return b.String()
}
