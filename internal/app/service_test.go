package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/provider/mock"
	"github.com/apiwat/apw-agent/internal/session"
	"github.com/apiwat/apw-agent/internal/store"
)

type testProvider struct {
	ask func(context.Context, core.ModelRequest) (*core.ModelResponse, error)
}

func (p *testProvider) Name() string { return "test" }
func (p *testProvider) Close() error { return nil }
func (p *testProvider) Ask(ctx context.Context, r core.ModelRequest) (*core.ModelResponse, error) {
	return p.ask(ctx, r)
}
func appConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{Workspace: t.TempDir()}
	cfg.Budget.TaskTimeoutSec = 120
	cfg.Budget.MaxRepairs = 1
	// These tests script every model response; model auditors would consume them.
	cfg.Planning.Audit = executor.AuditStatic
	if err := os.WriteFile(filepath.Join(cfg.Workspace, "main.go"), []byte("package main\n\nfunc Greet() string {\n\treturn \"hi\"\n}\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return cfg
}
func awaitStatus(t *testing.T, s *Service, id, owner string, want ...core.TaskStatus) *store.Job {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		j, err := s.Get(context.Background(), id, owner)
		if err != nil {
			t.Fatal(err)
		}
		for _, status := range want {
			if j.Status == status {
				return j
			}
		}
		if j.Status == core.StatusFailed {
			t.Fatalf("task failed: %s", j.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for task status")
	return nil
}
func reviewFactory(provider *testProvider) Factory {
	return func(cfg *config.Config) (*executor.Executor, func(), error) {
		return executor.New(cfg.Workspace, provider, provider, "test", "test", 1), func() {}, nil
	}
}

func TestDurableTasksDeduplicateAndEnforceOwner(t *testing.T) {
	cfg := appConfig(t)
	var mu sync.Mutex
	calls := 0
	p := &testProvider{ask: func(ctx context.Context, r core.ModelRequest) (*core.ModelResponse, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		if !strings.Contains(r.Prompt, "return \"hi\"") {
			t.Error("review lost source content")
		}
		return &core.ModelResponse{Completed: true, Content: "source reviewed"}, nil
	}}
	s, err := Open(context.Background(), cfg, reviewFactory(p))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := Request{RequestID: "same-event", Instruction: "review only", Mode: core.ModeReview}
	one, err := s.Submit(context.Background(), "telegram:111", r)
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.Submit(context.Background(), "telegram:111", r)
	if err != nil || one.ID != two.ID {
		t.Fatalf("duplicate created another task: %+v %v", two, err)
	}
	awaitStatus(t, s, one.ID, "telegram:111", core.StatusSucceeded)
	if _, err := s.Apply(context.Background(), one.ID, "telegram:111"); err == nil {
		t.Fatal("source review was accepted as an applyable patch")
	}
	mu.Lock()
	count := calls
	mu.Unlock()
	if count != 1 {
		t.Fatalf("request was executed %d times", count)
	}
	if _, err = s.Get(context.Background(), one.ID, "telegram:222"); err == nil {
		t.Fatal("another owner read the task")
	}
	if err = s.Cancel(context.Background(), one.ID, "telegram:222"); err == nil {
		t.Fatal("another owner cancelled the task")
	}
	if events, err := s.Events(context.Background(), one.ID, "telegram:111", 0); err != nil || len(events) == 0 {
		t.Fatalf("events not persisted: %v", err)
	}
	r.Instruction = "different"
	if _, err = s.Submit(context.Background(), "telegram:111", r); err == nil {
		t.Fatal("reused idempotency key changed an existing task")
	}
}

func TestCancellationAndRestartDoNotAutomaticallyReplayProvider(t *testing.T) {
	for _, cancelByOwner := range []bool{true, false} {
		cfg := appConfig(t)
		started := make(chan struct{})
		var once sync.Once
		p := &testProvider{ask: func(ctx context.Context, _ core.ModelRequest) (*core.ModelResponse, error) {
			once.Do(func() { close(started) })
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		s, err := Open(context.Background(), cfg, reviewFactory(p))
		if err != nil {
			t.Fatal(err)
		}
		initialService := s
		t.Cleanup(func() { _ = initialService.Close() })
		job, err := s.Submit(context.Background(), "local", Request{RequestID: "blocked", Instruction: "review", Mode: core.ModeReview})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("provider did not start")
		}
		if cancelByOwner {
			if err = s.Cancel(context.Background(), job.ID, "local"); err != nil {
				t.Fatal(err)
			}
			awaitStatus(t, s, job.ID, "local", core.StatusCancelled)
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
		calls := 0
		fresh := &testProvider{ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
			calls++
			return &core.ModelResponse{Completed: true, Content: "unexpected replay"}, nil
		}}
		s, err = Open(context.Background(), cfg, reviewFactory(fresh))
		if err != nil {
			t.Fatal(err)
		}
		want := core.StatusInterrupted
		if cancelByOwner {
			want = core.StatusCancelled
		}
		j, err := s.Get(context.Background(), job.ID, "local")
		if err != nil || j.Status != want {
			t.Fatalf("restart state: %+v %v", j, err)
		}
		if calls != 0 {
			t.Fatal("restart replayed an uncertain provider request")
		}
		s.Close()
	}
}

func TestExecuteRequiresExplicitApplyAndProtectsConcurrentEdits(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		cfg := appConfig(t)
		cfg.Policy.Trusted = true
		if err := os.WriteFile(filepath.Join(cfg.Workspace, "go.mod"), []byte("module example.com/app\n\ngo 1.24\n"), 0644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"init"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"}, {"config", "commit.gpgsign", "false"}, {"config", "core.autocrlf", "false"}, {"add", "."}, {"commit", "-m", "fixture"}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = cfg.Workspace
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git: %s %v", out, err)
			}
		}
		factory := func(cfg *config.Config) (*executor.Executor, func(), error) {
			plan := architect.ExecutionPlan{Objective: "Greet returns hello", AcceptanceCriteria: []string{"Greet returns hello"}, Checks: []architect.AcceptanceCheck{{Criterion: "Greet returns hello", Kind: "contains", File: "main.go", Value: `return "hello"`}}, Subtasks: []architect.Subtask{{ID: "patch", Action: architect.ActionPatch, TargetFile: "main.go", Instruction: "return hello"}}}
			data, _ := json.Marshal(plan)
			p := mock.New(string(data), `{"old_content":"return \"hi\"","new_content":"return \"hello\""}`)
			return executor.New(cfg.Workspace, p, p, "test", "test", 1), func() {}, nil
		}
		s, err := Open(context.Background(), cfg, factory)
		if err != nil {
			t.Fatal(err)
		}
		initialService := s
		t.Cleanup(func() { _ = initialService.Close() })
		job, err := s.Submit(context.Background(), "local", Request{RequestID: "execute", Instruction: "return hello", Mode: core.ModeExecute})
		if err != nil {
			t.Fatal(err)
		}
		job = awaitStatus(t, s, job.ID, "local", core.StatusReady)
		original, _ := os.ReadFile(filepath.Join(cfg.Workspace, "main.go"))
		if strings.Contains(string(original), "hello") {
			t.Fatal("execute automatically applied changes")
		}
		if conflict {
			if err = os.WriteFile(filepath.Join(cfg.Workspace, "main.go"), []byte("user edit"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = s.Apply(context.Background(), job.ID, "local"); err != nil {
			t.Fatal(err)
		}
		want := core.StatusSucceeded
		if conflict {
			want = core.StatusFailed
		}
		job = awaitStatus(t, s, job.ID, "local", want)
		actual, _ := os.ReadFile(filepath.Join(cfg.Workspace, "main.go"))
		if conflict {
			if string(actual) != "user edit" || !strings.Contains(job.Error, "workspace changed") {
				t.Fatalf("conflict overwrote user: %s %s", actual, job.Error)
			}
		} else if !strings.Contains(string(actual), "hello") {
			t.Fatal("explicit apply did not write verified change")
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPersistenceFailureStopsService(t *testing.T) {
	cfg := appConfig(t)
	s, err := Open(context.Background(), cfg, reviewFactory(&testProvider{}))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Store.Close(); err != nil {
		t.Fatal(err)
	}
	s.signal()
	select {
	case <-s.Done():
		if s.Err() == nil {
			t.Fatal("service hid the persistence failure")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("service remained healthy after its store failed")
	}
}

func TestConversationsKeepContextWithinOwnerAndSession(t *testing.T) {
	cfg := appConfig(t)
	provider := &testProvider{ask: func(ctx context.Context, r core.ModelRequest) (*core.ModelResponse, error) {
		if strings.Contains(r.Prompt, "Instruction: follow-up") && !strings.Contains(r.Prompt, "private-conversation-sentinel") {
			t.Error("same session lost working context")
		}
		if strings.Contains(r.Prompt, "Instruction: fresh") && strings.Contains(r.Prompt, "private-conversation-sentinel") {
			t.Error("new session inherited another conversation")
		}
		return &core.ModelResponse{Completed: true, Content: "private-conversation-sentinel"}, nil
	}}
	s, err := Open(context.Background(), cfg, reviewFactory(provider))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, request := range []struct{ owner, session, instruction string }{{"local", "one", "remember"}, {"local", "one", "follow-up"}, {"local", "two", "fresh"}, {"telegram:111", "one", "fresh"}} {
		job, err := s.Submit(context.Background(), request.owner, Request{RequestID: NewID(), SessionID: request.session, Instruction: request.instruction, Mode: core.ModeReview})
		if err != nil {
			t.Fatal(err)
		}
		awaitStatus(t, s, job.ID, request.owner, core.StatusSucceeded)
		stored, err := s.Get(context.Background(), job.ID, request.owner)
		if err != nil || stored.SessionID != request.session {
			t.Fatal("conversation identity was not persisted")
		}
	}
	for _, want := range []struct{ owner, conversation string }{{"local", "one"}, {"local", "two"}, {"telegram:111", "one"}} {
		sess, err := session.Load(cfg.Workspace, SessionFileID(want.owner, want.conversation))
		if err != nil {
			t.Fatalf("session file for %v is not at SessionFileID: %v", want, err)
		}
		if sess.Owner != want.owner || sess.Conversation != want.conversation {
			t.Errorf("session identity = %q/%q, want %q/%q", sess.Owner, sess.Conversation, want.owner, want.conversation)
		}
	}
	if SessionFileID("local", "one") == SessionFileID("local", "two") || SessionFileID("local", "one") == SessionFileID("telegram:111", "one") {
		t.Error("session file ids collide across owners or conversations")
	}
	r := Request{RequestID: "dedupe", SessionID: "one", Instruction: "same", Mode: core.ModeReview}
	if _, err := s.Submit(context.Background(), "local", r); err != nil {
		t.Fatal(err)
	}
	r.SessionID = "two"
	if _, err := s.Submit(context.Background(), "local", r); err == nil {
		t.Fatal("same request ID was accepted for another conversation")
	}
}

func TestReadOnlyTasksDoNotRequireUnusedRepairCredentials(t *testing.T) {
	cfg := appConfig(t)
	t.Setenv("APW_UNUSED_REFLEX_KEY", "")
	cfg.Providers = map[string]config.ProviderConfig{
		"architect": {Type: config.ProviderMock, Model: "offline"},
		"repair":    {Type: config.ProviderAPI, Model: "unused", APIKeyEnv: "APW_UNUSED_REFLEX_KEY"},
	}
	cfg.Roles.Architect = "architect"
	cfg.Roles.Reflex = "repair"
	cfg.Failover.Architect = []string{"repair"}
	s, err := Open(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	job, err := s.Submit(context.Background(), "local", Request{RequestID: "review", Instruction: "inspect", Mode: core.ModeReview})
	if err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, s, job.ID, "local", core.StatusSucceeded)
	if _, err := s.Plan(context.Background(), "local", "inspect"); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptConversationIsPreservedInsteadOfReset(t *testing.T) {
	cfg := appConfig(t)
	provider := &testProvider{ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
		return &core.ModelResponse{Completed: true, Content: "reviewed"}, nil
	}}
	s, err := Open(context.Background(), cfg, reviewFactory(provider))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	first, err := s.Submit(context.Background(), "local", Request{RequestID: "first", Instruction: "inspect", Mode: core.ModeReview})
	if err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, s, first.ID, "local", core.StatusSucceeded)
	files, err := filepath.Glob(filepath.Join(cfg.Workspace, ".apw", "sessions", "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatal("expected one saved conversation")
	}
	broken := []byte("broken conversation")
	if err := os.WriteFile(files[0], broken, 0600); err != nil {
		t.Fatal(err)
	}
	second, err := s.Submit(context.Background(), "local", Request{RequestID: "second", Instruction: "inspect", Mode: core.ModeReview})
	if err != nil {
		t.Fatal(err)
	}
	job := awaitStatus(t, s, second.ID, "local", core.StatusFailed)
	if !strings.Contains(job.Error, "saved file preserved") {
		t.Fatal("conversation error was hidden")
	}
	data, _ := os.ReadFile(files[0])
	if string(data) != string(broken) {
		t.Fatal("corrupt conversation was overwritten by a fresh one")
	}
}

func TestTaskConfigCacheInvalidatesAndReturnsCopies(t *testing.T) {
	cfg := appConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{"m": {Type: config.ProviderMock, Model: "offline"}}
	cfg.Roles.Architect = "m"
	cfg.Roles.Reflex = "m"
	path := filepath.Join(t.TempDir(), "apw.json")
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.ConfigPath = path
	a, err := s.taskConfig()
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.taskConfig()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("cache must return distinct copies")
	}
	a.Budget.MaxRepairs = 99
	c, _ := s.taskConfig()
	if c.Budget.MaxRepairs == 99 {
		t.Fatal("mutating a returned config leaked into the cache")
	}
	cfg.Budget.MaxRepairs = 7
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(path, future, future)
	d, err := s.taskConfig()
	if err != nil || d.Budget.MaxRepairs != 7 {
		t.Fatalf("cache not invalidated after file change: %+v %v", d, err)
	}
}

func TestConfigurePlanningDefaultsToModelAuditors(t *testing.T) {
	for raw, want := range map[string]string{"": executor.AuditModel, "static": executor.AuditStatic, "off": executor.AuditOff} {
		e := executor.New(t.TempDir(), nil, nil, "", "", 1)
		cfg := &config.Config{}
		cfg.Planning.Audit = raw
		configurePlanning(e, cfg)
		if e.Audit != want {
			t.Fatalf("planning.audit %q -> %q, want %q", raw, e.Audit, want)
		}
	}
}

func TestPlaceholderModelIsRefusedWithGuidance(t *testing.T) {
	cfg := appConfig(t)
	cfg.Providers = map[string]config.ProviderConfig{config.BuiltinProvider: {Type: config.ProviderMock, Model: "APW Engine"}}
	cfg.Roles.Architect, cfg.Roles.Reflex = config.BuiltinProvider, config.BuiltinProvider
	s, err := Open(context.Background(), cfg, reviewFactory(&testProvider{ask: func(context.Context, core.ModelRequest) (*core.ModelResponse, error) {
		t.Error("placeholder role reached a model")
		return nil, errors.New("unreachable")
	}}))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.Submit(context.Background(), "local", Request{RequestID: "r1", Instruction: "x", Mode: core.ModeReview})
	if !errors.Is(err, ErrNoModelConnected) || !strings.Contains(err.Error(), "architect and reflex") || !strings.Contains(err.Error(), "agy") {
		t.Fatalf("submit err = %v", err)
	}
	if _, err := s.Plan(context.Background(), "local", "x"); !errors.Is(err, ErrNoModelConnected) {
		t.Fatalf("plan err = %v", err)
	}
	// Discovered models are listed so the user can pick one.
	cfg.Providers["agy:gemini-3.8-flash-high"] = config.ProviderConfig{Type: config.ProviderAGY, Model: "gemini-3.8-flash-high"}
	if err := noModelError(cfg); err == nil || !strings.Contains(err.Error(), "Connected models found: agy:gemini-3.8-flash-high") {
		t.Fatalf("guidance = %v", err)
	}
	cfg.Roles.Architect, cfg.Roles.Reflex = "agy:gemini-3.8-flash-high", "agy:gemini-3.8-flash-high"
	if err := noModelError(cfg); err != nil {
		t.Fatalf("connected roles refused: %v", err)
	}
}
