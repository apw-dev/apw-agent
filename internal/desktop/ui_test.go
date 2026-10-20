package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// uiScript returns the contents of the page's inline <script> block.
func uiScript(t *testing.T) string {
	t.Helper()
	start := strings.Index(desktopHTML, "<script>")
	end := strings.Index(desktopHTML, "</script>")
	if start < 0 || end < start {
		t.Fatal("desktopHTML has no inline <script> block")
	}
	return desktopHTML[start+len("<script>") : end]
}

func TestDesktopHTMLLoadsNoRemoteCode(t *testing.T) {
	// The page holds the API token, so it must not reference any remote host.
	for _, bad := range []string{"https://", "http://", "<script src", "<link ", "@import"} {
		if strings.Contains(desktopHTML, bad) {
			t.Errorf("desktopHTML must not contain %q", bad)
		}
	}
	if !strings.Contains(desktopHTML, "<style>") {
		t.Error("desktopHTML must embed compiled CSS in a <style> block")
	}
	if len(desktopCSS) == 0 {
		t.Fatal("desktopCSS is empty; run python scripts/desktop_css/build.py")
	}
	if !strings.Contains(desktopCSS, ".flex") {
		t.Error("desktopCSS lacks .flex; regenerate with scripts/desktop_css/build.py")
	}
	if !strings.Contains(desktopHTML, desktopCSS) {
		t.Error("desktopHTML must embed desktopCSS")
	}
	if strings.Count(desktopHTML, "__APW_TOKEN__") != 1 {
		t.Errorf("desktopHTML must contain exactly one token placeholder, got %d", strings.Count(desktopHTML, "__APW_TOKEN__"))
	}
}

func TestDesktopHTMLRemovedConcepts(t *testing.T) {
	for _, bad := range []string{
		"chan bool", "CLAUDE.md", "AGENT.md", "require_approval",
		"/api/approvals/request", "triggerDemoApproval", "hitl-toggle", "/api/agent-md",
	} {
		if strings.Contains(desktopHTML, bad) {
			t.Errorf("desktopHTML must not contain %q", bad)
		}
	}
}

func TestDesktopHTMLUsesContract(t *testing.T) {
	for _, want := range []string{
		"/api/tasks/", "/api/instructions", "/api/instructions/init", "\"trusted\"",
		"/api/trust", "/api/approvals", "/api/file?path=", "&task=", "APW.md",
		"/api/models", "task-list", "settings-dialog", "left-resizer", "right-resizer",
		"artifact-conflict-box", "acceptance-structured", "apw_desktop_task",
	} {
		if !strings.Contains(desktopHTML, want) {
			t.Errorf("desktopHTML must reference %q", want)
		}
	}
}

func TestDesktopPreviewIframeStaysOpaqueOrigin(t *testing.T) {
	if !strings.Contains(desktopHTML, `sandbox="allow-scripts"`) {
		t.Error("preview iframe must keep sandbox=\"allow-scripts\"")
	}
	if strings.Contains(desktopHTML, "allow-same-origin") {
		t.Error("preview iframe must never allow-same-origin: previewed HTML could read the API token")
	}
}

// TestDesktopScriptBuildsDOMWithoutHTMLStrings enforces the rule that server,
// model and repository data never reach an HTML parser: the script must not use
// innerHTML/outerHTML/insertAdjacentHTML/document.write/eval at all (dynamic
// values go through textContent). This is stricter than, and replaces, a
// "no variable concatenated into innerHTML" heuristic, which is brittle
// across multi-line string concatenation.
func TestDesktopScriptBuildsDOMWithoutHTMLStrings(t *testing.T) {
	script := uiScript(t)
	for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function"} {
		if strings.Contains(script, bad) {
			t.Errorf("script must not use %q", bad)
		}
	}
}

// Mention completion must send the typed query to the server instead of filtering a
// one-time capped index on the client (which hid files beyond the first 100).
func TestDesktopMentionSearchQueriesServer(t *testing.T) {
	script := uiScript(t)
	if !strings.Contains(script, `"/api/search?q=" + encodeURIComponent(`) {
		t.Fatal("mention search does not pass the query to /api/search")
	}
	if strings.Contains(script, "cachedWorkspaceSearch") {
		t.Fatal("mention search still uses a one-time cached index")
	}
}

// Inside the native shell (Wails globals present) browser shortcuts are blocked, external
// links go to the default browser and Ctrl+O opens the workspace chooser; in a plain
// browser (`apw desktop`) the guards stay off.
func TestDesktopScriptShellGuards(t *testing.T) {
	script := uiScript(t)
	for _, want := range []string{
		"function inDesktopShell()", `"F5"`, `"F12"`,
		"window.runtime.BrowserOpenURL(", "window.go.main.App.ChooseWorkspace(",
		`"dragover"`, `"drop"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("desktop script lacks %q", want)
		}
	}
}

func TestDesktopScriptOnlyCallsLocalPaths(t *testing.T) {
	// Every apiFetch/getAPI/postAPI target must be a local absolute path.
	re := regexp.MustCompile(`(?:apiFetch|getAPI|postAPI)\(\s*"([^"]*)"`)
	for _, m := range re.FindAllStringSubmatch(uiScript(t), -1) {
		if !strings.HasPrefix(m[1], "/") {
			t.Errorf("non-local API path %q", m[1])
		}
	}
}

// TestDesktopScriptSyntax runs `node --check` over the inline script when Node
// is installed; it is skipped otherwise.
func TestDesktopScriptSyntax(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	path := filepath.Join(t.TempDir(), "desktop_ui.js")
	if err := os.WriteFile(path, []byte(uiScript(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, "--check", path).CombinedOutput(); err != nil {
		t.Fatalf("node --check failed: %v\n%s", err, out)
	}
}
