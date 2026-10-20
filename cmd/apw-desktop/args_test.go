package main

import (
	"path/filepath"
	"testing"
)

func TestWorkspaceArg(t *testing.T) {
	dir := t.TempDir()
	if got := WorkspaceArg([]string{"--devtools", dir}); got != dir {
		t.Fatalf("WorkspaceArg = %q, want %q", got, dir)
	}
	if got := WorkspaceArg(nil); got != "" {
		t.Fatalf("WorkspaceArg(nil) = %q", got)
	}
	if got := WorkspaceArg([]string{"--devtools"}); got != "" {
		t.Fatalf("flags only = %q", got)
	}
	rel := WorkspaceArg([]string{"repo"})
	if !filepath.IsAbs(rel) || filepath.Base(rel) != "repo" {
		t.Fatalf("relative arg not made absolute: %q", rel)
	}
}

func TestDevtoolsRequested(t *testing.T) {
	if !DevtoolsRequested([]string{"x", "--devtools"}) {
		t.Fatal("want true")
	}
	if DevtoolsRequested([]string{"--devtools=false", "x"}) || DevtoolsRequested(nil) {
		t.Fatal("want false")
	}
}
