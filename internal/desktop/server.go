package desktop

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/browser"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"github.com/apiwat/apw-agent/internal/transport"
)

// WailsHost is the host WebView2 uses for pages served by the Wails asset server.
const WailsHost = "wails.localhost"

type Server struct {
	ConfigPath, Version, Token string
	Backend                    app.API
	Gatekeeper                 *ApprovalGatekeeper
	// InProcessHost, when set, is a host whose requests are trusted as local because the
	// handler is served in-process (the Desktop shell) rather than on a loopback listener.
	InProcessHost string

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
	mux.HandleFunc("GET /api/models", s.handleGetModels)
	mux.HandleFunc("POST /api/models", s.handleUpdateModels)
	mux.HandleFunc("GET /api/channels", s.handleGetChannels)
	mux.HandleFunc("POST /api/channels", s.handleUpdateChannels)
	mux.HandleFunc("GET /api/symbols", s.handleSymbols)
	mux.HandleFunc("GET /api/graph", s.handleGraph)
	mux.HandleFunc("GET /api/board", s.handleGetBoard)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/autonomy", s.handleGetAutonomy)
	mux.HandleFunc("POST /api/autonomy/governor", s.handleUpdateGovernor)
	mux.HandleFunc("POST /api/autonomy/goals", s.handleManageGoals)
	mux.HandleFunc("POST /api/autonomy/schedules", s.handleManageSchedules)
	mux.HandleFunc("POST /api/autonomy/skills", s.handleCreateSkill)
	mux.HandleFunc("GET /api/file", s.handleFile)
	mux.HandleFunc("GET /api/files", s.handleFiles)
	mux.HandleFunc("GET /api/instructions", s.handleInstructions)
	mux.HandleFunc("POST /api/instructions/init", s.handleInstructionsInit)
	mux.HandleFunc("POST /api/trust", s.handleTrust)
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("POST /api/sessions/{id}/delete", s.handleDeleteSession)
	mux.HandleFunc("GET /api/approvals", s.handleListApprovals)
	mux.HandleFunc("POST /api/approvals/{id}/decide", s.handleDecideApproval)
	mux.HandleFunc("GET /api/terminal", s.handleGetTerminal)
	mux.HandleFunc("POST /api/terminal/run", s.handleRunTerminal)
	mux.HandleFunc("POST /api/terminal/stop", s.handleStopTerminal)
	mux.HandleFunc("POST /api/plan", s.handlePlan)
	mux.HandleFunc("POST /api/run", s.handleRun)
	mux.HandleFunc("POST /api/verify", s.handleVerify)
	mux.HandleFunc("POST /api/tasks/{id}/apply", s.handleApply)
	mux.Handle("/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st, err := s.current(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		st.handler.ServeHTTP(w, r)
	}))
	guarded := transport.GuardWith(s.Token, s.allowRequest, mux)
	distFS, _ := webDistFS()
	staticServer := http.FileServer(http.FS(distFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			if r.Method != "GET" {
				http.Error(w, "method not allowed", 405)
				return
			}
			if !s.allowRequest(r) {
				http.Error(w, "local origin required", 403)
				return
			}
			s.handleIndex(w, r)
			return
		}
		if r.URL.Path == "/favicon.ico" {
			// Requested by the browser itself, without the token; there is no favicon to serve.
			if !s.allowRequest(r) {
				http.Error(w, "local origin required", 403)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			if r.Method != "GET" && r.Method != "HEAD" {
				http.Error(w, "method not allowed", 405)
				return
			}
			if !s.allowRequest(r) {
				http.Error(w, "local origin required", 403)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			staticServer.ServeHTTP(w, r)
			return
		}
		guarded.ServeHTTP(w, r)
	})
}

// allowRequest accepts loopback requests, and requests to InProcessHost (when set) whose
// Origin, if any, is that same host.
func (s *Server) allowRequest(r *http.Request) bool {
	return transport.LocalRequest(r) || s.inProcess(r)
}

