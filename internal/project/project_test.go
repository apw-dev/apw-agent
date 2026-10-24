package project

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/config"
)

func write(t *testing.T, root string, files map[string]string) {
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

func stage(p Profile, name string) *Stage {
	for i := range p.Stages {
		if p.Stages[i].Name == name {
			return &p.Stages[i]
		}
	}
	return nil
}

func TestDetectGo(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"go.mod": "module x\n"})
	p := Detect(root, nil)
	if p.Kind != "go" || !reflect.DeepEqual(p.StageNames(), []string{"fmt", "vet", "build", "test"}) || p.Custom {
		t.Fatalf("unexpected Go profile: %+v", p)
	}
}

func TestDetectViteReactTypeScriptWithPnpm(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"package.json": `{"scripts":{"dev":"vite","build":"tsc -b && vite build","lint":"eslint .","test":"vitest"},
			"dependencies":{"react":"^19.0.0"},"devDependencies":{"typescript":"~5.6.0","vite":"^6.0.0","vitest":"^2.1.0"}}`,
		"pnpm-lock.yaml": "lockfileVersion: '9.0'\n",
		"tsconfig.json":  "{}",
	})
	p := Detect(root, nil)
	if p.Kind != "node" || p.PackageManager != "pnpm" {
		t.Fatalf("kind/pm: %+v", p)
	}
	if got := p.StageNames(); !reflect.DeepEqual(got, []string{"install", "lint", "build", "test"}) {
		t.Fatalf("stages = %v (build already runs tsc, so no separate typecheck)", got)
	}
	install := stage(p, "install")
	if !reflect.DeepEqual(install.Command, []string{"pnpm", "install", "--frozen-lockfile"}) || !install.Once || install.StampDir != "node_modules" ||
		!reflect.DeepEqual(install.OnceInputs, []string{"package.json", "pnpm-lock.yaml"}) {
		t.Fatalf("install stage: %+v", install)
	}
	test := stage(p, "test")
	if !reflect.DeepEqual(test.Command, []string{"pnpm", "run", "test"}) || !contains(test.Env, "CI=true") {
		t.Fatalf("test stage must run once under CI=true: %+v", test)
	}
	for _, want := range []string{"TypeScript", "Vite", "React", "Vitest", "pnpm"} {
		if !strings.Contains(p.Summary, want) {
			t.Fatalf("summary %q lacks %s", p.Summary, want)
		}
	}
}

func TestDetectBunFromPackageManagerField(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"package.json": `{"packageManager":"bun@1.2.0","dependencies":{"hono":"^4"},"scripts":{"test":"bun test"}}`})
	p := Detect(root, nil)
	if p.PackageManager != "bun" {
		t.Fatalf("pm = %s", p.PackageManager)
	}
	if install := stage(p, "install"); install == nil || !reflect.DeepEqual(install.Command, []string{"bun", "install", "--no-save"}) {
		t.Fatalf("unlocked bun install must not write a lockfile: %+v", install)
	}
	if !strings.Contains(p.Summary, "Hono") {
		t.Fatalf("summary: %s", p.Summary)
	}
}

func TestDetectNpmImplicitTypecheckAndPlaceholderTest(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"package.json":      `{"scripts":{"build":"vite build","test":"echo \"Error: no test specified\" && exit 1"},"devDependencies":{"typescript":"5"}}`,
		"package-lock.json": "{}",
		"tsconfig.json":     "{}",
	})
	p := Detect(root, nil)
	if got := p.StageNames(); !reflect.DeepEqual(got, []string{"install", "typecheck", "build"}) {
		t.Fatalf("stages = %v", got)
	}
	if !reflect.DeepEqual(stage(p, "install").Command, []string{"npm", "ci", "--no-audit", "--no-fund"}) {
		t.Fatalf("locked npm install must use npm ci")
	}
	if !reflect.DeepEqual(stage(p, "typecheck").Command, []string{"npx", "--no-install", "tsc", "--noEmit"}) {
		t.Fatalf("typecheck: %v", stage(p, "typecheck").Command)
	}
}

func TestDetectNodeWithoutDependenciesSkipsInstall(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"package.json": `{"scripts":{"test":"node --test"}}`})
	if got := Detect(root, nil).StageNames(); !reflect.DeepEqual(got, []string{"test"}) {
		t.Fatalf("stages = %v", got)
	}
}

func TestDetectOtherLanguages(t *testing.T) {
	cases := []struct {
		files  map[string]string
		kind   string
		stages []string
	}{
		{map[string]string{"Cargo.toml": "[package]\n"}, "rust", []string{"fmt", "build", "test"}},
		{map[string]string{"pyproject.toml": "[tool.pytest.ini_options]\n"}, "python", []string{"lint", "test"}},
		{map[string]string{"requirements.txt": ""}, "python", []string{"lint"}},
		{map[string]string{"index.html": "<html></html>"}, "static-html", []string{"links"}},
		{map[string]string{"notes.txt": ""}, "unknown", nil},
	}
	for _, c := range cases {
		root := t.TempDir()
		write(t, root, c.files)
		p := Detect(root, nil)
		if p.Kind != c.kind || strings.Join(p.StageNames(), ",") != strings.Join(c.stages, ",") {
			t.Fatalf("%v: got kind %s stages %v", c.files, p.Kind, p.StageNames())
		}
	}
}

func TestOverrideFromConfigReplacesPipeline(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"notes.txt": ""})
	stages := FromConfig(config.VerifyConfig{Stages: []config.VerifyStage{
		{Name: "check", Run: []string{"make", "check"}, Env: map[string]string{"B": "2", "A": "1"}, TimeoutSec: 30},
	}})
	p := Detect(root, stages)
	if p.Kind != "custom" || !p.Custom || !reflect.DeepEqual(p.StageNames(), []string{"check"}) {
		t.Fatalf("override: %+v", p)
	}
	if s := p.Stages[0]; !reflect.DeepEqual(s.Env, []string{"A=1", "B=2"}) || s.Timeout.Seconds() != 30 {
		t.Fatalf("stage: %+v", s)
	}
	if !strings.Contains(p.Describe(), "Verification pipeline: check") {
		t.Fatalf("describe: %s", p.Describe())
	}
	if !strings.Contains(Detect(root, nil).Describe(), "configure verify.stages") {
		t.Fatal("unknown project must tell the user how to configure verification")
	}
}

func TestDetectNodeMultiPackageSubprojects(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"api/package.json": `{"scripts":{"check":"tsc --noEmit","test":"vitest"},"dependencies":{"express":"^4"}}`,
		"web/package.json": `{"scripts":{"check":"svelte-check","test":"vitest"},"dependencies":{"svelte":"^5"}}`,
	})
	p := Detect(root, nil)
	if p.Kind != "node" || !strings.Contains(p.Summary, "api, web") {
		t.Fatalf("unexpected multi-package profile: %+v", p)
	}
	wantStages := []string{"install", "check", "test", "install", "check", "test"}
	if got := p.StageNames(); !reflect.DeepEqual(got, wantStages) {
		t.Fatalf("stages = %v, want %v", got, wantStages)
	}
	if !reflect.DeepEqual(p.Stages[0].Command, []string{"npm", "--prefix", "api", "install", "--no-package-lock", "--no-audit", "--no-fund"}) || p.Stages[0].StampDir != "api/node_modules" {
		t.Fatalf("api install stage: %+v", p.Stages[0])
	}
	if !reflect.DeepEqual(p.Stages[4].Command, []string{"npm", "--prefix", "web", "run", "check"}) {
		t.Fatalf("web check stage: %+v", p.Stages[4])
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
