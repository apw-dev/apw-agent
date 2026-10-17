//go:build windows

package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/process"
)

func run(t *testing.T, s *Session, spec Spec) (string, error) {
	t.Helper()
	p, err := s.Command(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Cleanup()
	var out process.Buffer
	p.Cmd.Stdout = &out
	p.Cmd.Stderr = &out
	err = process.RunLimited(context.Background(), p.Cmd, p.Limits)
	return out.String(), err
}

func TestRestrictedWritesOnlyTheWorktree(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".apw"), 0700); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, ".apw", "worktree.json")
	if err := os.WriteFile(manifest, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	gitLink := filepath.Join(root, ".git")
	if err := os.WriteFile(gitLink, []byte("gitdir: elsewhere\n"), 0644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APW_TEST_TOKEN", "secret-value")
	t.Setenv("SAFE_SETTING", "visible")
	s, err := Open(Policy{Mode: ModeRestricted, Network: NetworkInstall}, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if out, err := run(t, s, Spec{Args: []string{"cmd", "/c", "echo inside>inside.txt"}}); err != nil {
		t.Fatalf("write inside worktree failed: %v %s", err, out)
	}
	if data, err := os.ReadFile(filepath.Join(root, "inside.txt")); err != nil || !strings.Contains(string(data), "inside") {
		t.Fatalf("inside write missing: %q %v", data, err)
	}
	for _, target := range []string{outside, manifest, gitLink} {
		_, _ = run(t, s, Spec{Args: []string{"cmd", "/c", "echo changed>" + target}})
	}
	if data, _ := os.ReadFile(outside); string(data) != "keep" {
		t.Fatalf("outside file changed: %q", data)
	}
	if data, _ := os.ReadFile(manifest); string(data) != "{}" {
		t.Fatalf("worktree manifest changed: %q", data)
	}
	if data, _ := os.ReadFile(gitLink); string(data) != "gitdir: elsewhere\n" {
		t.Fatalf("worktree .git link changed: %q", data)
	}
	out, err := run(t, s, Spec{Args: []string{"cmd", "/c", "set"}, Env: []string{"STAGE_VAR=1"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "secret-value") || !strings.Contains(out, "SAFE_SETTING=visible") || !strings.Contains(out, "STAGE_VAR=1") {
		t.Fatalf("environment not scrubbed as expected:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), strings.ToLower("TEMP="+filepath.Join(root, ".apw-sandbox", "tmp"))) {
		t.Fatalf("TEMP not redirected:\n%s", out)
	}
	// Reopening an already labeled worktree is cheap and keeps working.
	s2, err := Open(Policy{Mode: ModeRestricted}, root)
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
}

func TestRestrictedRunsGoToolchain(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the standard library into a cold cache")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	root := t.TempDir()
	files := map[string]string{
		"go.mod":       "module example.com/sb\n\ngo 1.24\n",
		"main.go":      "package main\n\nfunc Add(a, b int) int { return a + b }\n\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Open(Policy{Mode: ModeRestricted}, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if out, err := run(t, s, Spec{Args: []string{"go", "test", "-mod=readonly", "./..."}}); err != nil {
		t.Fatalf("go test in restricted sandbox: %v\n%s", err, out)
	}
}
