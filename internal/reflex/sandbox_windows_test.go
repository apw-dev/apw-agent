//go:build windows

package reflex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/project"
	"github.com/apiwat/apw-agent/internal/sandbox"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

// TestRestrictedSandboxVerifiesRealWorktree runs the Go pipeline in a low-integrity sandbox inside a
// real task worktree: verification passes, sandbox caches never count as task changes, and the
// manifest and cleanup still work.
func TestRestrictedSandboxVerifiesRealWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the standard library into a cold sandbox cache")
	}
	repo := t.TempDir()
	files := map[string]string{
		"go.mod":       "module example.com/sb\n\ngo 1.24\n",
		"main.go":      "package main\n\nfunc Add(a, b int) int { return a + b }\n\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init"}, {"config", "user.name", "T"}, {"config", "user.email", "t@e"}, {"config", "commit.gpgsign", "false"}, {"add", "."}, {"commit", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	t.Setenv(surgeon.WorktreeRootEnv, filepath.Join(t.TempDir(), "wt"))
	wt, err := surgeon.CreateWorktree(context.Background(), repo, surgeon.WorktreeOptions{TaskID: "task-sb", SyncDirtyFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	v := NewVerifierForProfile(wt.Path, project.Detect(wt.Path, nil))
	v.Sandbox = sandbox.Policy{Mode: sandbox.ModeRestricted}
	res := v.Verify(context.Background())
	if !res.Passed || !strings.HasPrefix(res.Sandbox, "restricted") {
		t.Fatalf("sandboxed verification = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(wt.Path, surgeon.SandboxDir, "go-build")); err != nil {
		t.Fatalf("sandbox cache missing: %v", err)
	}
	changed, err := wt.ChangedFiles(context.Background())
	if err != nil || len(changed) != 0 {
		t.Fatalf("sandbox state counted as changes: %v %v", changed, err)
	}
	if _, err := surgeon.LoadWorktree(wt.Path); err != nil {
		t.Fatal(err)
	}
	if err := wt.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
}
