package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestServer_Endpoints(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module example.com/ui\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "main.go"), []byte("package main\n\nfunc Hello() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(ws, "apw.json")
	cfgJSON := `{"workspace": ` + mustQuote(ws) + `, "providers": {"m": {"type": "mock", "model": "APW Engine"}}, "roles": {"architect": "m", "reflex": "m"}}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := app.Open(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	srv := NewServer(cfgPath, "0.1.0-alpha")
	srv.Backend = backend
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	get := func(path string) (*http.Response, error) {
		req, _ := http.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+srv.Token)
		return http.DefaultClient.Do(req)
	}
	post := func(path, body string) (*http.Response, error) {
		req, _ := http.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+srv.Token)
		req.Header.Set("Content-Type", "application/json")
		return http.DefaultClient.Do(req)
	}

	// 1. Index HTML
	resp, err := get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d", resp.StatusCode)
	}

	// 2. Status API
	resp, err = get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var st statusPayload
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Brain1Name != "Reflex Engine" || st.DaemonRunning {
		t.Errorf("unexpected status payload: %+v", st)
	}

	// 3. Plan API
	resp2, err := post(ts.URL+"/api/plan", `{"instruction":"add feature"}`)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/plan status = %d", resp2.StatusCode)
	}

	// 4. Instructions init & status detection
	respInit, err := post(ts.URL+"/api/instructions/init", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	respInit.Body.Close()
	if respInit.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/instructions/init status = %d", respInit.StatusCode)
	}

	respStatus2, err := get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer respStatus2.Body.Close()
	var st2 statusPayload
	if err := json.NewDecoder(respStatus2.Body).Decode(&st2); err != nil {
		t.Fatal(err)
	}
	if !st2.InstructionsLoaded || st2.InstructionsPath != "APW.md" || st2.InstructionsLines == 0 {
		t.Fatalf("expected APW.md to be loaded in status, got %+v", st2)
	}

	// 5. A new conversation id does not create a session file; the list stays empty.
	respSess, err := post(ts.URL+"/api/sessions", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	respSess.Body.Close()
	if respSess.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/sessions status = %d", respSess.StatusCode)
	}

	respListSess, err := get(ts.URL + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer respListSess.Body.Close()
	var sessions []sessionSummary
	if err := json.NewDecoder(respListSess.Body).Decode(&sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no sessions before a task is submitted, got %d", len(sessions))
	}
}

func TestApprovalGatekeeper_Decisions(t *testing.T) {
	gk := NewApprovalGatekeeper()

	// 1. "once" (อนุญาตครั้งเดียว) releases the waiter with true, but does not persist always-allow.
	reqOnce, chOnce := gk.EnqueueApproval("task-1", "SystemCommand", "go test ./...", ".", "run tests")
	if reqOnce.Status != "pending" {
		t.Fatalf("expected pending status, got %s", reqOnce.Status)
	}
	if _, err := gk.Resolve(reqOnce.ID, DecisionOnce); err != nil {
		t.Fatal(err)
	}
	if allowed := <-chOnce; !allowed {
		t.Fatal("expected DecisionOnce to send true to the waiter")
	}
	if gk.IsAlwaysAllowed("SystemCommand", "go test ./...") {
		t.Fatal("expected DecisionOnce not to mark command as always allowed")
	}

	// 2. "deny" (ปฏิเสธ) releases the waiter with false.
	reqDeny, chDeny := gk.EnqueueApproval("task-2", "SystemCommand", "rm -rf /tmp/x", ".", "cleanup")
	if _, err := gk.Resolve(reqDeny.ID, DecisionDeny); err != nil {
		t.Fatal(err)
	}
	if allowed := <-chDeny; allowed {
		t.Fatal("expected DecisionDeny to send false to the waiter")
	}

	// 3. "always" (อนุญาตเสมอสำหรับคำสั่งนี้) releases the waiter with true AND auto-approves next call.
	reqAlways, chAlways := gk.EnqueueApproval("task-3", "SystemCommand", "go vet ./...", ".", "vet code")
	if _, err := gk.Resolve(reqAlways.ID, DecisionAlways); err != nil {
		t.Fatal(err)
	}
	if allowed := <-chAlways; !allowed {
		t.Fatal("expected DecisionAlways to send true to the waiter")
	}
	if !gk.IsAlwaysAllowed("SystemCommand", "go vet ./...") {
		t.Fatal("expected DecisionAlways to persist in alwaysAllow map")
	}

	// Subsequent RequestApproval for the same command returns immediately without blocking.
	allowed, err := gk.RequestApproval(context.Background(), "task-4", "SystemCommand", "go vet ./...", ".", "vet again")
	if err != nil || !allowed {
		t.Fatalf("expected immediate auto-approval for always-allowed command, got allowed=%v err=%v", allowed, err)
	}
}

func mustQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestResolveInvalidDecisionKeepsRequestPending(t *testing.T) {
	gk := NewApprovalGatekeeper()
	req, ch := gk.EnqueueApproval("task-1", applyTool, "apply task task-1", ".", "apply")
	if _, err := gk.Resolve(req.ID, ApprovalDecisionType("maybe")); err == nil {
		t.Fatal("invalid decision accepted")
	}
	if len(gk.ListPending()) != 1 {
		t.Fatal("invalid decision removed the pending request")
	}
	if _, err := gk.Resolve(req.ID, DecisionDeny); err != nil {
		t.Fatal(err)
	}
	if allowed := <-ch; allowed {
		t.Fatal("deny released the caller with true")
	}
}

func TestIndexServesStrictCSP(t *testing.T) {
	s := NewServer(filepath.Join(t.TempDir(), "apw.json"), "test")
	rec := httptest.NewRecorder()
	s.handleIndex(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "script-src 'self'", "font-src 'self'", "connect-src 'self'", "frame-ancestors 'none'", "form-action 'none'"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP %q lacks %q", csp, want)
		}
	}
	if strings.Contains(csp, "script-src 'unsafe-inline'") || strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("CSP must not allow inline scripts: %q", csp)
	}
	if strings.Contains(rec.Body.String(), "__APW_TOKEN__") || !strings.Contains(rec.Body.String(), `<meta name="apw-token" content="`+s.Token+`"`) {
		t.Fatalf("token not injected into <meta name=\"apw-token\">: %s", rec.Body.String())
	}
}

func TestHandleSearchAndPolyglotSymbols(t *testing.T) {
	ws := t.TempDir()
	cfgPath := filepath.Join(ws, "apw.json")
	cfgJSON := `{"workspace": ` + mustQuote(ws) + `, "providers": {"m": {"type": "mock", "model": "APW Engine"}}, "roles": {"architect": "m", "reflex": "m"}}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "src", "widget.ts"), []byte("export function renderWidget() {\n  return 1;\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".env"), []byte("SECRET=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(cfgPath, "test")

	rec := httptest.NewRecorder()
	srv.handleSearch(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=widget", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("handleSearch status = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "src/widget.ts") || !strings.Contains(body, "renderWidget") || strings.Contains(body, ".env") {
		t.Fatalf("unexpected search payload:\n%s", body)
	}

	recSym := httptest.NewRecorder()
	srv.handleSymbols(recSym, httptest.NewRequest(http.MethodGet, "/api/symbols", nil))
	if recSym.Code != http.StatusOK || !strings.Contains(recSym.Body.String(), "renderWidget") {
		t.Fatalf("handleSymbols = %d: %s", recSym.Code, recSym.Body.String())
	}
}

