package desktop

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"github.com/apiwat/apw-agent/internal/workspace"
)

const instructionsTruncatedMarker = "\n[instructions truncated]"

// starterInstructions is the APW.md written by /api/instructions/init. It is a neutral outline
// for the user to fill in; it makes no claims about the project.
const starterInstructions = `# Project instructions

APW reads this file before it plans changes. Keep it short and specific.

## Project overview

Describe what this project does and how the code is organised.

## Conventions

- Naming, formatting and structure rules that every change should follow.
- How errors are handled and how tests are written.

## Commands to prefer

- Build:
- Test:
- Lint or format:

## Things to avoid

- Files, folders or dependencies that must not be changed.
- Patterns the team has decided against.
`

func (s *Server) handleInstructions(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	name, text := executor.ProjectInstructions(cfg.Workspace)
	if name == "" {
		writeError(w, http.StatusNotFound, "not_found", "no APW.md or AGENTS.md in the workspace")
		return
	}
	truncated := strings.HasSuffix(text, instructionsTruncatedMarker)
	text = strings.TrimSuffix(text, instructionsTruncatedMarker)
	writeJSON(w, map[string]any{"path": name, "content": text, "truncated": truncated})
}

func (s *Server) handleInstructionsInit(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	lock, err := workspace.Acquire(cfg.Workspace, "write")
	if err != nil {
		if strings.Contains(err.Error(), "is busy") {
			writeError(w, http.StatusConflict, "busy", "a task is using the workspace right now; try again when it finishes")
			return
		}
		writeError(w, http.StatusInternalServerError, "lock_error", err.Error())
		return
	}
	defer lock.Close()

	for _, name := range executor.InstructionFiles {
		_, err := os.Lstat(filepath.Join(cfg.Workspace, name))
		if err == nil {
			writeJSON(w, map[string]any{"path": name, "created": false})
			return
		}
		if !errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusInternalServerError, "read_error", err.Error())
			return
		}
	}
	if err := surgeon.WriteFileAtomic(filepath.Join(cfg.Workspace, "APW.md"), []byte(starterInstructions), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, "write_error", err.Error())
		return
	}
	writeJSON(w, map[string]any{"path": "APW.md", "created": true})
}
