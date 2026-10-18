package eval

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

const suiteDir = "../../eval/testdata"

// TestSuiteIsMeaningful proves every task can discriminate: the fixture's own tests pass, the
// hidden tests fail on the unchanged fixture, and they pass with the reference solution.
func TestSuiteIsMeaningful(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	tasks, err := LoadSuite(suiteDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) < 10 {
		t.Fatalf("suite has only %d tasks", len(tasks))
	}
	for _, task := range tasks {
		t.Run(task.Name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			check := func(layers ...string) error {
				dir := t.TempDir()
				for _, layer := range layers {
					if err := copyTree(filepath.Join(task.Dir, layer), dir); err != nil {
						t.Fatal(err)
					}
				}
				_, err := runCheck(ctx, dir, task.Check)
				return err
			}
			if err := check("repo"); err != nil {
				t.Fatalf("fixture's own tests fail: %v", err)
			}
			if err := check("repo", "hidden"); err == nil {
				t.Fatal("hidden tests already pass on the unchanged fixture")
			}
			if err := check("repo", "solution", "hidden"); err != nil {
				out, _ := runCheck(ctx, func() string {
					dir := t.TempDir()
					for _, layer := range []string{"repo", "solution", "hidden"} {
						_ = copyTree(filepath.Join(task.Dir, layer), dir)
					}
					return dir
				}(), task.Check)
				t.Fatalf("reference solution fails the hidden tests: %v\n%s", err, out)
			}
		})
	}
}