// An old file in a workspace with more than 100 files must still be found by its name:
// the search is filtered server-side, not capped before the query is applied.
func TestHandleSearchFindsOldFileBeyondFirstHundred(t *testing.T) {
	ws := t.TempDir()
	cfgPath := filepath.Join(ws, "apw.json")
	cfgJSON := `{"workspace": ` + mustQuote(ws) + `, "providers": {"m": {"type": "mock", "model": "APW Engine"}}, "roles": {"architect": "m", "reflex": "m"}}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(ws, "legacy_target.txt")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 150; i++ {
		if err := os.WriteFile(filepath.Join(ws, fmt.Sprintf("recent_%03d.txt", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv := NewServer(cfgPath, "test")

	rec := httptest.NewRecorder()
	srv.handleSearch(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=legacy_tar", nil))
	var got searchPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("handleSearch = %d %v: %s", rec.Code, err, rec.Body.String())
	}
	if len(got.Files) == 0 || got.Files[0] != "legacy_target.txt" {
		t.Fatalf("old file not found first: %v", got.Files)
	}

	rec = httptest.NewRecorder()
	srv.handleSearch(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=skill:any", nil))
	got = searchPayload{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 0 || len(got.Symbols) != 0 {
		t.Fatalf("skill: query must not list files or symbols: %+v", got)
	}
}

// A skill title derived from a long Thai instruction stays valid UTF-8, and a second
// skill with the same slug is refused instead of overwriting the first.
func TestHandleCreateSkillThaiTitleAndNoOverwrite(t *testing.T) {
	ws := t.TempDir()
	cfgPath := filepath.Join(ws, "apw.json")
	cfgJSON := `{"workspace": ` + mustQuote(ws) + `, "providers": {"m": {"type": "mock", "model": "APW Engine"}}, "roles": {"architect": "m", "reflex": "m"}}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(cfgPath, "test")
	instr := "x" + strings.Repeat("ปรับปรุงหน้าตั้งค่า", 10)
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/autonomy/skills", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.handleCreateSkill(rec, req)
		return rec
	}
	body := `{"slug":"thai-skill","instruction":` + mustQuote(instr) + `}`
	if rec := post(body); rec.Code != http.StatusOK {
		t.Fatalf("create skill = %d: %s", rec.Code, rec.Body.String())
	}
	raw, err := os.ReadFile(filepath.Join(ws, ".apw", "skills", "thai-skill", "SKILL.md"))
	if err != nil || !utf8.Valid(raw) {
		t.Fatalf("SKILL.md invalid UTF-8 or missing: %v", err)
	}
	if rec := post(`{"slug":"thai-skill","instruction":"replace it"}`); rec.Code != http.StatusConflict {
		t.Fatalf("overwrite = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if after, _ := os.ReadFile(filepath.Join(ws, ".apw", "skills", "thai-skill", "SKILL.md")); string(after) != string(raw) {
		t.Fatal("existing skill was overwritten")
	}
}

