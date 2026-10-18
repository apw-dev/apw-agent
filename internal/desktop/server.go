package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/session"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"github.com/apiwat/apw-agent/internal/transport"
)

type Server struct {
	ConfigPath, Version, Token string
	Backend                    app.API
	Gatekeeper                 *ApprovalGatekeeper

	cache   backendCache
	connect func(context.Context) (app.API, error) // defaults to daemon.Connect(ConfigPath)
}

func NewServer(path, version string) *Server {
	if path == "" {
		path = "./apw.json"
	}
	if version == "" {
		version = "0.1.0-alpha"
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		panic(err)
	}
	return &Server{
		ConfigPath: path,
		Version:    version,
		Token:      hex.EncodeToString(secret[:]),
		Gatekeeper: NewApprovalGatekeeper(),
	}
}

func (s *Server) gatekeeper() *ApprovalGatekeeper {
	if s.Gatekeeper == nil {
		s.Gatekeeper = NewApprovalGatekeeper()
	}
	return s.Gatekeeper
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/symbols", s.handleSymbols)
	mux.HandleFunc("GET /api/file", s.handleFile)
	mux.HandleFunc("POST /api/agent-md/init", s.handleInitAgentMD)
	mux.HandleFunc("POST /api/trust", s.handleTrust)
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("GET /api/approvals", s.handleListApprovals)
	mux.HandleFunc("POST /api/approvals/request", s.handleRequestApproval)
	mux.HandleFunc("POST /api/approvals/{id}/decide", s.handleDecideApproval)
	mux.HandleFunc("POST /api/plan", s.handlePlan)
	mux.HandleFunc("POST /api/run", s.handleRun)
	mux.HandleFunc("POST /api/verify", s.handleVerify)
	mux.Handle("/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st, err := s.current(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		st.handler.ServeHTTP(w, r)
	}))
	guarded := transport.Guard(s.Token, mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			if r.Method != "GET" {
				http.Error(w, "method not allowed", 405)
				return
			}
			if !transport.LocalRequest(r) {
				http.Error(w, "local origin required", 403)
				return
			}
			s.handleIndex(w, r)
			return
		}
		guarded.ServeHTTP(w, r)
	})
}

func (s *Server) StartAndOpen(ctx context.Context, addr string, open bool) (string, error) {
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("desktop must bind a loopback address")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	server := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	go server.Serve(ln)
	url := "http://" + ln.Addr().String()
	if open {
		OpenAppWindow(url)
	}
	return url, nil
}

// OpenAppWindow launches a native app-mode WebView window (Edge/Chrome --app) on Windows,
// falling back to the system default browser handler.
func OpenAppWindow(url string) {
	if runtime.GOOS == "windows" {
		for _, bin := range []string{"msedge", "chrome"} {
			if p, err := exec.LookPath(bin); err == nil {
				cmd := exec.Command(p, "--app="+url, "--window-size=1440,900")
				if cmd.Start() == nil {
					_ = cmd.Process.Release()
					return
				}
			}
		}
		cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		if cmd.Start() == nil {
			_ = cmd.Process.Release()
		}
	}
}

type statusPayload struct {
	Version        string `json:"version"`
	Brain1Name     string `json:"brain1_name"`
	Brain1Model    string `json:"brain1_model,omitempty"`
	Brain1State    string `json:"brain1_state"`
	Brain1Latency  int64  `json:"brain1_latency_ms"`
	Brain2Name     string `json:"brain2_name"`
	Brain2Model    string `json:"brain2_model,omitempty"`
	Brain2State    string `json:"brain2_state"`
	DaemonRunning  bool   `json:"daemon_running"`
	DaemonPID      int    `json:"daemon_pid"`
	SkillsLoaded   int    `json:"skills_loaded"`
	Workspace      string `json:"workspace"`
	Stack          string `json:"stack"`
	Trusted        bool   `json:"trusted"`
	AgentMDLoaded  bool   `json:"agent_md_loaded"`
	AgentMDPath    string `json:"agent_md_path,omitempty"`
	AgentMDLines   int    `json:"agent_md_lines,omitempty"`
	AgentMDSize    int    `json:"agent_md_size,omitempty"`
	AgentMDContent string `json:"agent_md_content,omitempty"`
}

