package desktop

import (
	"context"
	"encoding/json"
	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
