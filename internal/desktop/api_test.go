package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/session"
	"github.com/apiwat/apw-agent/internal/store"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"github.com/apiwat/apw-agent/internal/workspace"
)

// stubBackend is an app.API that records submissions and applies.
type stubBackend struct {
	mu        sync.Mutex
	jobs      map[string]*store.Job
	submitted []app.Request
	applied   []string
}

func (b *stubBackend) Plan(context.Context, string, string) (*architect.ExecutionPlan, error) {
	return &architect.ExecutionPlan{}, nil
}
func (b *stubBackend) Submit(_ context.Context, _ string, r app.Request) (*store.Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.submitted = append(b.submitted, r)
	return &store.Job{Task: core.Task{ID: "task-new", Mode: string(r.Mode), Status: core.StatusQueued}, Owner: "local", RequestID: r.RequestID}, nil
}
func (b *stubBackend) Get(_ context.Context, id, owner string) (*store.Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if j, ok := b.jobs[id]; ok && owner == "local" {
		c := *j
		return &c, nil
	}
	return nil, errors.New("sql: no rows in result set")
}
func (b *stubBackend) List(context.Context, string) ([]store.Job, error) { return nil, nil }
func (b *stubBackend) Events(context.Context, string, string, int64) ([]store.Event, error) {
	return nil, nil
}
func (b *stubBackend) Cancel(context.Context, string, string) error { return nil }
func (b *stubBackend) Resume(context.Context, string, string) (*store.Job, error) {
	return nil, errors.New("unused")
}
func (b *stubBackend) Apply(_ context.Context, id, owner string) (*store.Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.applied = append(b.applied, id)
	j := *b.jobs[id]
	j.Status = core.StatusSucceeded
	return &j, nil
}

func (b *stubBackend) appliedIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.applied...)
}
func (b *stubBackend) submissions() []app.Request {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]app.Request(nil), b.submitted...)
}

type testEnv struct {
	t       *testing.T
	ws      string
	cfgPath string
	srv     *Server
	ts      *httptest.Server
	backend *stubBackend
}

