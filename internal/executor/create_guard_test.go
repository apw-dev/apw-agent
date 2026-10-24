package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/provider/mock"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

func TestCreateOnExistingFileShowsAndReplacesCurrentContent(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "notes.md"), []byte("# Notes\nkeep this line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mp := mock.New(`{"file_content":"# Notes\nkeep this line\nnew line\n"}`)
	ex := New(ws, mp, mp, "arch", "reflex", 1)
	st := architect.Subtask{ID: "c1", Action: architect.ActionCreate, TargetFile: "notes.md", Instruction: "Add a new line"}
	if _, _, err := ex.executeSubtask(context.Background(), ws, surgeon.NewSurgeon(), st); err != nil {
		t.Fatal(err)
	}
	if len(mp.Requests) != 1 || !strings.Contains(mp.Requests[0].Prompt, "keep this line") || !strings.Contains(mp.Requests[0].Prompt, "already exists") {
		t.Fatalf("create prompt did not show the existing file: %+v", mp.Requests)
	}
	got, err := os.ReadFile(filepath.Join(ws, "notes.md"))
	if err != nil || string(got) != "# Notes\nkeep this line\nnew line\n" {
		t.Fatalf("file = %q, %v", got, err)
	}
}

func TestWriteCreatedRefusesUnreadOrChangedFiles(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "a.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ex := New(ws, nil, nil, "", "", 1)

	// Existing file that the model was never shown.
	if err := ex.writeCreated(ws, "a.txt", "replaced\n", ""); !errors.Is(err, surgeon.ErrContentDrift) {
		t.Fatalf("unread existing file: err = %v", err)
	}
	// File changed after it was shown.
	shown := surgeon.CalculateHash("original\n")
	if err := os.WriteFile(path, []byte("edited elsewhere\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ex.writeCreated(ws, "a.txt", "replaced\n", shown); !errors.Is(err, surgeon.ErrContentDrift) {
		t.Fatalf("changed file: err = %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "edited elsewhere\n" {
		t.Fatalf("refused write still changed the file: %q", got)
	}
	// File removed after it was shown.
	if err := ex.writeCreated(ws, "gone.txt", "x\n", shown); !errors.Is(err, surgeon.ErrContentDrift) {
		t.Fatalf("vanished file: err = %v", err)
	}
	// New file.
	if err := ex.writeCreated(ws, "new.txt", "fresh\n", ""); err != nil {
		t.Fatal(err)
	}
}
