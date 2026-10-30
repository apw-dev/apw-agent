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

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/desktop"
)

func init() {
	// Never run account CLIs (agy, claude, codex) from tests.
	discoverProviders = func(string) ([]string, error) { return nil, nil }
}

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

func TestBootstrapWorkspaceInitializesConfigAndGit(t *testing.T) {
	tmplDir := t.TempDir()
	writeConfig(t, tmplDir)
	_ = os.MkdirAll(filepath.Join(tmplDir, ".apw"), 0o700)
	_ = os.WriteFile(filepath.Join(tmplDir, ".apw", "auth.json"), []byte(`{"TEST_KEY":"secret"}`), 0o600)

	targetDir := t.TempDir()
	if err := BootstrapWorkspace(targetDir, tmplDir); err != nil {
		t.Fatalf("BootstrapWorkspace: %v", err)
	}

	h := NewWorkspaceHost()
	if err := h.Open(targetDir); err != nil {
		t.Fatalf("Open bootstrapped workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, ".apw", "auth.json")); err != nil {
		t.Fatalf("expected auth.json to be copied to bootstrapped workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, ".git")); err != nil {
		t.Fatalf("expected .git to be initialized in bootstrapped workspace: %v", err)
	}
}

func TestBootstrapWorkspaceDoesNotTrustNewFolders(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	tmplDir := t.TempDir()
	trusted := `{"workspace": ".", "policy": {"trusted": true}, "providers": {"m": {"type": "mock", "model": "APW Engine"}}, "roles": {"architect": "m", "reflex": "m"}}`
	if err := os.WriteFile(filepath.Join(tmplDir, "apw.json"), []byte(trusted), 0o600); err != nil {
		t.Fatal(err)
	}
	fromTemplate, fromDefault := t.TempDir(), t.TempDir()
	if err := BootstrapWorkspace(fromTemplate, tmplDir); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapWorkspace(fromDefault); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{fromTemplate, fromDefault} {
		cfg, err := config.Load(filepath.Join(dir, "apw.json"))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Policy.Trusted {
			t.Fatalf("%s was trusted without the user's consent", dir)
		}
	}
	// A workspace the user already trusted keeps its trust when it is opened again.
	if err := BootstrapWorkspace(tmplDir); err != nil {
		t.Fatal(err)
	}
	if cfg, err := config.Load(filepath.Join(tmplDir, "apw.json")); err != nil || !cfg.Policy.Trusted {
		t.Fatalf("existing trust lost: %+v %v", cfg, err)
	}
}

func TestWorkspaceHostKeepsMultipleWorkspacesAlive(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	writeConfig(t, dirA)
	writeConfig(t, dirB)
	h := NewWorkspaceHost()

	if err := h.Open(dirA); err != nil {
		t.Fatal(err)
	}
	indexA1 := inProcessGet(t, h, "/").Body.String()

	if err := h.Open(dirB); err != nil {
		t.Fatal(err)
	}
	indexB := inProcessGet(t, h, "/").Body.String()
	// Shared host token allows in-place project switching without reloading the WebView.
	if indexA1 != indexB {
		t.Fatal("expected shared session token across workspaces in the same WorkspaceHost")
	}

	// Switching back to dirA should reuse the existing server.
	if err := h.Open(dirA); err != nil {
		t.Fatal(err)
	}
	if h.Current() != dirA {
		t.Fatalf("Current = %q, want %q", h.Current(), dirA)
	}
}

func TestListProjectFoldersAndNewWindowArgs(t *testing.T) {
	root := t.TempDir()
	dirA := filepath.Join(root, "alpha-proj")
	dirEmpty := filepath.Join(root, "beta-proj")
	sessDir := filepath.Join(dirA, ".apw", "sessions")
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dirEmpty, ".apw", "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	sessJSON := `{"id":"sess-1","owner":"local","conversation":"conv-1","updated_at":"2026-10-28T10:00:00Z","history":[{"instruction":"Refactor pipeline"}]}`
	if err := os.WriteFile(filepath.Join(sessDir, "sess-1.json"), []byte(sessJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	projects := ListProjectFolders(dirA, []string{dirEmpty, dirA})
	if len(projects) != 2 || !projects[0].Active || len(projects[0].Sessions) != 1 {
		t.Fatalf("unexpected projects: %+v", projects)
	}
	if projects[1].Sessions == nil {
		t.Fatal("expected non-nil empty Sessions slice for folder with empty .apw/sessions")
	}
	if projects[0].Sessions[0].Title != "Refactor pipeline" {
		t.Fatalf("unexpected session title: %+v", projects[0].Sessions[0])
	}
	// Switching active workspace to dirEmpty must NOT reorder the project folders.
	projectsSwitched := ListProjectFolders(dirEmpty, []string{dirEmpty, dirA})
	if projectsSwitched[0].Path != projects[0].Path || projectsSwitched[1].Path != projects[1].Path {
		t.Fatalf("folder order jumped after switching workspace: %v vs %v", projects, projectsSwitched)
	}
	if projectsSwitched[0].Active || !projectsSwitched[1].Active {
		t.Fatalf("active flag not updated in place: %+v", projectsSwitched)
	}

	if !NewWindowRequested([]string{"--new-window", dirA}) {
		t.Fatal("expected NewWindowRequested=true for --new-window")
	}
	if got := WorkspaceArg([]string{"--new-window", dirA}); got != filepath.Clean(dirA) {
		t.Fatalf("WorkspaceArg with --new-window = %q, want %q", got, filepath.Clean(dirA))
	}
}