// newEnv serves the desktop API for a fresh temp workspace with a stub backend.
func newEnv(t *testing.T, trusted bool, extraConfig string) *testEnv {
	t.Helper()
	ws := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "apw.json")
	cfg := `{"workspace": ` + mustQuote(ws) + `, "policy": {"trusted": ` + map[bool]string{true: "true", false: "false"}[trusted] + `}, ` + extraConfig +
		`"providers": {"m": {"type": "mock", "model": "APW Engine"}}, "roles": {"architect": "m", "reflex": "m"}}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := &stubBackend{jobs: map[string]*store.Job{}}
	e := serve(t, ws, cfgPath, backend)
	e.backend = backend
	return e
}

// serve starts the desktop API for the given config and backend.
func serve(t *testing.T, ws, cfgPath string, api app.API) *testEnv {
	t.Helper()
	srv := NewServer(cfgPath, "test")
	srv.Backend = api
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &testEnv{t: t, ws: ws, cfgPath: cfgPath, srv: srv, ts: ts}
}

func (e *testEnv) do(method, path, body string) (int, []byte) {
	e.t.Helper()
	req, err := http.NewRequest(method, e.ts.URL+path, strings.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+e.srv.Token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes()
}

// json decodes body into a generic map.
func asMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("not a JSON object: %v (%s)", err, body)
	}
	return m
}

func (e *testEnv) write(rel, content string) {
	e.t.Helper()
	path := filepath.Join(e.ws, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func wantError(t *testing.T, status int, body []byte, wantStatus int, wantCode string) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("status = %d, want %d (%s)", status, wantStatus, body)
	}
	m := asMap(t, body)
	if m["error"] != wantCode {
		t.Fatalf("error code = %v, want %q (%s)", m["error"], wantCode, body)
	}
	if msg, _ := m["message"].(string); msg == "" {
		t.Fatalf("error has no message: %s", body)
	}
}

func fileQuery(path, task string) string {
	q := "/api/file?path=" + url.QueryEscape(path)
	if task != "" {
		q += "&task=" + url.QueryEscape(task)
	}
	return q
}

func TestFile_WorkspaceReads(t *testing.T) {
	e := newEnv(t, true, "")
	e.write("src/app.txt", "hello\nworld\n")
	e.write("bin.dat", "ab\x00cd")
	e.write(".env", "SECRET=1")
	e.write(".env.local", "SECRET=2")
	e.write("key.pem", "-----BEGIN-----")
	e.write(".apw/daemon.json", "{}")
	e.write(".git/config", "[core]")
	e.write("big.txt", strings.Repeat("é", 200*1024)) // 400 KiB of two-byte characters

	status, body := e.do("GET", fileQuery("src/app.txt", ""), "")
	if status != 200 {
		t.Fatalf("status = %d (%s)", status, body)
	}
	m := asMap(t, body)
	if m["content"] != "hello\nworld\n" || m["path"] != "src/app.txt" || m["source"] != "workspace" || m["truncated"] != false || m["binary"] != false {
		t.Fatalf("unexpected payload: %v", m)
	}

	status, body = e.do("GET", fileQuery("bin.dat", ""), "")
	m = asMap(t, body)
	if status != 200 || m["binary"] != true || m["content"] != "" {
		t.Fatalf("binary file: %d %v", status, m)
	}

	status, body = e.do("GET", fileQuery("big.txt", ""), "")
	m = asMap(t, body)
	content, _ := m["content"].(string)
	if status != 200 || m["truncated"] != true || len(content) == 0 || len(content) > 256*1024 || !utf8.ValidString(content) || strings.ContainsRune(content, utf8.RuneError) {
		t.Fatalf("big file: status %d truncated=%v len=%d", status, m["truncated"], len(content))
	}

	for _, p := range []string{".env", ".env.local", "key.pem", ".apw/daemon.json", ".git/config", "sub/../.apw/daemon.json"} {
		status, body = e.do("GET", fileQuery(p, ""), "")
		wantError(t, status, body, http.StatusForbidden, "protected")
	}
	for _, p := range []string{"../outside.txt", "src/../../outside.txt", filepath.Join(e.ws, "src", "app.txt"), "", "."} {
		status, body = e.do("GET", fileQuery(p, ""), "")
		if status != http.StatusBadRequest || asMap(t, body)["error"] != "invalid_path" {
			t.Fatalf("path %q: status %d (%s), want 400 invalid_path", p, status, body)
		}
	}
	status, body = e.do("GET", fileQuery("missing.txt", ""), "")
	wantError(t, status, body, http.StatusNotFound, "not_found")
}

func gitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"},
		{"config", "commit.gpgsign", "false"}, {"config", "core.autocrlf", "false"},
		{"add", "."}, {"commit", "-m", "fixture"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
}

func TestFile_TaskWorktree(t *testing.T) {
	e := newEnv(t, true, "")
	e.write("main.go", "package main\n")
	gitRepo(t, e.ws)
	wt, err := surgeon.CreateWorktree(context.Background(), e.ws, surgeon.WorktreeOptions{TaskID: "task-view"})
	if err != nil {
		t.Fatal(err)
	}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			_ = wt.Cleanup(context.Background())
		}
	})
	if err := os.MkdirAll(filepath.Join(wt.Path, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "src", "new.txt"), []byte("only in the worktree"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "main.go"), []byte("package main // changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := executor.SaveCheckpoint(e.ws, executor.PlanCheckpoint{PlanID: "plan-view", TaskID: "task-view", WorktreeBranch: wt.Branch, WorktreePath: wt.Path, ModifiedFiles: []string{"src/new.txt"}}); err != nil {
		t.Fatal(err)
	}

	status, body := e.do("GET", fileQuery("src/new.txt", "task-view"), "")
	m := asMap(t, body)
	if status != 200 || m["content"] != "only in the worktree" || m["source"] != "worktree" {
		t.Fatalf("worktree read: %d %s", status, body)
	}
	status, body = e.do("GET", fileQuery("main.go", "task-view"), "")
	if m = asMap(t, body); status != 200 || m["content"] != "package main // changed\n" {
		t.Fatalf("worktree copy of main.go: %d %s", status, body)
	}
	// The same path in the source workspace is untouched.
	status, body = e.do("GET", fileQuery("main.go", ""), "")
	if m = asMap(t, body); status != 200 || m["content"] != "package main\n" || m["source"] != "workspace" {
		t.Fatalf("workspace main.go: %d %s", status, body)
	}
	status, _ = e.do("GET", fileQuery("src/new.txt", ""), "")
	if status != http.StatusNotFound {
		t.Fatalf("worktree-only file visible in workspace: %d", status)
	}
	// Protected paths stay protected inside the worktree.
	status, body = e.do("GET", fileQuery(".git", "task-view"), "")
	wantError(t, status, body, http.StatusForbidden, "protected")

	status, body = e.do("GET", fileQuery("main.go", "task-unknown"), "")
	wantError(t, status, body, http.StatusNotFound, "task_not_found")

	// A checkpoint that points outside the task worktree directory is refused.
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "main.go"), []byte("stolen"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := executor.SaveCheckpoint(e.ws, executor.PlanCheckpoint{PlanID: "plan-evil", TaskID: "task-evil", WorktreePath: outside}); err != nil {
		t.Fatal(err)
	}
	status, body = e.do("GET", fileQuery("main.go", "task-evil"), "")
	wantError(t, status, body, http.StatusBadRequest, "invalid_worktree")

	// Once the worktree is removed the task view reports it as gone.
	if err := wt.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	cleaned = true
	status, body = e.do("GET", fileQuery("src/new.txt", "task-view"), "")
	wantError(t, status, body, http.StatusNotFound, "worktree_gone")
}

func TestTrust_SetsExplicitValue(t *testing.T) {
	e := newEnv(t, false, "")
	trusted := func() bool {
		_, body := e.do("GET", "/api/status", "")
		return asMap(t, body)["trusted"] == true
	}
	for _, step := range []struct {
		body string
		want bool
	}{{`{"trusted": true}`, true}, {`{"trusted": true}`, true}, {`{"trusted": false}`, false}, {`{"trusted": false}`, false}} {
		status, body := e.do("POST", "/api/trust", step.body)
		if status != 200 || asMap(t, body)["trusted"] != step.want {
			t.Fatalf("%s: %d %s", step.body, status, body)
		}
		if trusted() != step.want {
			t.Fatalf("%s: persisted trust != %v", step.body, step.want)
		}
	}
	for _, bad := range []string{`{}`, ``, `{"trusted": "yes"}`, `{"trusted": null}`, `[]`, `{"trusted": true, "x": 1}`} {
		status, body := e.do("POST", "/api/trust", bad)
		wantError(t, status, body, http.StatusBadRequest, "invalid_request")
		if trusted() {
			t.Fatalf("body %q changed trust", bad)
		}
	}
}

func TestStatus_SandboxAndInstructionsFields(t *testing.T) {
	e := newEnv(t, true, "")
	_, body := e.do("GET", "/api/status", "")
	m := asMap(t, body)
	if m["sandbox_mode"] != "off" || m["instructions_loaded"] != false || m["instructions_path"] != "" {
		t.Fatalf("defaults: %v", m)
	}
	for _, old := range []string{"agent_md_loaded", "agent_md_path", "agent_md_content"} {
		if _, ok := m[old]; ok {
			t.Fatalf("status still has %s", old)
		}
	}
	e2 := newEnv(t, true, `"sandbox": {"mode": "restricted"}, `)
	_, body = e2.do("GET", "/api/status", "")
	if m = asMap(t, body); m["sandbox_mode"] != "restricted" {
		t.Fatalf("sandbox_mode = %v", m["sandbox_mode"])
	}
}

func TestSessions_CreateListGet(t *testing.T) {
	e := newEnv(t, true, "")

	status, body := e.do("POST", "/api/sessions", `{}`)
	id, _ := asMap(t, body)["id"].(string)
	if status != 200 || !strings.HasPrefix(id, "conv-") || len(id) != len("conv-")+16 {
		t.Fatalf("create: %d %s", status, body)
	}
	if entries, _ := os.ReadDir(filepath.Join(e.ws, ".apw", "sessions")); len(entries) != 0 {
		t.Fatalf("creating a conversation id wrote %d session files", len(entries))
	}
	_, body2 := e.do("POST", "/api/sessions", `{}`)
	if asMap(t, body2)["id"] == id {
		t.Fatal("conversation ids repeat")
	}

	// A fresh id reads as an empty conversation (a JSON array, not null).
	status, body = e.do("GET", "/api/sessions/"+id, "")
	if status != 200 || !strings.Contains(string(body), `"turns":[]`) || asMap(t, body)["id"] != id {
		t.Fatalf("unused id: %d %s", status, body)
	}

	save := func(owner, conv string, instructions ...string) {
		sess := session.New(e.ws)
		sess.ID = app.SessionFileID(owner, conv)
		sess.Owner, sess.Conversation = owner, conv
		for i, instr := range instructions {
			sess.RecordTurn(session.Turn{Instruction: instr, Summary: "did " + instr, Success: true, Applied: i == 0, ModifiedFiles: []string{"a.go"}})
		}
		if err := sess.Save(); err != nil {
			t.Fatal(err)
		}
	}
	save("local", "conv-mine", "first", "second")
	save("telegram:111", "conv-mine", "private telegram message")
	save("telegram:222", "", "telegram without a conversation")
	// A legacy session without owner metadata is not listed either.
	legacy := session.New(e.ws)
	legacy.ID = "owner-legacy"
	legacy.RecordTurn(session.Turn{Instruction: "legacy"})
	if err := legacy.Save(); err != nil {
		t.Fatal(err)
	}
	// An unreadable file is skipped, not fatal.
	if err := os.WriteFile(filepath.Join(e.ws, ".apw", "sessions", "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	status, body = e.do("GET", "/api/sessions", "")
	var list []map[string]any
	if err := json.Unmarshal(body, &list); err != nil || status != 200 {
		t.Fatalf("list: %d %s", status, body)
	}
	if len(list) != 1 || list[0]["id"] != "conv-mine" || list[0]["turn_count"] != float64(2) || list[0]["last_instruction"] != "second" || list[0]["updated_at"] == nil {
		t.Fatalf("list = %v", list)
	}
	if strings.Contains(string(body), "telegram") {
		t.Fatalf("list leaks another owner's session: %s", body)
	}

	status, body = e.do("GET", "/api/sessions/conv-mine", "")
	m := asMap(t, body)
	turns, _ := m["turns"].([]any)
	if status != 200 || m["id"] != "conv-mine" || len(turns) != 2 {
		t.Fatalf("get: %d %s", status, body)
	}
	first := turns[0].(map[string]any)
	if first["index"] != float64(1) || first["instruction"] != "first" || first["summary"] != "did first" || first["success"] != true || first["applied"] != true || first["timestamp"] == nil {
		t.Fatalf("turn = %v", first)
	}
	if turns[1].(map[string]any)["applied"] != false {
		t.Fatalf("second turn applied: %v", turns[1])
	}
	for _, leak := range []string{"memory", "worktree", "workspace", "modified_files", "telegram"} {
		if strings.Contains(string(body), leak) {
			t.Fatalf("session view leaks %q: %s", leak, body)
		}
	}
	// Another owner's conversation with the same id is not readable through the desktop.
	if strings.Contains(string(body), "private telegram") {
		t.Fatal("desktop read a telegram session")
	}

	for _, bad := range []string{"bad.id", "a%20b", strings.Repeat("x", 65)} {
		status, body = e.do("GET", "/api/sessions/"+bad, "")
		if status != http.StatusBadRequest && status != http.StatusNotFound {
			t.Fatalf("id %q: status %d (%s)", bad, status, body)
		}
		if status == http.StatusBadRequest && asMap(t, body)["error"] != "invalid_id" {
			t.Fatalf("id %q: %s", bad, body)
		}
	}
	status, body = e.do("GET", "/api/sessions/bad.id", "")
	wantError(t, status, body, http.StatusBadRequest, "invalid_id")
}

// A real service writes the conversation under app.SessionFileID with the owner metadata, and the
// Desktop lists it.
func TestSessions_ServiceConversationIsListed(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(t.TempDir(), "apw.json")
	cfgJSON := `{"workspace": ` + mustQuote(ws) + `, "policy": {"trusted": true}, "providers": {"m": {"type": "mock", "model": "APW Engine"}}, "roles": {"architect": "m", "reflex": "m"}}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := app.Open(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	e := serve(t, ws, cfgPath, svc)

	status, body := e.do("POST", "/api/run", `{"request_id":"r1","session_id":"conv-e2e","instruction":"explain main.go","mode":"review"}`)
	if status != 200 {
		t.Fatalf("run: %d %s", status, body)
	}
	taskID, _ := asMap(t, body)["id"].(string)
	deadline := time.Now().Add(20 * time.Second)
	for {
		_, jb := e.do("GET", "/v1/tasks/"+taskID, "")
		st, _ := asMap(t, jb)["status"].(string)
		if st == string(core.StatusSucceeded) || st == string(core.StatusFailed) || st == string(core.StatusCancelled) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task did not finish: %s", jb)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(ws, ".apw", "sessions", app.SessionFileID("local", "conv-e2e")+".json")); err != nil {
		t.Fatalf("service did not save the session under SessionFileID: %v", err)
	}
	status, body = e.do("GET", "/api/sessions", "")
	var list []map[string]any
	if err := json.Unmarshal(body, &list); err != nil || status != 200 || len(list) != 1 || list[0]["id"] != "conv-e2e" || list[0]["turn_count"] != float64(1) {
		t.Fatalf("list: %d %s", status, body)
	}
	status, body = e.do("GET", "/api/sessions/conv-e2e", "")
	if turns, _ := asMap(t, body)["turns"].([]any); status != 200 || len(turns) != 1 || turns[0].(map[string]any)["instruction"] != "explain main.go" {
		t.Fatalf("get: %d %s", status, body)
	}
}

