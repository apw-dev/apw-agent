package browser

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeBrowserEnv turns the test binary into a stand-in for a headless browser whose shutdown never
// finishes, as Edge's does when its background-priority shutdown threads starve on a busy machine.
const fakeBrowserEnv = "APW_FAKE_BROWSER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeBrowserEnv); mode != "" {
		fakeBrowser(mode, os.Args[1:])
		return
	}
	os.Exit(m.Run())
}

// fakeBrowser mimics `--dump-dom [--screenshot=f] url` and then hangs. Modes: "complete" writes the
// screenshot and the whole DOM, "no-screenshot" skips the screenshot, "partial" writes half a DOM.
func fakeBrowser(mode string, args []string) {
	url, shot := args[len(args)-1], ""
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, "--screenshot="); ok {
			shot = v
		}
	}
	resp, err := http.Get(url)
	if err != nil {
		os.Exit(4)
	}
	dom, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	fmt.Fprintf(os.Stderr, "[1:2:1020/1.0:INFO:CONSOLE:1] \"Uncaught Error: late\", source: %s (1)\n", url)
	if shot != "" && mode == "complete" {
		if os.WriteFile(shot, []byte("png"), 0600) != nil {
			os.Exit(5)
		}
		fmt.Fprintf(os.Stderr, "[1:2:1020/1.0:INFO:headless_command_handler.cc:1] 3 bytes written to file %s\n", shot)
	}
	if mode == "partial" {
		dom = dom[:len(dom)/2]
	}
	os.Stdout.Write(append(dom, '\n'))
	time.Sleep(time.Hour)
}

func TestVisitStopsBrowserOnceOutputIsComplete(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><html><head></head><body><p>Count is 10</p></body></html>"), 0644); err != nil {
		t.Fatal(err)
	}
	shot := filepath.Join(t.TempDir(), "index.png")
	t.Setenv(fakeBrowserEnv, "complete")
	start := time.Now()
	page, err := Visit(context.Background(), Options{Browser: bin, Dir: dir, Screenshot: shot, Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("Visit waited %v for a browser that had already written its page", elapsed)
	}
	if page.Text != "Count is 10" || page.Screenshot != shot || len(page.Errors) != 1 {
		t.Fatalf("page = %+v", page)
	}
	for _, mode := range []string{"no-screenshot", "partial"} {
		t.Setenv(fakeBrowserEnv, mode)
		if _, err := Visit(context.Background(), Options{Browser: bin, Dir: dir, Screenshot: shot + "." + mode, Timeout: 2 * time.Second}); err == nil {
			t.Fatalf("%s: unfinished output accepted", mode)
		}
	}
}
