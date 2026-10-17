package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/store"
	"github.com/apiwat/apw-agent/internal/workspace"
)

func TestCleanupDryRunAndServiceLease(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "apw.json")
	if err := os.WriteFile(cfgPath, []byte(`{"providers":{"m":{"type":"mock"}},"roles":{"architect":"m","reflex":"m"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"cleanup", "--config", cfgPath, "--dry-run"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "nothing to clean up") {
		t.Fatalf("empty workspace: %d %s %s", code, stdout.String(), stderr.String())
	}
	if err := os.MkdirAll(filepath.Join(root, ".apw"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(root, ".apw", "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, _, err = db.CreateJob(ctx, store.Job{Task: core.Task{ID: "task-r", Workspace: root, Instruction: "x", Mode: "review"}, Owner: "local", RequestID: "r"}); err != nil {
		t.Fatal(err)
	}
	if err = db.SetJob(ctx, "task-r", core.StatusReady, nil, ""); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// A dry run must leave the database file byte-for-byte untouched (no migrations or checkpoints).
	dbFile := filepath.Join(root, ".apw", "tasks.db")
	before, err := os.ReadFile(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if code := run([]string{"cleanup", "--config", cfgPath, "--dry-run"}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run failed: %d %s", code, stderr.String())
	}
	after, err := os.ReadFile(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("cleanup --dry-run modified tasks.db")
	}

	lease, err := workspace.Acquire(root, "service")
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"cleanup", "--config", cfgPath}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "apw daemon stop") {
		t.Fatalf("cleanup ran while the service lease was held: %d %s", code, stderr.String())
	}
	stdout.Reset()
	if code := run([]string{"cleanup", "--config", cfgPath, "--dry-run"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "Dry run") || !strings.Contains(stdout.String(), "task-r [ready_to_apply]: resumable") {
		t.Fatalf("dry run while daemon owns workspace: %d %s %s", code, stdout.String(), stderr.String())
	}
	lease.Close()
	stdout.Reset()
	if code := run([]string{"cleanup", "--config", cfgPath, "--json"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), `"dry_run": false`) {
		t.Fatalf("cleanup: %d %s %s", code, stdout.String(), stderr.String())
	}
}