func TestInstructions_StatusGetInit(t *testing.T) {
	e := newEnv(t, true, "")
	// CLAUDE.md and the old AGENT.md names are never instructions.
	e.write("CLAUDE.md", "claude only")
	e.write("AGENT.md", "old name")
	e.write(".apw/AGENT.md", "old state name")

	status, body := e.do("GET", "/api/instructions", "")
	wantError(t, status, body, http.StatusNotFound, "not_found")
	_, body = e.do("GET", "/api/status", "")
	if m := asMap(t, body); m["instructions_loaded"] != false || m["instructions_path"] != "" {
		t.Fatalf("status counted a foreign file: %v", m)
	}

	status, body = e.do("POST", "/api/instructions/init", `{}`)
	if m := asMap(t, body); status != 200 || m["path"] != "APW.md" || m["created"] != true {
		t.Fatalf("init: %d %s", status, body)
	}
	written, err := os.ReadFile(filepath.Join(e.ws, "APW.md"))
	if err != nil || !strings.Contains(string(written), "Project overview") || !strings.Contains(string(written), "Commands to prefer") {
		t.Fatalf("starter file: %v %q", err, written)
	}
	if strings.Contains(string(written), "\r") {
		t.Fatal("starter file has CR characters")
	}

	status, body = e.do("GET", "/api/status", "")
	m := asMap(t, body)
	if m["instructions_loaded"] != true || m["instructions_path"] != "APW.md" || m["instructions_lines"].(float64) < 5 || m["instructions_size"].(float64) < 50 {
		t.Fatalf("status after init: %v", m)
	}
	if strings.Contains(string(body), "Project overview") {
		t.Fatal("status carries file content")
	}

	status, body = e.do("GET", "/api/instructions", "")
	m = asMap(t, body)
	if status != 200 || m["path"] != "APW.md" || m["truncated"] != false || !strings.Contains(m["content"].(string), "Project overview") {
		t.Fatalf("get: %d %s", status, body)
	}

	// A second init changes nothing.
	e.write("APW.md", "my own rules\n")
	status, body = e.do("POST", "/api/instructions/init", `{}`)
	if m = asMap(t, body); status != 200 || m["path"] != "APW.md" || m["created"] != false {
		t.Fatalf("second init: %d %s", status, body)
	}
	if got, _ := os.ReadFile(filepath.Join(e.ws, "APW.md")); string(got) != "my own rules\n" {
		t.Fatalf("init overwrote APW.md: %q", got)
	}
}

