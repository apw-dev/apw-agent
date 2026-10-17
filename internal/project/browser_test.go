package project

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/browser"
)

// TestMain hides any installed browser so pipelines are deterministic; browser tests opt back in.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "apw-project-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv(browser.Env, filepath.Join(dir, "no-browser"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func fakeBrowser(t *testing.T) {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(exe, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(browser.Env, exe)
}

func TestBrowserStageForWebApps(t *testing.T) {
	fakeBrowser(t)
	vite := t.TempDir()
	write(t, vite, map[string]string{"package.json": `{"scripts":{"build":"vite build","test":"vitest"},"dependencies":{"react":"^19"},"devDependencies":{"vite":"^6"}}`})
	p := Detect(vite, nil)
	if got := p.StageNames(); !reflect.DeepEqual(got, []string{"install", "build", "browser", "test"}) {
		t.Fatalf("vite stages = %v", got)
	}
	if s, ok := p.BrowserStage(); !ok || s.Dir != "dist" || !strings.Contains(p.Describe(), `kind "browser"`) {
		t.Fatalf("browser stage = %+v %v\n%s", s, ok, p.Describe())
	}
	if got := p.WithoutBrowser().StageNames(); !reflect.DeepEqual(got, []string{"install", "build", "test"}) {
		t.Fatalf("without browser = %v", got)
	}
	cra := t.TempDir()
	write(t, cra, map[string]string{"package.json": `{"scripts":{"build":"react-scripts build"},"dependencies":{"react":"^18","react-scripts":"5"}}`})
	if s, ok := Detect(cra, nil).BrowserStage(); !ok || s.Dir != "build" {
		t.Fatalf("CRA browser stage = %+v %v", s, ok)
	}
	html := t.TempDir()
	write(t, html, map[string]string{"index.html": "<html></html>"})
	if s, ok := Detect(html, nil).BrowserStage(); !ok || s.Dir != "." {
		t.Fatalf("static browser stage = %+v %v", s, ok)
	}
	for name, files := range map[string]map[string]string{
		"next":     {"package.json": `{"scripts":{"build":"next build"},"dependencies":{"react":"^19","next":"^15"}}`},
		"api":      {"package.json": `{"scripts":{"build":"tsc"},"dependencies":{"express":"^5"}}`},
		"no build": {"package.json": `{"scripts":{"test":"vitest"},"dependencies":{"react":"^19"}}`},
	} {
		root := t.TempDir()
		write(t, root, files)
		if _, ok := Detect(root, nil).BrowserStage(); ok {
			t.Fatalf("%s project got a browser stage", name)
		}
	}
}
