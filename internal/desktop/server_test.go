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
}

func mustQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
