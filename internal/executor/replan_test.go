package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/surgeon"
)

func TestExtractDiagnosticFilesAndContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "svc"), 0o755); err != nil {
		t.Fatal(err)
	}
	aSrc := `package svc

func ComputeValue() int {
	return 42
}
`
	bSrc := `package svc

func RunService() int {
	return ComputeValue()
}
`
	if err := os.WriteFile(filepath.Join(dir, "internal", "svc", "a.go"), []byte(aSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "svc", "b.go"), []byte(bSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	diag := "internal/svc/b.go:4:9: not enough arguments in call to ComputeValue\n./internal/svc/a.go:3:6: ComputeValue defined here"
	files := extractDiagnosticFiles(dir, diag)
	if len(files) != 2 {
		t.Fatalf("expected 2 diagnostic files, got %v", files)
	}
	if files[0] != "internal/svc/b.go" || files[1] != "internal/svc/a.go" {
		t.Fatalf("unexpected diagnostic files order: %v", files)
	}

	surg := surgeon.NewSurgeon()
	ctxText := buildDiagnosticContext(dir, surg, "internal/svc/a.go", files, []string{"internal/svc/a.go"})
	if !strings.Contains(ctxText, "Referenced diagnostic file internal/svc/b.go:") {
		t.Fatalf("expected diagnostic context to include b.go contents, got:\n%s", ctxText)
	}
	if !strings.Contains(ctxText, "RunService") {
		t.Fatalf("expected cross-file impact or diagnostic source to mention RunService, got:\n%s", ctxText)
	}
}
