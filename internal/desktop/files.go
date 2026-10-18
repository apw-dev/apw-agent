package desktop

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

// maxFileView bounds the file content /api/file returns.
const maxFileView = 256 * 1024

type filePayload struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Source    string `json:"source"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
}

// handleFile serves one file for the Desktop's file viewer: from the source workspace or, with
// ?task=<id>, from that task's isolated worktree. Reads are link-safe and never reach protected
// paths (.git, .apw, credentials).
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("path"))
	if raw == "" {
		writeError(w, http.StatusBadRequest, "invalid_path", "path is required")
		return
	}
	rel := filepath.Clean(filepath.FromSlash(raw))
	if rel == "." || !filepath.IsLocal(rel) {
		writeError(w, http.StatusBadRequest, "invalid_path", "path must be relative and stay inside the workspace")
		return
	}
	if err := surgeon.AllowedPath(rel); err != nil {
		writeError(w, http.StatusForbidden, "protected", "this path is protected and cannot be viewed")
		return
	}

	base, source := cfg.Workspace, "workspace"
	if task := strings.TrimSpace(r.URL.Query().Get("task")); task != "" {
		wt, status, code, msg := taskWorktree(cfg.Workspace, cfg.WorktreeRoot, task)
		if status != 0 {
			writeError(w, status, code, msg)
			return
		}
		base, source = wt, "worktree"
	}

	data, _, err := surgeon.ReadInRoot(base, rel)
	switch {
	case err == nil:
	case errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusNotFound, "not_found", "file not found")
		return
	case errors.Is(err, surgeon.ErrPathEscape):
		writeError(w, http.StatusBadRequest, "invalid_path", "path leaves the workspace or goes through a link")
		return
	case strings.Contains(err.Error(), "exceeds"):
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "file is too large to view")
		return
	case strings.Contains(err.Error(), "not a regular file"):
		writeError(w, http.StatusBadRequest, "invalid_path", "not a regular file")
		return
	default:
		writeError(w, http.StatusInternalServerError, "read_error", err.Error())
		return
	}

	out := filePayload{Path: filepath.ToSlash(rel), Source: source}
	if bytes.IndexByte(data, 0) >= 0 {
		out.Binary = true
		writeJSON(w, out)
		return
	}
	if len(data) > maxFileView {
		// Do not cut a multi-byte character in half.
		cut := maxFileView
		for cut > 0 && !utf8.RuneStart(data[cut]) {
			cut--
		}
		data = data[:cut]
		out.Truncated = true
	}
	out.Content = string(data)
	writeJSON(w, out)
}

// taskWorktree resolves a task's isolated worktree directory. When it cannot, it returns a
// non-zero HTTP status with the error code and message to report.
func taskWorktree(workspace, override, task string) (dir string, status int, code, msg string) {
	if len(task) > 256 {
		return "", http.StatusBadRequest, "invalid_task", "invalid task id"
	}
	cp, err := executor.CheckpointForTask(workspace, task)
	if err != nil {
		return "", http.StatusNotFound, "task_not_found", "no checkpoint for this task"
	}
	if cp.WorktreePath == "" {
		return "", http.StatusNotFound, "worktree_gone", "this task has no worktree"
	}
	parent, name, err := surgeon.LocateWorktree(workspace, override, cp.WorktreePath)
	if err != nil {
		return "", http.StatusBadRequest, "invalid_worktree", "the task worktree is not a valid APW worktree"
	}
	dir = filepath.Join(parent, name)
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return "", http.StatusNotFound, "worktree_gone", "the task worktree no longer exists"
	}
	return dir, 0, "", ""
}