func TestInstructions_AgentsMdFallbackAndPreference(t *testing.T) {
	e := newEnv(t, true, "")
	e.write("AGENTS.md", "# agents rules\nbe nice\n")
	status, body := e.do("GET", "/api/instructions", "")
	if m := asMap(t, body); status != 200 || m["path"] != "AGENTS.md" || !strings.Contains(m["content"].(string), "be nice") {
		t.Fatalf("fallback: %d %s", status, body)
	}
	_, body = e.do("GET", "/api/status", "")
	if m := asMap(t, body); m["instructions_path"] != "AGENTS.md" || m["instructions_lines"] != float64(2) {
		t.Fatalf("status: %v", m)
	}
	// init must not create APW.md next to an existing AGENTS.md.
	status, body = e.do("POST", "/api/instructions/init", `{}`)
	if m := asMap(t, body); status != 200 || m["created"] != false || m["path"] != "AGENTS.md" {
		t.Fatalf("init: %d %s", status, body)
	}
	if _, err := os.Stat(filepath.Join(e.ws, "APW.md")); err == nil {
		t.Fatal("APW.md created beside AGENTS.md")
	}
	e.write("APW.md", "apw first\n")
	_, body = e.do("GET", "/api/instructions", "")
	if m := asMap(t, body); m["path"] != "APW.md" {
		t.Fatalf("APW.md must win: %v", m)
	}
}

