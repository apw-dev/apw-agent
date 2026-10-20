package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/desktop"
)

func inProcessGet(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "http://"+desktop.WailsHost+target, nil)
	r.Host = desktop.WailsHost
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func writeConfig(t *testing.T, dir string) {
	t.Helper()
	cfg := `{"workspace": ` + strconv.Quote(dir) + `, "providers": {"m": {"type": "mock", "model": "APW Engine"}}, "roles": {"architect": "m", "reflex": "m"}}`
	if err := os.WriteFile(filepath.Join(dir, "apw.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceHostServesChooserUntilOpened(t *testing.T) {
	h := NewWorkspaceHost()
	rec := inProcessGet(t, h, "/")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ChooseWorkspace") {
		t.Fatalf("chooser = %d %s", rec.Code, rec.Body.String())
	}
	if h.Current() != "" {
		t.Fatalf("Current = %q before Open", h.Current())
	}
}

func TestWorkspaceHostOpenRejectsMissingConfig(t *testing.T) {
	h := NewWorkspaceHost()
	if err := h.Open(t.TempDir()); !errors.Is(err, ErrNoConfig) {
		t.Fatalf("Open without apw.json = %v, want ErrNoConfig", err)
	}
	if h.Current() != "" {
		t.Fatalf("Current = %q after failed Open", h.Current())
	}
	if rec := inProcessGet(t, h, "/"); !strings.Contains(rec.Body.String(), "ChooseWorkspace") {
		t.Fatal("chooser no longer served after failed Open")
	}
}

func TestWorkspaceHostOpenServesDesktop(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir)
	h := NewWorkspaceHost()
	if err := h.Open(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if h.Current() != dir {
		t.Fatalf("Current = %q, want %q", h.Current(), dir)
	}
	rec := inProcessGet(t, h, "/")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "apw-token") {
		t.Fatalf("desktop index = %d", rec.Code)
	}
	if rec := inProcessGet(t, h, "/api/status"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/status without token = %d, want 401", rec.Code)
	}
}

// A previously opened workspace keeps serving when a later Open fails.
func TestWorkspaceHostFailedOpenKeepsCurrent(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir)
	h := NewWorkspaceHost()
	if err := h.Open(dir); err != nil {
		t.Fatal(err)
	}
	if err := h.Open(t.TempDir()); !errors.Is(err, ErrNoConfig) {
		t.Fatalf("second Open = %v", err)
	}
	if h.Current() != dir {
		t.Fatalf("Current = %q, want %q", h.Current(), dir)
	}
}

// When the last workspace can no longer be opened at startup, the chooser says why.
func TestWorkspaceHostChooserShowsNotice(t *testing.T) {
	h := NewWorkspaceHost()
	h.SetNotice(`เปิด workspace เดิมไม่ได้: <D:\gone>`)
	body := inProcessGet(t, h, "/").Body.String()
	if !strings.Contains(body, "เปิด workspace เดิมไม่ได้") || !strings.Contains(body, `&lt;D:\gone&gt;`) {
		t.Fatalf("notice missing or unescaped: %s", body)
	}
}

func TestWorkspaceHostChooserEscapesError(t *testing.T) {
	rec := inProcessGet(t, NewWorkspaceHost(), "/?error=%3Cscript%3Ealert(1)%3C%2Fscript%3E")
	body := rec.Body.String()
	if strings.Contains(body, "<script>alert(1)") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("error not escaped: %s", body)
	}
}

func TestWorkspaceHostReopenSameDirKeepsServer(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir)
	h := NewWorkspaceHost()
	if err := h.Open(dir); err != nil {
		t.Fatal(err)
	}
	first := inProcessGet(t, h, "/").Body.String()
	if err := h.Open(dir + string(filepath.Separator)); err != nil {
		t.Fatal(err)
	}
	second := inProcessGet(t, h, "/").Body.String()
	if first != second {
		t.Fatal("reopening the same workspace replaced the desktop.Server (token changed)")
	}
}
