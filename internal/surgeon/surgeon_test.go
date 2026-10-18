package surgeon

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSurgeon_ClaudeCodePrinciples(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "example.go")

	initialContent := `package main

import "fmt"

type Greeter struct {
	Name string
}

func (g *Greeter) Greet() string {
	return "Hello, " + g.Name
}

func main() {
	g := &Greeter{Name: "World"}
	fmt.Println(g.Greet())
}
`

	err := os.WriteFile(testFile, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("failed to setup test file: %v", err)
	}

	s := NewSurgeon()

	// 1. Attempt patch without reading first -> must fail with ErrReadRequired
	_, err = s.ApplyPatch(PatchOperation{
		Path:       testFile,
		OldContent: `"World"`,
		NewContent: `"Claude"`,
	})
	if err != ErrReadRequired {
		t.Fatalf("expected ErrReadRequired, got %v", err)
	}

	// 2. Read file properly
	content, hash, err := s.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if content != initialContent {
		t.Fatalf("read content mismatch")
	}

	// 3. Test AST symbol inspection
	symbols, err := s.InspectGoSymbols(testFile)
	if err != nil {
		t.Fatalf("InspectGoSymbols failed: %v", err)
	}
	if len(symbols) < 3 {
		t.Fatalf("expected at least 3 symbols (Greeter, Greet, main), got %d", len(symbols))
	}

	// 4. Apply surgical targeted patch
	newHash, err := s.ApplyPatch(PatchOperation{
		Path:       testFile,
		BaseHash:   hash,
		OldContent: `"World"`,
		NewContent: `"Argon"`,
	})
	if err != nil {
		t.Fatalf("ApplyPatch failed: %v", err)
	}
	if newHash == hash {
		t.Fatalf("expected new hash after patch, but got identical hash")
	}

	// 5. Verify patched file on disk
	patchedContent, _, err := s.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to re-read patched file: %v", err)
	}
	if patchedContent == initialContent {
		t.Fatalf("file on disk was not changed")
	}

	// 6. Test drift detection with stale base hash
	_, err = s.ApplyPatch(PatchOperation{
		Path:       testFile,
		BaseHash:   hash, // stale hash
		OldContent: `"Argon"`,
		NewContent: `"Claude"`,
	})
	if err == nil {
		t.Fatalf("expected error due to content drift, got nil")
	}
}

func TestSurgeon_CRLFPreservation(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "crlf.go")
	initial := "package main\r\n\r\nfunc a() {\r\n\tx := 1\r\n}\r\n"
	if err := os.WriteFile(p, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewSurgeon()
	if _, _, err := s.ReadFile(p); err != nil {
		t.Fatal(err)
	}

	// Patch using LF-only OldContent and NewContent
	_, err := s.ApplyPatch(PatchOperation{
		Path:       p,
		OldContent: "func a() {\n\tx := 1\n}",
		NewContent: "func a() {\n\tx := 2\n\ty := 3\n}",
	})
	if err != nil {
		t.Fatalf("ApplyPatch failed on CRLF file: %v", err)
	}

	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Errorf("expected all line endings to remain CRLF, got %q", got)
	}
}

func TestSurgeon_EmptyTarget(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.go")
	if err := os.WriteFile(p, []byte("package x\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewSurgeon()
	if _, _, err := s.ReadFile(p); err != nil {
		t.Fatal(err)
	}

	_, err := s.ApplyPatch(PatchOperation{
		Path:       p,
		OldContent: "",
		NewContent: "boom",
	})
	if !errors.Is(err, ErrEmptyTarget) {
		t.Fatalf("expected ErrEmptyTarget, got %v", err)
	}
}

func TestSurgeon_SnapshotKeyNormalized(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.go")
	if err := os.WriteFile(p, []byte("package x\nvar a = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewSurgeon()
	if _, _, err := s.ReadFile(p); err != nil {
		t.Fatal(err)
	}

	altPath := p + string(filepath.Separator) + "."
	if _, err := s.ApplyPatch(PatchOperation{
		Path:       altPath,
		OldContent: "var a = 1",
		NewContent: "var a = 2",
	}); err != nil {
		t.Fatalf("expected normalized path to match snapshot, got %v", err)
	}
}

func TestInspectSymbolsIncludesVarsAndConsts(t *testing.T) {
	dir := t.TempDir()
	src := "package age\n\nimport \"errors\"\n\n// ErrInvalidAge reports bad input.\nvar ErrInvalidAge = errors.New(\"invalid age\")\n\nconst (\n\tMaxAge = 150\n\t_      = 0\n)\n\nvar a, b int\n\ntype T struct{}\n\nfunc (T) M() {}\n\nfunc Parse() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "age.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	syms, err := InspectWorkspaceSymbols(dir, "age.go")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range syms {
		got[s.Name] = s.Kind
	}
	for name, kind := range map[string]string{"ErrInvalidAge": "var", "MaxAge": "const", "a": "var", "b": "var", "T": "type", "M": "method", "Parse": "func"} {
		if got[name] != kind {
			t.Fatalf("%s kind = %q, want %q (all: %v)", name, got[name], kind, got)
		}
	}
	if _, ok := got["_"]; ok {
		t.Fatal("blank identifier listed")
	}
}
