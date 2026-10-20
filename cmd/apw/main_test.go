package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/daemon"
)

func TestRun_InteractiveBannerAndCommands(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout, stderr bytes.Buffer
	code := runWithStdin(nil, strings.NewReader("status\nexit\n"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"▄▀█ █▀█ █░█░█",
		"Brain 1 (Reflex) : Reflex Engine [Available",
		"Brain 2 (Deep)   : Architect     [Standby]",
		"Daemon (apwd)    : Stopped (PID:",
		"Skills Loaded",
		"Workspace        :",
		"apw > ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("interactive output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestRun_PlanWithMockConfig(t *testing.T) {
	wsDir := t.TempDir()

	sampleGo := "package sample\n\ntype Service struct{}\n\nfunc (s *Service) Start() {}\n"
	if err := os.WriteFile(filepath.Join(wsDir, "sample.go"), []byte(sampleGo), 0644); err != nil {
		t.Fatal(err)
	}

	cfgContent := `{
		"workspace": ` + mustQuote(wsDir) + `,
		"providers": {
			"mock-arch": {
				"type": "mock",
				"model": "mock-v1"
			}
		},
		"roles": {
			"architect": "mock-arch",
			"reflex": "mock-arch"
		}
	}`
	cfgPath := filepath.Join(wsDir, "apw.json")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

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
	for !daemon.Status(wsDir).Running {
		if time.Now().After(deadline) {
			t.Fatal("plan backend did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"plan", "--config", cfgPath, "Add healthcheck endpoint"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", code, stderr.String())
	}

	var printedPlan architect.ExecutionPlan
	if err := json.Unmarshal(stdout.Bytes(), &printedPlan); err != nil {
		t.Fatalf("failed to unmarshal stdout plan JSON: %v\nstdout: %s", err, stdout.String())
	}
	if printedPlan.PlanID == "" {
		t.Fatal("expected non-empty PlanID in output")
	}

	savedPath := filepath.Join(wsDir, ".apw", "plans", printedPlan.PlanID+".json")
	savedBytes, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("expected saved plan file at %s, got error: %v", savedPath, err)
	}

	var savedPlan architect.ExecutionPlan
	if err := json.Unmarshal(savedBytes, &savedPlan); err != nil {
		t.Fatalf("failed to unmarshal saved plan file: %v", err)
	}
	if savedPlan.PlanID != printedPlan.PlanID {
		t.Errorf("saved PlanID = %q, want %q", savedPlan.PlanID, printedPlan.PlanID)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"nonexistent-cmd"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit code 2 for unknown command, got %d", code)
	}
}

func TestFindSiblingNativeDesktop(t *testing.T) {
	dir := t.TempDir()
	apwExe := filepath.Join(dir, "apw.exe")
	if err := os.WriteFile(apwExe, []byte("cli"), 0o600); err != nil {
		t.Fatal(err)
	}
	// On case-insensitive Windows filesystems, apw.exe must never match itself as APW.exe.
	if _, ok := findSiblingNativeDesktop("windows", apwExe); ok {
		t.Fatal("expected false when native desktop binary is absent")
	}
	nativeExe := filepath.Join(dir, "apw-desktop.exe")
	if err := os.WriteFile(nativeExe, []byte("gui"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := findSiblingNativeDesktop("windows", apwExe)
	if !ok || got != nativeExe {
		t.Fatalf("findSiblingNativeDesktop = %q, %v; want %q, true", got, ok, nativeExe)
	}
	// Also supports parent ../APW.exe layout (e.g. $INSTDIR/bin/apw.exe -> $INSTDIR/APW.exe).
	instDir := t.TempDir()
	binDir := filepath.Join(instDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binApw := filepath.Join(binDir, "apw.exe")
	parentAPW := filepath.Join(instDir, "APW.exe")
	if err := os.WriteFile(binApw, []byte("cli"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parentAPW, []byte("gui"), 0o600); err != nil {
		t.Fatal(err)
	}
	gotParent, ok := findSiblingNativeDesktop("windows", binApw)
	if !ok || gotParent != parentAPW {
		t.Fatalf("findSiblingNativeDesktop(bin/apw.exe) = %q, %v; want %q, true", gotParent, ok, parentAPW)
	}
	if _, ok := findSiblingNativeDesktop("linux", apwExe); ok {
		t.Fatal("expected false on non-windows GOOS")
	}
}

func mustQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
