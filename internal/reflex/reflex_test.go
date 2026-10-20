package reflex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/surgeon"
)

func TestParseGoErrors(t *testing.T) {
	raw := `internal\core\models.go:12:5: undefined: TaskStatus
internal\foo\bar.go:45: missing return at end of function`

	diags := ParseGoErrors("build", raw)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}

	if diags[0].Line != 12 || diags[0].Col != 5 || diags[0].Message != "undefined: TaskStatus" {
		t.Errorf("diag[0] parsed incorrectly: %+v", diags[0])
	}
	if diags[1].Line != 45 || diags[1].Col != 0 || diags[1].Message != "missing return at end of function" {
		t.Errorf("diag[1] parsed incorrectly: %+v", diags[1])
	}
}

func TestParseGoErrors_Prefixes(t *testing.T) {
	raw := "vet.exe: internal\\a\\b.go:92:25: undefined: X\n" +
		"    foo_test.go:12: expected 1, got 2"
	diags := ParseGoErrors("vet", raw)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if diags[0].File != `internal\a\b.go` {
		t.Errorf("diag[0].File = %q", diags[0].File)
	}
	if diags[1].File != "foo_test.go" || diags[1].Line != 12 {
		t.Errorf("diag[1] = %+v", diags[1])
	}
}

func TestFormatDiagnostics(t *testing.T) {
	t.Run("with col", func(t *testing.T) {
		res := &VerificationResult{
			Passed:      false,
			FailedStage: "build",
			Diagnostics: []Diagnostic{
				{
					Stage:   "build",
					File:    "main.go",
					Line:    10,
					Col:     2,
					Message: "undefined: Foo",
				},
			},
			RawOutput: "main.go:10:2: undefined: Foo\n",
		}
		msg := FormatDiagnostics(res)
		if msg == "" || len(msg) < 20 {
			t.Fatalf("formatted diagnostics message is too short or empty")
		}
		if !strings.Contains(msg, "main.go:10:2") {
			t.Errorf("expected col to appear in output, got: %s", msg)
		}
	})

	t.Run("col=0 omits :0", func(t *testing.T) {
		res := &VerificationResult{
			Passed:      false,
			FailedStage: "build",
			Diagnostics: []Diagnostic{
				{
					Stage:   "build",
					File:    "main.go",
					Line:    45,
					Col:     0,
					Message: "missing return at end of function",
				},
			},
			RawOutput: "main.go:45: missing return at end of function\n",
		}
		msg := FormatDiagnostics(res)
		if strings.Contains(msg, ":0") {
			t.Errorf("expected :0 to be absent when Col=0, got: %s", msg)
		}
		if !strings.Contains(msg, "main.go:45") {
			t.Errorf("expected file:line to appear in output, got: %s", msg)
		}
	})
}

func TestVerifier_CustomPackages(t *testing.T) {
	v := NewVerifier(".", "./internal/reflex")
	if len(v.Packages) != 1 || v.Packages[0] != "./internal/reflex" {
		t.Fatalf("expected Packages=[./internal/reflex], got %v", v.Packages)
	}
	out, err := v.RunCommand(context.Background(), "go", "version")
	if err != nil {
		t.Fatalf("RunCommand failed: %v", err)
	}
	if !strings.Contains(out, "go version") {
		t.Errorf("expected 'go version' in output, got: %s", out)
	}
}

func TestVerifier(t *testing.T) {
	v := NewVerifier(".")
	out, err := v.RunCommand(context.Background(), "go", "version")
	if err != nil || out == "" {
		t.Fatalf("expected go version to succeed, got %v: %s", err, out)
	}
}

