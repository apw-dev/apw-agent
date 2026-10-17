package surgeon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// diffWorktree builds a Worktree whose baseline is before and whose current
// files are after. A missing key means the file does not exist on that side.
func diffWorktree(t *testing.T, before, after map[string]string) *Worktree {
	t.Helper()
	dir := t.TempDir()
	w := &Worktree{Path: dir, Baseline: map[string]FileState{}, BaseContents: map[string][]byte{}}
	write := func(path, content string) {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range before {
		write(path, content)
		state, data, err := readState(dir, filepath.FromSlash(path))
		if err != nil {
			t.Fatal(err)
		}
		w.Baseline[filepath.FromSlash(path)] = state
		w.BaseContents[filepath.FromSlash(path)] = data
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			if err := os.Remove(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
				t.Fatal(err)
			}
		}
	}
	for path, content := range after {
		write(path, content)
	}
	return w
}

func diffOf(t *testing.T, before, after map[string]string, paths ...string) string {
	t.Helper()
	diff, err := diffWorktree(t, before, after).DiffFiles(context.Background(), paths)
	if err != nil {
		t.Fatalf("DiffFiles: %v", err)
	}
	return diff
}

func numbered(prefix string, from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&b, "%s%d\n", prefix, i)
	}
	return b.String()
}

