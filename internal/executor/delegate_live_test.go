package executor

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/provider/cli"
)

// TestLiveDelegate runs one delegated subagent against an explicitly selected real CLI model.
// Opt-in: APW_LIVE_TEST=1 APW_LIVE_BACKEND=agy|claude-cli|codex-cli APW_LIVE_MODEL=<model>.
func TestLiveDelegate(t *testing.T) {
	if os.Getenv("APW_LIVE_TEST") != "1" {
		t.Skip("set APW_LIVE_TEST=1 and explicitly select backend/model")
	}
	backend, model := cli.BackendType(os.Getenv("APW_LIVE_BACKEND")), os.Getenv("APW_LIVE_MODEL")
	if model == "" {
		t.Fatal("APW_LIVE_MODEL is required")
	}
	ws := delegateWorkspace(t)
	prov := cli.New(backend, model, 120*time.Second)
	e := New(ws, prov, prov, model, model, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	summary, turns, err := e.runDelegate(ctx, ws, delegateRequest{Role: "Port Tracer", Goal: "Which TCP port does the Serve function listen on, and in which file and line?"})
	t.Logf("turns=%d summary=%s", turns, summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(summary, "8080") {
		t.Fatalf("summary does not report the port: %s", summary)
	}
}