// Browsers request /favicon.ico on their own, without the API token; answering 401 logs a
// console error on every load. It carries no data, so it is answered without the token.
func TestFaviconNeedsNoToken(t *testing.T) {
	srv := NewServer(filepath.Join(t.TempDir(), "apw.json"), "test")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/favicon.ico", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("favicon = %d, want 204", rec.Code)
	}
	r := httptest.NewRequest(http.MethodGet, "http://evil.example/favicon.ico", nil)
	r.Host = "evil.example"
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("favicon from a foreign host = %d, want 403", rec.Code)
	}
}

func TestInProcessHostTrustedOnlyWhenConfigured(t *testing.T) {
	srv := NewServer(filepath.Join(t.TempDir(), "apw.json"), "test")
	srv.Backend = &stubBackend{}
	req := func(path string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "http://wails.localhost"+path, nil)
		r.Host = "wails.localhost"
		return r
	}
	for _, path := range []string{"/", "/api/status", "/v1/health"} {
		r := req(path)
		r.Header.Set("Authorization", "Bearer "+srv.Token)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, r)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("unset InProcessHost %s: got %d, want 403", path, rec.Code)
		}
	}
	srv.InProcessHost = WailsHost
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req("/"))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatalf("in-process index: %d csp=%q", rec.Code, rec.Header().Get("Content-Security-Policy"))
	}
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req("/api/status"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("in-process API without token: got %d, want 401", rec.Code)
	}
	// /v1/ goes through the transport handler's own guard; it must trust the in-process host too.
	v1 := req("/v1/health")
	v1.Header.Set("Authorization", "Bearer "+srv.Token)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, v1)
	if rec.Code != http.StatusOK {
		t.Fatalf("in-process /v1/health with token: got %d %s", rec.Code, rec.Body.String())
	}
	// A foreign Origin on the in-process host is still rejected.
	evil := req("/api/status")
	evil.Header.Set("Origin", "http://attacker.example")
	evil.Header.Set("Authorization", "Bearer "+srv.Token)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, evil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign origin: got %d, want 403", rec.Code)
	}
}
