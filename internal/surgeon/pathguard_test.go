package surgeon

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRootSeparators(t *testing.T) {
	tmpDir := t.TempDir()
	root, err := os.OpenRoot(tmpDir)
	if err != nil {
		t.Fatalf("OpenRoot failed: %v", err)
	}
	defer root.Close()

	if err := root.MkdirAll("a/b", 0755); err != nil {
		t.Fatalf("MkdirAll slash failed: %v", err)
	}
	if err := root.WriteFile("a/b/c.txt", []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile slash failed: %v", err)
	}
	if _, err := root.ReadFile("a/b/c.txt"); err != nil {
		t.Fatalf("ReadFile slash failed: %v", err)
	}
	if _, err := root.ReadFile("a\\b\\c.txt"); err != nil {
		t.Fatalf("ReadFile backslash failed: %v", err)
	}
}

func TestSymlinkEscape(t *testing.T) {
	tmpDir := t.TempDir()
	outsideDir := t.TempDir()
	symlinkPath := filepath.Join(tmpDir, "escape_link")
	err := os.Symlink(outsideDir, symlinkPath)
	if err != nil {
		t.Skipf("os.Symlink not supported or requires privilege: %v", err)
	}

	_, err = ResolveInRoot(tmpDir, "escape_link/secret.txt")
	if !errors.Is(err, ErrPathEscape) {
		t.Fatalf("expected ErrPathEscape, got: %v", err)
	}
}

func TestResolveInRoot_Local(t *testing.T) {
	tmpDir := t.TempDir()
	p, err := ResolveInRoot(tmpDir, filepath.Join("a", "b", "c.go"))
	if err != nil {
		t.Fatalf("ResolveInRoot failed: %v", err)
	}
	expected := filepath.Join(tmpDir, "a", "b", "c.go")
	if p != expected {
		t.Fatalf("expected %q, got %q", expected, p)
	}

	// Traversal should fail
	_, err = ResolveInRoot(tmpDir, "../escape.go")
	if !errors.Is(err, ErrPathEscape) {
		t.Fatalf("expected ErrPathEscape, got: %v", err)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "sub", "test.txt")
	if err := WriteFileAtomic(target, []byte("hello atomic"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic failed: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "hello atomic" {
		t.Fatalf("unexpected content: %s, err: %v", string(data), err)
	}
}
