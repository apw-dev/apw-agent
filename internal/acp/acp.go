// Package acp lets editors and agent hubs drive APW over the Agent Client Protocol (ACP, protocol
// version 1): newline-delimited JSON-RPC 2.0 on stdin/stdout. Each prompt becomes one APW task in
// the session's workspace, reported as a tool call with streamed progress; applying the verified
// change to the workspace is asked for through session/request_permission. APW's approval modes
// (ask, auto, plan, unverified) are offered as ACP session modes.
package acp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/store"
)

// ProtocolVersion is the ACP major version this agent speaks.
const ProtocolVersion = 1

const owner = "local"

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

var modes = []map[string]string{
	{"id": "ask", "name": "Ask", "description": "Plan, patch and verify in an isolated worktree, then ask before applying"},
	{"id": "auto", "name": "Auto", "description": "Plan, patch and verify, then apply verified changes without asking"},
	{"id": "plan", "name": "Plan", "description": "Only plan; nothing is changed or run"},
	{"id": "unverified", "name": "Unverified", "description": "Patch without running install, build or test; ask before applying"},
}

func validMode(id string) bool {
	for _, m := range modes {
		if m["id"] == id {
			return true
		}
	}
	return false
}

// Server is one ACP connection.
type Server struct {
	// ConfigPath selects the configuration; empty uses <session cwd>/apw.json.
	ConfigPath string
	Version    string
	// Connect returns the task API for a configuration (default: the workspace daemon).
	Connect func(ctx context.Context, cfgPath string) (app.API, error)
	// LoadConfig loads a configuration (default: daemon.LoadConfig).
	LoadConfig func(path string) (*config.Config, string, error)
	// Poll is the task progress polling interval (default 300ms).
	Poll time.Duration

	writeMu  sync.Mutex
	out      io.Writer
	mu       sync.Mutex
	nextID   int64
	pending  map[string]chan message
	sessions map[string]*session
	wg       sync.WaitGroup
}

type session struct {
	id, workspace, cfgPath string
	api                    app.API
	mu                     sync.Mutex
	mode                   string
	cancel                 context.CancelFunc
}

type message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("%d: %s", e.Code, e.Message) }

// Serve reads messages from in until EOF or ctx ends, writing responses and notifications to out.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.out = out
	s.pending = map[string]chan message{}
	s.sessions = map[string]*session{}
	if s.Connect == nil {
		s.Connect = func(ctx context.Context, path string) (app.API, error) { return daemon.Connect(ctx, path) }
	}
	if s.LoadConfig == nil {
		s.LoadConfig = daemon.LoadConfig
	}
	if s.Poll <= 0 {
		s.Poll = 300 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		s.wg.Wait()
	}()
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg message
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg.JSONRPC != "2.0" {
			s.replyError(nil, codeParse, "invalid JSON-RPC message")
			continue
		}
		switch {
		case msg.Method != "" && msg.ID != nil:
			s.handleRequest(ctx, msg)
		case msg.Method != "":
			s.handleNotification(msg)
		case msg.ID != nil:
			s.mu.Lock()
			ch := s.pending[string(*msg.ID)]
			delete(s.pending, string(*msg.ID))
			s.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
		}
	}
	return scanner.Err()
}

func (s *Server) write(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, _ = s.out.Write(append(data, '\n'))
}

func (s *Server) reply(id *json.RawMessage, result any) {
	s.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *Server) replyError(id *json.RawMessage, code int, msg string) {
	var idValue any = id
	if id == nil {
		idValue = nil
	}
	s.write(map[string]any{"jsonrpc": "2.0", "id": idValue, "error": rpcError{Code: code, Message: msg}})
}

