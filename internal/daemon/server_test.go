package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/buildinfo"
	"github.com/apiwat/apw-agent/internal/config"
)

func TestPackagedClientRejectsMismatchedDaemonBuild(t *testing.T) {
	previous := buildinfo.SourceHash
	buildinfo.SourceHash = "expected-build"
	t.Cleanup(func() { buildinfo.SourceHash = previous })
	if err := compatibleBuild(State{Build: buildinfo.Current()}); err != nil {
		t.Fatal(err)
	}
	if err := compatibleBuild(State{Build: buildinfo.Info{SourceHash: "old-build"}}); err == nil {
		t.Fatal("mismatched daemon build accepted")
	}
}

func TestDaemonHealthLifecycleAndStaleMetadata(t *testing.T) {
	t.Setenv("APW_TELEGRAM_TOKEN", "")
	t.Setenv("APW_TELEGRAM_ALLOW", "")
	root := t.TempDir()
	cfg := &config.Config{Workspace: root, Providers: map[string]config.ProviderConfig{"mock": {Type: config.ProviderMock}}}
	cfg.Roles.Architect = "mock"
	cfg.Roles.Reflex = "mock"
	path := filepath.Join(root, "apw.json")
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, path, "test") }()
	deadline := time.Now().Add(5 * time.Second)
	var state State
	for time.Now().Before(deadline) {
		state = Status(root)
		if state.Running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !state.Running || state.PID != os.Getpid() {
		t.Fatalf("daemon never became healthy: %+v", state)
	}
	otherPath := filepath.Join(root, "other.json")
	if err := cfg.Save(otherPath); err != nil {
		t.Fatal(err)
	}
	if _, err := Connect(context.Background(), otherPath); err == nil {
		t.Fatal("daemon silently used another config file")
	}
	if _, err := Connect(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if err := Stop(root); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not shut down")
	}
	if Status(root).Running {
		t.Fatal("stopped daemon reported running")
	}
	if _, err := os.Stat(stateFilePath(root)); !os.IsNotExist(err) {
		t.Fatal("graceful shutdown left stale metadata")
	}
	if err := os.MkdirAll(filepath.Join(root, ".apw"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFilePath(root), []byte(`{"pid":1,"running":true,"address":"http://127.0.0.1:1","token":"stale"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if Status(root).Running {
		t.Fatal("stale PID metadata was treated as live daemon")
	}
}
