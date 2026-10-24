package desktop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/transport"
)

type QuickScript struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Command      string `json:"command"`
	Category     string `json:"category"` // "dev", "start", "build", "test", "docker"
	Source       string `json:"source"`   // e.g. "package.json", "web/package.json", "deploy/docker-compose.yml"
	DefaultPort  int    `json:"default_port,omitempty"`
	TargetPort   int    `json:"target_port,omitempty"`
	PortConflict bool   `json:"port_conflict,omitempty"`
	HintURL      string `json:"hint_url,omitempty"`
	Description  string `json:"description,omitempty"`
}

type TerminalSessionView struct {
	ID        string   `json:"id"`
	Command   string   `json:"command"`
	Cwd       string   `json:"cwd"`
	Port      int      `json:"port,omitempty"`
	Status    string   `json:"status"` // "running", "exited", "failed", "stopped"
	PID       int      `json:"pid,omitempty"`
	ExitCode  int      `json:"exit_code"`
	StartedAt string   `json:"started_at"`
	UpdatedAt string   `json:"updated_at"`
	Output    string   `json:"output"`
	URLs      []string `json:"urls"`
}

type ActivePortInfo struct {
	Port   int    `json:"port"`
	URL    string `json:"url"`
	Source string `json:"source,omitempty"`
}

type TerminalStatePayload struct {
	Workspace   string                `json:"workspace"`
	Scripts     []QuickScript         `json:"scripts"`
	Sessions    []TerminalSessionView `json:"sessions"`
	ActivePorts []ActivePortInfo      `json:"active_ports"`
}

type managedProcess struct {
	mu        sync.Mutex
	id        string
	command   string
	cwd       string
	port      int
	hintURL   string
	status    string
	pid       int
	exitCode  int
	startedAt time.Time
	updatedAt time.Time
	output    bytes.Buffer
	urls      []string
	cmd       *exec.Cmd
}

const maxTerminalOutputBytes = 64 * 1024

var (
	ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07`)
	urlDetectRe  = regexp.MustCompile(`https?://(?:localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1\]):(\d+)(/[^\s"'\)\]]*)?`)
	portWordRe   = regexp.MustCompile(`(?i)(?:port|listening\s+on|started\s+on|http://localhost:|127\.0\.0\.1:|0\.0\.0\.0:)\s*:?\s*(\d{4,5})\b`)
)

func (p *managedProcess) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	cleaned := ansiEscapeRe.ReplaceAll(b, nil)
	if p.output.Len()+len(cleaned) > maxTerminalOutputBytes {
		excess := (p.output.Len() + len(cleaned)) - maxTerminalOutputBytes
		cur := p.output.Bytes()
		if excess < len(cur) {
			trimmed := append([]byte(nil), cur[excess:]...)
			p.output.Reset()
			p.output.Write(trimmed)
		} else {
			p.output.Reset()
		}
	}
	p.output.Write(cleaned)
	p.updatedAt = time.Now().UTC()
	p.extractURLsLocked(string(cleaned))
	return len(b), nil
}

func (p *managedProcess) extractURLsLocked(chunk string) {
	addURL := func(raw string) {
		raw = strings.TrimRight(raw, ".,;:!?)")
		raw = strings.Replace(raw, "://0.0.0.0:", "://localhost:", 1)
		raw = strings.Replace(raw, "://127.0.0.1:", "://localhost:", 1)
		raw = strings.Replace(raw, "://[::1]:", "://localhost:", 1)
		for _, existing := range p.urls {
			if existing == raw {
				return
			}
		}
		p.urls = append(p.urls, raw)
	}

	for _, m := range urlDetectRe.FindAllString(chunk, -1) {
		addURL(m)
	}
	for _, sm := range portWordRe.FindAllStringSubmatch(chunk, -1) {
		if len(sm) >= 2 {
			port, err := strconv.Atoi(sm[1])
			if err == nil && port >= 1024 && port <= 65535 {
				addURL(fmt.Sprintf("http://localhost:%d", port))
			}
		}
	}
}

