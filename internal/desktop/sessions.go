package desktop

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/session"
)

// desktopOwner is the job owner of everything the Desktop and CLI submit.
const desktopOwner = "local"

var conversationIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type sessionSummary struct {
	ID              string    `json:"id"`
	UpdatedAt       time.Time `json:"updated_at"`
	TurnCount       int       `json:"turn_count"`
	LastInstruction string    `json:"last_instruction"`
}

type turnView struct {
	Index       int       `json:"index"`
	Timestamp   time.Time `json:"timestamp"`
	Instruction string    `json:"instruction"`
	Success     bool      `json:"success"`
	Applied     bool      `json:"applied"`
	Summary     string    `json:"summary"`
}

// handleCreateSession hands out a new conversation id. It creates no file: the conversation
// becomes real when a task is submitted with the id.
func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, map[string]string{"id": "conv-" + hex.EncodeToString(b[:])})
}

// handleListSessions lists the Desktop/CLI conversations, newest first. Sessions of other owners
// (for example Telegram users) and sessions without a conversation id are never listed.
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	out := []sessionSummary{}
	entries, err := os.ReadDir(filepath.Join(cfg.Workspace, ".apw", "sessions"))
	if err != nil {
		writeJSON(w, out)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		sess, err := session.Load(cfg.Workspace, strings.TrimSuffix(e.Name(), ".json"))
		if err != nil || sess.Owner != desktopOwner || sess.Conversation == "" {
			continue
		}
		last := ""
		if n := len(sess.History); n > 0 {
			last = sess.History[n-1].Instruction
		}
		out = append(out, sessionSummary{
			ID:              sess.Conversation,
			UpdatedAt:       sess.UpdatedAt,
			TurnCount:       len(sess.History),
			LastInstruction: last,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	writeJSON(w, out)
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !conversationIDPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid_id", "conversation id must be 1-64 letters, digits, '_' or '-'")
		return
	}
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	turns := []turnView{}
	sess, err := session.Load(cfg.Workspace, app.SessionFileID(desktopOwner, id))
	switch {
	case err == nil:
		for _, t := range sess.History {
			turns = append(turns, turnView{Index: t.Index, Timestamp: t.Timestamp, Instruction: t.Instruction, Success: t.Success, Applied: t.Applied, Summary: t.Summary})
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		writeError(w, http.StatusInternalServerError, "session_error", err.Error())
		return
	}
	writeJSON(w, map[string]any{"id": id, "turns": turns})
}