func TestInstructions_LongFileIsTruncated(t *testing.T) {
	e := newEnv(t, true, "")
	e.write("APW.md", strings.Repeat("rule line\n", 3000))
	_, body := e.do("GET", "/api/instructions", "")
	m := asMap(t, body)
	content := m["content"].(string)
	if m["truncated"] != true || len(content) > 12*1024 || strings.Contains(content, "[instructions truncated]") {
		t.Fatalf("truncated=%v len=%d", m["truncated"], len(content))
	}
}

func TestInstructionsInit_BusyWhenWorkspaceLocked(t *testing.T) {
	e := newEnv(t, true, "")
	lock, err := workspace.Acquire(e.ws, "write")
	if err != nil {
		t.Fatal(err)
	}
	status, body := e.do("POST", "/api/instructions/init", `{}`)
	wantError(t, status, body, http.StatusConflict, "busy")
	if _, err := os.Stat(filepath.Join(e.ws, "APW.md")); err == nil {
		t.Fatal("APW.md written while the workspace was locked")
	}
	lock.Close()
	status, body = e.do("POST", "/api/instructions/init", `{}`)
	if m := asMap(t, body); status != 200 || m["created"] != true {
		t.Fatalf("after unlock: %d %s", status, body)
	}
	// The old route is gone.
	if status, _ = e.do("POST", "/api/agent-md/init", `{}`); status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		t.Fatalf("/api/agent-md/init status = %d", status)
	}
}

