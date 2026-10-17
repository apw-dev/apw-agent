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

// Memory compaction keeps the prompt bounded however long a session runs: the last recentTurns
// turns are shown in detail, older ones as one-line digests (within digestBudget bytes), turns that
// leave History survive as digests in Memory.Earlier (at most maxEarlier), and only the newest
// maxPromptFiles touched files are listed. Compaction is deterministic; it never asks a model.
const (
	recentTurns    = 3
	digestBudget   = 3 * 1024
	maxEarlier     = 30
	maxPromptFiles = 40
)

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
	// Earlier holds one-line digests of turns that no longer fit History, oldest first.
	Earlier []string `json:"earlier,omitempty"`
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
		evicted := s.History[:len(s.History)-maxHistoryTurns]
		for _, old := range evicted {
			s.Memory.Earlier = append(s.Memory.Earlier, digest(old))
		}
		if n := len(s.Memory.Earlier); n > maxEarlier {
			s.Memory.Earlier = append([]string(nil), s.Memory.Earlier[n-maxEarlier:]...)
		}
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

	if files := s.Memory.TouchedFiles; len(files) > 0 {
		sb.WriteString("Workspace files changed by applied tasks in this session:\n")
		if len(files) > maxPromptFiles {
			sb.WriteString(fmt.Sprintf("  (%d earlier files omitted; newest shown)\n", len(files)-maxPromptFiles))
			files = files[len(files)-maxPromptFiles:]
		}
		for _, f := range files {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}

	if s.Memory.LastWorktreeBranch != "" {
		sb.WriteString(fmt.Sprintf("Active Worktree Checkpoint: branch %s (at %s)\n", s.Memory.LastWorktreeBranch, s.Memory.LastWorktreePath))
	}

	// Show the most recent turns in detail and older ones as compact digests.
	recentCount := recentTurns
	if len(s.History) < recentCount {
		recentCount = len(s.History)
	}
	startIdx := len(s.History) - recentCount
	var older []string
	older = append(older, s.Memory.Earlier...)
	for _, turn := range s.History[:startIdx] {
		older = append(older, digest(turn))
	}
	if len(older) > 0 {
		// Keep the newest digests that fit the budget, then print them oldest first.
		kept, size := 0, 0
		for i := len(older) - 1; i >= 0 && size+len(older[i])+1 <= digestBudget; i-- {
			size += len(older[i]) + 1
			kept++
		}
		sb.WriteString("\nEarlier in this session (compacted):\n")
		if skipped := len(older) - kept; skipped > 0 {
			sb.WriteString(fmt.Sprintf("  (%d older turns omitted)\n", skipped))
		}
		for _, line := range older[len(older)-kept:] {
			sb.WriteString("  " + line + "\n")
		}
	}

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

// digest compresses a turn into one bounded line: index, outcome, request and changed files.
func digest(t Turn) string {
	status := "failed"
	switch {
	case t.Applied:
		status = "applied"
	case t.Success && len(t.ModifiedFiles) > 0:
		status = "not applied"
	case t.Success:
		status = "ok"
	}
	line := fmt.Sprintf("#%d [%s] %s", t.Index, status, clip(strings.Join(strings.Fields(t.Instruction), " "), 120))
	if len(t.ModifiedFiles) > 0 {
		files := t.ModifiedFiles
		more := ""
		if len(files) > 4 {
			more = fmt.Sprintf(" +%d", len(files)-4)
			files = files[:4]
		}
		line += " (files: " + strings.Join(files, ", ") + more + ")"
	}
	return line
}

// clip shortens s to at most n runes.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
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
