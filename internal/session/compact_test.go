package session

import (
	"fmt"
	"strings"
	"testing"
)

func TestContextSummaryStaysBoundedInLongSessions(t *testing.T) {
	s := New(t.TempDir())
	for i := 1; i <= 120; i++ {
		s.RecordTurn(Turn{
			Instruction:   fmt.Sprintf("task %d %s", i, strings.Repeat("detail ", 40)),
			Success:       true,
			Applied:       i%2 == 0,
			ModifiedFiles: []string{fmt.Sprintf("src/file%03d.ts", i)},
		})
	}
	if len(s.History) != maxHistoryTurns || len(s.Memory.Earlier) != maxEarlier {
		t.Fatalf("history=%d earlier=%d", len(s.History), len(s.Memory.Earlier))
	}
	summary := s.BuildContextSummary()
	if len(summary) > 12*1024 {
		t.Fatalf("summary grew to %d bytes", len(summary))
	}
	for _, want := range []string{"Earlier in this session (compacted):", "older turns omitted", "#117 [not applied] task 117", "Turn #120", "earlier files omitted", "src/file120.ts"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary lacks %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "src/file002.ts") {
		t.Fatal("oldest touched file still listed")
	}
	// Recent turns stay detailed and are not duplicated as digests.
	if strings.Contains(summary, "  #120 [") || strings.Count(summary, "\nTurn #") != recentTurns {
		t.Fatalf("recent turns digested or missing:\n%s", summary)
	}
}

func TestDigest(t *testing.T) {
	d := digest(Turn{Index: 7, Instruction: "fix\n  the   bug", Success: true, ModifiedFiles: []string{"a", "b", "c", "d", "e", "f"}})
	if d != "#7 [not applied] fix the bug (files: a, b, c, d +2)" {
		t.Fatalf("digest = %q", d)
	}
	if got := digest(Turn{Index: 1, Instruction: strings.Repeat("ก", 200)}); !strings.HasPrefix(got, "#1 [failed] ") || !strings.HasSuffix(got, "...") {
		t.Fatalf("long digest = %q", got)
	}
}
