package main

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/apiwat/apw-agent/internal/buildinfo"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/desktop"
	"github.com/apiwat/apw-agent/internal/process"
	"github.com/apiwat/apw-agent/internal/session"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

var ErrNoConfig = errors.New("folder has no apw.json")

// discoverProviders adds connected-account models while a role still uses the built-in
// placeholder (desktop.DiscoverProviders); tests replace it so they never run account CLIs.
var discoverProviders = desktop.DiscoverProviders

// BootstrapWorkspace ensures dir has a valid, untrusted apw.json (inheriting providers, roles, and saved
// credentials from thefirst available fallback workspace) and a Git repository with an initial
// commit so any project folder selected in APW Desktop works out-of-the-box.
func BootstrapWorkspace(dir string, fallbackDirs ...string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("not a directory: %s", abs)
	}

	cfgPath := filepath.Join(abs, "apw.json")
	if info, err := os.Stat(cfgPath); err != nil || !info.Mode().IsRegular() {
		var candidates []string
		candidates = append(candidates, fallbackDirs...)
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			candidates = append(candidates, exeDir, filepath.Dir(exeDir))
		}
		if cwd, err := os.Getwd(); err == nil {
			candidates = append(candidates, cwd)
		}

		initialized := false
		for _, cand := range candidates {
			cand = strings.TrimSpace(cand)
			if cand == "" || samePath(cand, abs) {
				continue
			}
			candCfgPath := filepath.Join(cand, "apw.json")
			tmpl, err := config.Load(candCfgPath)
			if err != nil {
				continue
			}
			tmpl.Workspace = "."
			// Trust is per folder: a new workspace runs no build or test commands until the
			// user trusts it in the workspace popover, even when the template folder is trusted.
			tmpl.Policy.Trusted = false
			if err := tmpl.Save(cfgPath); err == nil {
				initialized = true
				srcAuth := filepath.Join(cand, ".apw", "auth.json")
				dstAuth := filepath.Join(abs, ".apw", "auth.json")
				if _, err := os.Stat(dstAuth); errors.Is(err, os.ErrNotExist) {
					if authBytes, err := os.ReadFile(srcAuth); err == nil {
						_ = os.MkdirAll(filepath.Dir(dstAuth), 0o700)
						_ = os.WriteFile(dstAuth, authBytes, 0o600)
					}
				}
				break
			}
		}

		if !initialized {
			cfg, err := config.LoadOrDefault(cfgPath)
			if err != nil {
				return err
			}
			cfg.Workspace = "."
			cfg.Policy.Trusted = false
			if err := cfg.Save(cfgPath); err != nil {
				return err
			}
		}
	} else if existingCfg, err := config.Load(cfgPath); err == nil {
		changed := false
		for i, k := range existingCfg.Failover.Architect {
			if strings.HasPrefix(k, "claude-sub:") {
				if _, ok := existingCfg.Providers["agy:claude-sonnet-5-5-high"]; ok {
					existingCfg.Failover.Architect[i] = "agy:claude-sonnet-5-5-high"
					changed = true
				} else if _, ok := existingCfg.Providers["chatgpt-sub:gpt-6-sol"]; ok {
					existingCfg.Failover.Architect[i] = "chatgpt-sub:gpt-6-sol"
					changed = true
				}
			}
		}
		if changed {
			_ = existingCfg.Save(cfgPath)
		}
	}

	ensureWorkspaceGitReady(abs)
	return nil
}

func ensureWorkspaceGitReady(abs string) {
	gitEntry := filepath.Join(abs, ".git")
	if _, err := os.Stat(gitEntry); errors.Is(err, os.ErrNotExist) {
		cmd := exec.Command("git", "init")
		cmd.Dir = abs
		process.HideWindow(cmd)
		_ = cmd.Run()
	}
	surgeon.EnsureGitExclude(abs, ".apw/")
	surgeon.EnsureGitExclude(abs, "apw.json")
	headCmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	headCmd.Dir = abs
	process.HideWindow(headCmd)
	if err := headCmd.Run(); err != nil {
		commitCmd := exec.Command("git",
			"-c", "user.name=APW Agent",
			"-c", "user.email=apw@local",
			"-c", "commit.gpgsign=false",
			"commit", "--allow-empty", "-m", "chore: initialize workspace",
		)
		commitCmd.Dir = abs
		process.HideWindow(commitCmd)
		_ = commitCmd.Run()
	}
}