func (p *managedProcess) snapshot() TerminalSessionView {
	p.mu.Lock()
	defer p.mu.Unlock()

	urls := append([]string(nil), p.urls...)
	if p.status == "running" && len(urls) == 0 {
		if p.port > 0 && isLocalPortOpen(p.port) {
			urls = append(urls, fmt.Sprintf("http://localhost:%d", p.port))
		} else if uPort := extractPortFromURL(p.hintURL); uPort > 0 && isLocalPortOpen(uPort) {
			urls = append(urls, p.hintURL)
		}
	}
	return TerminalSessionView{
		ID:        p.id,
		Command:   p.command,
		Cwd:       p.cwd,
		Port:      p.port,
		Status:    p.status,
		PID:       p.pid,
		ExitCode:  p.exitCode,
		StartedAt: p.startedAt.Format(time.RFC3339),
		UpdatedAt: p.updatedAt.Format(time.RFC3339),
		Output:    p.output.String(),
		URLs:      urls,
	}
}

type terminalManager struct {
	mu    sync.Mutex
	procs []*managedProcess
	seq   int
}

var globalTerminalManager = &terminalManager{}

func (m *terminalManager) list(workspace string) []TerminalSessionView {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanWS := filepath.Clean(workspace)
	out := make([]TerminalSessionView, 0, len(m.procs))
	for _, p := range m.procs {
		if workspace == "" || strings.EqualFold(filepath.Clean(p.cwd), cleanWS) {
			out = append(out, p.snapshot())
		}
	}
	return out
}

func (m *terminalManager) workspaceRunningPorts(workspace string) map[int]bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanWS := filepath.Clean(workspace)
	owned := map[int]bool{}
	for _, p := range m.procs {
		p.mu.Lock()
		sameWS := workspace == "" || strings.EqualFold(filepath.Clean(p.cwd), cleanWS)
		running := p.status == "running"
		port := p.port
		urls := append([]string(nil), p.urls...)
		p.mu.Unlock()
		if !sameWS || !running {
			continue
		}
		if port > 0 {
			owned[port] = true
		}
		for _, u := range urls {
			if up := extractPortFromURL(u); up > 0 {
				owned[up] = true
			}
		}
	}
	return owned
}

func applyPortToCommand(command string, targetPort int) string {
	if targetPort <= 0 {
		return command
	}
	lower := strings.ToLower(command)
	// If running Vite via npm and no --port is passed yet, pass -- --port <targetPort>
	if strings.Contains(lower, "vite") && !strings.Contains(lower, "--port") {
		return fmt.Sprintf("%s --port %d", command, targetPort)
	}
	if (strings.Contains(lower, "--prefix web run dev") || strings.Contains(lower, "--workspace web")) && !strings.Contains(lower, "--port") {
		return fmt.Sprintf("%s -- --port %d", command, targetPort)
	}
	return command
}

func needsNodeModulesInstall(workspace, command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	if !strings.HasPrefix(lower, "npm ") && !strings.HasPrefix(lower, "npx ") {
		return false
	}
	if strings.HasPrefix(lower, "npm install") || strings.HasPrefix(lower, "npm i") || strings.HasPrefix(lower, "npm ci") {
		return false
	}
	if _, err := os.Stat(filepath.Join(workspace, "package.json")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(workspace, "node_modules")); err == nil {
		return false
	}
	return true
}

