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

func TestCodeLookupModesContextSymbolAndMultiTurn(t *testing.T) {
	ex := lookupWorkspace(t)
	if err := os.WriteFile(filepath.Join(ex.Workspace, ".gitignore"), []byte("ignored_dir/\n*.gen.ts\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"ignored_dir/secret.ts": "export const clamp = 1\n",
		"src/skip.gen.ts":       "export const clamp = 2\n",
		"src/math.go":           "package src\n// ComputeScore calculates score\nfunc ComputeScore(x int) int {\n\treturn x * 2\n}\n",
		"src/caller.go":         "package src\nfunc RunAll() int {\n\treturn ComputeScore(5)\n}\n",
	} {
		p := filepath.Join(ex.Workspace, filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// 1. Test grep modes (files_with_matches, count, context lines) + symbol lookup + .gitignore filtering
	text, labels := ex.codeLookup(context.Background(),
		[]grepQuery{
			{Pattern: "clamp", Mode: "files_with_matches"},
			{Pattern: "clamp", Mode: "count"},
			{Pattern: "return <h1>Count</h1>", Context: 1},
		},
		nil,
		nil,
		"ComputeScore",
	)
	if len(labels) != 4 {
		t.Fatalf("expected 4 labels, got %v", labels)
	}
	if strings.Contains(text, "ignored_dir") || strings.Contains(text, "skip.gen.ts") {
		t.Fatalf("expected .gitignore files to be excluded:\n%s", text)
	}
	for _, want := range []string{
		"src/util/clamp.ts\n",
		"src/util/clamp.ts: 1\n",
		"src/App.tsx-1- export function App() {",
		"src/App.tsx:2:   return <h1>Count</h1>",
		"src/math.go:3-5 [func ComputeScore]",
		"Callers:\n  - src/caller.go:3 (RunAll)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected lookup output to contain %q, got:\n%s", want, text)
		}
	}

	// 2. Test multi-turn lookup ("continue": true on turn 1 -> turn 2 symbol lookup -> plan)
	ex.Lookup = "always"
	arch := &scripted{name: "arch", replies: []func(core.ModelRequest) (*core.ModelResponse, error){
		reply(`{"grep":[{"pattern":"ComputeScore","mode":"files_with_matches"}],"continue":true}`),
		reply(`{"symbol":["ComputeScore"],"continue":false}`),
		reply(validPlanJSON),
	}}
	ex.Architect.Provider, ex.Architect.Model = arch, "m"
	plan, err := ex.PlanTask(context.Background(), core.Task{ID: "task-mt", Instruction: "update ComputeScore"}, "overview")
	if err != nil || plan == nil {
		t.Fatalf("PlanTask multi-turn failed: %v", err)
	}
	if len(arch.prompts) != 3 {
		t.Fatalf("expected 3 architect calls (2 lookup hops + 1 plan), got %d", len(arch.prompts))
	}
	if !strings.Contains(arch.prompts[2], "src/math.go:3-5 [func ComputeScore]") {
		t.Fatalf("expected final planning prompt to contain turn-2 symbol lookup results:\n%s", arch.prompts[2])
	}

	// 3. Test instruction-aware CodeContextForInstruction & @#Symbol mention expansion
	ctxText := CodeContextForInstruction(ex.Workspace, "Fix @#ComputeScore in caller")
	if !strings.Contains(ctxText, "Pinned Symbol @#ComputeScore") {
		t.Fatalf("expected @#ComputeScore to be expanded in CodeContextForInstruction:\n%s", ctxText)
	}
	idxMath := strings.Index(ctxText, "File: src/math.go")
	idxApp := strings.Index(ctxText, "File: src/App.tsx")
	if idxMath < 0 || idxApp < 0 || idxMath > idxApp {
		t.Fatalf("expected relevance-ranked src/math.go (%d) before unrelated src/App.tsx (%d)", idxMath, idxApp)
	}
}

