package desktop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/store"
)

// maxApprovalFiles bounds how many changed files an approval description lists.
const maxApprovalFiles = 20

// handleApply applies a ready task's verified changes to the workspace. With approval "ask" (the
// default) the user must allow it in the Desktop first; "auto" skips the question.
func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	body := struct {
		Approval string `json:"approval"`
	}{Approval: "ask"}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "body must be {\"approval\": \"ask\"|\"auto\"}: "+err.Error())
			return
		}
	}
	switch body.Approval {
	case "":
		body.Approval = "ask"
	case "ask", "auto":
	default:
		writeError(w, http.StatusBadRequest, "invalid_approval", "approval must be ask or auto")
		return
	}

	backend, err := s.backend(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "backend_unavailable", err.Error())
		return
	}
	job, err := backend.Get(r.Context(), id, desktopOwner)
	if err != nil {
		if isTransportError(err) {
			writeError(w, http.StatusServiceUnavailable, "backend_unavailable", err.Error())
		} else {
			writeError(w, http.StatusNotFound, "not_found", "task not found")
		}
		return
	}
	if job.Status != core.StatusReady {
		writeError(w, http.StatusConflict, "not_ready", fmt.Sprintf("task is %s, not ready to apply", job.Status))
		return
	}

	if body.Approval == "ask" {
		cfg, _, err := daemon.LoadConfig(s.ConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "config_error", err.Error())
			return
		}
		allowed, err := s.gatekeeper().RequestApproval(r.Context(), id, applyTool, "apply task "+id, cfg.Workspace, applyDescription(job))
		if err != nil {
			writeError(w, http.StatusRequestTimeout, "approval_cancelled", "the approval request was cancelled")
			return
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "denied", "applying the changes was denied")
			return
		}
	}

	applied, err := backend.Apply(r.Context(), id, desktopOwner)
	if err != nil {
		switch {
		case isTransportError(err):
			writeError(w, http.StatusServiceUnavailable, "backend_unavailable", err.Error())
		case strings.Contains(err.Error(), "not ready"):
			writeError(w, http.StatusConflict, "not_ready", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "apply_failed", err.Error())
		}
		return
	}
	writeJSON(w, applied)
}

// applyDescription tells the user what an apply will change, and warns when nothing verified it.
func applyDescription(job *store.Job) string {
	var report struct {
		ModifiedFiles []string `json:"modified_files"`
		Unverified    bool     `json:"unverified"`
	}
	if len(job.Report) > 0 {
		_ = json.Unmarshal(job.Report, &report)
	}
	var b strings.Builder
	if report.Unverified || job.Mode == string(core.ModeUnverified) {
		b.WriteString("UNVERIFIED: no build or test checked these changes. ")
	}
	files := report.ModifiedFiles
	if len(files) == 0 {
		b.WriteString("Apply the task's changes to the workspace.")
		return b.String()
	}
	b.WriteString(fmt.Sprintf("Apply changes to %d file(s) in the workspace: ", len(files)))
	if len(files) > maxApprovalFiles {
		b.WriteString(strings.Join(files[:maxApprovalFiles], ", "))
		b.WriteString(fmt.Sprintf(" and %d more", len(files)-maxApprovalFiles))
	} else {
		b.WriteString(strings.Join(files, ", "))
	}
	return b.String()
}
