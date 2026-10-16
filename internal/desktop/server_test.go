package desktop

import (
	"encoding/json"
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
	cfgJSON := `{"workspace": ` + mustQuote(ws) + `, "providers": {"m": {"type": "mock", "model": "Jev Engine"}}, "roles": {"architect": "m", "reflex": "m"}}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(cfgPath, "0.1.0-alpha")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1. Index HTML
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d", resp.StatusCode)
	}

	// 2. Status API
	resp, err = http.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var st statusPayload
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Brain1Name != "Jev Engine" || !st.DaemonRunning {
		t.Errorf("unexpected status payload: %+v", st)
	}

	// 3. Plan API
	resp2, err := http.Post(ts.URL+"/api/plan", "application/json", strings.NewReader(`{"instruction":"add feature"}`))
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
