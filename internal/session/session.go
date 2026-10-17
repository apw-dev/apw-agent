package session

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apiwat/apw-agent/internal/surgeon"
)

// maxHistoryTurns bounds how many recent turns are retained in a session.
const maxHistoryTurns = 50

// Turn represents a single conversational and execution cycle in a session.
type Turn struct {
	Index          int       `json:"index"`
	Timestamp      time.Time `json:"timestamp"`
	Instruction    string    `json:"instruction"`
	PlanID         string    `json:"plan_id,omitempty"`
	CompletedSteps int       `json:"completed_steps"`
	TotalSteps     int       `json:"total_steps"`
	ModifiedFiles  []string  `json:"modified_files,omitempty"`
	Summary        string    `json:"summary,omitempty"`
	Success        bool      `json:"success"`
	// Applied is true when the changes reached the workspace; otherwise they exist
	// only in an isolated worktree (or not at all) and must not be assumed present.
	Applied bool   `json:"applied,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Memory represents persistent working context across multiple turns.
type Memory struct {
	TouchedFiles       []string          `json:"touched_files"`
	DiscoveredInsights []string          `json:"discovered_insights,omitempty"`
	LastPlanID         string            `json:"last_plan_id,omitempty"`
	LastWorktreeBranch string            `json:"last_worktree_branch,omitempty"`
	LastWorktreePath   string            `json:"last_worktree_path,omitempty"`
	FileCheckpoints    map[string]string `json:"file_checkpoints,omitempty"` // path -> base sha256
}

// Session encapsulates stateful interactive operation inside APW.
type Session struct {
	ID        string    `json:"id"`
	Workspace string    `json:"workspace"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	History   []Turn    `json:"history"`
	Memory    Memory    `json:"memory"`

	mu sync.RWMutex
}

// New creates a fresh stateful Session.
func New(workspace string) *Session {
	if workspace == "" {
		workspace = "."
	}
	now := time.Now()
	id := "sess-" + rand.Text()
	return &Session{
		ID:        id,
		Workspace: workspace,
		CreatedAt: now,
		UpdatedAt: now,
		Memory: Memory{
			FileCheckpoints: make(map[string]string),
		},
	}
}

// RecordTurn appends an execution cycle to session history and updates working memory.
func (s *Session) RecordTurn(t Turn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t.Index = s.nextIndex()
	if t.Timestamp.IsZero() {
		t.Timestamp = time.Now()
	}
	s.History = append(s.History, t)
	if len(s.History) > maxHistoryTurns {
		// Copy the tail so the old, larger backing array can be collected.
		s.History = append([]Turn(nil), s.History[len(s.History)-maxHistoryTurns:]...)
	}
	s.UpdatedAt = time.Now()

	// Only applied changes exist in the workspace; unapplied or failed work must not
	// be presented to the next plan as files it can build on.
	if !t.Applied {
		if t.PlanID != "" {
			s.Memory.LastPlanID = t.PlanID
		}
		return
	}
	// Update touched files in memory (unique preservation)
	existing := make(map[string]bool)
	for _, f := range s.Memory.TouchedFiles {
		existing[f] = true
	}
	for _, f := range t.ModifiedFiles {
		if !existing[f] && f != "" {
			existing[f] = true
			s.Memory.TouchedFiles = append(s.Memory.TouchedFiles, f)
		}
	}

	if t.PlanID != "" {
		s.Memory.LastPlanID = t.PlanID
	}
}

// nextIndex returns the index the next recorded turn will receive. Caller must hold s.mu.
func (s *Session) nextIndex() int {
	if n := len(s.History); n > 0 {
		return s.History[n-1].Index + 1
	}
	return 1
}

// SetWorktreeCheckpoint registers an active or interrupted worktree for resume capabilities.
func (s *Session) SetWorktreeCheckpoint(branch, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Memory.LastWorktreeBranch = branch
	s.Memory.LastWorktreePath = path
	s.UpdatedAt = time.Now()
}

// ClearWorktreeCheckpoint clears the worktree checkpoint once verified and applied.
func (s *Session) ClearWorktreeCheckpoint() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Memory.LastWorktreeBranch = ""
	s.Memory.LastWorktreePath = ""
	s.UpdatedAt = time.Now()
}

