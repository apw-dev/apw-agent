package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/core"
)

func lookupWorkspace(t *testing.T) *Executor {
	t.Helper()
	ex := New(t.TempDir(), nil, nil, "", "", 1)
	for name, body := range map[string]string{
		"src/App.tsx":                 "export function App() {\n  return <h1>Count</h1>\n}\n",
		"src/util/clamp.ts":           "export const clamp = (n: number) => n\n",
		"src/util/clamp.test.ts":      "import { clamp } from './clamp'\n",
		".env":                        "SECRET=clamp\n",
		"node_modules/x/clamp.js":     "clamp\n",
		filepath.Join(".git", "HEAD"): "clamp\n",
	} {
		path := filepath.Join(ex.Workspace, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return ex
}

func TestGlobRegexp(t *testing.T) {
	for glob, cases := range map[string]map[string]bool{
		"src/**/*.ts":  {"src/a.ts": true, "src/util/clamp.ts": true, "src/a.tsx": false, "lib/a.ts": false},
		"*.md":         {"README.md": true, "docs/x.md": false},
		"**/*.test.ts": {"a.test.ts": true, "src/util/clamp.test.ts": true, "src/clamp.ts": false},
	} {
		re, err := globRegexp(glob)
		if err != nil {
			t.Fatal(err)
		}
		for path, want := range cases {
			if re.MatchString(path) != want {
				t.Errorf("%s vs %s: want %v", glob, path, want)
			}
		}
	}
}

func TestCodeLookupIsReadOnlyAndHidesProtectedPaths(t *testing.T) {
	ex := lookupWorkspace(t)
	text, labels := ex.codeLookup(context.Background(),
		[]grepQuery{{Pattern: "clamp"}, {Pattern: "(unclosed"}},
		[]string{"src/**/*.ts"},
		[]readRange{{File: "src/App.tsx", Start: 2, End: 2}, {File: ".env"}, {File: "../outside.txt"}})
	if len(labels) != 6 {
		t.Fatalf("labels = %v", labels)
	}
	for _, want := range []string{"src/util/clamp.ts:1: export const clamp", "src/util/clamp.test.ts", "invalid regular expression", "2\t  return <h1>Count</h1>", "[file continues to line"} {
		if !strings.Contains(text, want) {
			t.Fatalf("lookup output lacks %q:\n%s", want, text)
		}
	}
	for _, leak := range []string{"SECRET", "node_modules", ".git"} {
		if strings.Contains(text, leak+"/") || strings.Contains(text, "SECRET=") {
			t.Fatalf("lookup exposed %s:\n%s", leak, text)
		}
	}
	if strings.Count(text, "error: protected workspace path") != 1 || !strings.Contains(text, "escapes workspace") {
		t.Fatalf("protected/escaping reads not refused:\n%s", text)
	}
}

func TestLookupOffersOnlyWhenContextIsTruncated(t *testing.T) {
	ex := lookupWorkspace(t)
	if ex.lookupWanted("small overview") || !ex.lookupWanted("x\n"+contextTruncatedMarker+" files") {
		t.Fatal("auto lookup decision wrong")
	}
	ex.Lookup = "off"
	if ex.lookupWanted(contextTruncatedMarker) {
		t.Fatal("lookup offered while off")
	}
	ex.Lookup = "always"
	arch := &scripted{name: "arch", replies: []func(core.ModelRequest) (*core.ModelResponse, error){
		reply(`{"grep":[{"pattern":"export const","glob":"src/**/*.ts"}],"fetch":["https://pkg.go.dev/x"]}`),
		reply(validPlanJSON),
	}}
	ex.Architect.Provider, ex.Architect.Model = arch, "m"
	plan, err := ex.PlanTask(context.Background(), core.Task{ID: "task-1", Instruction: "clamp"}, "overview")
	if err != nil || plan == nil {
		t.Fatal(err)
	}
	if !strings.Contains(arch.prompts[0], `"grep":[]`) || strings.Contains(arch.prompts[0], `"fetch":[]`) {
		t.Fatalf("gather prompt offers wrong abilities:\n%s", arch.prompts[0])
	}
	if !strings.Contains(arch.prompts[1], "src/util/clamp.ts:1: export const clamp") || strings.Contains(arch.prompts[1], "External documentation") {
		t.Fatalf("planning prompt lacks lookup results or fetched docs without research:\n%.600s", arch.prompts[1])
	}
	if len(ex.ResearchSources) != 1 || !strings.HasPrefix(ex.ResearchSources[0].Query, "code grep") {
		t.Fatalf("sources = %+v", ex.ResearchSources)
	}
}
