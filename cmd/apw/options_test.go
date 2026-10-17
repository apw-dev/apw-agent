package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/store"
)

func TestParseOptionsLiteralInstructionAndStableRequest(t *testing.T) {
	var stderr bytes.Buffer
	o, code := parseOptions("review", []string{"--request-id", "retry-1", "--session=s1", "--", "-literal", "instruction"}, &stderr)
	if code != 0 || o.RequestID != "retry-1" || o.SessionID != "s1" || strings.Join(o.Args, " ") != "-literal instruction" {
		t.Fatalf("options=%+v code=%d %s", o, code, stderr.String())
	}
	for _, args := range [][]string{{"--config"}, {"--config="}, {"--unknown", "x"}, {"--config", filepath.Join(t.TempDir(), "missing.json")}} {
		if _, code := parseOptions("review", args, &stderr); code == 0 {
			t.Errorf("invalid flags accepted: %v", args)
		}
	}
}

func TestSplitCommandPreservesQuotedWindowsPaths(t *testing.T) {
	args, err := splitCommand(`--config "D:\My Project\apw.json" task-1`)
	if err != nil || len(args) != 3 || args[1] != `D:\My Project\apw.json` {
		t.Fatalf("%v %v", args, err)
	}
	if _, err := splitCommand(`--config "unfinished`); err == nil {
		t.Fatal("unclosed quote accepted")
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("broken input") }

func TestShellEmptySlashAndInputErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	var out, stderr bytes.Buffer
	if code := runWithStdin(nil, strings.NewReader("/\n / \nexit\n"), &out, &stderr); code != 0 {
		t.Fatalf("empty slash: %d %s", code, stderr.String())
	}
	if code := runWithStdin(nil, brokenReader{}, &out, &stderr); code == 0 || !strings.Contains(stderr.String(), "broken input") {
		t.Fatal("input failure was reported as clean EOF")
	}
}

func TestInvalidConfigCannotBeReplacedByModelCommands(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bad.json")
	original := []byte("{broken config")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"model", "auth", "plan", "trust", "review", "shell", "daemon"} {
		var out, stderr bytes.Buffer
		args := []string{command, "--config", path}
		if command == "plan" || command == "review" {
			args = append(args, "inspect")
		}
		if code := runWithStdin(args, strings.NewReader("exit\n"), &out, &stderr); code == 0 {
			t.Errorf("%s accepted invalid config", command)
		}
		actual, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(actual, original) {
			t.Fatalf("%s overwrote invalid config", command)
		}
	}
}

type interruptedBackend struct {
	app.API
	cancelled bool
}

func (b *interruptedBackend) Events(ctx context.Context, _, _ string, _ int64) ([]store.Event, error) {
	return nil, ctx.Err()
}
func (b *interruptedBackend) Cancel(context.Context, string, string) error {
	b.cancelled = true
	return nil
}

func TestWatchDetachesWhileRunCancels(t *testing.T) {
	for _, cancelOnInterrupt := range []bool{false, true} {
		backend := &interruptedBackend{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		job := &store.Job{}
		job.ID = "task-interrupted"
		if code := followTaskWithCancel(ctx, backend, job, io.Discard, io.Discard, cancelOnInterrupt); code != 130 {
			t.Fatalf("exit=%d", code)
		}
		if backend.cancelled != cancelOnInterrupt {
			t.Fatal("watch and run have incorrect cancellation behavior")
		}
	}
}
