package session

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSessionLoadBindsMetadataToSelectedWorkspace(t *testing.T) {
	originalRoot, newRoot := t.TempDir(), t.TempDir()
	original := New(originalRoot)
	if err := original.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(originalRoot, ".apw", "sessions", original.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(newRoot, ".apw", "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "selected.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(newRoot, "selected")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != "selected" || loaded.Workspace != newRoot {
		t.Fatal("stored metadata redirected the selected conversation")
	}
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(newRoot, "../outside"); err == nil {
		t.Fatal("traversal session ID accepted")
	}
}

func TestSession_RecordTurnAndMemory(t *testing.T) {
	ws := t.TempDir()
	sess := New(ws)

	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}

	// Turn 1
	sess.RecordTurn(Turn{
		Instruction:   "Create user service",
		PlanID:        "plan-1",
		TotalSteps:    2,
		ModifiedFiles: []string{"user.go", "user_test.go"},
		Summary:       "Implemented User struct and CRUD handlers",
		Success:       true,
		Applied:       true,
	})

	if len(sess.History) != 1 {
		t.Fatalf("expected 1 turn in history, got %d", len(sess.History))
	}
	if len(sess.Memory.TouchedFiles) != 2 {
		t.Fatalf("expected 2 touched files, got %d", len(sess.Memory.TouchedFiles))
	}

	// Turn 2: modified same file plus a new file
	sess.RecordTurn(Turn{
		Instruction:   "Add validation to user",
		PlanID:        "plan-2",
		TotalSteps:    1,
		ModifiedFiles: []string{"user.go", "validator.go"},
		Summary:       "Added email validation",
		Success:       true,
		Applied:       true,
	})

	if len(sess.History) != 2 {
		t.Fatalf("expected 2 turns in history, got %d", len(sess.History))
	}
	// Total unique touched files should be 3: user.go, user_test.go, validator.go
	if len(sess.Memory.TouchedFiles) != 3 {
		t.Fatalf("expected 3 unique touched files, got %v", sess.Memory.TouchedFiles)
	}

	summary := sess.BuildContextSummary()
	if !strings.Contains(summary, "user.go") || !strings.Contains(summary, "validator.go") {
		t.Errorf("expected context summary to contain touched files, got:\n%s", summary)
	}
	if !strings.Contains(summary, "Add validation to user") {
		t.Errorf("expected context summary to contain recent instruction, got:\n%s", summary)
	}
}

func TestSession_SaveAndLoad(t *testing.T) {
	ws := t.TempDir()
	sess := New(ws)
	sess.RecordTurn(Turn{
		Instruction:   "Fix bug in auth",
		ModifiedFiles: []string{"auth.go"},
		Success:       true,
		Summary:       "Fixed token validation",
	})
	sess.SetWorktreeCheckpoint("apw/task-42", filepath.Join(ws, ".worktrees", "task-42"))

	if err := sess.Save(); err != nil {
		t.Fatalf("Save session failed: %v", err)
	}

	loaded, err := Load(ws, sess.ID)
	if err != nil {
		t.Fatalf("Load session failed: %v", err)
	}

	if loaded.ID != sess.ID {
		t.Errorf("expected loaded session ID %q, got %q", sess.ID, loaded.ID)
	}
	if len(loaded.History) != 1 {
		t.Fatalf("expected 1 history turn, got %d", len(loaded.History))
	}
	if loaded.Memory.LastWorktreeBranch != "apw/task-42" {
		t.Errorf("expected worktree branch apw/task-42, got %q", loaded.Memory.LastWorktreeBranch)
	}

	// Test LoadLatest
	time.Sleep(10 * time.Millisecond)
	latest, err := LoadLatest(ws)
	if err != nil {
		t.Fatalf("LoadLatest failed: %v", err)
	}
	if latest.ID != sess.ID {
		t.Errorf("expected latest ID %q, got %q", sess.ID, latest.ID)
	}
}

func TestRecordTurnTrimsHistoryAndKeepsIndexesIncreasing(t *testing.T) {
	s := New(t.TempDir())
	total := maxHistoryTurns + 25
	for i := 1; i <= total; i++ {
		s.RecordTurn(Turn{Instruction: "task", Success: true})
	}
	if len(s.History) != maxHistoryTurns {
		t.Fatalf("history len=%d want %d", len(s.History), maxHistoryTurns)
	}
	if first := s.History[0].Index; first != total-maxHistoryTurns+1 {
		t.Fatalf("first retained index=%d", first)
	}
	for i := 1; i < len(s.History); i++ {
		if s.History[i].Index != s.History[i-1].Index+1 {
			t.Fatalf("indexes not consecutive at %d", i)
		}
	}
	if last := s.History[len(s.History)-1].Index; last != total {
		t.Fatalf("last index=%d want %d", last, total)
	}
	if !strings.Contains(s.BuildContextSummary(), "Turn #"+strconv.Itoa(total+1)) {
		t.Fatal("context summary should report the next monotonic turn number")
	}
}

func TestRecordTurnFirstIndexIsOne(t *testing.T) {
	s := New(t.TempDir())
	s.RecordTurn(Turn{Instruction: "a"})
	if s.History[0].Index != 1 {
		t.Fatalf("index=%d", s.History[0].Index)
	}
}

// Regression: a failed, never-applied task was listed as "files modified in this
// session", so the next plan assumed src/clamp.ts existed in the workspace.
func TestUnappliedTurnsDoNotClaimWorkspaceFiles(t *testing.T) {
	s := New(t.TempDir())
	s.RecordTurn(Turn{Instruction: "add clamp", ModifiedFiles: []string{"src/clamp.ts"}, Success: false, Error: "unplanned worktree change"})
	s.RecordTurn(Turn{Instruction: "preview tweak", ModifiedFiles: []string{"src/App.tsx"}, Success: true})
	if len(s.Memory.TouchedFiles) != 0 {
		t.Fatalf("unapplied work must not be recorded as workspace files: %v", s.Memory.TouchedFiles)
	}
	summary := s.BuildContextSummary()
	if !strings.Contains(summary, "FAILED, NOT APPLIED (workspace unchanged)") || !strings.Contains(summary, "SUCCESS, NOT APPLIED") {
		t.Fatalf("summary must say the changes were not applied:\n%s", summary)
	}
	s.RecordTurn(Turn{Instruction: "apply", ModifiedFiles: []string{"src/App.tsx"}, Success: true, Applied: true})
	if len(s.Memory.TouchedFiles) != 1 || !strings.Contains(s.BuildContextSummary(), "SUCCESS, APPLIED TO WORKSPACE") {
		t.Fatalf("applied work must be recorded: %v", s.Memory.TouchedFiles)
	}
}
