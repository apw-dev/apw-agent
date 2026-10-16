package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/provider/factory"
	"github.com/apiwat/apw-agent/internal/reflex"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

// Server serves the Claude Code Desktop-style Windows UI and API endpoints.
type Server struct {
	ConfigPath string
	Version    string
}

// NewServer creates a new Desktop UI server.
func NewServer(configPath, version string) *Server {
	if configPath == "" {
		configPath = "./apw.json"
	}
	if version == "" {
		version = "0.1.0-alpha"
	}
	return &Server{
		ConfigPath: configPath,
		Version:    version,
	}
}

// Handler returns the HTTP handler for the Desktop UI and REST API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/verify", s.handleVerify)
	mux.HandleFunc("/api/symbols", s.handleSymbols)
	mux.HandleFunc("/api/plan", s.handlePlan)
	mux.HandleFunc("/api/run", s.handleRun)
	return mux
}

// StartAndOpen starts the local desktop server, opens a native Windows app window, and blocks until ctx is done.
func (s *Server) StartAndOpen(ctx context.Context, addr string, openWindow bool) (string, error) {
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("http://%s", ln.Addr().String())
	httpSrv := &http.Server{
		Handler: s.Handler(),
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	go func() {
		_ = httpSrv.Serve(ln)
	}()

	if openWindow {
		OpenAppWindow(url)
	}
	return url, nil
}

// OpenAppWindow launches a frameless native-like Windows App window (using Edge --app mode on Windows).
func OpenAppWindow(url string) {
	if runtime.GOOS == "windows" {
		// Try Microsoft Edge in standalone app window mode first (looks like a native desktop app)
		cmd := exec.Command("cmd", "/c", "start", "", "msedge", "--app="+url, "--window-size=1220,820")
		if err := cmd.Start(); err == nil {
			_ = cmd.Process.Release()
			return
		}
		// Fallback to default browser on Windows
		fallback := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		if err := fallback.Start(); err == nil {
			_ = fallback.Process.Release()
		}
	}
}

type statusPayload struct {
	Version       string `json:"version"`
	Brain1Name    string `json:"brain1_name"`
	Brain1State   string `json:"brain1_state"`
	Brain1Latency int64  `json:"brain1_latency_ms"`
	Brain2Name    string `json:"brain2_name"`
	Brain2State   string `json:"brain2_state"`
	DaemonRunning bool   `json:"daemon_running"`
	DaemonPID     int    `json:"daemon_pid"`
	SkillsLoaded  int    `json:"skills_loaded"`
	Workspace     string `json:"workspace"`
	Stack         string `json:"stack"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrDefault(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dState := daemon.EnsureRunning(cfg.Workspace)
	absWS, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		absWS = cfg.Workspace
	}

	writeJSON(w, statusPayload{
		Version:       s.Version,
		Brain1Name:    "Jev Engine",
		Brain1State:   "Active",
		Brain1Latency: dState.LatencyMs,
		Brain2Name:    "Architect",
		Brain2State:   "Standby",
		DaemonRunning: dState.Running,
		DaemonPID:     dState.PID,
		SkillsLoaded:  dState.SkillsLoaded,
		Workspace:     filepath.ToSlash(absWS),
		Stack:         config.DetectStack(cfg.Workspace),
	})
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrDefault(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v := reflex.NewVerifier(cfg.Workspace)
	res := v.Verify(r.Context())
	writeJSON(w, map[string]any{
		"passed":       res.Passed,
		"failed_stage": res.FailedStage,
		"diagnostics":  res.Diagnostics,
		"duration_ms":  res.Duration.Milliseconds(),
		"formatted":    reflex.FormatDiagnostics(res),
	})
}

type fileSymbols struct {
	File    string               `json:"file"`
	Symbols []surgeon.SymbolSpan `json:"symbols"`
}

func (s *Server) handleSymbols(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadOrDefault(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sg := surgeon.NewSurgeon()
	var list []fileSymbols
	_ = filepath.WalkDir(cfg.Workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			n := d.Name()
			if n == ".git" || n == "vendor" || n == "testdata" || n == ".apw" || n == ".worktrees" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		rel, relErr := filepath.Rel(cfg.Workspace, path)
		if relErr != nil {
			rel = path
		}
		syms, symErr := sg.InspectGoSymbols(path)
		if symErr == nil && len(syms) > 0 {
			list = append(list, fileSymbols{
				File:    filepath.ToSlash(rel),
				Symbols: syms,
			})
		}
		return nil
	})
	writeJSON(w, list)
}

type taskPromptRequest struct {
	Instruction string `json:"instruction"`
}

func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	var req taskPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Instruction) == "" {
		http.Error(w, "instruction is required", http.StatusBadRequest)
		return
	}

	cfg, err := config.LoadOrDefault(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	provCfg := cfg.Providers[cfg.Roles.Architect]
	prov, err := factory.New(provCfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer prov.Close()

	arch := architect.NewArchitect(prov)
	arch.Model = provCfg.Model
	plan, err := arch.Plan(r.Context(), core.Task{
		ID:          fmt.Sprintf("task-%d", time.Now().UnixNano()),
		Workspace:   cfg.Workspace,
		Instruction: req.Instruction,
	}, "Workspace stack: "+config.DetectStack(cfg.Workspace))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, plan)
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req taskPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Instruction) == "" {
		http.Error(w, "instruction is required", http.StatusBadRequest)
		return
	}

	cfg, err := config.LoadOrDefault(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	archCfg := cfg.Providers[cfg.Roles.Architect]
	archProv, err := factory.New(archCfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer archProv.Close()

	reflexCfg := cfg.Providers[cfg.Roles.Reflex]
	reflexProv, err := factory.New(reflexCfg)
	if err != nil {
		reflexProv = archProv
	} else {
		defer reflexProv.Close()
	}

	ex := executor.New(cfg.Workspace, archProv, reflexProv, archCfg.Model, reflexCfg.Model, cfg.Budget.MaxRepairs)
	rep, err := ex.Execute(r.Context(), req.Instruction, "Workspace stack: "+config.DetectStack(cfg.Workspace))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, rep)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(desktopHTML))
}

const desktopHTML = `<!DOCTYPE html>
<html lang="th">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>APW Desktop — Apiwat Agent (Triad Engine)</title>
<style>
  :root {
    --bg: #0d0f12;
    --panel: #15181e;
    --panel-alt: #1b2028;
    --border: #262c36;
    --text: #e6edf3;
    --muted: #8b949e;
    --accent: #d97757;
    --accent-hover: #e68a6b;
    --green: #3fb950;
    --amber: #d29922;
    --red: #f85149;
    --mono: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
    --sans: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    background: var(--bg);
    color: var(--text);
    font-family: var(--sans);
    height: 100vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  header {
    background: var(--panel);
    border-bottom: 1px solid var(--border);
    padding: 12px 20px;
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  .logo {
    font-family: var(--mono);
    font-weight: 700;
    font-size: 13px;
    line-height: 1.15;
    color: var(--accent);
    white-space: pre;
  }
  .brand-title {
    font-size: 15px;
    font-weight: 600;
  }
  .brand-sub {
    font-size: 12px;
    color: var(--muted);
    font-family: var(--mono);
  }
  .status-pills {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
  }
  .pill {
    background: var(--panel-alt);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 11px;
    font-size: 12px;
    font-family: var(--mono);
    display: flex;
    align-items: center;
    gap: 7px;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--green);
    box-shadow: 0 0 8px var(--green);
  }
  .dot.standby {
    background: var(--amber);
    box-shadow: none;
  }
  .layout {
    display: grid;
    grid-template-columns: 310px 1fr;
    flex: 1;
    overflow: hidden;
  }
  aside {
    background: var(--panel);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .section-hdr {
    padding: 12px 16px;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--muted);
    border-bottom: 1px solid var(--border);
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .symbol-list {
    flex: 1;
    overflow-y: auto;
    padding: 10px;
    font-family: var(--mono);
    font-size: 12px;
  }
  .file-group {
    margin-bottom: 10px;
    background: var(--panel-alt);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 10px;
  }
  .file-name {
    color: var(--accent);
    font-weight: 600;
    margin-bottom: 6px;
  }
  .sym-item {
    color: var(--muted);
    padding: 2px 0;
    display: flex;
    justify-content: space-between;
  }
  main {
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .terminal-banner {
    background: #111419;
    border-bottom: 1px solid var(--border);
    padding: 14px 20px;
    font-family: var(--mono);
    font-size: 12.5px;
    line-height: 1.55;
    color: #c9d1d9;
    white-space: pre-wrap;
  }
  .feed {
    flex: 1;
    overflow-y: auto;
    padding: 20px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .card {
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 14px 16px;
  }
  .card-title {
    font-size: 13px;
    font-weight: 600;
    margin-bottom: 8px;
    color: var(--accent);
    font-family: var(--mono);
  }
  .card pre {
    font-family: var(--mono);
    font-size: 12px;
    white-space: pre-wrap;
    color: var(--text);
    background: #0d0f12;
    padding: 10px;
    border-radius: 6px;
    border: 1px solid var(--border);
    overflow-x: auto;
  }
  .composer {
    background: var(--panel);
    border-top: 1px solid var(--border);
    padding: 14px 20px;
    display: flex;
    gap: 10px;
    align-items: center;
  }
  .prompt-prefix {
    font-family: var(--mono);
    color: var(--accent);
    font-weight: 700;
    font-size: 14px;
  }
  input[type="text"] {
    flex: 1;
    background: var(--bg);
    border: 1px solid var(--border);
    color: var(--text);
    padding: 10px 14px;
    border-radius: 6px;
    font-family: var(--mono);
    font-size: 13px;
    outline: none;
  }
  input[type="text"]:focus {
    border-color: var(--accent);
  }
  button {
    background: var(--accent);
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 10px 15px;
    font-size: 13px;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s;
  }
  button:hover { background: var(--accent-hover); }
  button.secondary {
    background: var(--panel-alt);
    border: 1px solid var(--border);
    color: var(--text);
  }
  button.secondary:hover { background: #262c36; }
</style>
</head>
<body>
<header>
  <div class="brand">
    <div class="logo">▄▀█ █▀█ █░█░█
█▀█ █▀▀ ▀▄▀▄▀</div>
    <div>
      <div class="brand-title">Apiwat Agent Desktop</div>
      <div class="brand-sub" id="hdr-workspace">Loading workspace...</div>
    </div>
  </div>
  <div class="status-pills">
    <div class="pill"><span class="dot"></span><span id="pill-brain1">Brain 1 (Reflex): Jev Engine</span></div>
    <div class="pill"><span class="dot standby" id="dot-brain2"></span><span id="pill-brain2">Brain 2 (Deep): Architect [Standby]</span></div>
    <div class="pill"><span class="dot"></span><span id="pill-daemon">Daemon (Hermes): Running</span></div>
  </div>
</header>

<div class="layout">
  <aside>
    <div class="section-hdr">
      <span>Go AST Symbols (Surgeon)</span>
      <button class="secondary" style="padding:4px 8px;font-size:11px;" onclick="loadSymbols()">Refresh</button>
    </div>
    <div class="symbol-list" id="symbol-list">Loading symbols...</div>
  </aside>

  <main>
    <div class="terminal-banner" id="banner-box">Loading Triad Engine status...</div>
    <div class="feed" id="feed"></div>
    <div class="composer">
      <span class="prompt-prefix">apw &gt;</span>
      <input type="text" id="task-input" placeholder="สั่งงานเป็นภาษาธรรมชาติ หรือพิมพ์คำสั่ง เช่น เพิ่มฟีเจอร์ / ตรวจสอบโค้ด..." onkeydown="if(event.key==='Enter') runTask('run')" />
      <button onclick="runTask('run')">Execute (Triad)</button>
      <button class="secondary" onclick="runTask('plan')">Plan Only</button>
      <button class="secondary" onclick="runVerify()">Verify (Reflex)</button>
    </div>
  </main>
</div>

<script>
async function loadStatus() {
  const r = await fetch('/api/status');
  const s = await r.json();
  document.getElementById('hdr-workspace').textContent = s.workspace + ' (' + s.stack + ')';
  document.getElementById('pill-brain1').textContent = 'Brain 1 (Reflex): ' + s.brain1_name + ' [' + s.brain1_state + ' - ' + s.brain1_latency_ms + 'ms]';
  document.getElementById('pill-brain2').textContent = 'Brain 2 (Deep): ' + s.brain2_name + ' [' + s.brain2_state + ']';
  document.getElementById('pill-daemon').textContent = 'Daemon (Hermes): Running (PID: ' + s.daemon_pid + ') | ' + s.skills_loaded + ' Skills';
  document.getElementById('banner-box').textContent =
    "  ▄▀█ █▀█ █░█░█\n" +
    "  █▀█ █▀▀ ▀▄▀▄▀   v" + s.version + " (Apiwat Agent)\n" +
    " ───────────────────────────────────────────────────────\n" +
    "  Brain 1 (Reflex) : " + s.brain1_name + " [" + s.brain1_state + " - " + s.brain1_latency_ms + "ms]\n" +
    "  Brain 2 (Deep)   : " + s.brain2_name + "  [" + s.brain2_state + "]\n" +
    "  Daemon (Hermes)  : Running (PID: " + s.daemon_pid + ") | " + s.skills_loaded + " Skills Loaded\n" +
    " ───────────────────────────────────────────────────────\n" +
    "  Workspace        : " + s.workspace + " (" + s.stack + ")";
}

async function loadSymbols() {
  const r = await fetch('/api/symbols');
  const files = await r.json();
  const el = document.getElementById('symbol-list');
  if (!files || files.length === 0) {
    el.textContent = 'No Go symbols found.';
    return;
  }
  el.innerHTML = files.map(f =>
    '<div class="file-group"><div class="file-name">' + f.file + '</div>' +
    f.symbols.map(sym => '<div class="sym-item"><span>[' + sym.Kind + '] ' + sym.Name + '</span><span>L' + sym.StartLine + '-' + sym.EndLine + '</span></div>').join('') +
    '</div>'
  ).join('');
}

function appendCard(title, body, badgeText, badgeColor) {
  const feed = document.getElementById('feed');
  const card = document.createElement('div');
  card.className = 'card';
  const h = document.createElement('div');
  h.className = 'card-title';
  h.style.display = 'flex';
  h.style.justifyContent = 'space-between';
  h.style.alignItems = 'center';

  const tSpan = document.createElement('span');
  tSpan.textContent = title;
  h.appendChild(tSpan);

  if (badgeText) {
    const bSpan = document.createElement('span');
    bSpan.textContent = badgeText;
    bSpan.style.padding = '2px 8px';
    bSpan.style.borderRadius = '4px';
    bSpan.style.fontSize = '11px';
    bSpan.style.background = badgeColor || '#262c36';
    bSpan.style.color = '#fff';
    h.appendChild(bSpan);
  }

  const pre = document.createElement('pre');
  pre.textContent = typeof body === 'string' ? body : JSON.stringify(body, null, 2);
  card.appendChild(h);
  card.appendChild(pre);
  feed.appendChild(card);
  feed.scrollTop = feed.scrollHeight;
}

function formatNs(ns) {
  if (!ns) return '0ms';
  const ms = Math.round(ns / 1e6);
  return ms >= 1000 ? (ms / 1000).toFixed(2) + 's' : ms + 'ms';
}

function appendExecutionReport(data) {
  const plan = data.plan || {};
  const steps = (data.steps || []).map(s =>
    '  ' + (s.success ? '✓' : '✗') + ' [' + s.action.toUpperCase() + '] ' + s.subtask_id + (s.target_file ? ' (' + s.target_file + ')' : '') + ' — ' + s.message
  ).join('\n');

  const attempts = (data.reflex_attempts || []).map(a =>
    '  • รอบที่ ' + a.Number + ': ' + (a.Result && a.Result.passed ? 'PASS (fmt -> vet -> build -> test ผ่านทั้งหมด)' : 'FAIL ที่ขั้นตอน ' + (a.Result ? a.Result.failed_stage : 'unknown')) + ' [' + formatNs(a.Result ? a.Result.duration : 0) + ']'
  ).join('\n');

  const summary =
    'เป้าหมาย (Objective): ' + (plan.objective || '-') + '\n' +
    'ไฟล์ที่เกี่ยวข้อง     : ' + ((plan.affected_files || []).join(', ') || '-') + '\n\n' +
    'ขั้นตอนการทำงาน (Surgeon Steps):\n' + (steps || '  (ไม่มีขั้นตอน)') + '\n\n' +
    'ผลการตรวจสอบ (Reflex Verification):\n' + (attempts || '  (ไม่ได้รัน)');

  appendCard(
    'Triad Execution Report — ' + (plan.plan_id || data.task_id),
    summary,
    data.passed ? 'PASS · ' + formatNs(data.duration) : 'FAIL · ' + formatNs(data.duration),
    data.passed ? '#2ea043' : '#f85149'
  );
}

function appendPlanCard(plan) {
  const subtasks = (plan.subtasks || []).map(st =>
    '  • [' + st.action.toUpperCase() + '] ' + st.id + (st.target_file ? ' (' + st.target_file + ')' : '') + ': ' + st.description
  ).join('\n');

  const summary =
    'เป้าหมาย (Objective): ' + (plan.objective || '-') + '\n' +
    'ไฟล์ที่เกี่ยวข้อง     : ' + ((plan.affected_files || []).join(', ') || '-') + '\n\n' +
    'แผนงานย่อย (Subtasks):\n' + subtasks + '\n\n' +
    'เกณฑ์ผ่าน (Acceptance Criteria):\n  - ' + ((plan.acceptance_criteria || []).join('\n  - ') || '-');

  appendCard('Architect ExecutionPlan — ' + plan.plan_id, summary, 'VALIDATED', '#d97757');
}

async function runVerify() {
  appendCard('Reflex Engine — กำลังตรวจสอบโค้ด...', 'gofmt -> go vet -> go build -> go test', 'RUNNING', '#d29922');
  const r = await fetch('/api/verify', { method: 'POST' });
  const res = await r.json();
  if (res.passed) {
    appendCard('Reflex Verification', 'ผ่านครบทุกขั้นตอน (gofmt, go vet, go build, go test)', 'PASS · ' + res.duration_ms + 'ms', '#2ea043');
  } else {
    appendCard('Reflex Verification — ล้มเหลวที่ขั้นตอน: ' + res.failed_stage, res.formatted, 'FAIL', '#f85149');
  }
}

async function runTask(mode) {
  const input = document.getElementById('task-input');
  const instruction = input.value.trim();
  if (!instruction) return;
  input.value = '';

  document.getElementById('pill-brain2').textContent = 'Brain 2 (Deep): Architect [Active]';
  appendCard('apw > ' + instruction, 'กำลังประมวลผลโหมด ' + (mode === 'plan' ? 'PLAN (วางแผน)' : 'TRIAD EXECUTE (วางแผน -> แก้ไข -> ตรวจสอบ)') + '...', mode.toUpperCase(), '#d97757');

  try {
    const endpoint = mode === 'plan' ? '/api/plan' : '/api/run';
    const r = await fetch(endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ instruction })
    });
    if (!r.ok) {
      const errText = await r.text();
      appendCard('เกิดข้อผิดพลาด', errText, 'ERROR', '#f85149');
    } else {
      const data = await r.json();
      if (mode === 'plan') {
        appendPlanCard(data);
      } else {
        appendExecutionReport(data);
      }
      loadSymbols();
    }
  } finally {
    document.getElementById('pill-brain2').textContent = 'Brain 2 (Deep): Architect [Standby]';
  }
}

loadStatus();
loadSymbols();
</script>
</body>
</html>`
