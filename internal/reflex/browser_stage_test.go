package reflex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/browser"
	"github.com/apiwat/apw-agent/internal/project"
)

func TestBrowserStage(t *testing.T) {
	if _, ok := browser.Find(); !ok {
		t.Skip("no headless browser installed")
	}
	root := t.TempDir()
	profile := project.Profile{Kind: "node", Stages: []project.Stage{{Name: "browser", Builtin: project.BuiltinBrowser, Dir: "dist"}}}
	verify := func() *VerificationResult { return NewVerifierForProfile(root, profile).Verify(context.Background()) }
	if res := verify(); !res.Passed {
		t.Fatalf("missing build output must skip the browser stage: %+v", res)
	}
	page := func(body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, "dist"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "dist", "index.html"), []byte("<!doctype html><body><div id=\"root\"></div><script>"+body+"</script></body>"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	page(`document.getElementById("root").textContent = "Hello"; Promise.reject(new Error("api offline"));`)
	if res := verify(); !res.Passed {
		t.Fatalf("rendering page failed: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, BrowserScreenshot)); err != nil {
		t.Fatalf("screenshot missing: %v", err)
	}
	page(`document.getElementById("root").textContent = "Hello"; throw new Error("boom in render");`)
	res := verify()
	if res.Passed || res.FailedStage != "browser" || len(res.Diagnostics) != 1 || !strings.Contains(res.Diagnostics[0].Message, "boom in render") || res.Diagnostics[0].File != "dist/index.html" {
		t.Fatalf("uncaught error not reported: %+v", res)
	}
	page(``)
	if res := verify(); res.Passed || !strings.Contains(res.RawOutput, "blank page") {
		t.Fatalf("blank page not reported: %+v", res)
	}
}
