package browser

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestVisibleText(t *testing.T) {
	dom := `<html><head><title>x</title><style>.a{}</style></head><body><div id="root"><h1>Count &amp; more</h1>
<script>var hidden = "no";</script><p>is   10</p></div></body></html>`
	if got := VisibleText(dom); got != "Count & more is 10" {
		t.Fatalf("VisibleText = %q", got)
	}
}

func TestConsoleMessagesKeepOnlyPageOrigin(t *testing.T) {
	log := strings.Join([]string{
		`[1:2:1017/1.0:INFO:CONSOLE:5] "hello", source: http://127.0.0.1:5000/assets/a.js (5)`,
		`[1:2:1017/1.0:INFO:CONSOLE:9] "Uncaught TypeError: x is undefined", source: http://127.0.0.1:5000/assets/a.js (9)`,
		`[1:2:1017/1.0:INFO:CONSOLE:9] "Uncaught (in promise) Error: fetch failed", source: http://127.0.0.1:5000/ (9)`,
		`[1:2:1017/1.0:INFO:CONSOLE:1] "Uncaught extension noise", source: chrome-extension://abc/bg.js (1)`,
	}, "\n")
	errs, rejections, other := consoleMessages(log, "http://127.0.0.1:5000")
	if len(errs) != 1 || !strings.Contains(errs[0], "TypeError") || len(rejections) != 1 || len(other) != 1 {
		t.Fatalf("errs=%v rejections=%v other=%v", errs, rejections, other)
	}
}

func TestStaticHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":         {Data: []byte("<html>app</html>")},
		"assets/app.js":      {Data: []byte("console.log(1)")},
		".apw/worktree.json": {Data: []byte("{}")},
	}
	srv := httptest.NewServer(staticHandler(fsys))
	defer srv.Close()
	get := func(p string) (int, string) {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	if code, body := get("/"); code != 200 || !strings.Contains(body, "app") {
		t.Fatalf("/ = %d %q", code, body)
	}
	if code, body := get("/settings/profile"); code != 200 || !strings.Contains(body, "app") {
		t.Fatalf("SPA route = %d %q", code, body)
	}
	if code, _ := get("/missing.js"); code != 404 {
		t.Fatalf("missing asset = %d", code)
	}
	if code, _ := get("/.apw/worktree.json"); code != 404 {
		t.Fatalf("APW state served: %d", code)
	}
	resp, err := http.Post(srv.URL+"/", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d", resp.StatusCode)
	}
}

func TestVisitRendersPage(t *testing.T) {
	if _, ok := Find(); !ok {
		t.Skip("no headless browser installed")
	}
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("index.html", `<!doctype html><body><div id="root"></div><script src="/app.js"></script></body>`)
	write("app.js", `document.getElementById("root").textContent = "Count is 10"; Promise.reject(new Error("api down"));`)
	write("broken.html", `<!doctype html><body><div id="root"></div><script>throw new Error("render crashed")</script></body>`)
	shot := filepath.Join(t.TempDir(), "shots", "index.png")
	page, err := Visit(context.Background(), Options{Dir: dir, Screenshot: shot, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if page.Text != "Count is 10" || page.Blank || len(page.Errors) != 0 || len(page.Rejections) != 1 || page.Screenshot != shot {
		t.Fatalf("page = %+v", page)
	}
	broken, err := Visit(context.Background(), Options{Dir: dir, Path: "/broken.html"})
	if err != nil {
		t.Fatal(err)
	}
	if !broken.Blank || len(broken.Errors) != 1 || !strings.Contains(broken.Errors[0], "render crashed") {
		t.Fatalf("broken page = %+v", broken)
	}
	if _, err := Visit(context.Background(), Options{Dir: dir, Path: "https://example.com/"}); err == nil {
		t.Fatal("external URL accepted")
	}
}