func TestRun_ModesAndTrust(t *testing.T) {
	e := newEnv(t, false, "")
	run := func(mode string) (int, []byte) {
		body := `{"request_id":"r-` + mode + `","session_id":"conv-1","instruction":"do it","mode":"` + mode + `"}`
		return e.do("POST", "/api/run", body)
	}

	status, body := run("execute")
	wantError(t, status, body, http.StatusConflict, "untrusted")
	if n := len(e.backend.submissions()); n != 0 {
		t.Fatalf("untrusted execute reached the backend (%d submissions)", n)
	}
	for _, mode := range []string{"preview", "review", "unverified"} {
		if status, body = run(mode); status != 200 {
			t.Fatalf("%s while untrusted: %d %s", mode, status, body)
		}
	}
	subs := e.backend.submissions()
	if len(subs) != 3 || subs[2].Mode != core.ModeUnverified || subs[0].SessionID != "conv-1" || subs[0].Instruction != "do it" {
		t.Fatalf("submissions = %+v", subs)
	}

	for _, mode := range []string{"verify", "", "bogus", "EXECUTE"} {
		status, body = e.do("POST", "/api/run", `{"request_id":"r","instruction":"x","mode":"`+mode+`"}`)
		wantError(t, status, body, http.StatusBadRequest, "invalid_mode")
	}
	// The old approval flag is no longer part of the request.
	status, body = e.do("POST", "/api/run", `{"request_id":"r","instruction":"x","mode":"preview","require_approval":true}`)
	wantError(t, status, body, http.StatusBadRequest, "invalid_request")
	status, body = e.do("POST", "/api/run", `{`)
	wantError(t, status, body, http.StatusBadRequest, "invalid_request")

	if status, body = e.do("POST", "/api/trust", `{"trusted": true}`); status != 200 {
		t.Fatalf("trust: %d %s", status, body)
	}
	if status, body = run("execute"); status != 200 {
		t.Fatalf("trusted execute: %d %s", status, body)
	}
	if n := len(e.backend.submissions()); n != 4 {
		t.Fatalf("submissions = %d, want 4", n)
	}
}

func TestVerify_BodyOptionalNoApproval(t *testing.T) {
	e := newEnv(t, false, "")
	status, body := e.do("POST", "/api/verify", ``)
	wantError(t, status, body, http.StatusConflict, "untrusted")

	e.do("POST", "/api/trust", `{"trusted": true}`)
	for _, ok := range []string{``, `{}`, `  `} {
		if status, body = e.do("POST", "/api/verify", ok); status != 200 {
			t.Fatalf("verify %q: %d %s", ok, status, body)
		}
	}
	for _, bad := range []string{`{`, `nope`, `[1]`} {
		status, body = e.do("POST", "/api/verify", bad)
		wantError(t, status, body, http.StatusBadRequest, "invalid_request")
	}
	subs := e.backend.submissions()
	if len(subs) != 3 || subs[0].Mode != core.ModeVerify {
		t.Fatalf("submissions = %+v", subs)
	}
	if pending := e.srv.gatekeeper().ListPending(); len(pending) != 0 {
		t.Fatalf("verify created approvals: %v", pending)
	}
}

func readyJob(id string, files []string, unverified bool) *store.Job {
	report, _ := json.Marshal(map[string]any{"modified_files": files, "unverified": unverified, "passed": true})
	mode := "execute"
	if unverified {
		mode = "unverified"
	}
	return &store.Job{Task: core.Task{ID: id, Mode: mode, Status: core.StatusReady}, Owner: "local", Report: report}
}

