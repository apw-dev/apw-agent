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

// A second launch reports its own working directory; a relative folder argument is resolved
// against it, not against the first instance's directory.
func TestWorkspaceArgFromResolvesAgainstGivenDir(t *testing.T) {
	wd := t.TempDir()
	if got, want := WorkspaceArgFrom([]string{"repo"}, wd), filepath.Join(wd, "repo"); got != want {
		t.Fatalf("WorkspaceArgFrom = %q, want %q", got, want)
	}
	abs := filepath.Join(t.TempDir(), "other")
	if got := WorkspaceArgFrom([]string{abs}, wd); got != abs {
		t.Fatalf("absolute arg changed: %q", got)
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