func (s *Server) notify(method string, params any) {
	s.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// request sends an agent-to-client request and waits for its response.
func (s *Server) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s.mu.Lock()
	s.nextID++
	id := json.RawMessage(fmt.Sprintf("%d", s.nextID))
	ch := make(chan message, 1)
	s.pending[string(id)] = ch
	s.mu.Unlock()
	s.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	select {
	case msg := <-ch:
		if msg.Error != nil {
			return nil, msg.Error
		}
		return msg.Result, nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, string(id))
		s.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (s *Server) update(sessionID string, update map[string]any) {
	s.notify("session/update", map[string]any{"sessionId": sessionID, "update": update})
}

func (s *Server) say(sessionID, text string) {
	s.update(sessionID, map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": text}})
}

func (s *Server) handleNotification(msg message) {
	if msg.Method != "session/cancel" {
		return
	}
	var p struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(msg.Params, &p) != nil {
		return
	}
	if sess := s.session(p.SessionID); sess != nil {
		sess.mu.Lock()
		if sess.cancel != nil {
			sess.cancel()
		}
		sess.mu.Unlock()
	}
}

func (s *Server) session(id string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[id]
}

func (s *Server) handleRequest(ctx context.Context, msg message) {
	switch msg.Method {
	case "initialize":
		var p struct {
			ProtocolVersion int `json:"protocolVersion"`
		}
		_ = json.Unmarshal(msg.Params, &p)
		s.reply(msg.ID, map[string]any{
			"protocolVersion": ProtocolVersion,
			"agentCapabilities": map[string]any{
				"loadSession":        false,
				"promptCapabilities": map[string]any{"image": false, "audio": false, "embeddedContext": true},
				"mcpCapabilities":    map[string]any{"http": false, "sse": false},
			},
			"agentInfo":   map[string]any{"name": "apw", "title": "APW (Architect, Surgeon, Reflex)", "version": s.Version},
			"authMethods": []any{},
		})
	case "authenticate":
		s.reply(msg.ID, map[string]any{})
	case "session/new":
		s.newSession(ctx, msg)
	case "session/set_mode":
		var p struct {
			SessionID string `json:"sessionId"`
			ModeID    string `json:"modeId"`
		}
		if json.Unmarshal(msg.Params, &p) != nil || !validMode(p.ModeID) {
			s.replyError(msg.ID, codeInvalidParams, "unknown mode; use ask, auto, plan or unverified")
			return
		}
		sess := s.session(p.SessionID)
		if sess == nil {
			s.replyError(msg.ID, codeInvalidParams, "unknown session")
			return
		}
		sess.mu.Lock()
		sess.mode = p.ModeID
		sess.mu.Unlock()
		s.reply(msg.ID, map[string]any{})
	case "session/prompt":
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.prompt(ctx, msg)
		}()
	default:
		s.replyError(msg.ID, codeMethodNotFound, "method not supported: "+msg.Method)
	}
}

func (s *Server) newSession(ctx context.Context, msg message) {
	var p struct {
		Cwd string `json:"cwd"`
	}
	if json.Unmarshal(msg.Params, &p) != nil || p.Cwd == "" || !filepath.IsAbs(p.Cwd) {
		s.replyError(msg.ID, codeInvalidParams, "cwd must be an absolute path")
		return
	}
	cfgPath := s.ConfigPath
	if cfgPath == "" {
		cfgPath = filepath.Join(p.Cwd, "apw.json")
	}
	cfg, abs, err := s.LoadConfig(cfgPath)
	if err != nil {
		s.replyError(msg.ID, codeInvalidParams, "load APW configuration: "+err.Error())
		return
	}
	a, e1 := os.Stat(cfg.Workspace)
	b, e2 := os.Stat(p.Cwd)
	if e1 != nil || e2 != nil || !os.SameFile(a, b) {
		s.replyError(msg.ID, codeInvalidParams, fmt.Sprintf("cwd %s is not the workspace of %s (%s)", p.Cwd, abs, cfg.Workspace))
		return
	}
	api, err := s.Connect(ctx, cfgPath)
	if err != nil {
		s.replyError(msg.ID, codeInternal, "connect to the APW daemon: "+err.Error())
		return
	}
	mode := cfg.Approval
	if !validMode(mode) {
		mode = "ask"
	}
	var raw [8]byte
	_, _ = rand.Read(raw[:])
	sess := &session{id: "acp-" + hex.EncodeToString(raw[:]), workspace: cfg.Workspace, cfgPath: cfgPath, api: api, mode: mode}
	s.mu.Lock()
	s.sessions[sess.id] = sess
	s.mu.Unlock()
	s.reply(msg.ID, map[string]any{"sessionId": sess.id, "modes": map[string]any{"currentModeId": mode, "availableModes": modes}})
}

type contentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	URI      string `json:"uri"`
	Name     string `json:"name"`
	Resource *struct {
		URI  string `json:"uri"`
		Text string `json:"text"`
	} `json:"resource"`
}

