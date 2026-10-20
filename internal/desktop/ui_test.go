package desktop

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func readDistFile(t *testing.T, name string) string {
	t.Helper()
	distFS, err := webDistFS()
	if err != nil {
		t.Fatal(err)
	}
	data, err := fs.ReadFile(distFS, name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func TestEmbeddedDistIndexAndFonts(t *testing.T) {
	index := readDistFile(t, "index.html")
	if strings.Count(index, "__APW_TOKEN__") != 1 || !strings.Contains(index, `<meta name="apw-token" content="__APW_TOKEN__"`) {
		t.Fatalf("index.html must inject __APW_TOKEN__ via <meta name=\"apw-token\">: %s", index)
	}
	// No inline scripts: all scripts must load from /assets/.
	reScript := regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)
	for _, m := range reScript.FindAllStringSubmatch(index, -1) {
		attrs, body := m[1], strings.TrimSpace(m[2])
		if !strings.Contains(attrs, "src=") || body != "" {
			t.Fatalf("index.html contains an inline script: %q", m[0])
		}
	}

	distFS, err := webDistFS()
	if err != nil {
		t.Fatal(err)
	}
	var hasLatinFont, hasThaiFont, hasAppJS, hasCSS bool
	err = fs.WalkDir(distFS, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		switch {
		case strings.Contains(path, "noto-sans-latin") && strings.HasSuffix(path, ".woff2"):
			hasLatinFont = true
		case strings.Contains(path, "noto-sans-thai") && strings.HasSuffix(path, ".woff2"):
			hasThaiFont = true
		case path == "assets/app.js":
			hasAppJS = true
		case strings.HasSuffix(path, ".css"):
			hasCSS = true
		}
		if strings.HasSuffix(path, ".html") || strings.HasSuffix(path, ".css") {
			text := readDistFile(t, path)
			for _, remote := range []string{"https://", "http://", "@import"} {
				if strings.Contains(text, remote) {
					t.Errorf("%s must not reference remote URL %q", path, remote)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasLatinFont || !hasThaiFont {
		t.Fatalf("bundled Noto Sans / Noto Sans Thai woff2 fonts missing (latin=%v thai=%v)", hasLatinFont, hasThaiFont)
	}
	if !hasAppJS || !hasCSS {
		t.Fatalf("bundled app.js or CSS missing (js=%v css=%v)", hasAppJS, hasCSS)
	}
}

func TestEmbeddedAppBundleContractAndGuards(t *testing.T) {
	appJS := readDistFile(t, "assets/app.js")
	for _, want := range []string{
		"/api/status", "/api/models", "/api/trust", "/api/instructions",
		"/api/sessions", "/api/approvals", "/api/search?q=", "/api/file?path=",
		"/api/tasks/", "/v1/tasks", "BrowserOpenURL", "ChooseWorkspace",
		"F5", "F12", "KeyO", "KeyR", "dragover", "drop",
	} {
		if !strings.Contains(appJS, want) {
			t.Errorf("assets/app.js lacks expected contract/guard token %q", want)
		}
	}

	for _, bad := range []string{
		"CLAUDE.md", "AGENT.md", "/api/approvals/request", "/api/agent-md",
		"https://fonts.googleapis.com", "https://cdn.",
	} {
		if strings.Contains(appJS, bad) {
			t.Errorf("assets/app.js must not contain %q", bad)
		}
	}
}