func (m *terminalManager) start(workspace, command, hintURL string, targetPort int) TerminalSessionView {
	effectiveCmd := applyPortToCommand(strings.TrimSpace(command), targetPort)
	autoInstall := needsNodeModulesInstall(workspace, effectiveCmd)
	if targetPort > 0 && hintURL == "" {
		hintURL = fmt.Sprintf("http://localhost:%d", targetPort)
	} else if targetPort > 0 {
		hintURL = fmt.Sprintf("http://localhost:%d", targetPort)
	}

	m.mu.Lock()
	cleanWS := filepath.Clean(workspace)
	for _, existing := range m.procs {
		existing.mu.Lock()
		sameRunning := strings.EqualFold(filepath.Clean(existing.cwd), cleanWS) &&
			strings.TrimSpace(existing.command) == effectiveCmd &&
			existing.status == "running"
		existing.mu.Unlock()
		if sameRunning {
			m.mu.Unlock()
			return existing.snapshot()
		}
	}

	m.seq++
	id := fmt.Sprintf("term-%d-%d", time.Now().Unix(), m.seq)
	now := time.Now().UTC()
	proc := &managedProcess{
		id:        id,
		command:   effectiveCmd,
		cwd:       workspace,
		port:      targetPort,
		hintURL:   hintURL,
		status:    "running",
		startedAt: now,
		updatedAt: now,
		urls:      []string{},
	}

	runBody := effectiveCmd
	if autoInstall {
		runBody = "npm install --no-audit --no-fund; if ($LASTEXITCODE -eq 0) { " + effectiveCmd + " }"
		if runtime.GOOS != "windows" {
			runBody = "npm install --no-audit --no-fund && " + effectiveCmd
		}
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		psScript := runBody
		if targetPort > 0 {
			psScript = fmt.Sprintf("$env:PORT='%d'; $env:WEB_PORT='%d'; $env:VITE_PORT='%d'; $env:SITE_ADDRESS=':%d'; %s", targetPort, targetPort, targetPort, targetPort, runBody)
		}
		cmd = exec.Command("powershell", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", psScript)
	} else {
		shScript := runBody
		if targetPort > 0 {
			shScript = fmt.Sprintf("PORT=%d WEB_PORT=%d VITE_PORT=%d SITE_ADDRESS=:%d %s", targetPort, targetPort, targetPort, targetPort, runBody)
		}
		cmd = exec.Command("sh", "-c", shScript)
	}
	cmd.Dir = workspace
	if targetPort > 0 {
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("PORT=%d", targetPort),
			fmt.Sprintf("WEB_PORT=%d", targetPort),
			fmt.Sprintf("VITE_PORT=%d", targetPort),
			fmt.Sprintf("SITE_ADDRESS=:%d", targetPort),
		)
	}
	cmd.Stdout = proc
	cmd.Stderr = proc
	proc.cmd = cmd

	if autoInstall {
		_, _ = proc.Write([]byte("[APW Runner] ยังไม่พบ node_modules — กำลังติดตั้ง dependencies (npm install) อัตโนมัติ...\n"))
	}
	if targetPort > 0 {
		_, _ = proc.Write([]byte(fmt.Sprintf("[APW Runner] กำหนดพอร์ตการทำงาน: PORT=%d (http://localhost:%d)\n", targetPort, targetPort)))
	}

	if err := cmd.Start(); err != nil {
		proc.status = "failed"
		proc.exitCode = 1
		_, _ = proc.Write([]byte("Failed to start command: " + err.Error() + "\n"))
		m.procs = append(m.procs, proc)
		m.mu.Unlock()
		return proc.snapshot()
	}

	if cmd.Process != nil {
		proc.pid = cmd.Process.Pid
	}
	m.procs = append(m.procs, proc)
	if len(m.procs) > 12 {
		for i, old := range m.procs {
			old.mu.Lock()
			running := old.status == "running"
			old.mu.Unlock()
			if !running {
				m.procs = append(m.procs[:i], m.procs[i+1:]...)
				break
			}
		}
	}
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		proc.mu.Lock()
		defer proc.mu.Unlock()
		proc.updatedAt = time.Now().UTC()
		if proc.status == "stopped" {
			return
		}
		if err != nil {
			proc.status = "failed"
			if exitErr, ok := err.(*exec.ExitError); ok {
				proc.exitCode = exitErr.ExitCode()
			} else {
				proc.exitCode = 1
			}
		} else {
			proc.status = "exited"
			proc.exitCode = 0
		}
	}()

	return proc.snapshot()
}