func TestDiffFilesAdditionsOnly(t *testing.T) {
	base := numbered("line ", 1, 9)
	got := diffOf(t, map[string]string{"a.txt": base}, map[string]string{"a.txt": base + numbered("new ", 1, 9)}, "a.txt")
	want := "--- a/a.txt\n+++ b/a.txt\n@@ -7,3 +7,12 @@\n line 7\n line 8\n line 9\n" +
		numbered("+new ", 1, 9)
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDiffFilesDeletion(t *testing.T) {
	base := numbered("line ", 1, 10)
	after := strings.Replace(base, "line 5\nline 6\n", "", 1)
	got := diffOf(t, map[string]string{"a.txt": base}, map[string]string{"a.txt": after}, "a.txt")
	want := "--- a/a.txt\n+++ b/a.txt\n@@ -2,8 +2,6 @@\n line 2\n line 3\n line 4\n-line 5\n-line 6\n line 7\n line 8\n line 9\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDiffFilesModificationKeepsContextAndSplitsHunks(t *testing.T) {
	base := numbered("line ", 1, 30)
	after := strings.Replace(base, "line 10\n", "line ten\n", 1)
	after = strings.Replace(after, "line 25\n", "line twenty-five\n", 1)
	got := diffOf(t, map[string]string{"a.txt": base}, map[string]string{"a.txt": after}, "a.txt")
	want := "--- a/a.txt\n+++ b/a.txt\n" +
		"@@ -7,7 +7,7 @@\n line 7\n line 8\n line 9\n-line 10\n+line ten\n line 11\n line 12\n line 13\n" +
		"@@ -22,7 +22,7 @@\n line 22\n line 23\n line 24\n-line 25\n+line twenty-five\n line 26\n line 27\n line 28\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDiffFilesMergesNearbyChanges(t *testing.T) {
	base := numbered("line ", 1, 12)
	after := strings.Replace(base, "line 3\n", "line three\n", 1)
	after = strings.Replace(after, "line 9\n", "line nine\n", 1)
	got := diffOf(t, map[string]string{"a.txt": base}, map[string]string{"a.txt": after}, "a.txt")
	if strings.Count(got, "@@ -") != 1 || !strings.Contains(got, "@@ -1,12 +1,12 @@\n") {
		t.Fatalf("expected one merged hunk, got:\n%s", got)
	}
}

func TestDiffFilesNewFile(t *testing.T) {
	got := diffOf(t, nil, map[string]string{"dir/new.go": "package x\n\nfunc F() {}\n"}, "dir/new.go")
	want := "# new file\n--- a/dir/new.go\n+++ b/dir/new.go\n@@ -0,0 +1,3 @@\n+package x\n+\n+func F() {}\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDiffFilesDeletedFile(t *testing.T) {
	got := diffOf(t, map[string]string{"old.txt": "one\ntwo\n"}, nil, "old.txt")
	want := "# deleted file\n--- a/old.txt\n+++ b/old.txt\n@@ -1,2 +0,0 @@\n-one\n-two\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDiffFilesCRLFOnlyChange(t *testing.T) {
	lf := numbered("line ", 1, 50)
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")
	got := diffOf(t, map[string]string{"a.txt": crlf}, map[string]string{"a.txt": lf}, "a.txt")
	want := "# line endings changed: CRLF -> LF\n--- a/a.txt\n+++ b/a.txt\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDiffFilesCRLFFileContentChangeIgnoresCarriageReturns(t *testing.T) {
	base := strings.ReplaceAll(numbered("line ", 1, 5), "\n", "\r\n")
	after := strings.Replace(base, "line 3\r\n", "line three\r\n", 1)
	got := diffOf(t, map[string]string{"a.txt": base}, map[string]string{"a.txt": after}, "a.txt")
	want := "--- a/a.txt\n+++ b/a.txt\n@@ -1,5 +1,5 @@\n line 1\n line 2\n-line 3\n+line three\n line 4\n line 5\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestDiffFilesSingleLineEOLChangeInMixedFile(t *testing.T) {
	base := "a\r\nb\nc\n"
	got := diffOf(t, map[string]string{"a.txt": base}, map[string]string{"a.txt": "a\nb\r\nc\n"}, "a.txt")
	want := "# line endings changed on 2 unchanged lines\n--- a/a.txt\n+++ b/a.txt\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDiffFilesMissingFinalNewline(t *testing.T) {
	got := diffOf(t, map[string]string{"a.txt": "a\nb\n"}, map[string]string{"a.txt": "a\nb"}, "a.txt")
	want := "--- a/a.txt\n+++ b/a.txt\n@@ -1,2 +1,2 @@\n a\n-b\n+b\n\\ No newline at end of file\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDiffFilesSortsDedupesAndSkipsUnchanged(t *testing.T) {
	got := diffOf(t,
		map[string]string{"b.txt": "b\n", "a.txt": "a\n", "same.txt": "s\n"},
		map[string]string{"b.txt": "B\n", "a.txt": "A\n", "same.txt": "s\n"},
		"b.txt", "a.txt", "same.txt", "a.txt")
	want := "--- a/a.txt\n+++ b/a.txt\n@@ -1,1 +1,1 @@\n-a\n+A\n" +
		"--- a/b.txt\n+++ b/b.txt\n@@ -1,1 +1,1 @@\n-b\n+B\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDiffFilesTruncation(t *testing.T) {
	big := strings.Repeat(strings.Repeat("x", 99)+"\n", 12000) // ~1.2 MB
	got := diffOf(t, map[string]string{"a.txt": "a\n"}, map[string]string{"a.txt": "A\n", "big.txt": big}, "a.txt", "big.txt")
	if !strings.HasPrefix(got, "--- a/a.txt\n") || !strings.HasSuffix(got, "\n[diff truncated]\n") {
		t.Fatalf("expected small diff followed by truncation marker, got %d bytes:\n%.300s", len(got), got)
	}
	if strings.Contains(got, "big.txt") {
		t.Fatalf("truncated file should not be partially rendered")
	}
}

func TestDiffFilesLargeFileSmallChangeNotTruncated(t *testing.T) {
	big := strings.Repeat(strings.Repeat("y", 99)+"\n", 12000)
	got := diffOf(t, map[string]string{"big.txt": big}, map[string]string{"big.txt": big + "tail\n"}, "big.txt")
	if strings.Contains(got, "[diff truncated]") || !strings.HasSuffix(got, "+tail\n") || len(got) > 1000 {
		t.Fatalf("unexpected diff for small change to large file (%d bytes):\n%.500s", len(got), got)
	}
}

func TestDiffFilesBaselineGuard(t *testing.T) {
	w := diffWorktree(t, map[string]string{"a.txt": "a\n"}, map[string]string{"a.txt": "b\n"})
	w.BaseContents["a.txt"] = []byte("tampered\n")
	if _, err := w.DiffFiles(context.Background(), []string{"a.txt"}); err == nil || !strings.Contains(err.Error(), "baseline contents unavailable for diff") {
		t.Fatalf("expected baseline guard error, got %v", err)
	}
}

func TestLineDiffFallsBackBeyondEditLimit(t *testing.T) {
	a := strings.Split(strings.TrimSuffix(numbered("a", 1, 50), "\n"), "\n")
	b := strings.Split(strings.TrimSuffix(numbered("b", 1, 50), "\n"), "\n")
	ops := lineDiff(a, b, 10)
	var dels, adds int
	for _, op := range ops {
		switch op.kind {
		case '-':
			dels++
		case '+':
			adds++
		default:
			t.Fatalf("unexpected op %q", op.kind)
		}
		if op.kind == '+' && dels != 50 {
			t.Fatalf("fallback should list deletions before additions")
		}
	}
	if dels != 50 || adds != 50 {
		t.Fatalf("got %d deletions, %d additions", dels, adds)
	}
}

func TestLineDiffMinimalEdits(t *testing.T) {
	a := []string{"a", "b", "c", "a", "b", "b", "a"}
	b := []string{"c", "b", "a", "b", "a", "c"}
	ops := lineDiff(a, b, maxDiffEdits)
	var edits int
	var gotA, gotB []string
	for _, op := range ops {
		if op.kind != ' ' {
			edits++
		}
		if op.kind != '+' {
			gotA = append(gotA, a[op.a])
		}
		if op.kind != '-' {
			gotB = append(gotB, b[op.b])
		}
	}
	if edits != 5 {
		t.Fatalf("expected minimal edit script of 5, got %d: %+v", edits, ops)
	}
	if strings.Join(gotA, "") != strings.Join(a, "") || strings.Join(gotB, "") != strings.Join(b, "") {
		t.Fatalf("edit script does not reproduce inputs: %+v", ops)
	}
}
