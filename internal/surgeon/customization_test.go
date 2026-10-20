package surgeon

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAPWCustomizationCreatesOnceAndRejectsOtherPaths(t *testing.T) {
	ws := t.TempDir()
	rel := filepath.Join(".apw", "skills", "demo", "SKILL.md")
	if err := CreateAPWCustomization(ws, rel, []byte("first")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := CreateAPWCustomization(ws, rel, []byte("second")); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected ErrExist on overwrite, got %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(ws, rel)); string(data) != "first" {
		t.Fatalf("existing skill overwritten: %q", data)
	}
	for _, bad := range []string{"../x/SKILL.md", ".apw/auth.json", ".apw/tasks.db", "main.go", ".apw/skills/SKILL.md"} {
		if err := CreateAPWCustomization(ws, filepath.FromSlash(bad), []byte("x")); err == nil {
			t.Fatalf("allowed disallowed path %s", bad)
		}
	}
}

func TestCreateAPWCustomizationRejectsSymlinkedParent(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".apw"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, ".apw", "skills")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	err := CreateAPWCustomization(ws, filepath.Join(".apw", "skills", "evil", "SKILL.md"), []byte("x"))
	if err == nil {
		t.Fatal("wrote through symlinked .apw/skills")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "evil")); !os.IsNotExist(statErr) {
		t.Fatalf("directory created outside workspace: %v", statErr)
	}
}
