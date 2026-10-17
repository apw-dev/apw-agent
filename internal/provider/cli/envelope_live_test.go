package cli

import (
	"context"
	"github.com/apiwat/apw-agent/internal/process"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLiveAGYEnvelope(t *testing.T) {
	if os.Getenv("APW_LIVE_TEST") != "1" || os.Getenv("APW_LIVE_ENVELOPE") != "1" {
		t.Skip("explicit envelope acceptance opt-in required")
	}
	model := os.Getenv("APW_LIVE_MODEL")
	if model == "" || os.Getenv("APW_LIVE_BACKEND") != "agy" {
		t.Fatal("explicit AGY model required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := New(BackendAGY, model, time.Minute).buildArgs(model, "", "")
	args[1] = "json"
	cmd := exec.CommandContext(ctx, "agy", args...)
	cmd.Dir = t.TempDir()
	cmd.Stdin = strings.NewReader("Do not use tools or read files. Return exactly APW_ENVELOPE_OK")
	var stdout, stderr process.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := process.Run(ctx, cmd); err != nil {
		t.Fatalf("envelope failed: %v %s", err, stderr.String())
	}
	if err := os.WriteFile(os.Getenv("APW_LIVE_REPORT"), []byte(stdout.String()), 0600); err != nil {
		t.Fatal(err)
	}
}
