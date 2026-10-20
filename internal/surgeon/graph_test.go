package surgeon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectWorkspaceGraphAndImpact(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "pkg", "auth"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pkg", "api"), 0o755); err != nil {
		t.Fatal(err)
	}

	authSrc := `package auth

type TokenValidator struct{}

func ValidateToken(raw string) bool {
	return raw != ""
}
`
	apiSrc := `package api

import "example.com/pkg/auth"

func HandleRequest(tok string) bool {
	return auth.ValidateToken(tok)
}
`
	if err := os.WriteFile(filepath.Join(dir, "pkg", "auth", "validator.go"), []byte(authSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "api", "handler.go"), []byte(apiSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	graph, err := InspectWorkspaceGraph(dir)
	if err != nil {
		t.Fatalf("InspectWorkspaceGraph: %v", err)
	}
	if len(graph.Definitions) < 3 {
		t.Fatalf("expected at least 3 symbol definitions, got %d: %+v", len(graph.Definitions), graph.Definitions)
	}

	callers := graph.CallersOf("ValidateToken")
	if len(callers) != 1 {
		t.Fatalf("expected 1 caller of ValidateToken, got %d: %+v", len(callers), callers)
	}
	if callers[0].FromFile != "pkg/api/handler.go" || callers[0].FromSymbol != "HandleRequest" {
		t.Fatalf("unexpected caller edge: %+v", callers[0])
	}

	impact := graph.ImpactOfFiles([]string{"pkg/auth/validator.go"})
	if !strings.Contains(impact, "ValidateToken") || !strings.Contains(impact, "pkg/api/handler.go") {
		t.Fatalf("expected cross-file impact to mention ValidateToken and pkg/api/handler.go, got:\n%s", impact)
	}
}

func TestInspectWorkspaceGraphPolyglotAndImportFiltering(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"internal/db/db.go": `package db
func Close() error { return nil }
`,
		"internal/app/app.go": `package app
import "os"
func RunFile(f *os.File) {
	_ = os.Remove("x")
}
`,
		"src/math.ts": `export function clampValue(n: number): number {
	return n;
}
export interface ClampOptions {
	min: number;
}
`,
		"src/view.tsx": `import { clampValue } from "./math";
export const renderScore = (x: number) => {
	return clampValue(x);
};
`,
		"scripts/worker.py": `def compute_metric(x):
    return x + 1

class Runner:
    def execute_job(self):
        return compute_metric(5)
`,
		"crates/lib.rs": `pub struct Config {}
pub fn init_engine() -> bool {
    true
}
`,
	}
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	graph, err := InspectWorkspaceGraph(dir)
	if err != nil {
		t.Fatalf("InspectWorkspaceGraph: %v", err)
	}

	for _, wantSym := range []string{"Close", "RunFile", "clampValue", "ClampOptions", "renderScore", "compute_metric", "Runner", "execute_job", "Config", "init_engine"} {
		matches := graph.FindSymbols(wantSym)
		if len(matches) == 0 {
			t.Fatalf("expected FindSymbols(%q) to find symbol in polyglot graph", wantSym)
		}
	}

	// TS call edge: renderScore -> clampValue
	callers := graph.CallersOf("clampValue")
	if len(callers) != 1 || callers[0].FromFile != "src/view.tsx" || callers[0].FromSymbol != "renderScore" {
		t.Fatalf("expected renderScore in src/view.tsx to call clampValue, got %+v", callers)
	}

	// NeighborsOfFile for src/math.ts should include src/view.tsx
	neighbors := graph.NeighborsOfFile("src/math.ts")
	if len(neighbors) != 1 || neighbors[0] != "src/view.tsx" {
		t.Fatalf("expected NeighborsOfFile(src/math.ts) = [src/view.tsx], got %v", neighbors)
	}

	// InspectWorkspaceSymbols should also work on TypeScript and Python files
	tsSyms, err := InspectWorkspaceSymbols(dir, "src/math.ts")
	if err != nil || len(tsSyms) < 2 {
		t.Fatalf("InspectWorkspaceSymbols(src/math.ts) = %+v, err=%v", tsSyms, err)
	}
}