type openWorkspace struct {
	dir     string
	handler http.Handler
}

// WorkspaceHost is the Wails AssetServer handler. Until a workspace is open it serves the
// chooser page; afterwards it delegates to that workspace's desktop.Server. Safe for
// concurrent use: Open retains opened workspace servers in memory so multiple projects
// can run tasks concurrently and switch instantaneously.
type WorkspaceHost struct {
	current atomic.Pointer[openWorkspace]
	notice  atomic.Pointer[string]
	mu      sync.Mutex
	token   string
	servers map[string]*openWorkspace
}

// SetNotice sets the message the chooser shows when no ?error= is given, e.g. why the last
// workspace could not be reopened at startup.
func (h *WorkspaceHost) SetNotice(msg string) { h.notice.Store(&msg) }

// TakeNotice returns and clears the pending notice.
func (h *WorkspaceHost) TakeNotice() string {
	if n := h.notice.Swap(nil); n != nil {
		return *n
	}
	return ""
}

func NewWorkspaceHost() *WorkspaceHost {
	return &WorkspaceHost{
		servers: make(map[string]*openWorkspace),
	}
}

func (h *WorkspaceHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if ws := h.current.Load(); ws != nil {
		ws.handler.ServeHTTP(w, r)
		return
	}
	if r.URL.Path != "/" {
		http.Error(w, "no workspace open", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'unsafe-inline'")
	msg := r.URL.Query().Get("error")
	if n := h.notice.Load(); msg == "" && n != nil {
		msg = *n
	}
	page := strings.Replace(chooserHTML, "__ERROR__", html.EscapeString(msg), 1)
	_, _ = w.Write([]byte(page))
}

// Open switches to the workspace in dir. dir must contain a regular apw.json; on failure the
// currently open workspace (if any) keeps serving. Previously opened workspaces are cached in
// h.servers so switching between project folders keeps their desktop.Server and background
// tasks alive concurrently.
func (h *WorkspaceHost) Open(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	cfg := filepath.Join(abs, "apw.json")
	fi, err := os.Stat(cfg)
	if err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", ErrNoConfig, cfg)
	}
	h.notice.Store(nil)
	if cur := h.Current(); cur != "" && samePath(cur, abs) {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.servers == nil {
		h.servers = make(map[string]*openWorkspace)
	}
	for _, existing := range h.servers {
		if samePath(existing.dir, abs) {
			h.current.Store(existing)
			return nil
		}
	}

	// First launch on a machine (or a workspace without a model): offer the models of connected
	// accounts in Settings. A failed discovery leaves the placeholder, which tasks then refuse.
	_, _ = discoverProviders(cfg)
	srv := desktop.NewServer(cfg, buildinfo.Version)
	if h.token == "" {
		h.token = srv.Token
	} else {
		srv.Token = h.token
	}
	srv.InProcessHost = desktop.WailsHost
	ws := &openWorkspace{dir: abs, handler: srv.Handler()}
	h.servers[strings.ToLower(filepath.Clean(abs))] = ws
	h.current.Store(ws)
	return nil
}

// Current returns the open workspace directory, or "" when the chooser is showing.
func (h *WorkspaceHost) Current() string {
	if ws := h.current.Load(); ws != nil {
		return ws.dir
	}
	return ""
}

// Remove drops dir from the cached workspace servers and clears current if it matched dir.
func (h *WorkspaceHost) Remove(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, ws := range h.servers {
		if samePath(ws.dir, abs) {
			delete(h.servers, k)
		}
	}
	if ws := h.current.Load(); ws != nil && samePath(ws.dir, abs) {
		h.current.Store(nil)
	}
}

// ProjectSessionItem summarizes one conversation inside a project folder for the Sidebar Projects tree.
type ProjectSessionItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updated_at"`
	TurnCount int       `json:"turn_count"`
	Running   bool      `json:"running,omitempty"`
}

// ProjectFolderView summarizes a project workspace folder and its conversations for the Sidebar Projects tree.
type ProjectFolderView struct {
	Path     string               `json:"path"`
	Name     string               `json:"name"`
	Active   bool                 `json:"active"`
	Running  bool                 `json:"running,omitempty"`
	Sessions []ProjectSessionItem `json:"sessions"`
}

// ListProjectFolders builds the multi-project tree (like Antigravity's Projects sidebar) across
// the active workspace and recent project folders.
func ListProjectFolders(current string, recent []string) []ProjectFolderView {
	var dirs []string
	addDir := func(d string) {
		d = strings.TrimSpace(d)
		if d == "" {
			return
		}
		abs, err := filepath.Abs(d)
		if err != nil {
			return
		}
		for _, seen := range dirs {
			if samePath(seen, abs) {
				return
			}
		}
		if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
			dirs = append(dirs, abs)
		}
	}
	addDir(current)
	for _, r := range recent {
		addDir(r)
	}

	out := make([]ProjectFolderView, 0, len(dirs))
	for _, dir := range dirs {
		name := filepath.Base(dir)
		if name == "" || name == "." {
			name = dir
		}
		sessions := listFolderSessions(dir)
		folderRunning := false
		for _, s := range sessions {
			if s.Running {
				folderRunning = true
				break
			}
		}
		out = append(out, ProjectFolderView{
			Path:     dir,
			Name:     name,
			Active:   current != "" && samePath(current, dir),
			Running:  folderRunning,
			Sessions: sessions,
		})
	}
	// Keep project folders in a stable alphabetical order so switching between folders
	// never causes them to jump or swap positions in the Sidebar Projects tree.
	sort.Slice(out, func(i, j int) bool {
		ni, nj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if ni != nj {
			return ni < nj
		}
		return strings.ToLower(out[i].Path) < strings.ToLower(out[j].Path)
	})
	return out
}

