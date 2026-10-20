package main

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/apiwat/apw-agent/internal/buildinfo"
	"github.com/apiwat/apw-agent/internal/desktop"
)

var ErrNoConfig = errors.New("folder has no apw.json")

type openWorkspace struct {
	dir     string
	handler http.Handler
}

// WorkspaceHost is the Wails AssetServer handler. Until a workspace is open it serves the
// chooser page; afterwards it delegates to that workspace's desktop.Server. Safe for
// concurrent use: Open swaps the whole workspace atomically.
type WorkspaceHost struct {
	current atomic.Pointer[openWorkspace]
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
	page := strings.Replace(chooserHTML, "__ERROR__", html.EscapeString(r.URL.Query().Get("error")), 1)
	_, _ = w.Write([]byte(page))
}

// Open switches to the workspace in dir. dir must contain a regular apw.json; on failure the
// currently open workspace (if any) keeps serving.
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
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "o") { e.preventDefault(); choose(); }
  });
</script>
</body>
</html>`
