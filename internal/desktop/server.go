package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"github.com/apiwat/apw-agent/internal/transport"
)

type Server struct {
	ConfigPath, Version, Token string
	Backend                    app.API

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
	return &Server{ConfigPath: path, Version: version, Token: hex.EncodeToString(secret[:])}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/symbols", s.handleSymbols)
	mux.HandleFunc("POST /api/plan", s.handlePlan)
	mux.HandleFunc("POST /api/run", s.handleRun)
	mux.HandleFunc("POST /api/verify", s.handleVerify)
	mux.Handle("/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backend, err := s.backend(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		transport.Handler(backend, s.Token, map[string]string{"protocol": transport.Version}, nil).ServeHTTP(w, r)
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
	server := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second}
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
func OpenAppWindow(url string) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		if cmd.Start() == nil {
			_ = cmd.Process.Release()
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
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	state := daemon.Status(cfg.Workspace)
	writeJSON(w, statusPayload{Version: s.Version, Brain1Name: "Reflex Engine", Brain1State: "Available", Brain2Name: "Architect", Brain2State: "Standby", Brain1Latency: state.LatencyMs, DaemonRunning: state.Running, DaemonPID: state.PID, SkillsLoaded: daemon.CountSkills(cfg.Workspace), Workspace: cfg.Workspace, Stack: config.DetectStack(cfg.Workspace)})
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
	var request app.Request
	if err := transport.Decode(r, &request); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	backend, err := s.backend(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	job, err := backend.Submit(r.Context(), "local", request)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, job)
}
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	backend, err := s.backend(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	job, err := backend.Submit(r.Context(), "local", app.Request{RequestID: app.NewID(), Instruction: "Verify project without repairs", Mode: core.ModeVerify})
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
	w.Header().Set("Content-Security-Policy", "frame-ancestors "+string(rune(39))+"none"+string(rune(39))+"; base-uri "+string(rune(39))+"none"+string(rune(39)))
	_, _ = w.Write([]byte(strings.ReplaceAll(desktopHTML, "__APW_TOKEN__", s.Token)))
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
    <div class="pill"><span class="dot"></span><span id="pill-brain1">Brain 1 (Reflex): Reflex Engine</span></div>
    <div class="pill"><span class="dot standby" id="dot-brain2"></span><span id="pill-brain2">Brain 2 (Deep): Architect [Standby]</span></div>
    <div class="pill"><span class="dot"></span><span id="pill-daemon">Daemon (apwd): Checking…</span></div>
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
      <button onclick="runTask('review')">Review</button>
      <button class="secondary" onclick="runTask('preview')">Preview changes</button>
      <button class="secondary" onclick="runTask('execute')">Execute in worktree</button>
      <button class="secondary" onclick="runTask('plan')">Plan Only</button>
      <button class="secondary" onclick="runVerify()">Verify (Reflex)</button>
    </div>
  </main>
</div>

<script>
const apiToken = "__APW_TOKEN__";
async function apiFetch(path, options = {}) {
 if (!path.startsWith("/")) throw new Error("Local API path required");
 const headers = { ...(options.headers || {}), Authorization: "Bearer " + apiToken };
 if (options.method === "POST") headers["Content-Type"] = "application/json";
 const request = { ...options, headers };
 try { return await window.fetch(path, request); }
 catch (error) { return window.fetch(path, request); }
}
const watchedTasks = new Set();
const activeStates = new Set(["queued","planning","analyzing","patching","verifying","applying"]);
function jobActions(job) {
 const rowID = "actions-" + job.id;
 const previous = document.getElementById(rowID);
 if (previous) previous.remove();
 const row = document.createElement("div");
 row.id = rowID;
 const actions = activeStates.has(job.status) ? ["cancel"] :
   job.status === "ready_to_apply" ? ["apply"] :
   ["preview_ready","failed","interrupted","cancelled"].includes(job.status) ? ["resume"] : [];
 actions.forEach(action => {
  const button = document.createElement("button");
  button.textContent = action === "apply" ? "Apply verified changes" : action === "resume" ? "Resume / verify" : "Cancel task";
  button.onclick = async () => {
   const res = await apiFetch("/v1/tasks/" + encodeURIComponent(job.id) + "/" + action, {method:"POST",body:"{}"});
   if (!res.ok) { appendCard("Task request failed", await res.text(), "ERROR", "#f85149"); return; }
   watchTask(await res.json());
  };
  row.appendChild(button);
 });
 document.getElementById("feed").appendChild(row);
}
async function watchTask(initial) {
 if (watchedTasks.has(initial.id)) return;
 watchedTasks.add(initial.id);
 let job = initial, cursor = 0;
 appendCard(job.id, job.instruction, job.status, "#d29922");
 if (activeStates.has(job.status)) jobActions(job);
 try {
  while (true) {
   const eventResponse = await apiFetch("/v1/tasks/" + encodeURIComponent(job.id) + "/events?after=" + cursor);
   if (!eventResponse.ok) throw new Error(await eventResponse.text());
   for (const event of await eventResponse.json() || []) {
    cursor = event.seq;
    appendCard(event.stage, event.message, job.id, "#262c36");
   }
   const response = await apiFetch("/v1/tasks/" + encodeURIComponent(job.id));
   if (!response.ok) throw new Error(await response.text());
   job = await response.json();
   if (!activeStates.has(job.status)) {
    if (job.report) {
     appendExecutionReport(job.report);
     if (job.report.summary) appendCard("Summary", job.report.summary, job.status, "#262c36");
     if (job.report.diff) appendCard("Changes for review", job.report.diff, "DIFF", "#262c36");
     if (job.report.acceptance) appendCard("Acceptance evidence", job.report.acceptance, job.status, "#262c36");
    }
    if (job.error) appendCard("Task stopped", job.error, job.status, "#f85149");
    jobActions(job);
    break;
   }
   await new Promise(resolve => setTimeout(resolve, 500));
  }
 } catch (error) { appendCard("Connection interrupted", error.message + " — task remains tracked by the daemon.", "ERROR", "#f85149"); }
 finally { watchedTasks.delete(initial.id); }
}
async function loadTasks() {
 const response = await apiFetch("/v1/tasks");
 if (response.ok) { loadStatus(); for (const job of (await response.json() || []).slice(0, 10)) watchTask(job); }
}

async function loadStatus() {
  const r = await apiFetch('/api/status');
  const s = await r.json();
  document.getElementById('hdr-workspace').textContent = s.workspace + ' (' + s.stack + ')';
  document.getElementById('pill-brain1').textContent = 'Brain 1 (Reflex): ' + s.brain1_name + ' [' + s.brain1_state + ']';
  document.getElementById('pill-brain2').textContent = 'Brain 2 (Deep): ' + s.brain2_name + ' [' + s.brain2_state + ']';
  document.getElementById('pill-daemon').textContent = 'Daemon (apwd): ' + (s.daemon_running ? 'Running (PID: ' + s.daemon_pid + ')' : 'Stopped') + ' | ping ' + s.brain1_latency_ms + 'ms';
  document.getElementById('banner-box').textContent =
    "  ▄▀█ █▀█ █░█░█\n" +
    "  █▀█ █▀▀ ▀▄▀▄▀   v" + s.version + " (Apiwat Agent)\n" +
    " ───────────────────────────────────────────────────────\n" +
    "  Brain 1 (Reflex) : " + s.brain1_name + " [" + s.brain1_state + "]\n" +
    "  Brain 2 (Deep)   : " + s.brain2_name + "     [" + s.brain2_state + "]\n" +
    "  Daemon (apwd)    : " + (s.daemon_running ? "Running (PID: " + s.daemon_pid + ")" : "Stopped") + " | ping " + s.brain1_latency_ms + "ms | " + s.skills_loaded + " Skills Loaded\n" +
    " ───────────────────────────────────────────────────────\n" +
    "  Workspace        : " + s.workspace + " (" + s.stack + ")";
}

async function loadSymbols() {
  const r = await apiFetch('/api/symbols');
  const files = await r.json();
  const el = document.getElementById('symbol-list');
  if (!files || files.length === 0) {
    el.textContent = 'No Go symbols found.';
    return;
  }
  el.replaceChildren();
  for (const file of files) {
   const group = document.createElement("div"); group.className = "file-group";
   const title = document.createElement("div"); title.className = "file-name"; title.textContent = file.file; group.appendChild(title);
   for (const symbol of file.symbols || []) {
    const item = document.createElement("div"); item.className = "sym-item";
    const name = document.createElement("span"); name.textContent = "[" + symbol.Kind + "] " + symbol.Name;
    const location = document.createElement("span"); location.textContent = "L" + symbol.StartLine + "-" + symbol.EndLine;
    item.append(name,location); group.appendChild(item);
   }
   el.appendChild(group);
  }
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
    data.mode === 'preview' ? 'PREVIEW · ' + formatNs(data.duration) : data.passed ? 'PASS · ' + formatNs(data.duration) : 'FAIL · ' + formatNs(data.duration),
    data.mode === 'preview' ? '#d29922' : data.passed ? '#2ea043' : '#f85149'
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
 const response = await apiFetch("/api/verify", {method:"POST",body:"{}"});
 if (!response.ok) { appendCard("Verification request failed",await response.text(),"ERROR","#f85149"); return; }
 watchTask(await response.json());
}
async function runTask(mode) {
 const input = document.getElementById("task-input");
 const instruction = input.value.trim();
 if (!instruction) return;
 try {
  if (mode === "plan") {
   const response = await apiFetch("/api/plan",{method:"POST",body:JSON.stringify({instruction})});
   if (!response.ok) throw new Error(await response.text());
   appendPlanCard(await response.json());
  } else {
   const response = await apiFetch("/api/run",{method:"POST",body:JSON.stringify({request_id:crypto.randomUUID(),instruction,mode})});
   if (!response.ok) throw new Error(await response.text());
   watchTask(await response.json());
  }
  input.value = "";
 } catch (error) { appendCard("Task request failed",error.message,"ERROR","#f85149"); }
}
loadStatus();
loadSymbols();
loadTasks();
</script>
</body>
</html>`