func (e *testEnv) waitPending() ToolApprovalRequest {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p := e.srv.gatekeeper().ListPending(); len(p) > 0 {
			return p[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	e.t.Fatal("no approval became pending")
	return ToolApprovalRequest{}
}

type result struct {
	status int
	body   []byte
}

func (e *testEnv) applyAsync(id, body string) <-chan result {
	ch := make(chan result, 1)
	go func() {
		s, b := e.do("POST", "/api/tasks/"+id+"/apply", body)
		ch <- result{s, b}
	}()
	return ch
}

func (e *testEnv) await(ch <-chan result) result {
	e.t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(10 * time.Second):
		e.t.Fatal("request did not finish")
		return result{}
	}
}

func TestApply_AskOnceThenDeny(t *testing.T) {
	e := newEnv(t, true, "")
	e.backend.jobs["task-a"] = readyJob("task-a", []string{"a.go", "dir/b.go"}, false)
	e.backend.jobs["task-u"] = readyJob("task-u", []string{"c.go"}, true)

	ch := e.applyAsync("task-a", `{"approval":"ask"}`)
	req := e.waitPending()
	if req.ToolName != "Apply" || req.TaskID != "task-a" || req.Status != "pending" || req.TargetPath != e.ws {
		t.Fatalf("approval = %+v", req)
	}
	if !strings.Contains(req.Description, "a.go") || !strings.Contains(req.Description, "dir/b.go") || strings.Contains(req.Description, "UNVERIFIED") {
		t.Fatalf("description = %q", req.Description)
	}
	if got := e.backend.appliedIDs(); len(got) != 0 {
		t.Fatalf("applied before the decision: %v", got)
	}
	status, body := e.do("POST", "/api/approvals/"+req.ID+"/decide", `{"decision":"once"}`)
	if status != 200 {
		t.Fatalf("decide: %d %s", status, body)
	}
	res := e.await(ch)
	if res.status != 200 || asMap(t, res.body)["id"] != "task-a" {
		t.Fatalf("apply: %d %s", res.status, res.body)
	}
	if got := e.backend.appliedIDs(); len(got) != 1 || got[0] != "task-a" {
		t.Fatalf("applied = %v", got)
	}
	// "once" does not stick: the next apply asks again, and an unverified task says so.
	ch = e.applyAsync("task-u", `{}`)
	req = e.waitPending()
	if !strings.Contains(req.Description, "UNVERIFIED") || !strings.Contains(req.Description, "c.go") {
		t.Fatalf("unverified description = %q", req.Description)
	}
	e.do("POST", "/api/approvals/"+req.ID+"/decide", `{"decision":"deny"}`)
	res = e.await(ch)
	wantError(t, res.status, res.body, http.StatusForbidden, "denied")
	if got := e.backend.appliedIDs(); len(got) != 1 {
		t.Fatalf("denied apply reached the backend: %v", got)
	}
	status, body = e.do("GET", "/api/approvals", "")
	m := asMap(t, body)
	if pending, _ := m["pending"].([]any); status != 200 || len(pending) != 0 {
		t.Fatalf("approvals: %s", body)
	}
	if allowed, _ := m["always_allowed"].([]any); len(allowed) != 0 {
		t.Fatalf("always_allowed = %v", allowed)
	}
}

func TestApply_AlwaysCoversLaterTasks(t *testing.T) {
	e := newEnv(t, true, "")
	e.backend.jobs["task-1"] = readyJob("task-1", []string{"one.go"}, false)
	e.backend.jobs["task-2"] = readyJob("task-2", []string{"two.go"}, false)

	ch := e.applyAsync("task-1", `{"approval":"ask"}`)
	req := e.waitPending()
	e.do("POST", "/api/approvals/"+req.ID+"/decide", `{"decision":"always"}`)
	if res := e.await(ch); res.status != 200 {
		t.Fatalf("first apply: %d %s", res.status, res.body)
	}
	_, body := e.do("GET", "/api/approvals", "")
	if allowed, _ := asMap(t, body)["always_allowed"].([]any); len(allowed) != 1 || allowed[0] != "apply" {
		t.Fatalf("always_allowed = %s", body)
	}
	// A different task, default approval mode: no question.
	res := e.await(e.applyAsync("task-2", ``))
	if res.status != 200 {
		t.Fatalf("second apply: %d %s", res.status, res.body)
	}
	if p := e.srv.gatekeeper().ListPending(); len(p) != 0 {
		t.Fatalf("pending after always: %v", p)
	}
	if got := e.backend.appliedIDs(); len(got) != 2 {
		t.Fatalf("applied = %v", got)
	}
}

func TestApply_AutoAndErrors(t *testing.T) {
	e := newEnv(t, true, "")
	e.backend.jobs["task-ok"] = readyJob("task-ok", []string{"ok.go"}, false)
	e.backend.jobs["task-busy"] = &store.Job{Task: core.Task{ID: "task-busy", Status: core.StatusPatching}, Owner: "local"}
	e.backend.jobs["task-done"] = &store.Job{Task: core.Task{ID: "task-done", Status: core.StatusSucceeded}, Owner: "local"}

	res := e.await(e.applyAsync("task-ok", `{"approval":"auto"}`))
	if res.status != 200 {
		t.Fatalf("auto: %d %s", res.status, res.body)
	}
	if p := e.srv.gatekeeper().ListPending(); len(p) != 0 || len(e.srv.gatekeeper().history) != 0 {
		t.Fatalf("auto apply created approvals")
	}
	if got := e.backend.appliedIDs(); len(got) != 1 {
		t.Fatalf("applied = %v", got)
	}

	for _, id := range []string{"task-busy", "task-done"} {
		status, body := e.do("POST", "/api/tasks/"+id+"/apply", `{"approval":"auto"}`)
		wantError(t, status, body, http.StatusConflict, "not_ready")
	}
	status, body := e.do("POST", "/api/tasks/task-none/apply", `{}`)
	wantError(t, status, body, http.StatusNotFound, "not_found")
	status, body = e.do("POST", "/api/tasks/task-ok/apply", `{"approval":"maybe"}`)
	wantError(t, status, body, http.StatusBadRequest, "invalid_approval")
	status, body = e.do("POST", "/api/tasks/task-ok/apply", `{`)
	wantError(t, status, body, http.StatusBadRequest, "invalid_request")
	if got := e.backend.appliedIDs(); len(got) != 1 {
		t.Fatalf("a rejected apply reached the backend: %v", got)
	}
	if p := e.srv.gatekeeper().ListPending(); len(p) != 0 {
		t.Fatalf("pending = %v", p)
	}
}

func TestApprovals_GenericRequestRouteIsGone(t *testing.T) {
	e := newEnv(t, true, "")
	status, _ := e.do("POST", "/api/approvals/request", `{"tool_name":"SystemCommand","command":"rm -rf /"}`)
	if status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", status)
	}
	if p := e.srv.gatekeeper().ListPending(); len(p) != 0 {
		t.Fatalf("route still creates approvals: %v", p)
	}
	status, body := e.do("POST", "/api/approvals/appr-missing/decide", `{"decision":"once"}`)
	wantError(t, status, body, http.StatusNotFound, "not_found")
	status, body = e.do("POST", "/api/approvals/appr-missing/decide", `{"decision":"sometimes"}`)
	wantError(t, status, body, http.StatusBadRequest, "invalid_decision")
}

