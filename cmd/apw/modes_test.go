package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/daemon"
)

// modeWorkspace creates an untrusted Git workspace with a mock-provider config.
func modeWorkspace(t *testing.T) (string, string) {
	t.Helper()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init"}, {"config", "user.name", "T"}, {"config", "user.email", "t@e"}, {"config", "commit.gpgsign", "false"}, {"add", "."}, {"commit", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", ws}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	cfgPath := filepath.Join(ws, "apw.json")
	cfg := `{"workspace": ` + mustQuote(ws) + `, "providers": {"m": {"type": "mock", "model": "mock-v1"}}, "roles": {"architect": "m", "reflex": "m"}}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".git", "info", "exclude"), []byte("apw.json\n.apw/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return ws, cfgPath
}

func startDaemon(t *testing.T, ws, cfgPath string) {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- daemon.Run(ctx, cfgPath, "test") }()
	t.Cleanup(func() {
		stop()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for !daemon.Status(ws).Running {
		if time.Now().After(deadline) {
			t.Fatal("daemon did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func trusted(t *testing.T, cfgPath string) bool {
	t.Helper()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Policy.Trusted
}

func TestShellModeCommand(t *testing.T) {
	_, cfgPath := modeWorkspace(t)
	var stdout, stderr bytes.Buffer
	if code := runShellConfig(cfgPath, strings.NewReader("/mode\n/mode plan\n/mode nope\n/mode AUTO\nexit\n"), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Mode: ask (verify, then ask before applying)", "Available: ask, auto, plan, unverified", "Mode: plan (plan only", "Mode: auto (verify, then apply automatically)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(stderr.String(), "unknown mode: nope") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestShellAsksForTrustBeforeVerifiedRuns(t *testing.T) {
	_, cfgPath := modeWorkspace(t)
	var stdout, stderr bytes.Buffer
	runShellConfig(cfgPath, strings.NewReader("add a greeting\nn\nexit\n"), &stdout, &stderr)
	if !strings.Contains(stdout.String(), "Trust this folder? [y/N]: ") || !strings.Contains(stdout.String(), "Not trusted.") || trusted(t, cfgPath) {
		t.Fatalf("declined trust: trusted=%v\n%s", trusted(t, cfgPath), stdout.String())
	}
	if strings.Contains(stdout.String(), "Task task-") {
		t.Fatal("task submitted without trust")
	}
	stdout.Reset()
	runShellConfig(cfgPath, strings.NewReader("add a greeting\ny\nexit\n"), &stdout, &stderr)
	if !trusted(t, cfgPath) || !strings.Contains(stdout.String(), "Trusted workspace for build/test") {
		t.Fatalf("accepted trust not saved:\n%s", stdout.String())
	}
	// Unverified and plan modes never ask for trust.
	_, other := modeWorkspace(t)
	stdout.Reset()
	runShellConfig(other, strings.NewReader("/mode unverified\nadd a greeting\n/mode plan\nadd a greeting\nexit\n"), &stdout, &stderr)
	if strings.Contains(stdout.String(), "Trust this folder") {
		t.Fatalf("trust asked in unverified/plan mode:\n%s", stdout.String())
	}
}

func TestRunNoVerifyAutoAppliesWithoutTrust(t *testing.T) {
	ws, cfgPath := modeWorkspace(t)
	startDaemon(t, ws, cfgPath)
	var stdout, stderr bytes.Buffer
	code := run([]string{"run", "--config", cfgPath, "--no-verify", "--auto", "check the project"}, &stdout, &stderr)
	out := stdout.String()
	if code != 0 || !strings.Contains(out, "(unverified)") || !strings.Contains(out, "UNVERIFIED") || !strings.Contains(out, "Applying automatically") || !strings.Contains(out, "Status: succeeded") {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, out, stderr.String())
	}
}

func TestShellApprovalPrompt(t *testing.T) {
	ws, cfgPath := modeWorkspace(t)
	startDaemon(t, ws, cfgPath)
	var stdout, stderr bytes.Buffer
	runShellConfig(cfgPath, strings.NewReader("/mode unverified\ncheck the project\nn\nexit\n"), &stdout, &stderr)
	out := stdout.String()
	if !strings.Contains(out, "Apply these UNVERIFIED changes (no build or test ran)") || !strings.Contains(out, "Not applied. Apply later with 'apw apply task-") || strings.Contains(out, "Status: succeeded") {
		t.Fatalf("declined approval:\n%s\nstderr:\n%s", out, stderr.String())
	}
	stdout.Reset()
	runShellConfig(cfgPath, strings.NewReader("/mode unverified\ncheck the project again\ny\nexit\n"), &stdout, &stderr)
	if out := stdout.String(); !strings.Contains(out, "Status: succeeded") {
		t.Fatalf("approved task not applied:\n%s\nstderr:\n%s", out, stderr.String())
	}
}

func TestShellPlanModeOnlyPlans(t *testing.T) {
	ws, cfgPath := modeWorkspace(t)
	startDaemon(t, ws, cfgPath)
	var stdout, stderr bytes.Buffer
	runShellConfig(cfgPath, strings.NewReader("/mode plan\nadd a healthcheck\nexit\n"), &stdout, &stderr)
	if out := stdout.String(); !strings.Contains(out, `"plan_id"`) || strings.Contains(out, "Task task-") {
		t.Fatalf("plan mode output:\n%s\nstderr:\n%s", out, stderr.String())
	}
}
