package research

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/config"
)

// testClient routes every allowed https host to srv while keeping the URL and redirect rules.
func testClient(t *testing.T, srv *httptest.Server, hosts ...string) *Client {
	t.Helper()
	c := New(config.ResearchConfig{Enabled: true, Hosts: hosts})
	c.http = c.newHTTP()
	transport := c.http.Transport.(*http.Transport)
	addr := srv.Listener.Addr().String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test server
	return c
}

func TestCheckURL(t *testing.T) {
	c := New(config.ResearchConfig{Enabled: true, Hosts: []string{"docs.example.com"}})
	for raw, ok := range map[string]bool{
		"https://pkg.go.dev/net/http":                    true,
		"https://docs.example.com/guide#section":         true,
		"https://sub.readthedocs.io/en/latest/":          true,
		"http://pkg.go.dev/net/http":                     false,
		"https://pkg.go.dev/search?q=secret":             false,
		"https://pkg.go.dev/?":                           false,
		"https://user:pw@pkg.go.dev/":                    false,
		"https://pkg.go.dev:8443/":                       false,
		"https://evil.example/":                          false,
		"https://pkg.go.dev.evil.example/":               false,
		"https://github.com/owner/repo":                  false,
		"https://127.0.0.1/":                             false,
		"https://pkg.go.dev/" + strings.Repeat("a", 400): false,
	} {
		_, err := c.checkURL(raw)
		if (err == nil) != ok {
			t.Errorf("checkURL(%q) err=%v want ok=%v", raw, err, ok)
		}
	}
	if u, err := c.checkURL("https://docs.example.com/guide#section"); err != nil || u.String() != "https://docs.example.com/guide" {
		t.Fatalf("fragment kept: %v %v", u, err)
	}
}

func TestFetchExtractsTextAndEnforcesRules(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
				http.Error(w, "credentials sent", 400)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><head><script>steal()</script></head><body><h1>useState</h1><p>Returns a stateful value.</p></body></html>`))
		case "/pkg":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"left-pad","readme":"# left-pad\nPads strings."}`))
		case "/binary":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte{0, 1, 2})
		case "/big":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(strings.Repeat("x", 200*1024)))
		case "/redirect-out":
			http.Redirect(w, r, "https://evil.example/collect", http.StatusFound)
		case "/redirect-in":
			http.Redirect(w, r, "https://docs.test/page", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := testClient(t, srv, "docs.test")
	ctx := context.Background()
	if s := c.Fetch(ctx, "https://docs.test/page"); s.Error != "" || s.Text != "useState Returns a stateful value." {
		t.Fatalf("html = %+v", s)
	}
	if s := c.Fetch(ctx, "https://docs.test/pkg"); s.Error != "" || !strings.Contains(s.Text, "Pads strings") || strings.Contains(s.Text, `"name"`) {
		t.Fatalf("registry readme = %+v", s)
	}
	if s := c.Fetch(ctx, "https://docs.test/redirect-in"); s.Error != "" || !strings.Contains(s.Text, "useState") {
		t.Fatalf("allowed redirect = %+v", s)
	}
	for _, path := range []string{"/binary", "/redirect-out", "/missing"} {
		if s := c.Fetch(ctx, "https://docs.test"+path); s.Error == "" {
			t.Fatalf("%s accepted: %+v", path, s)
		}
	}
	if s := c.Fetch(ctx, "https://docs.test/big"); s.Error != "" || len(s.Text) > maxSourceText+20 || !strings.HasSuffix(s.Text, "[truncated]") {
		t.Fatalf("big page not bounded: %d %q", len(s.Text), s.Error)
	}
}

func TestFetchRefusesPrivateAddresses(t *testing.T) {
	c := New(config.ResearchConfig{Enabled: true, Hosts: []string{"localhost"}})
	s := c.Fetch(context.Background(), "https://localhost/secret")
	if s.Error == "" || !strings.Contains(s.Error, "non-public") {
		t.Fatalf("loopback fetch = %+v", s)
	}
	for ip, public := range map[string]bool{"8.8.8.8": true, "10.0.0.1": false, "192.168.1.1": false, "169.254.169.254": false, "100.64.0.1": false, "::1": false, "fe80::1": false} {
		if publicIP(net.ParseIP(ip)) != public {
			t.Errorf("publicIP(%s) != %v", ip, public)
		}
	}
}

func TestSearchAndRun(t *testing.T) {
	var gotKey, gotQuery string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			gotKey, gotQuery = r.Header.Get("X-Subscription-Token"), r.URL.Query().Get("q")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"web":{"results":[{"title":"Vite <b>config</b>","url":"https://vite.dev/config/","description":"Configure Vite"}]}}`))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("doc " + r.URL.Path))
	}))
	defer srv.Close()
	t.Setenv("TEST_BRAVE_KEY", "k-123")
	c := New(config.ResearchConfig{Enabled: true, Hosts: []string{"docs.test"}, Search: "brave", SearchKeyEnv: "TEST_BRAVE_KEY", MaxSources: 3})
	routed := testClient(t, srv, "docs.test")
	c.http = routed.http
	c.searchURL = "https://docs.test/search"
	sources := c.Run(context.Background(), Request{Fetch: []string{"https://docs.test/a", "https://docs.test/b"}, Search: []string{"vite config outDir", "ignored: over the limit"}})
	if len(sources) != 3 || sources[2].Query != "vite config outDir" || gotKey != "k-123" || gotQuery != "vite config outDir" {
		t.Fatalf("sources = %+v key=%q query=%q", sources, gotKey, gotQuery)
	}
	if !strings.Contains(sources[2].Text, "Vite config <https://vite.dev/config/>: Configure Vite") {
		t.Fatalf("search text = %q", sources[2].Text)
	}
	ctxText := Context(sources)
	if !strings.Contains(ctxText, "UNTRUSTED") || !strings.Contains(ctxText, "doc /a") || !strings.Contains(ctxText, "web search: vite config outDir") {
		t.Fatalf("context = %s", ctxText)
	}
	if !strings.Contains(c.Prompt("task", "overview"), `"search": ["short web search query"`) {
		t.Fatal("prompt does not offer search")
	}
}

func TestParseRequest(t *testing.T) {
	req, err := ParseRequest("Sure:\n```json\n{\"fetch\":[\"https://react.dev/reference/react/useState\"],\"search\":[]}\n```")
	if err != nil || len(req.Fetch) != 1 {
		t.Fatalf("request = %+v %v", req, err)
	}
	if _, err := ParseRequest("no json here"); err == nil {
		t.Fatal("accepted non-JSON")
	}
	if New(config.ResearchConfig{}) != nil {
		t.Fatal("disabled research returned a client")
	}
	if Context([]Source{{URL: "x", Error: "failed"}}) != "" {
		t.Fatal("failed sources produced context")
	}
}