func (m *terminalManager) stop(id string) (TerminalSessionView, bool) {
	m.mu.Lock()
	var target *managedProcess
	for _, p := range m.procs {
		if p.id == id {
			target = p
			break
		}
	}
	m.mu.Unlock()

	if target == nil {
		return TerminalSessionView{}, false
	}

	target.mu.Lock()
	pid := target.pid
	cmd := target.cmd
	wasRunning := target.status == "running"
	if wasRunning {
		target.status = "stopped"
		target.updatedAt = time.Now().UTC()
		target.output.WriteString("\n[หยุดการทำงานโดยผู้ใช้]\n")
	}
	target.mu.Unlock()

	if wasRunning {
		if runtime.GOOS == "windows" && pid > 0 {
			_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
		}
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	return target.snapshot(), true
}

func (m *terminalManager) clearFinished(workspace string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanWS := filepath.Clean(workspace)
	kept := make([]*managedProcess, 0, len(m.procs))
	for _, p := range m.procs {
		p.mu.Lock()
		running := p.status == "running"
		sameWS := workspace == "" || strings.EqualFold(filepath.Clean(p.cwd), cleanWS)
		p.mu.Unlock()
		if running || !sameWS {
			kept = append(kept, p)
		}
	}
	m.procs = kept
}

func openExternalTerminalWindow(workspace, command string, targetPort int) error {
	effectiveCmd := applyPortToCommand(strings.TrimSpace(command), targetPort)
	if runtime.GOOS == "windows" {
		escapedWS := strings.ReplaceAll(workspace, "'", "''")
		envPrefix := ""
		if targetPort > 0 {
			envPrefix = fmt.Sprintf("$env:PORT='%d'; $env:WEB_PORT='%d'; $env:VITE_PORT='%d'; $env:SITE_ADDRESS=':%d'; Write-Host '🌐 PORT=%d (http://localhost:%d)' -ForegroundColor Green; ", targetPort, targetPort, targetPort, targetPort, targetPort, targetPort)
		}
		psCmd := fmt.Sprintf("Set-Location -LiteralPath '%s'; %sWrite-Host '▶ %s' -ForegroundColor Cyan; %s", escapedWS, envPrefix, strings.ReplaceAll(effectiveCmd, "'", "''"), effectiveCmd)
		cmd := exec.Command("cmd", "/c", "start", "APW Terminal", "powershell", "-NoLogo", "-NoExit", "-Command", psCmd)
		cmd.Dir = workspace
		return cmd.Start()
	}
	return fmt.Errorf("external terminal launch is only supported on Windows")
}

func (s *Server) handleGetTerminal(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	ownedPorts := globalTerminalManager.workspaceRunningPorts(cfg.Workspace)
	scripts := discoverWorkspaceScripts(cfg.Workspace, ownedPorts)
	sessions := globalTerminalManager.list(cfg.Workspace)
	activePorts := collectWorkspaceSessionPorts(sessions)
	writeJSON(w, TerminalStatePayload{
		Workspace:   cfg.Workspace,
		Scripts:     scripts,
		Sessions:    sessions,
		ActivePorts: activePorts,
	})
}

func (s *Server) handleRunTerminal(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Command string `json:"command"`
		Mode    string `json:"mode"` // "integrated" (default) or "external"
		HintURL string `json:"hint_url,omitempty"`
		Port    int    `json:"port,omitempty"`
	}
	if err := transport.Decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	cmdText := strings.TrimSpace(body.Command)
	if cmdText == "" {
		writeError(w, http.StatusBadRequest, "invalid_command", "command is required")
		return
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

	ownedPorts := globalTerminalManager.workspaceRunningPorts(cfg.Workspace)
	scripts := discoverWorkspaceScripts(cfg.Workspace, ownedPorts)

	targetPort := body.Port
	hintURL := body.HintURL
	if targetPort <= 0 {
		for _, sc := range scripts {
			if sc.Command == cmdText {
				if sc.TargetPort > 0 {
					targetPort = sc.TargetPort
				}
				if hintURL == "" {
					hintURL = sc.HintURL
				}
				break
			}
		}
	}
	// If targetPort is currently occupied by an external process, automatically pick a free port!
	if targetPort > 0 && isLocalPortOpen(targetPort) && !ownedPorts[targetPort] {
		targetPort = findNextFreePort(targetPort+1, ownedPorts)
	}
	if targetPort > 0 {
		hintURL = fmt.Sprintf("http://localhost:%d", targetPort)
	}

	if strings.EqualFold(body.Mode, "external") {
		if err := openExternalTerminalWindow(cfg.Workspace, cmdText, targetPort); err != nil {
			writeError(w, http.StatusInternalServerError, "terminal_error", err.Error())
			return
		}
		writeJSON(w, map[string]any{
			"opened_external": true,
			"command":         cmdText,
			"port":            targetPort,
		})
		return
	}

	session := globalTerminalManager.start(cfg.Workspace, cmdText, hintURL, targetPort)
	time.Sleep(180 * time.Millisecond)
	writeJSON(w, map[string]any{
		"session": globalTerminalManager.getOrFallback(session.ID, session),
	})
}

func (m *terminalManager) getOrFallback(id string, fallback TerminalSessionView) TerminalSessionView {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.procs {
		if p.id == id {
			return p.snapshot()
		}
	}
	return fallback
}

func (s *Server) handleStopTerminal(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Clear bool   `json:"clear"`
	}
	if err := transport.Decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	if body.Clear {
		globalTerminalManager.clearFinished(cfg.Workspace)
		writeJSON(w, map[string]any{"cleared": true})
		return
	}
	sess, ok := globalTerminalManager.stop(strings.TrimSpace(body.ID))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "terminal session not found")
		return
	}
	writeJSON(w, map[string]any{"session": sess})
}