func listFolderSessions(workspaceDir string) []ProjectSessionItem {
	entries, err := os.ReadDir(filepath.Join(workspaceDir, ".apw", "sessions"))
	if err != nil {
		return []ProjectSessionItem{}
	}
	items := []ProjectSessionItem{}
	seen := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		sess, err := session.Load(workspaceDir, strings.TrimSuffix(e.Name(), ".json"))
		if err != nil || sess.Owner != "local" || sess.Conversation == "" {
			continue
		}
		convID := sess.Conversation
		if seen[convID] {
			continue
		}
		seen[convID] = true
		title := ""
		isRunning := false
		if n := len(sess.History); n > 0 {
			for _, turn := range sess.History {
				cand := strings.TrimSpace(turn.Instruction)
				if cand != "" && cand != "." {
					title = cand
					break
				}
			}
			lastTurn := sess.History[n-1]
			if lastTurn.TaskID != "" && !lastTurn.Success && lastTurn.Error == "" && lastTurn.Summary == "" {
				if time.Since(sess.UpdatedAt) < 2*time.Hour {
					isRunning = true
				}
			}
		}
		if title == "" {
			title = convID
		}
		items = append(items, ProjectSessionItem{
			ID:        convID,
			Title:     title,
			UpdatedAt: sess.UpdatedAt,
			TurnCount: len(sess.History),
			Running:   isRunning,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	if len(items) > 25 {
		items = items[:25]
	}
	return items
}

const chooserHTML = `<!DOCTYPE html>
<html lang="th">
<head>
<meta charset="utf-8">
<title>APW</title>
<style>
  :root { color-scheme: light dark; }
  body { margin: 0; height: 100vh; display: grid; place-items: center; font-family: "Segoe UI Variable", "Segoe UI", "Leelawadee UI", sans-serif; background: Canvas; color: CanvasText; user-select: none; }
  main { text-align: center; max-width: 420px; padding: 24px; }
  h1 { font-size: 18px; font-weight: 600; margin: 0 0 8px; }
  p { margin: 0 0 20px; opacity: .75; line-height: 1.6; }
  #err { color: #c42b1c; opacity: 1; }
  #err:empty { display: none; }
  button { font: inherit; padding: 8px 18px; border-radius: 8px; border: 1px solid #c96442; background: #c96442; color: #fff; cursor: pointer; }
</style>
</head>
<body>
<main>
  <h1>เลือก workspace ที่มี apw.json</h1>
  <p>เปิดโฟลเดอร์โปรเจกต์ที่ตั้งค่า APW ไว้แล้ว (Ctrl+O)</p>
  <p id="err">__ERROR__</p>
  <button id="choose" type="button">เปิดโฟลเดอร์…</button>
</main>
<script>
  function choose() {
    if (window.go && window.go.main && window.go.main.App) window.go.main.App.ChooseWorkspace();
  }
  document.getElementById("choose").addEventListener("click", choose);
  document.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && (e.code === "KeyO" || e.key.toLowerCase() === "o")) { e.preventDefault(); choose(); }
  });
</script>
</body>
</html>`
