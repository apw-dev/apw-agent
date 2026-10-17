package surgeon

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestEmptyHooksDirReusedAndRecreated(t *testing.T) {
	first, err := emptyHooksDir()
	if err != nil {
		t.Fatal(err)
	}
	second, err := emptyHooksDir()
	if err != nil || second != first {
		t.Fatalf("hooks dir not reused: %q vs %q err=%v", first, second, err)
	}
	entries, err := os.ReadDir(first)
	if err != nil || len(entries) != 0 {
		t.Fatalf("hooks dir must be empty: %v err=%v", entries, err)
	}

	if err := os.RemoveAll(first); err != nil {
		t.Fatal(err)
	}
	dir := initTestGitRepo(t)
	out, err := gitOutput(context.Background(), dir, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		t.Fatalf("gitOutput after hooks dir removal: %q err=%v", out, err)
	}
	third, err := emptyHooksDir()
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(third); err != nil || !info.IsDir() {
		t.Fatalf("hooks dir not recreated: %v", err)
	}
}
