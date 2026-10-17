package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/project"
	"github.com/apiwat/apw-agent/internal/provider/mock"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCodeContextIncludesWebSourcesAndSkipsDependencies(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"package.json":                     `{"name":"web"}`,
		"pnpm-lock.yaml":                   "lockfileVersion: '9.0'",
		"index.html":                       "<div id=root></div>",
		"src/App.tsx":                      "export default function App() { return <h1>Hi</h1> }",
		"src/index.css":                    "h1 { color: red }",
		"src/vendor.min.js":                "minified",
		"node_modules/react/index.js":      "dependency",
		"dist/assets/index.js":             "build output",
		"server/routes.py":                 "def handler(): pass",
		".env":                             "SECRET=1",
		"coverage/lcov-report/index.html":  "coverage",
		"src/components/Button.test.tsx":   "test('x', () => {})",
		"docs/notes.txt":                   "not source",
		"internal/tool/main.go":            "package main",
		"crates/core/src/lib.rs":           "pub fn x() {}",
		"apps/api/src/handler.ts":          "export const h = 1",
		"apps/api/node_modules/x/index.js": "nested dependency",
		"apps/web/.next/server/page.js":    "next output",
		"scripts/deploy.sh":                "echo deploy",
		"src/assets/logo.svg":              "<svg/>",
		"src/styles/theme.scss":            "$c: red;",
		"src/pages/About.vue":              "<template></template>",
		"src/routes/+page.svelte":          "<h1/>",
	})
	ctx := CodeContext(root)
	for _, want := range []string{"File: package.json", "File: index.html", "File: src/App.tsx", "File: src/index.css", "File: server/routes.py",
		"File: src/components/Button.test.tsx", "File: internal/tool/main.go", "File: crates/core/src/lib.rs", "File: apps/api/src/handler.ts",
		"File: scripts/deploy.sh", "File: src/styles/theme.scss", "File: src/pages/About.vue", "File: src/routes/+page.svelte"} {
		if !strings.Contains(ctx, want) {
			t.Fatalf("context lacks %q:\n%s", want, ctx)
		}
	}
	for _, unwanted := range []string{"dependency", "build output", "SECRET", "minified", "pnpm-lock", "coverage", "not source", "next output", "<svg/>"} {
		if strings.Contains(ctx, unwanted) {
			t.Fatalf("context must not include %q", unwanted)
		}
	}
	if strings.Index(ctx, "File: package.json") > strings.Index(ctx, "File: src/App.tsx") {
		t.Fatal("manifests must come before sources")
	}
}

func TestCodeContextListsFilesBeyondBudget(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{"package.json": "{}"}
	chunk := strings.Repeat("x", 40*1024)
	for _, name := range []string{"src/a.ts", "src/b.ts", "src/c.ts", "src/d.ts"} {
		files[name] = chunk
	}
	files["src/huge.ts"] = strings.Repeat("y", contextFileLimit+1)
	writeTree(t, root, files)
	ctx := CodeContext(root)
	if !strings.Contains(ctx, "these files exist but were not included") || !strings.Contains(ctx, "src/huge.ts\n") || !strings.Contains(ctx, "src/c.ts\n") {
		t.Fatalf("omitted files must be listed:\n%s", ctx[len(ctx)-300:])
	}
	if strings.Contains(ctx, strings.Repeat("y", 100)) {
		t.Fatal("oversized file content must not be inlined")
	}
}

func TestProjectOverviewAndExecutorProfile(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"package.json": `{"scripts":{"test":"vitest"},"devDependencies":{"vitest":"2","typescript":"5"}}`, "bun.lock": ""})
	overview := ProjectOverview(root, nil)
	if !strings.Contains(overview, "package manager bun") || !strings.Contains(overview, "Verification pipeline: install -> test") {
		t.Fatalf("overview: %s", overview[:200])
	}
	e := New(root, nil, nil, "", "", 1)
	e.VerifyOverride = []project.Stage{{Name: "check", Command: []string{"bun", "run", "check"}}}
	if p := e.Profile(root); !p.Custom || strings.Join(p.StageNames(), ",") != "check" {
		t.Fatalf("override not applied: %+v", p)
	}
}

// A Node.js project goes through the same plan -> patch -> verify -> repair ->
// apply loop as Go, with `npm run test` as its detected verification stage.
func TestExecutor_NodeProjectEndToEndWithSelfHeal(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not installed")
	}
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"package.json": `{"name":"fixture","version":"1.0.0","scripts":{"test":"node test.js"}}`,
		"index.js":     "exports.greet = () => \"hi\";\n",
		"test.js":      "const assert = require(\"node:assert\");\nassert.strictEqual(require(\"./index.js\").greet(), \"hello\");\nconsole.log(\"ok\");\n",
	})
	planResp := "```json\n" + `{"objective":"Greet with hello","affected_files":["index.js"],
		"subtasks":[{"id":"step-1","action":"patch","target_file":"index.js","instruction":"return hello"},{"id":"step-2","action":"verify","instruction":"run tests"}],
		"acceptance_criteria":["tests pass"],"checks":[{"criterion":"tests pass","kind":"verification","value":"test"}]}` + "\n```"
	firstPatch := `{"old_content": "\"hi\"", "new_content": "undefinedVar"}`
	repair := `{"target_file": "index.js", "old_content": "undefinedVar", "new_content": "\"hello\""}`
	mp := mock.New(planResp, firstPatch, repair)
	initExecutorRepo(t, ws)
	e := New(ws, mp, mp, "mock-arch", "mock-reflex", 2)
	e.Trusted = true
	report, err := e.Execute(context.Background(), "Make greet return hello", "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !report.Passed || len(report.ReflexAttempts) != 2 {
		t.Fatalf("expected heal on second attempt: passed=%v attempts=%d", report.Passed, len(report.ReflexAttempts))
	}
	if first := report.ReflexAttempts[0].Result; first.FailedStage != "test" {
		t.Fatalf("first attempt should fail the npm test stage, got %q", first.FailedStage)
	}
	if report.Baseline == nil || report.Baseline.Passed {
		t.Fatal("baseline must record the failing test before editing")
	}
	applyPreparedTest(t, e, report)
	data, err := os.ReadFile(filepath.Join(ws, "index.js"))
	if err != nil || string(data) != "exports.greet = () => \"hello\";\n" {
		t.Fatalf("applied index.js = %q (%v)", data, err)
	}
}