func TestGatekeeper_ApplyAlwaysIsKeyedByTool(t *testing.T) {
	gk := NewApprovalGatekeeper()
	req, ch := gk.EnqueueApproval("task-1", "Apply", "apply task task-1", "/ws", "first")
	if _, err := gk.Resolve(req.ID, DecisionAlways); err != nil {
		t.Fatal(err)
	}
	if !<-ch {
		t.Fatal("always did not allow")
	}
	allowed, err := gk.RequestApproval(context.Background(), "task-2", "Apply", "apply task task-2", "/ws", "second")
	if err != nil || !allowed {
		t.Fatalf("later apply was not allowed: %v %v", allowed, err)
	}
	if got := gk.AlwaysAllowedCommands(); len(got) != 1 || got[0] != "apply" {
		t.Fatalf("AlwaysAllowedCommands = %v", got)
	}
	// Other tools stay keyed by command.
	req, _ = gk.EnqueueApproval("t", "SystemCommand", "go test ./...", ".", "")
	if _, err := gk.Resolve(req.ID, DecisionAlways); err != nil {
		t.Fatal(err)
	}
	if !gk.IsAlwaysAllowed("SystemCommand", "go test ./...") || gk.IsAlwaysAllowed("SystemCommand", "go vet ./...") {
		t.Fatal("command-level always-allow is wrong")
	}
}

func TestGatekeeper_HistoryIsCapped(t *testing.T) {
	gk := NewApprovalGatekeeper()
	for i := 0; i < maxApprovalHistory+50; i++ {
		req, _ := gk.EnqueueApproval("task", "SystemCommand", "cmd", ".", "")
		if _, err := gk.Resolve(req.ID, DecisionOnce); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(gk.history); n != maxApprovalHistory {
		t.Fatalf("history = %d, want %d", n, maxApprovalHistory)
	}
	if n := len(gk.ListPending()); n != 0 {
		t.Fatalf("pending = %d", n)
	}
}

func TestGatekeeper_CancelledRequestLeavesNothingPending(t *testing.T) {
	gk := NewApprovalGatekeeper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := gk.RequestApproval(ctx, "t", "Apply", "apply task t", ".", "")
		done <- err
	}()
	for len(gk.ListPending()) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled request returned no error")
	}
	if n := len(gk.ListPending()); n != 0 {
		t.Fatalf("pending = %d", n)
	}
}
