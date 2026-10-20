package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A descendant that keeps the parent's inherited output pipe open must not turn the parent's
// normal exit into exec.ErrWaitDelay: Run kills the rest of the tree as soon as the parent exits.
func TestRunKillsPipeHoldingDescendantOnParentExit(t *testing.T) {
	heartbeat := filepath.Join(t.TempDir(), "heartbeat")
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-test.run=^TestProcessFixture$")
	cmd.Env = append(os.Environ(), "APW_PROCESS_FIXTURE=orphan", "APW_HEARTBEAT="+heartbeat)
	var output Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	start := time.Now()
	if err := Run(ctx, cmd); err != nil {
		t.Fatalf("normal parent exit reported %v after %v: %s", err, time.Since(start), output.String())
	}
	before, _ := os.ReadFile(heartbeat)
	time.Sleep(100 * time.Millisecond)
	after, _ := os.ReadFile(heartbeat)
	if len(before) == 0 || string(before) != string(after) {
		t.Fatal("descendant survived the runner")
	}
}
