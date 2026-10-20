package main

import (
	"strings"
	"testing"
)

// The startup workspace is opened before the window loads its first page, so the page never
// races the open. A bad argument falls back to the last workspace and the chooser says why.
func TestOpenInitialWorkspace(t *testing.T) {
	good, last, bad := t.TempDir(), t.TempDir(), t.TempDir()
	writeConfig(t, good)
	writeConfig(t, last)
	cases := []struct {
		name       string
		args       []string
		last       string
		wantDir    string
		wantNotice string
	}{
		{"ArgWins", []string{good}, last, good, ""},
		{"BadArgFallsBackToLast", []string{bad}, last, last, bad},
		{"BadLastShowsChooser", nil, bad, "", bad},
		{"NothingShowsChooser", nil, "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewWorkspaceHost()
			notice := OpenInitialWorkspace(h, tc.args, tc.last)
			if h.Current() != tc.wantDir {
				t.Fatalf("Current = %q, want %q", h.Current(), tc.wantDir)
			}
			if tc.wantNotice == "" && notice != "" || tc.wantNotice != "" && !strings.Contains(notice, tc.wantNotice) {
				t.Fatalf("notice = %q, want it to mention %q", notice, tc.wantNotice)
			}
			if tc.wantDir == "" && tc.wantNotice != "" {
				if body := inProcessGet(t, h, "/").Body.String(); !strings.Contains(body, "apw.json") || !strings.Contains(body, "เปิด") {
					t.Fatalf("chooser does not show the notice: %s", body)
				}
			}
			if tc.wantDir != "" && tc.wantNotice != "" {
				if got := h.TakeNotice(); !strings.Contains(got, tc.wantNotice) {
					t.Fatalf("TakeNotice = %q, want it to mention %q so domReady can report the fallback", got, tc.wantNotice)
				}
			}
		})
	}
}