// promptText turns ACP content blocks into an APW instruction.
func promptText(blocks []contentBlock) string {
	var b strings.Builder
	for _, c := range blocks {
		switch c.Type {
		case "text":
			b.WriteString(c.Text + "\n")
		case "resource_link":
			fmt.Fprintf(&b, "Referenced file: %s\n", c.URI)
		case "resource":
			if c.Resource != nil && c.Resource.Text != "" {
				fmt.Fprintf(&b, "Attached %s:\n```\n%s\n```\n", c.Resource.URI, c.Resource.Text)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func (s *Server) prompt(ctx context.Context, msg message) {
	var p struct {
		SessionID string         `json:"sessionId"`
		Prompt    []contentBlock `json:"prompt"`
	}
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		s.replyError(msg.ID, codeInvalidParams, "invalid prompt")
		return
	}
	sess := s.session(p.SessionID)
	if sess == nil {
		s.replyError(msg.ID, codeInvalidParams, "unknown session")
		return
	}
	text := promptText(p.Prompt)
	if text == "" {
		s.replyError(msg.ID, codeInvalidParams, "the prompt has no text")
		return
	}
	turnCtx, cancel := context.WithCancel(ctx)
	sess.mu.Lock()
	if sess.cancel != nil {
		sess.mu.Unlock()
		cancel()
		s.replyError(msg.ID, codeInvalidRequest, "a prompt is already running in this session")
		return
	}
	sess.cancel = cancel
	mode := sess.mode
	sess.mu.Unlock()
	defer func() {
		cancel()
		sess.mu.Lock()
		sess.cancel = nil
		sess.mu.Unlock()
	}()
	stop := s.runTurn(turnCtx, sess, mode, text)
	if turnCtx.Err() != nil && ctx.Err() == nil {
		stop = "cancelled"
	}
	s.reply(msg.ID, map[string]any{"stopReason": stop})
}

// runTurn executes one prompt and returns the ACP stop reason.
func (s *Server) runTurn(ctx context.Context, sess *session, mode, text string) string {
	if mode == "plan" {
		plan, err := sess.api.Plan(ctx, owner, text)
		if err != nil {
			s.say(sess.id, "Planning failed: "+err.Error())
			return "end_turn"
		}
		var entries []map[string]any
		for _, st := range plan.Subtasks {
			entries = append(entries, map[string]any{"content": strings.TrimSpace(fmt.Sprintf("[%s] %s %s", st.Action, st.TargetFile, st.Instruction)), "priority": "medium", "status": "pending"})
		}
		s.update(sess.id, map[string]any{"sessionUpdate": "plan", "entries": entries})
		s.say(sess.id, fmt.Sprintf("Plan %s: %s\nAcceptance criteria:\n- %s\n(Plan mode changes nothing; switch to ask or auto to run it.)", plan.PlanID, plan.Objective, strings.Join(plan.AcceptanceCriteria, "\n- ")))
		return "end_turn"
	}
	taskMode := core.ModeExecute
	if mode == "unverified" {
		taskMode = core.ModeUnverified
	}
	if taskMode == core.ModeExecute {
		if cfg, _, err := s.LoadConfig(sess.cfgPath); err == nil && !cfg.Policy.Trusted {
			s.say(sess.id, "This workspace is not trusted, so APW will not run its build and tests. Run `apw trust` in the workspace (or `apw` and answer the trust question), or switch this session to the plan or unverified mode.")
			return "end_turn"
		}
	}
	job, err := sess.api.Submit(ctx, owner, app.Request{RequestID: app.NewID(), SessionID: sess.id, Mode: taskMode, Instruction: text})
	if err != nil {
		s.say(sess.id, "Could not start the task: "+err.Error())
		return "end_turn"
	}
	callID := "apw-" + job.ID
	s.update(sess.id, map[string]any{"sessionUpdate": "tool_call", "toolCallId": callID, "title": "APW task " + job.ID, "kind": "edit", "status": "in_progress"})
	final, err := s.follow(ctx, sess, job.ID, callID)
	if err != nil {
		if ctx.Err() != nil {
			cctx, done := context.WithTimeout(context.Background(), 5*time.Second)
			_ = sess.api.Cancel(cctx, job.ID, owner)
			done()
			s.update(sess.id, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": callID, "status": "failed"})
			return "cancelled"
		}
		s.say(sess.id, "Lost track of the task: "+err.Error())
		return "end_turn"
	}
	var report executor.ExecutionReport
	_ = json.Unmarshal(final.Report, &report)
	status := "completed"
	if final.Status != core.StatusReady && final.Status != core.StatusSucceeded && final.Status != core.StatusPreview {
		status = "failed"
	}
	update := map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": callID, "status": status}
	if report.Diff != "" {
		update["content"] = []any{textContent("```diff\n" + clip(report.Diff, 60000) + "\n```")}
	}
	s.update(sess.id, update)
	summary := strings.TrimSpace(report.Summary)
	if summary == "" {
		summary = "Task " + job.ID + " finished with status " + string(final.Status) + "."
	}
	if final.Error != "" {
		summary += "\n" + final.Error
	}
	s.say(sess.id, summary)
	if final.Status != core.StatusReady {
		return "end_turn"
	}
	if len(report.ModifiedFiles) == 0 {
		s.say(sess.id, "The task changed no files; nothing to apply.")
		return "end_turn"
	}
	if mode == "auto" && !report.Unverified {
		return s.apply(ctx, sess, job.ID, callID)
	}
	title := fmt.Sprintf("Apply verified changes to %d file(s): %s", len(report.ModifiedFiles), strings.Join(report.ModifiedFiles, ", "))
	if report.Unverified {
		title = "Apply UNVERIFIED changes (no build or test ran): " + strings.Join(report.ModifiedFiles, ", ")
	}
	raw, err := s.request(ctx, "session/request_permission", map[string]any{
		"sessionId": sess.id,
		"toolCall":  map[string]any{"toolCallId": callID + "-apply", "title": title, "kind": "edit", "status": "pending"},
		"options": []map[string]string{
			{"optionId": "apply", "name": "Apply to the workspace", "kind": "allow_once"},
			{"optionId": "keep", "name": "Keep it in the worktree", "kind": "reject_once"},
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			return "cancelled"
		}
		s.say(sess.id, "Permission request failed: "+err.Error())
		return "end_turn"
	}
	var answer struct {
		Outcome struct {
			Outcome  string `json:"outcome"`
			OptionID string `json:"optionId"`
		} `json:"outcome"`
	}
	_ = json.Unmarshal(raw, &answer)
	switch {
	case answer.Outcome.Outcome == "cancelled":
		return "cancelled"
	case answer.Outcome.Outcome == "selected" && answer.Outcome.OptionID == "apply":
		return s.apply(ctx, sess, job.ID, callID)
	}
	s.say(sess.id, "Not applied. The change stays in its worktree; `apw apply "+job.ID+"` applies it later.")
	return "end_turn"
}

func (s *Server) apply(ctx context.Context, sess *session, taskID, callID string) string {
	applyID := callID + "-apply"
	s.update(sess.id, map[string]any{"sessionUpdate": "tool_call", "toolCallId": applyID, "title": "Apply task " + taskID, "kind": "edit", "status": "in_progress"})
	job, err := sess.api.Apply(ctx, taskID, owner)
	if err == nil {
		job, err = s.follow(ctx, sess, job.ID, applyID)
	}
	if err != nil {
		s.update(sess.id, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": applyID, "status": "failed"})
		if ctx.Err() != nil {
			return "cancelled"
		}
		s.say(sess.id, "Apply failed: "+err.Error())
		return "end_turn"
	}
	if job.Status != core.StatusSucceeded {
		s.update(sess.id, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": applyID, "status": "failed"})
		s.say(sess.id, "Apply did not complete ("+string(job.Status)+"): "+job.Error)
		return "end_turn"
	}
	s.update(sess.id, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": applyID, "status": "completed"})
	s.say(sess.id, "Applied to the workspace.")
	return "end_turn"
}

// follow streams a task's progress events into the tool call until the task is terminal.
func (s *Server) follow(ctx context.Context, sess *session, taskID, callID string) (*store.Job, error) {
	var cursor int64
	var lines []string
	for {
		events, err := sess.api.Events(ctx, taskID, owner, cursor)
		if err != nil {
			return nil, err
		}
		if len(events) > 0 {
			for _, ev := range events {
				cursor = ev.Seq
				lines = append(lines, fmt.Sprintf("[%s] %s", ev.Stage, ev.Message))
			}
			if len(lines) > 40 {
				lines = lines[len(lines)-40:]
			}
			s.update(sess.id, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": callID, "status": "in_progress", "content": []any{textContent(strings.Join(lines, "\n"))}})
		}
		job, err := sess.api.Get(ctx, taskID, owner)
		if err != nil {
			return nil, err
		}
		switch job.Status {
		case core.StatusQueued, core.StatusAnalyzing, core.StatusPlanning, core.StatusPatching, core.StatusVerifying, core.StatusApplying:
		default:
			return job, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.Poll):
		}
	}
}

func textContent(text string) map[string]any {
	return map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": text}}
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n[truncated]"
}