func (s *Server) inProcess(r *http.Request) bool {
	if s.InProcessHost == "" {
		return false
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if !strings.EqualFold(host, s.InProcessHost) {
		return false
	}
	origin := r.Header.Get("Origin")
	return origin == "" || strings.EqualFold(origin, "http://"+s.InProcessHost)
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

// OpenAppWindow launches a native app-mode window (Edge/Chrome --app) when a Chromium-based
// browser is installed, falling back to the system default browser handler on Windows.
func OpenAppWindow(url string) {
	if runtime.GOOS != "windows" {
		return
	}
	if bin, ok := browser.Find(); ok {
		cmd := exec.Command(bin, "--app="+url, "--window-size=1440,900")
		if cmd.Start() == nil {
			_ = cmd.Process.Release()
			return
		}
	}
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	if cmd.Start() == nil {
		_ = cmd.Process.Release()
	}
}

type statusPayload struct {
	Version            string `json:"version"`
	Brain1Name         string `json:"brain1_name"`
	Brain1Model        string `json:"brain1_model,omitempty"`
	Brain1State        string `json:"brain1_state"`
	Brain1Latency      int64  `json:"brain1_latency_ms"`
	Brain2Name         string `json:"brain2_name"`
	Brain2Model        string `json:"brain2_model,omitempty"`
	Brain2State        string `json:"brain2_state"`
	DaemonRunning      bool   `json:"daemon_running"`
	DaemonPID          int    `json:"daemon_pid"`
	SkillsLoaded       int    `json:"skills_loaded"`
	Workspace          string `json:"workspace"`
	Stack              string `json:"stack"`
	Trusted            bool   `json:"trusted"`
	SandboxMode        string `json:"sandbox_mode"`
	InstructionsLoaded bool   `json:"instructions_loaded"`
	InstructionsPath   string `json:"instructions_path"`
	InstructionsLines  int    `json:"instructions_lines"`
	InstructionsSize   int    `json:"instructions_size"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
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
	sandboxMode := cfg.Sandbox.Mode
	if sandboxMode == "" {
		sandboxMode = "off"
	}
	instrPath, instrText := executor.ProjectInstructions(cfg.Workspace)
	instrLines := 0
	if instrText != "" {
		instrLines = strings.Count(instrText, "\n") + 1
	}
	writeJSON(w, statusPayload{
		Version:            s.Version,
		Brain1Name:         "Reflex Engine",
		Brain1Model:        b1Model,
		Brain1State:        "Available",
		Brain1Latency:      state.LatencyMs,
		Brain2Name:         "Architect",
		Brain2Model:        b2Model,
		Brain2State:        "Standby",
		DaemonRunning:      state.Running,
		DaemonPID:          state.PID,
		SkillsLoaded:       daemon.CountSkills(cfg.Workspace),
		Workspace:          cfg.Workspace,
		Stack:              config.DetectStack(cfg.Workspace),
		Trusted:            cfg.Policy.Trusted,
		SandboxMode:        sandboxMode,
		InstructionsLoaded: instrPath != "",
		InstructionsPath:   instrPath,
		InstructionsLines:  instrLines,
		InstructionsSize:   len(instrText),
	})
}

func (s *Server) handleTrust(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Trusted *bool `json:"trusted"`
	}
	if err := transport.Decode(r, &body); err != nil || body.Trusted == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", `body must be {"trusted": true} or {"trusted": false}`)
		return
	}
	cfg, cfgFile, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	cfg.Policy.Trusted = *body.Trusted
	if err := cfg.Save(cfgFile); err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	writeJSON(w, map[string]any{"trusted": cfg.Policy.Trusted})
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
			case ".git", ".apw", ".worktrees", "vendor", "node_modules", "dist", "build", ".next", "target", "__pycache__", ".venv", "venv":
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(cfg.Workspace, path)
		if relErr != nil || surgeon.AllowedPath(rel) != nil {
			return nil
		}
		symbols, err := surgeon.InspectWorkspaceSymbols(cfg.Workspace, rel)
		if err == nil && len(symbols) > 0 {
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

func runnableMode(m core.TaskMode) bool {
	switch m {
	case core.ModeExecute, core.ModePreview, core.ModeReview, core.ModeUnverified:
		return true
	}
	return false
}

const untrustedMessage = "This workspace is not trusted for build and test commands. Trust it first, or use preview, review or unverified mode."

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		RequestID   string        `json:"request_id"`
		SessionID   string        `json:"session_id,omitempty"`
		Instruction string        `json:"instruction"`
		Mode        core.TaskMode `json:"mode"`
	}
	if err := transport.Decode(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !runnableMode(payload.Mode) {
		writeError(w, http.StatusBadRequest, "invalid_mode", "mode must be execute, preview, review or unverified")
		return
	}
	if payload.Mode == core.ModeExecute {
		cfg, _, err := daemon.LoadConfig(s.ConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "config_error", err.Error())
			return
		}
		if !cfg.Policy.Trusted {
			writeError(w, http.StatusConflict, "untrusted", untrustedMessage)
			return
		}
	}
	backend, err := s.backend(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "backend_unavailable", err.Error())
		return
	}
	job, err := backend.Submit(r.Context(), "local", app.Request{
		RequestID:   payload.RequestID,
		SessionID:   payload.SessionID,
		Instruction: payload.Instruction,
		Mode:        payload.Mode,
	})
	if err != nil {
		writeSubmitError(w, err)
		return
	}
	writeJSON(w, job)
}

func writeSubmitError(w http.ResponseWriter, err error) {
	switch {
	case isTransportError(err):
		writeError(w, http.StatusServiceUnavailable, "backend_unavailable", err.Error())
	case strings.Contains(err.Error(), "not trusted"):
		writeError(w, http.StatusConflict, "untrusted", err.Error())
	case strings.Contains(err.Error(), app.ErrNoModelConnected.Error()):
		writeError(w, http.StatusConflict, "no_model", err.Error())
	default:
		writeError(w, http.StatusBadRequest, "rejected", err.Error())
	}
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	// The body is optional, but a body that is present must be a JSON object.
	data, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if len(bytes.TrimSpace(data)) > 0 {
		var ignored map[string]json.RawMessage
		if err := json.Unmarshal(data, &ignored); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "body must be a JSON object: "+err.Error())
			return
		}
	}
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	if !cfg.Policy.Trusted {
		writeError(w, http.StatusConflict, "untrusted", untrustedMessage)
		return
	}
	backend, err := s.backend(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "backend_unavailable", err.Error())
		return
	}
	job, err := backend.Submit(r.Context(), "local", app.Request{
		RequestID:   app.NewID(),
		Instruction: "Verify project without repairs",
		Mode:        core.ModeVerify,
	})
	if err != nil {
		writeSubmitError(w, err)
		return
	}
	writeJSON(w, job)
}

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	gk := s.gatekeeper()
	writeJSON(w, map[string]any{
		"pending":        gk.ListPending(),
		"always_allowed": gk.AlwaysAllowedCommands(),
	})
}

func (s *Server) handleDecideApproval(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Decision string `json:"decision"`
	}
	if err := transport.Decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	switch ApprovalDecisionType(body.Decision) {
	case DecisionOnce, DecisionAlways, DecisionDeny:
	default:
		writeError(w, http.StatusBadRequest, "invalid_decision", "decision must be once, always or deny")
		return
	}
	resolved, err := s.gatekeeper().Resolve(r.PathValue("id"), ApprovalDecisionType(body.Decision))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	writeJSON(w, resolved)
}

func writeJSON(w http.ResponseWriter, value any) { transport.WriteJSON(w, value) }

// writeError sends the Desktop API's JSON error: {"error": "<code>", "message": "<text>"}.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
}

const desktopCSP = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self' data:; img-src 'self' data:; connect-src 'self'; frame-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	distFS, err := webDistFS()
	if err != nil {
		http.Error(w, "desktop assets unavailable", http.StatusInternalServerError)
		return
	}
	raw, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		http.Error(w, "desktop index unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	// The page holds the API token in <meta name="apw-token">: it loads only same-origin
	// scripts/fonts/styles and talks only to this server, with inline scripts forbidden.
	w.Header().Set("Content-Security-Policy", desktopCSP)
	html := strings.ReplaceAll(string(raw), "__APW_TOKEN__", s.Token)
	ver := s.Version
	if len(s.Token) >= 8 {
		ver = s.Token[:8]
	}
	html = strings.ReplaceAll(html, "/assets/app.js", "/assets/app.js?v="+ver)
	html = strings.ReplaceAll(html, "/assets/index.css", "/assets/index.css?v="+ver)
	_, _ = w.Write([]byte(html))
}
