package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/core"
)

func TestStore_WALAndConcurrentWrites(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tasks.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	mode, err := st.JournalMode(ctx)
	if err != nil {
		t.Fatalf("JournalMode failed: %v", err)
	}
	if strings.ToLower(mode) != "wal" {
		t.Errorf("expected journal_mode=wal, got %q", mode)
	}

	// Have 2 goroutines write 25 tasks concurrently
	const (
		workers       = 2
		tasksPerGoro  = 25
		expectedTotal = workers * tasksPerGoro
	)

	var wg sync.WaitGroup
	errCh := make(chan error, expectedTotal)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < tasksPerGoro; i++ {
				taskID := fmt.Sprintf("task-w%d-%d", workerID, i)
				err := st.SaveTask(ctx, core.Task{
					ID:          taskID,
					Workspace:   ".",
					Instruction: "test concurrent write",
					Mode:        "preview",
					Status:      core.StatusQueued,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				})
				if err != nil {
					errCh <- fmt.Errorf("worker %d iter %d: %w", workerID, i, err)
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent write error: %v", err)
	}

	count, err := st.CountTasks(ctx)
	if err != nil {
		t.Fatalf("CountTasks failed: %v", err)
	}
	if count != expectedTotal {
		t.Errorf("CountTasks = %d, want %d", count, expectedTotal)
	}

	got, err := st.GetTask(ctx, "task-w0-0")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got.Status != core.StatusQueued {
		t.Errorf("got Status = %q, want %q", got.Status, core.StatusQueued)
	}
}
