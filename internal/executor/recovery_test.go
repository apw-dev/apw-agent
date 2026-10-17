package executor

import (
	"context"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryExportsWithoutChangingSourceOrWorktree(t *testing.T) {
	root, wt, cp := resumeFixture(t)
	if err := os.WriteFile(filepath.Join(wt.Path, "notes.txt"), []byte("untracked work"), 0644); err != nil {
		t.Fatal(err)
	}
	source, _ := os.ReadFile(filepath.Join(root, "main.go"))
	working, _ := os.ReadFile(filepath.Join(wt.Path, "main.go"))
	result, err := RecoverLegacyTask(context.Background(), root, cp.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "exported" || result.Completed != 1 {
		t.Fatal("incorrect recovery manifest")
	}
	for _, path := range []string{"main.go", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(result.Directory, "files", path)); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := os.ReadFile(filepath.Join(root, "main.go"))
	afterWorking, _ := os.ReadFile(filepath.Join(wt.Path, "main.go"))
	if string(after) != string(source) || string(afterWorking) != string(working) {
		t.Fatal("recovery altered source or original worktree")
	}
	state, _ := surgeon.StateInRoot(wt.Path, "main.go")
	if result.Files["main.go"] != state {
		t.Fatal("export hash differs from original work")
	}
}