func discoverWorkspaceScripts(workspace string, ownedPorts map[int]bool) []QuickScript {
	if workspace == "" {
		return nil
	}
	var scripts []QuickScript
	seenCmd := map[string]bool{}
	reservedTargetPorts := map[int]bool{}

	add := func(sc QuickScript) {
		if sc.Command == "" || seenCmd[sc.Command] {
			return
		}
		seenCmd[sc.Command] = true
		if sc.DefaultPort > 0 {
			busyByOther := isLocalPortOpen(sc.DefaultPort) && !ownedPorts[sc.DefaultPort]
			if busyByOther || reservedTargetPorts[sc.DefaultPort] {
				sc.PortConflict = busyByOther
				sc.TargetPort = findNextFreePort(sc.DefaultPort+1, ownedPorts)
				for reservedTargetPorts[sc.TargetPort] {
					sc.TargetPort = findNextFreePort(sc.TargetPort+1, ownedPorts)
				}
			} else {
				sc.TargetPort = sc.DefaultPort
			}
			reservedTargetPorts[sc.TargetPort] = true
			sc.HintURL = fmt.Sprintf("http://localhost:%d", sc.TargetPort)
		}
		scripts = append(scripts, sc)
	}

	pkgDirs := []string{"", "web", "api", "frontend", "backend", "client", "server", "app"}
	for _, relDir := range pkgDirs {
		pkgPath := filepath.Join(workspace, relDir, "package.json")
		raw, err := os.ReadFile(pkgPath)
		if err != nil {
			continue
		}
		var parsed struct {
			Name    string            `json:"name"`
			Scripts map[string]string `json:"scripts"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Scripts) == 0 {
			continue
		}
		order := []string{"dev", "start", "serve", "preview", "build", "test", "check"}
		for _, key := range order {
			body, ok := parsed.Scripts[key]
			if !ok {
				continue
			}
			cmd := "npm run " + key
			label := key
			source := "package.json"
			if relDir != "" {
				cmd = fmt.Sprintf("npm --prefix %s run %s", relDir, key)
				label = fmt.Sprintf("%s:%s", relDir, key)
				source = filepath.ToSlash(filepath.Join(relDir, "package.json"))
			}
			cat := categorizeScript(key)
			defPort := inferScriptDefaultPort(relDir, key, body)
			add(QuickScript{
				ID:          source + "#" + key,
				Label:       label,
				Command:     cmd,
				Category:    cat,
				Source:      source,
				DefaultPort: defPort,
				Description: body,
			})
		}
	}

	composePaths := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
		"deploy/docker-compose.yml",
		"deploy/docker-compose.yaml",
	}
	for _, relPath := range composePaths {
		full := filepath.Join(workspace, filepath.FromSlash(relPath))
		raw, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		cmd := "docker compose up --build"
		if strings.Contains(relPath, "/") {
			cmd = fmt.Sprintf("docker compose -f %s up --build", relPath)
		}
		defPort := inferComposeDefaultPort(string(raw))
		add(QuickScript{
			ID:          relPath + "#up",
			Label:       "docker:up",
			Command:     cmd,
			Category:    "docker",
			Source:      relPath,
			DefaultPort: defPort,
			Description: "รัน Stack ด้วย Docker Compose",
		})
		break
	}

	if _, err := os.Stat(filepath.Join(workspace, "go.mod")); err == nil {
		add(QuickScript{
			ID:          "go.mod#test",
			Label:       "go:test",
			Command:     "go test ./...",
			Category:    "test",
			Source:      "go.mod",
			Description: "รัน Go Unit Tests ทั้งโปรเจกต์",
		})
	}

	sort.SliceStable(scripts, func(i, j int) bool {
		return scriptPriority(scripts[i].Category) < scriptPriority(scripts[j].Category)
	})
	return scripts
}

func categorizeScript(name string) string {
	switch name {
	case "dev", "serve", "preview":
		return "dev"
	case "start":
		return "start"
	case "build":
		return "build"
	case "test", "check":
		return "test"
	default:
		return "other"
	}
}

func scriptPriority(cat string) int {
	switch cat {
	case "dev":
		return 1
	case "start":
		return 2
	case "docker":
		return 3
	case "build":
		return 4
	case "test":
		return 5
	default:
		return 6
	}
}

var (
	scriptPortFlagRe = regexp.MustCompile(`(?:--port|-p|PORT=)\s*(\d{4,5})`)
	composePortRe    = regexp.MustCompile(`["']?(\d{4,5}):\d{2,5}["']?`)
)

func inferScriptDefaultPort(relDir, scriptName, scriptBody string) int {
	lower := strings.ToLower(scriptBody)
	if m := scriptPortFlagRe.FindStringSubmatch(scriptBody); len(m) >= 2 {
		if p, err := strconv.Atoi(m[1]); err == nil {
			return p
		}
	}
	if strings.Contains(lower, "vite") || strings.Contains(lower, "scripts/dev.") {
		if scriptName == "preview" {
			return 4173
		}
		return 5173
	}
	if strings.Contains(lower, "--workspace api") || strings.Contains(lower, "--prefix api") || relDir == "api" || relDir == "backend" || relDir == "server" {
		if scriptName == "dev" || scriptName == "start" {
			return 3000
		}
	}
	if strings.Contains(lower, "--workspace web") || strings.Contains(lower, "--prefix web") || relDir == "web" || relDir == "frontend" || relDir == "client" {
		if scriptName == "dev" || scriptName == "start" {
			return 5173
		}
	}
	if strings.Contains(lower, "next ") || strings.Contains(lower, "react-scripts") || (scriptName == "dev" && (strings.Contains(lower, "node ") || strings.Contains(lower, "tsx "))) {
		return 3000
	}
	return 0
}

func inferComposeDefaultPort(content string) int {
	matches := composePortRe.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) >= 2 {
			port, err := strconv.Atoi(m[1])
			if err == nil && port >= 1024 && port <= 65535 {
				return port
			}
		}
	}
	return 0
}

func findNextFreePort(startPort int, ownedPorts map[int]bool) int {
	if startPort < 1024 {
		startPort = 3000
	}
	for p := startPort; p < startPort+100 && p <= 65535; p++ {
		if ownedPorts[p] || !isLocalPortOpen(p) {
			return p
		}
	}
	return startPort
}

func extractPortFromURL(u string) int {
	m := urlDetectRe.FindStringSubmatch(u)
	if len(m) >= 2 {
		p, _ := strconv.Atoi(m[1])
		return p
	}
	return 0
}

func isLocalPortOpen(port int) bool {
	if port <= 0 {
		return false
	}
	if conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 45*time.Millisecond); err == nil {
		_ = conn.Close()
		return true
	}
	if conn, err := net.DialTimeout("tcp", fmt.Sprintf("[::1]:%d", port), 45*time.Millisecond); err == nil {
		_ = conn.Close()
		return true
	}
	return false
}

// collectWorkspaceSessionPorts returns ONLY ports that belong to running processes
// started inside this workspace (never external unrelated processes on the host).
func collectWorkspaceSessionPorts(sessions []TerminalSessionView) []ActivePortInfo {
	seen := map[int]bool{}
	var active []ActivePortInfo
	for _, sess := range sessions {
		if sess.Status != "running" {
			continue
		}
		for _, u := range sess.URLs {
			p := extractPortFromURL(u)
			if p > 0 && !seen[p] && isLocalPortOpen(p) {
				seen[p] = true
				active = append(active, ActivePortInfo{
					Port:   p,
					URL:    fmt.Sprintf("http://localhost:%d", p),
					Source: sess.Command,
				})
			}
		}
		if sess.Port > 0 && !seen[sess.Port] && isLocalPortOpen(sess.Port) {
			seen[sess.Port] = true
			active = append(active, ActivePortInfo{
				Port:   sess.Port,
				URL:    fmt.Sprintf("http://localhost:%d", sess.Port),
				Source: sess.Command,
			})
		}
	}
	sort.Slice(active, func(i, j int) bool {
		return active[i].Port < active[j].Port
	})
	return active
}
