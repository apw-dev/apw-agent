package surgeon

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain keeps task worktrees created by tests out of the real user cache directory.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "apw-worktrees-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv(WorktreeRootEnv, filepath.Join(dir, "wt"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
