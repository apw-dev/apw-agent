package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/provider/mock"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

func TestApplyEditsMakesSeveralSeparateChanges(t *testing.T) {
	doc := "# Review\nDate: 2026-10-01\n\n## Evidence\nTests: not run\n\n## Verdict\nNot ready\n"
	got, _, err := applyEdits(doc, []patchEdit{
		{OldContent: "Date: 2026-10-01", NewContent: "Date: 2026-10-25"},
		{OldContent: "Tests: not run", NewContent: "Tests: 42 passed"},
		{OldContent: "Not ready", NewContent: "Ready for staging"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "# Review\nDate: 2026-10-25\n\n## Evidence\nTests: 42 passed\n\n## Verdict\nReady for staging\n"
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
	if _, rejected, err := applyEdits(doc, []patchEdit{{OldContent: "Date", NewContent: "x"}, {OldContent: "missing", NewContent: "y"}}); !errors.Is(err, surgeon.ErrTargetNotFound) || rejected != "missing" {
		t.Fatalf("missing edit: rejected=%q err=%v", rejected, err)
	}
	if _, _, err := applyEdits(doc, []patchEdit{{OldContent: "\n\n", NewContent: "\n"}}); !errors.Is(err, surgeon.ErrMultipleMatches) {
		t.Fatalf("ambiguous edit: %v", err)
	}
	crlf, _, err := applyEdits("a\r\nb\r\n", []patchEdit{{OldContent: "a\nb", NewContent: "a\nc"}})
	if err != nil || crlf != "a\r\nc\r\n" {
		t.Fatalf("crlf = %q, %v", crlf, err)
	}
}

func TestPatchDirectiveWithEditsWritesOnce(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "notes.md")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mp := mock.New(`{"edits":[{"old_content":"one","new_content":"ONE"},{"old_content":"three","new_content":"THREE"}]}`)
	ex := New(ws, mp, mp, "arch", "reflex", 1)
	ex.DelegateBudget = -1
	writes := 0
	ex.mutation = func(string, surgeon.FileState, surgeon.FileState) error { writes++; return nil }
	surg := surgeon.NewSurgeon()
	_, hash, err := surg.ReadWorkspaceFile(ws, "notes.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ex.patchWithRetry(context.Background(), ws, surg, "notes.md", hash, false, "Provide surgical patch for notes.md."); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "ONE\ntwo\nTHREE\n" || writes != 1 {
		t.Fatalf("file=%q writes=%d", got, writes)
	}
}

func TestScaledTimeoutGrowsWithRequestSize(t *testing.T) {
	base := 90 * time.Second
	for _, c := range []struct {
		bytes int
		want  time.Duration
	}{
		{1000, base},
		{16 * 1024, base},
		{40 * 1024, base + 60*time.Second},
		{10 << 20, 5 * time.Minute},
	} {
		if got := scaledTimeout(base, c.bytes); got != c.want {
			t.Fatalf("scaledTimeout(%d) = %v, want %v", c.bytes, got, c.want)
		}
	}
	if got := scaledTimeout(10*time.Minute, 40*1024); got != 10*time.Minute {
		t.Fatalf("a configured timeout above the cap must be kept, got %v", got)
	}
}

func TestMidPlanVerifyRunsAndFeedsLaterSteps(t *testing.T) {
	ws, _, cp := resumeFixture(t)
	cp.LastSuccessStep = 0 // main.go was already changed in the worktree by the fixture
	cp.Plan.Subtasks = []architect.Subtask{
		{ID: "verify", Action: architect.ActionVerify, TargetFile: "."},
		{ID: "report", Action: architect.ActionPatch, TargetFile: "helper.go", Instruction: "Record the test result"},
	}
	if err := SaveCheckpoint(ws, cp); err != nil {
		t.Fatal(err)
	}
	mp := mock.New(`{"old_content":"return \"old\"","new_content":"return \"tests passed\""}`)
	ex := New(ws, mp, mp, "arch", "reflex", 1)
	ex.Trusted = true
	ex.DelegateBudget = -1
	report, err := ex.Resume(context.Background(), cp)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Steps) < 2 || !strings.Contains(report.Steps[0].Message, "verification ran: passed") {
		t.Fatalf("mid-plan verify did not run: %+v", report.Steps)
	}
	if len(mp.Requests) == 0 || !strings.Contains(mp.Requests[0].Prompt, "Verification run before step 2 (passed)") || !strings.Contains(mp.Requests[0].Prompt, "[test passed]") {
		t.Fatalf("later step did not see the verification results")
	}
}
