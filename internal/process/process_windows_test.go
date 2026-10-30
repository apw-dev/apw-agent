package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestHideWindowSetsCreateNoWindow(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo", "ok")
	HideWindow(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
		t.Fatal("expected HideWindow=true on SysProcAttr")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatal("expected CREATE_NO_WINDOW flag on SysProcAttr.CreationFlags")
	}
}

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