func inspectAgentMD(workspace string) (loaded bool, relPath string, lines int, size int, content string) {
	candidates := []string{"AGENT.md", "AGENTS.md", ".apw/AGENT.md", "CLAUDE.md"}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return false, "AGENT.md", 0, 0, ""
	}
	defer root.Close()

	for _, rel := range candidates {
		f, err := root.Open(rel)
		if err != nil {
			continue
		}
		data, rErr := io.ReadAll(io.LimitReader(f, 64*1024))
		_ = f.Close()
		if rErr != nil {
			continue
		}
		text := string(data)
		lineCount := 0
		if len(text) > 0 {
			lineCount = strings.Count(text, "\n") + 1
		}
		return true, rel, lineCount, len(data), text
	}
	return false, "AGENT.md", 0, 0, ""
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	state := daemon.Status(cfg.Workspace)
	b1Model, b2Model := "APW Engine", "APW Engine"
	if p, ok := cfg.Providers[cfg.Roles.Reflex]; ok && p.Model != "" {
		b1Model = p.Model
	}
	if p, ok := cfg.Providers[cfg.Roles.Architect]; ok && p.Model != "" {
		b2Model = p.Model
	}
	mdLoaded, mdPath, mdLines, mdSize, mdContent := inspectAgentMD(cfg.Workspace)
	writeJSON(w, statusPayload{
		Version:        s.Version,
		Brain1Name:     "Reflex Engine",
		Brain1Model:    b1Model,
		Brain1State:    "Available",
		Brain1Latency:  state.LatencyMs,
		Brain2Name:     "Architect",
		Brain2Model:    b2Model,
		Brain2State:    "Standby",
		DaemonRunning:  state.Running,
		DaemonPID:      state.PID,
		SkillsLoaded:   daemon.CountSkills(cfg.Workspace),
		Workspace:      cfg.Workspace,
		Stack:          config.DetectStack(cfg.Workspace),
		Trusted:        cfg.Policy.Trusted,
		AgentMDLoaded:  mdLoaded,
		AgentMDPath:    mdPath,
		AgentMDLines:   mdLines,
		AgentMDSize:    mdSize,
		AgentMDContent: mdContent,
	})
}

func (s *Server) handleInitAgentMD(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	stack := config.DetectStack(cfg.Workspace)
	target := filepath.Join(cfg.Workspace, "AGENT.md")
	if _, err := os.Stat(target); err == nil {
		writeJSON(w, map[string]any{"ok": true, "path": "AGENT.md", "created": false})
		return
	}
	starter := fmt.Sprintf(`# AGENT.md — Workspace Operating Rules

## Project Context
- **Workspace**: %s
- **Detected Stack**: %s

## Core Guidelines
1. Always inspect existing symbols and tests before proposing changes.
2. Isolate mutations inside Git worktrees and pass Reflex verification before applying.
3. Require Human-in-the-Loop confirmation before executing destructive or external system commands.
`, filepath.Base(cfg.Workspace), stack)

	if err := surgeon.WriteFileAtomic(target, []byte(starter), 0644); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "path": "AGENT.md", "created": true})
}

func (s *Server) handleTrust(w http.ResponseWriter, r *http.Request) {
	cfg, cfgFile, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	cfg.Policy.Trusted = !cfg.Policy.Trusted
	if err := cfg.Save(cfgFile); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"trusted": cfg.Policy.Trusted})
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	rel := filepath.Clean(strings.TrimSpace(r.URL.Query().Get("path")))
	if rel == "" || rel == "." || !filepath.IsLocal(rel) {
		http.Error(w, "invalid relative path", 400)
		return
	}
	root, err := os.OpenRoot(cfg.Workspace)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer root.Close()
	f, err := root.Open(rel)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 256*1024))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{
		"path":    filepath.ToSlash(rel),
		"content": string(data),
	})
}