func TestReflexEngine_Run_HealsInSecondAttempt(t *testing.T) {
	dir := t.TempDir()
	goMod := "module example.com/tempmod\n\ngo 1.24\n"
	brokenMain := "package main\n\nfunc main() {\n\t_ = undefinedSymbol\n}\n"

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(brokenMain), 0644); err != nil {
		t.Fatal(err)
	}

	s := surgeon.NewSurgeon()
	engine := NewReflexEngine(dir, 3)

	attempts, err := engine.Run(context.Background(), func(ctx context.Context, diagnostics string) error {
		if !strings.Contains(diagnostics, "undefinedSymbol") {
			t.Errorf("expected diagnostics to mention undefinedSymbol, got: %s", diagnostics)
		}
		_, hash, err := s.ReadFile(mainPath)
		if err != nil {
			return err
		}
		_, err = s.ApplyPatch(surgeon.PatchOperation{
			Path:       mainPath,
			BaseHash:   hash,
			OldContent: "_ = undefinedSymbol",
			NewContent: "x := 42\n\t_ = x",
		})
		return err
	})
	if err != nil {
		t.Fatalf("engine.Run returned unexpected error: %v", err)
	}
	if len(attempts) != 2 {
		t.Fatalf("expected 2 attempts (initial fail + 1 repair pass), got %d", len(attempts))
	}
	if attempts[0].Number != 1 || attempts[0].Result.Passed {
		t.Errorf("attempt 1 should have failed, got %+v", attempts[0].Result)
	}
	if attempts[1].Number != 2 || !attempts[1].Result.Passed {
		t.Errorf("attempt 2 should have passed, got %+v", attempts[1].Result)
	}
}

func TestReflexEngine_Run_ExhaustsMaxRetries(t *testing.T) {
	dir := t.TempDir()
	goMod := "module example.com/tempmod\n\ngo 1.24\n"
	brokenMain := "package main\n\nfunc main() {\n\t_ = stillBroken\n}\n"

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(brokenMain), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewReflexEngine(dir, 2)
	fixCalls := 0

	attempts, err := engine.Run(context.Background(), func(ctx context.Context, diagnostics string) error {
		fixCalls++
		return nil // no-op fixer that fails to resolve the error
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixCalls != 2 {
		t.Errorf("expected fixer to be called MaxRetries (2) times, got %d", fixCalls)
	}
	if len(attempts) != 3 {
		t.Fatalf("expected 3 attempts (1 initial + 2 retries), got %d", len(attempts))
	}
	for i, a := range attempts {
		if a.Result.Passed {
			t.Errorf("attempt %d unexpectedly passed", i+1)
		}
	}
}

func TestFormatSkipsStateDependenciesFixturesAndGenerated(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{".apw/cache.go", ".worktrees/task/bad.go", "vendor/lib/bad.go", "node_modules/lib/bad.go", "testdata/invalid.go"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("this is deliberately not Go"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "generated.go"), []byte("// Code generated by fixture. DO NOT EDIT.\npackage main;func generated( ){ }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	verifier := NewVerifier(root)
	output, err := verifier.checkFormat(context.Background())
	if err != nil || strings.TrimSpace(output) != "" {
		t.Fatalf("ignored files affected formatting: %q %v", output, err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main;func main( ){ }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	output, err = verifier.checkFormat(context.Background())
	if err != nil || !strings.Contains(output, "main.go") {
		t.Fatalf("actual source was not checked: %q %v", output, err)
	}
}

func TestFormatAcceptsCRLFWithoutChangingSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	source := []byte("package main\r\n\r\nfunc main() {}\r\n")
	if err := os.WriteFile(path, source, 0644); err != nil {
		t.Fatal(err)
	}
	output, err := NewVerifier(root).checkFormat(context.Background())
	if err != nil || output != "" {
		t.Fatalf("formatted CRLF rejected: %q %v", output, err)
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != string(source) {
		t.Fatal("formatter changed the original line endings")
	}
	if err := os.WriteFile(path, []byte("package main;func main( ){ }\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	output, err = NewVerifier(root).checkFormat(context.Background())
	if err != nil || !strings.Contains(output, "main.go") {
		t.Fatal("CRLF concealed actual formatting errors")
	}
}

func TestCheckFormat_ScopedByOnlyFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "clean.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated_dirty.go"), []byte("package main;func dirty( ){ }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewVerifier(root)
	v.OnlyFiles = []string{"clean.go"}
	output, err := v.checkFormat(context.Background())
	if err != nil || strings.TrimSpace(output) != "" {
		t.Fatalf("expected OnlyFiles to ignore unrelated_dirty.go, got %q (%v)", output, err)
	}

	v.OnlyFiles = []string{"unrelated_dirty.go"}
	output, err = v.checkFormat(context.Background())
	if err != nil || !strings.Contains(output, "unrelated_dirty.go") {
		t.Fatalf("expected OnlyFiles to flag unrelated_dirty.go when included, got %q (%v)", output, err)
	}
}
