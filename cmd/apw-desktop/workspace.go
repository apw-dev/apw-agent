package main

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/apiwat/apw-agent/internal/buildinfo"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/desktop"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

var ErrNoConfig = errors.New("folder has no apw.json")

// BootstrapWorkspace ensures dir has a valid apw.json (inheriting providers, roles, and saved
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
			tmpl.Policy.Trusted = true
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
			cfg.Policy.Trusted = true
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
		_ = cmd.Run()
	}
	surgeon.EnsureGitExclude(abs, ".apw/")
	surgeon.EnsureGitExclude(abs, "apw.json")
	headCmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	headCmd.Dir = abs
	if err := headCmd.Run(); err != nil {
		commitCmd := exec.Command("git",
			"-c", "user.name=APW Agent",
			"-c", "user.email=apw@local",
			"-c", "commit.gpgsign=false",
			"commit", "--allow-empty", "-m", "chore: initialize workspace",
		)
		commitCmd.Dir = abs
		_ = commitCmd.Run()
	}
}

type openWorkspace struct {
	dir     string
	handler http.Handler
}

// WorkspaceHost is the Wails AssetServer handler. Until a workspace is open it serves the
// chooser page; afterwards it delegates to that workspace's desktop.Server. Safe for
// concurrent use: Open swaps the whole workspace atomically.
type WorkspaceHost struct {
	current atomic.Pointer[openWorkspace]
	notice  atomic.Pointer[string]
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

func NewWorkspaceHost() *WorkspaceHost { return &WorkspaceHost{} }

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
// currently open workspace (if any) keeps serving. Reopening the directory that is already
// open is a no-op so its desktop.Server (token and approval state) is preserved.
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
	srv := desktop.NewServer(cfg, buildinfo.Version)
	srv.InProcessHost = desktop.WailsHost
	h.current.Store(&openWorkspace{dir: abs, handler: srv.Handler()})
	return nil
}

// Current returns the open workspace directory, or "" when the chooser is showing.
func (h *WorkspaceHost) Current() string {
	if ws := h.current.Load(); ws != nil {
		return ws.dir
	}
	return ""
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