// BuildContextSummary synthesizes a rich, token-efficient memory prompt for the Architect.
func (s *Session) BuildContextSummary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.History) == 0 && len(s.Memory.TouchedFiles) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## Session Working Context (Continuous Interactive Memory)\n")
	sb.WriteString(fmt.Sprintf("Session ID: %s (Turn #%d)\n", s.ID, s.nextIndex()))

	if len(s.Memory.TouchedFiles) > 0 {
		sb.WriteString("Workspace files changed by applied tasks in this session:\n")
		for _, f := range s.Memory.TouchedFiles {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}

	if s.Memory.LastWorktreeBranch != "" {
		sb.WriteString(fmt.Sprintf("Active Worktree Checkpoint: branch %s (at %s)\n", s.Memory.LastWorktreeBranch, s.Memory.LastWorktreePath))
	}

	// Include up to 3 recent turns for high-signal continuity
	recentCount := 3
	if len(s.History) < recentCount {
		recentCount = len(s.History)
	}
	startIdx := len(s.History) - recentCount

	sb.WriteString("\nRecent session conversation history:\n")
	for i := startIdx; i < len(s.History); i++ {
		turn := s.History[i]
		statusStr := "SUCCESS"
		switch {
		case !turn.Success && len(turn.ModifiedFiles) > 0 && !turn.Applied:
			statusStr = fmt.Sprintf("INTERRUPTED/FAILED, NOT APPLIED (workspace unchanged): %s", turn.Error)
		case !turn.Success:
			statusStr = fmt.Sprintf("INTERRUPTED/FAILED: %s", turn.Error)
		case turn.Applied:
			statusStr = "SUCCESS, APPLIED TO WORKSPACE"
		case len(turn.ModifiedFiles) > 0:
			statusStr = "SUCCESS, NOT APPLIED (changes exist only in an isolated worktree; workspace unchanged)"
		}
		sb.WriteString(fmt.Sprintf("Turn #%d [%s]:\n", turn.Index, statusStr))
		sb.WriteString(fmt.Sprintf("  User Request : %s\n", turn.Instruction))
		if turn.Summary != "" {
			lines := strings.Split(strings.TrimSpace(turn.Summary), "\n")
			firstFew := lines
			if len(firstFew) > 3 {
				firstFew = lines[:3]
			}
			sb.WriteString(fmt.Sprintf("  Outcome Summary: %s\n", strings.Join(firstFew, " ")))
		}
	}
	sb.WriteString("Guidance: Build upon the working memory and touched files above rather than restarting from zero.\n")

	return sb.String()
}

// Save persists the session state to `.apw/sessions/<id>.json`.
func (s *Session) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !validID(s.ID) {
		return fmt.Errorf("invalid session ID")
	}

	dir := filepath.Join(s.Workspace, ".apw", "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}

	path := filepath.Join(dir, fmt.Sprintf("%s.json", s.ID))
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	return surgeon.WriteFileAtomic(path, data, 0600)
}

// Load loads an existing session from disk.
func Load(workspace, sessionID string) (*Session, error) {
	if !validID(sessionID) {
		return nil, fmt.Errorf("invalid session ID")
	}
	path := filepath.Join(workspace, ".apw", "sessions", fmt.Sprintf("%s.json", sessionID))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session file: %w", err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	// Bind metadata to the path the caller selected, including after a repository move.
	sess.ID = sessionID
	sess.Workspace = workspace
	if sess.Memory.FileCheckpoints == nil {
		sess.Memory.FileCheckpoints = make(map[string]string)
	}
	return &sess, nil
}

func validID(id string) bool {
	return id != "" && id != "." && filepath.IsLocal(id) && filepath.Base(id) == id
}

// LoadLatest finds and loads the most recently updated session in the workspace.
func LoadLatest(workspace string) (*Session, error) {
	dir := filepath.Join(workspace, ".apw", "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var latestFile string
	var latestTime time.Time

	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			info, err := e.Info()
			if err == nil && info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestFile = e.Name()
			}
		}
	}

	if latestFile == "" {
		return nil, os.ErrNotExist
	}

	id := strings.TrimSuffix(latestFile, ".json")
	return Load(workspace, id)
}
