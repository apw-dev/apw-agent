package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/apiwat/apw-agent/internal/architect"
)

func TestRun_PlanWithMockConfig(t *testing.T) {
	wsDir := t.TempDir()

	// Create a sample Go file in workspace to exercise buildCodebaseOverview
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

func mustQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
