// Package browser renders a built web page in a headless Chromium-based browser (Edge, Chrome or
// Chromium) and reports what a user would see: the rendered DOM, its visible text, uncaught page
// errors and a screenshot. APW serves the build output itself from a loopback-only static server,
// so no project server code runs; page scripts run inside the browser's own sandbox.
package browser

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/process"
)

// Env overrides browser discovery with an explicit executable path.
const Env = "APW_BROWSER"

// Find locates a Chromium-based browser executable: $APW_BROWSER, well-known install locations,
// then PATH.
func Find() (string, bool) {
	if p := os.Getenv(Env); p != "" {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
		return "", false
	}
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LOCALAPPDATA"} {
			base := os.Getenv(env)
			if base == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(base, "Chromium", "Application", "chrome.exe"))
		}
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, true
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "msedge", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, true
		}
	}
	return "", false
}

// Options describes one page visit.
type Options struct {
	Browser string // executable; empty uses Find
	Dir     string // directory served at the server root (the build output)
	Path    string // URL path to open; default "/"
	// Screenshot, when set, is where a 1280x800 PNG of the page is written.
	Screenshot string
	// Offline makes every host except the local server unresolvable.
	Offline bool
	Timeout time.Duration
}

// Page is what the browser rendered.
type Page struct {
	URL        string   `json:"url"`
	Text       string   `json:"text"`
	Errors     []string `json:"errors,omitempty"`     // uncaught exceptions
	Rejections []string `json:"rejections,omitempty"` // unhandled promise rejections
	Console    []string `json:"console,omitempty"`    // other console messages from the page
	Screenshot string   `json:"screenshot,omitempty"`
	Blank      bool     `json:"blank"`
	DOM        string   `json:"-"`
}

// ErrNoBrowser reports that no supported browser is installed.
var ErrNoBrowser = errors.New("no headless browser found (install Microsoft Edge, Google Chrome or Chromium, or set " + Env + ")")

// Visit serves opts.Dir on a loopback port, renders opts.Path headlessly and returns the result.
func Visit(ctx context.Context, opts Options) (*Page, error) {
	exe := opts.Browser
	if exe == "" {
		var ok bool
		if exe, ok = Find(); !ok {
			return nil, ErrNoBrowser
		}
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	target := opts.Path
	if target == "" {
		target = "/"
	}
	if !strings.HasPrefix(target, "/") || strings.Contains(target, "://") || strings.Contains(target, "..") {
		return nil, fmt.Errorf("browser: page path %q must be a local URL path such as /about", target)
	}
	root, err := os.OpenRoot(opts.Dir)
	if err != nil {
		return nil, fmt.Errorf("browser: open build output: %w", err)
	}
	defer root.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: staticHandler(root.FS()), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	origin := "http://" + listener.Addr().String()
	url := origin + target

	profile, err := os.MkdirTemp("", "apw-browser-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(profile)
	args := []string{"--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--disable-extensions", "--disable-sync", "--disable-background-networking", "--disable-component-update",
		"--mute-audio", "--user-data-dir=" + profile, "--enable-logging=stderr", "--v=0",
		"--virtual-time-budget=5000", "--window-size=1280,800"}
	if opts.Offline {
		args = append(args, "--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE 127.0.0.1")
	}
	if opts.Screenshot != "" {
		if err := os.MkdirAll(filepath.Dir(opts.Screenshot), 0700); err != nil {
			return nil, err
		}
		args = append(args, "--screenshot="+opts.Screenshot)
	}
	args = append(args, "--dump-dom", url)
	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, exe, args...)
	stdout := process.Buffer{Limit: 8 << 20}
	stderr := process.Buffer{Limit: 1 << 20}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := process.Run(runCtx, cmd); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("browser: render %s: %w\n%s", target, err, tail(stderr.String(), 2000))
	}
	page := &Page{URL: target, DOM: stdout.String()}
	if strings.TrimSpace(page.DOM) == "" {
		return nil, fmt.Errorf("browser: %s rendered no DOM\n%s", target, tail(stderr.String(), 2000))
	}
	page.Text = VisibleText(page.DOM)
	page.Blank = page.Text == "" && !mediaPattern.MatchString(bodyOf(page.DOM))
	page.Errors, page.Rejections, page.Console = consoleMessages(stderr.String(), origin)
	if opts.Screenshot != "" {
		if _, err := os.Stat(opts.Screenshot); err == nil {
			page.Screenshot = opts.Screenshot
		}
	}
	return page, nil
}

// staticHandler serves fsys read-only, hides APW and Git state, and falls back to index.html for
// extension-less paths so client-side routes of single-page apps render.
func staticHandler(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		for _, part := range strings.Split(name, "/") {
			if part == ".git" || part == ".apw" || part == ".apw-sandbox" {
				http.NotFound(w, r)
				return
			}
		}
		if name != "" {
			if _, err := fs.Stat(fsys, name); err != nil && path.Ext(name) == "" {
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/"
				files.ServeHTTP(w, r2)
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}

var (
	consolePattern = regexp.MustCompile(`:CONSOLE(?::\d+|\(\d+\))\] "(.*)", source: (\S*)`)
	hiddenPattern  = regexp.MustCompile(`(?is)<(script|style|noscript|template|head)\b.*?</(script|style|noscript|template|head)>`)
	tagPattern     = regexp.MustCompile(`(?s)<[^>]*>`)
	spacePattern   = regexp.MustCompile(`\s+`)
	bodyPattern    = regexp.MustCompile(`(?is)<body\b[^>]*>(.*)</body>`)
	mediaPattern   = regexp.MustCompile(`(?i)<(img|svg|canvas|video|picture|iframe)\b`)
)

// consoleMessages classifies console lines whose source is the page origin.
func consoleMessages(log, origin string) (errs, rejections, other []string) {
	for _, line := range strings.Split(log, "\n") {
		m := consolePattern.FindStringSubmatch(line)
		if m == nil || !strings.HasPrefix(m[2], origin) {
			continue
		}
		msg := m[1]
		switch {
		case strings.HasPrefix(msg, "Uncaught (in promise)"):
			rejections = appendBounded(rejections, msg)
		case strings.HasPrefix(msg, "Uncaught"):
			errs = appendBounded(errs, msg)
		default:
			other = appendBounded(other, msg)
		}
	}
	return errs, rejections, other
}

func appendBounded(list []string, msg string) []string {
	if len(list) >= 20 {
		return list
	}
	if len(msg) > 500 {
		msg = msg[:500] + "..."
	}
	return append(list, msg)
}

func bodyOf(dom string) string {
	if m := bodyPattern.FindStringSubmatch(dom); m != nil {
		return m[1]
	}
	return dom
}

// VisibleText approximates the text a user sees: body content without scripts, styles and tags,
// entities decoded and whitespace collapsed.
func VisibleText(dom string) string {
	body := hiddenPattern.ReplaceAllString(bodyOf(dom), " ")
	text := html.UnescapeString(tagPattern.ReplaceAllString(body, " "))
	return strings.TrimSpace(spacePattern.ReplaceAllString(text, " "))
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}
