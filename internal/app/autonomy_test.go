package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/store"
)

// A long Thai failure cause is shortened into the episodic lesson without splitting a rune.
func TestRecordEpisodeLessonStaysValidUTF8(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Service{Store: db}
	j := &store.Job{Task: core.Task{ID: "task-th", Instruction: "แก้ไข", Mode: "execute"}, Owner: "local"}
	cause := strings.Repeat("คอมไพล์ล้มเหลว", 40) // > 400 bytes; byte cut at 400 splits a rune
	s.recordAutonomyAndMemory(j, core.StatusFailed, nil, cause)

	eps, err := db.Episodes(context.Background(), "local", 5)
	if err != nil || len(eps) != 1 {
		t.Fatalf("episodes = %+v, %v", eps, err)
	}
	if l := eps[0].Lesson; !utf8.ValidString(l) || !strings.HasSuffix(l, "...") {
		t.Fatalf("lesson not a valid shortened string: %q", l)
	}
}
