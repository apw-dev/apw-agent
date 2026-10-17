package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestProcessFixture(t *testing.T) {
	role := os.Getenv("APW_PROCESS_FIXTURE")
	if role == "" {
		return
	}
	if role == "child" {
		for {
			_ = os.WriteFile(os.Getenv("APW_HEARTBEAT"), []byte(strconv.FormatInt(time.Now().UnixNano(), 10)), 0600)
			time.Sleep(10 * time.Millisecond)
		}
	}
	bin, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	child := exec.Command(bin, "-test.run=^TestProcessFixture$")
	child.Env = append(os.Environ(), "APW_PROCESS_FIXTURE=child")
	if child.Start() != nil {
		os.Exit(3)
	}
	for {
		if _, err := os.Stat(os.Getenv("APW_PARENT_EXIT")); err == nil {
			os.Exit(0)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRunnerKillsDescendantsOnCancelAndParentExit(t *testing.T) {
	for _, cancelParent := range []bool{true, false} {
		root := t.TempDir()
		heartbeat := filepath.Join(root, "heartbeat")
		exit := filepath.Join(root, "exit")
		bin, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := exec.CommandContext(ctx, bin, "-test.run=^TestProcessFixture$")
		cmd.Env = append(os.Environ(), "APW_PROCESS_FIXTURE=parent", "APW_HEARTBEAT="+heartbeat, "APW_PARENT_EXIT="+exit)
		var output Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		done := make(chan error, 1)
		go func() { done <- Run(ctx, cmd) }()
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(heartbeat); err == nil {
				break
			}
			if time.Now().After(deadline) {
				cancel()
				t.Fatalf("child did not start: %s", output.String())
			}
			time.Sleep(10 * time.Millisecond)
		}
		if cancelParent {
			cancel()
		} else if err = os.WriteFile(exit, nil, 0600); err != nil {
			cancel()
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if cancelParent && !errors.Is(err, context.Canceled) {
				t.Fatalf("wrong cancellation: %v %s", err, output.String())
			}
			if !cancelParent && err != nil {
				t.Fatalf("normal exit: %v %s", err, output.String())
			}
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("process tree did not terminate")
		}
		cancel()
		time.Sleep(100 * time.Millisecond)
		before, _ := os.ReadFile(heartbeat)
		time.Sleep(100 * time.Millisecond)
		after, _ := os.ReadFile(heartbeat)
		if string(before) != string(after) {
			t.Fatal("descendant survived the runner")
		}
	}
}

func TestBufferBoundsOutput(t *testing.T) {
	b := Buffer{Limit: 3}
	n, err := b.Write([]byte("abcdef"))
	if n != 6 || err != nil || b.String() != "abc\n[output truncated]" {
		t.Fatal("output buffer did not enforce its limit")
	}
}
