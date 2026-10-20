//go:build windows

package surgeon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAPWCustomizationRejectsJunctionParent(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".apw"), 0o755); err != nil {
		t.Fatal(err)
	}
	mklinkJunction(t, filepath.Join(ws, ".apw", "skills"), outside)
	if err := CreateAPWCustomization(ws, filepath.Join(".apw", "skills", "evil", "SKILL.md"), []byte("x")); err == nil {
		t.Fatal("wrote through junction .apw/skills")
	}
	if _, err := os.Stat(filepath.Join(outside, "evil")); !os.IsNotExist(err) {
		t.Fatalf("directory created outside workspace: %v", err)
	}
}
