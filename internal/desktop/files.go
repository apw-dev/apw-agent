package desktop

import (
	"bytes"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

type searchPayload struct {
	Files   []string                  `json:"files"`
	Symbols []surgeon.SymbolDef       `json:"symbols"`
	Skills  []executor.WorkspaceSkill `json:"skills"`
}

// fuzzyMatch checks if all characters of pattern appear in order in target (case-insensitive).
func fuzzyMatch(target, pattern string) bool {
	if pattern == "" {
		return true
	}
	t := strings.ToLower(target)
	p := strings.ToLower(pattern)
	if strings.Contains(t, p) {
		return true
	}
	pRunes := []rune(p)
	idx := 0
	for _, r := range t {
		if r == pRunes[idx] {
			idx++
			if idx == len(pRunes) {
				return true
			}
		}
	}
	return false
}

// handleSearch serves /api/search?q=<query> for @file and @#symbol fuzzy autocomplete in the Desktop UI.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	q := strings.TrimSpace(strings.TrimPrefix(r.URL.Query().Get("q"), "@"))
	symbolOnly := strings.HasPrefix(q, "#")
	skillOnly := strings.HasPrefix(strings.ToLower(q), "skill:")
	cleanQ := strings.TrimPrefix(q, "#")

	type fileEntry struct {
		path  string
		mtime int64
		exact bool
	}
	var entries []fileEntry
	if !symbolOnly && !skillOnly {
		_ = filepath.WalkDir(cfg.Workspace, func(fpath string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				switch d.Name() {
				case ".git", ".apw", ".worktrees", "vendor", "node_modules", "dist", "build", ".next", "target", "__pycache__", ".venv", "venv":
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
				return nil
			}
			rel, relErr := filepath.Rel(cfg.Workspace, fpath)
			if relErr != nil || surgeon.AllowedPath(rel) != nil {
				return nil
			}
			slash := filepath.ToSlash(rel)
			if !fuzzyMatch(slash, cleanQ) {
				return nil
			}
			var mtime int64
			if fi, fiErr := d.Info(); fiErr == nil {
				mtime = fi.ModTime().UnixNano()
			}
			exact := cleanQ != "" && strings.Contains(strings.ToLower(slash), strings.ToLower(cleanQ))
			entries = append(entries, fileEntry{path: slash, mtime: mtime, exact: exact})
			return nil
		})
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].exact != entries[j].exact {
				return entries[i].exact
			}
			if entries[i].mtime != entries[j].mtime {
				return entries[i].mtime > entries[j].mtime
			}
			return entries[i].path < entries[j].path
		})
	}

	files := make([]string, 0, min(len(entries), 100))
	for i := 0; i < len(entries) && i < 100; i++ {
		files = append(files, entries[i].path)
	}

	var symbols []surgeon.SymbolDef
	if skillOnly {
		// skill:<slug> completes from the skills list only.
	} else if g, gErr := surgeon.InspectWorkspaceGraph(cfg.Workspace); gErr == nil && g != nil {
		if cleanQ != "" {
			symbols = g.FindSymbols(cleanQ)
		} else {
			symbols = g.Definitions
		}
		if len(symbols) > 100 {
			symbols = symbols[:100]
		}
	}
	if symbols == nil {
		symbols = []surgeon.SymbolDef{}
	}
	skills := executor.DiscoverWorkspaceSkills(cfg.Workspace)
	if skills == nil {
		skills = []executor.WorkspaceSkill{}
	}
	writeJSON(w, searchPayload{Files: files, Symbols: symbols, Skills: skills})
}