type sessionSummary struct {
	ID              string    `json:"id"`
	UpdatedAt       time.Time `json:"updated_at"`
	TurnCount       int       `json:"turn_count"`
	LastInstruction string    `json:"last_instruction,omitempty"`
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	dir := filepath.Join(cfg.Workspace, ".apw", "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeJSON(w, []sessionSummary{})
		return
	}
	var out []sessionSummary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		sess, err := session.Load(cfg.Workspace, id)
		if err != nil {
			continue
		}
		lastInstr := ""
		if len(sess.History) > 0 {
			lastInstr = sess.History[len(sess.History)-1].Instruction
		}
		out = append(out, sessionSummary{
			ID:              sess.ID,
			UpdatedAt:       sess.UpdatedAt,
			TurnCount:       len(sess.History),
			LastInstruction: lastInstr,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	writeJSON(w, out)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	sess := session.New(cfg.Workspace)
	if err := sess.Save(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, sess)
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	sess, err := session.Load(cfg.Workspace, r.PathValue("id"))
	if err != nil {
		http.Error(w, "session not found", 404)
		return
	}
	writeJSON(w, sess)
}

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	gk := s.gatekeeper()
	writeJSON(w, map[string]any{
		"pending":        gk.ListPending(),
		"always_allowed": gk.AlwaysAllowedCommands(),
	})
}

func (s *Server) handleRequestApproval(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TaskID      string `json:"task_id,omitempty"`
		ToolName    string `json:"tool_name"`
		Command     string `json:"command"`
		TargetPath  string `json:"target_path,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := transport.Decode(r, &body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if strings.TrimSpace(body.ToolName) == "" {
		body.ToolName = "SystemCommand"
	}
	if strings.TrimSpace(body.Command) == "" {
		http.Error(w, "command is required", 400)
		return
	}
	allowed, err := s.gatekeeper().RequestApproval(
		r.Context(),
		body.TaskID,
		body.ToolName,
		body.Command,
		body.TargetPath,
		body.Description,
	)
	if err != nil {
		http.Error(w, err.Error(), 408)
		return
	}
	writeJSON(w, map[string]any{
		"approved": allowed,
		"tool":     body.ToolName,
		"command":  body.Command,
	})
}

func (s *Server) handleDecideApproval(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Decision string `json:"decision"`
	}
	if err := transport.Decode(r, &body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	resolved, err := s.gatekeeper().Resolve(r.PathValue("id"), ApprovalDecisionType(body.Decision))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	writeJSON(w, resolved)
}

type fileSymbols struct {
	File    string               `json:"file"`
	Symbols []surgeon.SymbolSpan `json:"symbols"`
}

func (s *Server) handleSymbols(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var list []fileSymbols
	_ = filepath.WalkDir(cfg.Workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".apw", ".worktrees", "vendor", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(cfg.Workspace, path)
		symbols, err := surgeon.InspectWorkspaceSymbols(cfg.Workspace, rel)
		if err == nil {
			list = append(list, fileSymbols{File: filepath.ToSlash(rel), Symbols: symbols})
		}
		return nil
	})
	writeJSON(w, list)
}

func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Instruction string `json:"instruction"`
	}
	if err := transport.Decode(r, &request); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	backend, err := s.backend(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	plan, err := backend.Plan(r.Context(), "local", request.Instruction)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, plan)
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		RequestID       string        `json:"request_id"`
		SessionID       string        `json:"session_id,omitempty"`
		Instruction     string        `json:"instruction"`
		Mode            core.TaskMode `json:"mode"`
		RequireApproval bool          `json:"require_approval,omitempty"`
	}
	if err := transport.Decode(r, &payload); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if payload.RequireApproval && (payload.Mode == core.ModeExecute || payload.Mode == "") {
		cmdLabel := fmt.Sprintf("apw run %q", payload.Instruction)
		allowed, err := s.gatekeeper().RequestApproval(
			r.Context(),
			payload.RequestID,
			"TriadExecute",
			cmdLabel,
			".",
			"ขออนุมัติรันการแก้ไขโค้ดใน Isolated Worktree และรันชุดทดสอบ Reflex Verification",
		)
		if err != nil || !allowed {
			http.Error(w, "การรันคำสั่งถูกปฏิเสธโดยผู้ใช้ (Human-in-the-loop denied)", http.StatusForbidden)
			return
		}
	}
	backend, err := s.backend(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	job, err := backend.Submit(r.Context(), "local", app.Request{
		RequestID:   payload.RequestID,
		SessionID:   payload.SessionID,
		Instruction: payload.Instruction,
		Mode:        payload.Mode,
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, job)
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		RequireApproval bool `json:"require_approval,omitempty"`
	}
	if r.ContentLength > 0 {
		_ = transport.Decode(r, &payload)
	}
	if payload.RequireApproval {
		allowed, err := s.gatekeeper().RequestApproval(
			r.Context(),
			"",
			"ReflexVerifier",
			"apw verify .",
			".",
			"ขออนุมัติรัน Pipeline ตรวจสอบโค้ดของโปรเจกต์ (fmt -> vet -> build -> test)",
		)
		if err != nil || !allowed {
			http.Error(w, "การรัน Verify ถูกปฏิเสธโดยผู้ใช้ (Human-in-the-loop denied)", http.StatusForbidden)
			return
		}
	}
	backend, err := s.backend(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	job, err := backend.Submit(r.Context(), "local", app.Request{
		RequestID:   app.NewID(),
		Instruction: "Verify project without repairs",
		Mode:        core.ModeVerify,
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, job)
}

func writeJSON(w http.ResponseWriter, value any) { transport.WriteJSON(w, value) }

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'; base-uri 'none'")
	_, _ = w.Write([]byte(strings.ReplaceAll(desktopHTML, "__APW_TOKEN__", s.Token)))
}
