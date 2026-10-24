// Package project detects a workspace's language and toolchain and derives the
// verification pipeline Reflex runs for it. Built-in presets cover Go, Node.js
// (npm, pnpm, bun, yarn; TypeScript, Vite, React, Vitest), Rust, Python and static
// HTML; any other language uses explicitly configured stages.
package project

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/browser"
	"github.com/apiwat/apw-agent/internal/config"
)

// Builtin stage implementations run inside apw instead of as commands.
const (
	BuiltinGoFmt     = "gofmt"
	BuiltinHTMLLinks = "html-links"
	// BuiltinBrowser renders the build output in a headless browser (see package browser).
	BuiltinBrowser = "browser"
)

// Stage is one verification step. Exactly one of Command or Builtin is set.
type Stage struct {
	Name    string
	Command []string
	Builtin string
	Env     []string
	Timeout time.Duration
	// Once marks dependency installation: it is skipped on later attempts while
	// the files in OnceInputs are unchanged. StampDir holds the marker (it must be
	// ignored by git, e.g. node_modules).
	Once       bool
	OnceInputs []string
	StampDir   string
	// SkipIfMissing skips the stage when Command[0] is not installed.
	SkipIfMissing bool
	// Dir is the directory a builtin browser stage serves (relative to the root).
	Dir string
}

// Profile describes the detected project and its verification pipeline.
type Profile struct {
	Kind           string // go, node, rust, python, static-html, custom, unknown
	Summary        string
	PackageManager string
	Stages         []Stage
	// Custom is set when apw.json verify.stages replaced the detected pipeline.
	Custom bool
}

// StageNames lists the pipeline's stage names in order.
func (p Profile) StageNames() []string {
	names := make([]string, 0, len(p.Stages))
	for _, s := range p.Stages {
		names = append(names, s.Name)
	}
	return names
}

// Describe renders the profile for model prompts and reports.
func (p Profile) Describe() string {
	pipeline := strings.Join(p.StageNames(), " -> ")
	if pipeline == "" {
		pipeline = "none (configure verify.stages in apw.json)"
	}
	out := "Project profile: " + p.Summary + "\nVerification pipeline: " + pipeline
	if s, ok := p.BrowserStage(); ok {
		out += "\nBrowser check: the " + s.Name + " stage renders " + s.Dir + "/index.html headlessly and fails on uncaught page errors or a blank page; acceptance checks of kind \"browser\" can assert visible page text"
	}
	return out
}

// BrowserStage returns the profile's headless browser stage, if any.
func (p Profile) BrowserStage() (Stage, bool) {
	for _, s := range p.Stages {
		if s.Builtin == BuiltinBrowser {
			return s, true
		}
	}
	return Stage{}, false
}

// WithoutBrowser returns the profile without its browser stage (verify.browser "off").
func (p Profile) WithoutBrowser() Profile {
	var stages []Stage
	for _, s := range p.Stages {
		if s.Builtin != BuiltinBrowser {
			stages = append(stages, s)
		}
	}
	p.Stages = stages
	return p
}

const (
	defaultStageTimeout   = 5 * time.Minute
	defaultInstallTimeout = 10 * time.Minute
)

// Detect inspects root. A non-empty override replaces the detected pipeline but
// keeps the detected summary.
func Detect(root string, override []Stage) Profile {
	p := detect(root)
	if len(override) > 0 {
		p.Stages = override
		p.Custom = true
		p.Summary += " (custom verification from apw.json)"
		if p.Kind == "unknown" {
			p.Kind = "custom"
		}
	}
	return p
}

func detect(root string) Profile {
	switch {
	case exists(root, "go.mod"):
		return Profile{Kind: "go", Summary: "Go module", Stages: []Stage{
			{Name: "fmt", Builtin: BuiltinGoFmt},
			{Name: "vet", Command: []string{"go", "vet", "-mod=readonly", "./..."}},
			{Name: "build", Command: []string{"go", "build", "-mod=readonly", "-o", os.DevNull, "./..."}},
			{Name: "test", Command: []string{"go", "test", "-mod=readonly", "./..."}},
		}}
	case exists(root, "package.json"):
		p := detectNode(root)
		if len(p.Stages) == 0 {
			if sub, ok := detectNodeSubprojects(root); ok {
				return sub
			}
		}
		return p
	case exists(root, "Cargo.toml"):
		return Profile{Kind: "rust", Summary: "Rust (Cargo)", Stages: []Stage{
			{Name: "fmt", Command: []string{"cargo", "fmt", "--check"}, SkipIfMissing: true},
			{Name: "build", Command: []string{"cargo", "build", "--all-targets"}, Timeout: defaultInstallTimeout},
			{Name: "test", Command: []string{"cargo", "test"}, Timeout: defaultInstallTimeout},
		}}
	case exists(root, "pyproject.toml") || exists(root, "requirements.txt") || exists(root, "setup.py") ||
		exists(root, "backend/pyproject.toml") || exists(root, "backend/requirements.txt") || exists(root, "backend/setup.py"):
		return detectPython(root)
	}
	if sub, ok := detectNodeSubprojects(root); ok {
		return sub
	}
	if exists(root, "index.html") || len(globHTML(root)) > 0 ||
		exists(root, "frontend/index.html") || exists(root, "static/index.html") {
		return Profile{Kind: "static-html", Summary: "Static HTML/CSS/JS site", Stages: withBrowser([]Stage{
			{Name: "links", Builtin: BuiltinHTMLLinks},
		}, ".")}
	}
	return Profile{Kind: "unknown", Summary: "Unrecognized project type"}
}

type packageJSON struct {
	PackageManager  string            `json:"packageManager"`
	Workspaces      any               `json:"workspaces"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func (p packageJSON) has(dep string) bool {
	_, a := p.Dependencies[dep]
	_, b := p.DevDependencies[dep]
	return a || b
}

func subprojectManifests(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var manifests []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		switch name {
		case "node_modules", "vendor", "dist", "build", "out", "coverage", "target", "testdata":
			continue
		}
		rel := filepath.ToSlash(filepath.Join(name, "package.json"))
		data, err := os.ReadFile(filepath.Join(root, name, "package.json"))
		if err != nil {
			continue
		}
		var sub packageJSON
		if json.Unmarshal(data, &sub) == nil && len(sub.Dependencies)+len(sub.DevDependencies) > 0 {
			manifests = append(manifests, rel)
		}
	}
	sort.Strings(manifests)
	return manifests
}

func detectNode(root string) Profile {
	var pkg packageJSON
	if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		_ = json.Unmarshal(data, &pkg)
	}
	pm, lockfile := nodePackageManager(root, pkg.PackageManager)
	env := []string{"NO_COLOR=1", "FORCE_COLOR=0"}

	var stages []Stage
	subManifests := subprojectManifests(root)
	if len(pkg.Dependencies)+len(pkg.DevDependencies) > 0 || pkg.Workspaces != nil || len(subManifests) > 0 {
		inputs := []string{"package.json"}
		inputs = append(inputs, subManifests...)
		if lockfile != "" {
			inputs = append(inputs, lockfile)
		}
		stages = append(stages, Stage{
			Name: "install", Command: nodeInstall(pm, lockfile != ""), Env: env,
			Timeout: defaultInstallTimeout, Once: true, OnceInputs: inputs, StampDir: "node_modules",
		})
	}
	run := func(name, script string, extraEnv ...string) {
		stages = append(stages, Stage{Name: name, Command: []string{pm, "run", script}, Env: append(append([]string{}, env...), extraEnv...)})
	}
	script := func(names ...string) string {
		for _, n := range names {
			if body, ok := pkg.Scripts[n]; ok && strings.TrimSpace(body) != "" {
				return n
			}
		}
		return ""
	}
	if s := script("format:check", "fmt:check", "prettier:check"); s != "" {
		run("format", s)
	}
	if s := script("lint"); s != "" {
		run("lint", s)
	}
	if s := script("check"); s != "" {
		run("check", s)
	}
	typescript := exists(root, "tsconfig.json") || pkg.has("typescript")
	build := pkg.Scripts["build"] + " " + pkg.Scripts["check"]
	if s := script("typecheck", "type-check", "check-types", "tsc"); s != "" {
		run("typecheck", s)
	} else if typescript && exists(root, "tsconfig.json") && pkg.has("typescript") && !strings.Contains(build, "tsc") {
		stages = append(stages, Stage{Name: "typecheck", Command: append(nodeExec(pm), "tsc", "--noEmit"), Env: env})
	}
	if s := script("build"); s != "" {
		run("build", s)
		// A client-side web app gets its built page rendered; server frameworks (Next.js) do not
		// produce a static index.html.
		if !pkg.has("next") && (pkg.has("vite") || pkg.has("react-scripts") || pkg.has("react") || pkg.has("vue") || pkg.has("svelte") || pkg.has("preact") || pkg.has("solid-js")) {
			dir := "dist"
			if pkg.has("react-scripts") {
				dir = "build"
			}
			stages = withBrowser(stages, dir)
		}
	}
	if s := script("test"); s != "" && !strings.Contains(pkg.Scripts[s], "no test specified") {
		run("test", s, "CI=true")
	}

	var traits []string
	if typescript {
		traits = append(traits, "TypeScript")
	} else {
		traits = append(traits, "JavaScript")
	}
	for _, t := range []struct{ dep, label string }{
		{"next", "Next.js"}, {"vite", "Vite"}, {"react", "React"}, {"vue", "Vue"}, {"svelte", "Svelte"},
		{"express", "Express"}, {"fastify", "Fastify"}, {"hono", "Hono"}, {"vitest", "Vitest"}, {"jest", "Jest"},
	} {
		if pkg.has(t.dep) {
			traits = append(traits, t.label)
		}
	}
	return Profile{Kind: "node", PackageManager: pm, Stages: stages,
		Summary: "Node.js (" + strings.Join(traits, ", ") + "; package manager " + pm + ")"}
}

// detectNodeSubprojects inspects immediate subdirectories (such as api/ and web/) for package.json
// when the workspace root has no manifest or no runnable stages of its own.
func detectNodeSubprojects(root string) (Profile, bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return Profile{}, false
	}
	var subs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		switch name {
		case "node_modules", "vendor", "dist", "build", "out", "coverage", "target", "testdata":
			continue
		}
		if exists(filepath.Join(root, name), "package.json") {
			subs = append(subs, name)
		}
	}
	if len(subs) == 0 {
		return Profile{}, false
	}
	sort.Strings(subs)
	var allStages []Stage
	primaryPM := "npm"
	for i, sub := range subs {
		subProf := detectNode(filepath.Join(root, sub))
		if i == 0 && subProf.PackageManager != "" {
			primaryPM = subProf.PackageManager
		}
		for _, st := range subProf.Stages {
			scoped := st
			if len(st.Command) > 0 {
				scoped.Command = scopeNodeSubdirCommand(sub, st.Command)
			}
			if st.StampDir != "" {
				scoped.StampDir = filepath.ToSlash(filepath.Join(sub, st.StampDir))
			}
			if st.Dir != "" {
				scoped.Dir = filepath.ToSlash(filepath.Join(sub, st.Dir))
			}
			if len(st.OnceInputs) > 0 {
				inputs := make([]string, len(st.OnceInputs))
				for j, in := range st.OnceInputs {
					inputs[j] = filepath.ToSlash(filepath.Join(sub, in))
				}
				scoped.OnceInputs = inputs
			}
			allStages = append(allStages, scoped)
		}
	}
	if len(allStages) == 0 {
		return Profile{}, false
	}
	return Profile{
		Kind:           "node",
		PackageManager: primaryPM,
		Stages:         allStages,
		Summary:        "Node.js multi-package workspace (" + strings.Join(subs, ", ") + "; package manager " + primaryPM + ")",
	}, true
}

func scopeNodeSubdirCommand(sub string, cmd []string) []string {
	if len(cmd) == 0 {
		return cmd
	}
	switch cmd[0] {
	case "npm":
		return append([]string{"npm", "--prefix", sub}, cmd[1:]...)
	case "npx":
		// ["npx", "--no-install", "tsc", "--noEmit"] -> ["npm", "--prefix", sub, "exec", "--", "tsc", "--noEmit"]
		rest := cmd[1:]
		if len(rest) > 0 && rest[0] == "--no-install" {
			rest = rest[1:]
		}
		return append([]string{"npm", "--prefix", sub, "exec", "--"}, rest...)
	case "pnpm":
		return append([]string{"pnpm", "--dir", sub}, cmd[1:]...)
	case "yarn":
		return append([]string{"yarn", "--cwd", sub}, cmd[1:]...)
	case "bun":
		return append([]string{"bun", "--cwd", sub}, cmd[1:]...)
	default:
		return cmd
	}
}

// nodePackageManager honors the packageManager field, then lockfiles; npm otherwise.
func nodePackageManager(root, field string) (string, string) {
	locks := []struct{ file, pm string }{
		{"bun.lock", "bun"}, {"bun.lockb", "bun"}, {"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"}, {"package-lock.json", "npm"}, {"npm-shrinkwrap.json", "npm"},
	}
	if name, _, ok := strings.Cut(strings.TrimSpace(field), "@"); ok || field != "" {
		switch name {
		case "npm", "pnpm", "bun", "yarn":
			for _, l := range locks {
				if l.pm == name && exists(root, l.file) {
					return name, l.file
				}
			}
			return name, ""
		}
	}
	for _, l := range locks {
		if exists(root, l.file) {
			return l.pm, l.file
		}
	}
	return "npm", ""
}

// nodeInstall installs exactly the locked dependencies when a lockfile exists and
// never creates a lockfile otherwise (a new file would be an unplanned change).
func nodeInstall(pm string, locked bool) []string {
	switch pm {
	case "pnpm":
		if locked {
			return []string{"pnpm", "install", "--frozen-lockfile"}
		}
		return []string{"pnpm", "install", "--no-lockfile"}
	case "bun":
		if locked {
			return []string{"bun", "install", "--frozen-lockfile"}
		}
		return []string{"bun", "install", "--no-save"}
	case "yarn":
		if locked {
			return []string{"yarn", "install", "--frozen-lockfile"}
		}
		return []string{"yarn", "install", "--no-lockfile"}
	default:
		if locked {
			return []string{"npm", "ci", "--no-audit", "--no-fund"}
		}
		return []string{"npm", "install", "--no-package-lock", "--no-audit", "--no-fund"}
	}
}

// nodeExec runs a locally installed binary without downloading it.
func nodeExec(pm string) []string {
	switch pm {
	case "pnpm":
		return []string{"pnpm", "exec"}
	case "bun":
		return []string{"bun", "x", "--no-install"}
	case "yarn":
		return []string{"yarn", "run"}
	default:
		return []string{"npx", "--no-install"}
	}
}

func detectPython(root string) Profile {
	env := []string{"PYTHONDONTWRITEBYTECODE=1"}
	p := Profile{Kind: "python", Summary: "Python"}
	p.Stages = append(p.Stages, Stage{Name: "lint", Command: []string{"ruff", "check", "."}, Env: env, SkipIfMissing: true})
	if exists(root, "tests") || exists(root, "pytest.ini") || exists(root, "conftest.py") || fileContains(root, "pyproject.toml", "[tool.pytest") ||
		exists(root, "backend/tests") || exists(root, "backend/pytest.ini") || exists(root, "backend/conftest.py") || fileContains(root, "backend/pyproject.toml", "[tool.pytest") {
		p.Stages = append(p.Stages, Stage{Name: "test", Command: []string{"python", "-m", "pytest", "-q", "-p", "no:cacheprovider"}, Env: env})
		p.Summary += " (pytest)"
	}
	return p
}

// withBrowser appends the headless browser stage when a supported browser is installed.
func withBrowser(stages []Stage, dir string) []Stage {
	if _, ok := browser.Find(); !ok {
		return stages
	}
	return append(stages, Stage{Name: "browser", Builtin: BuiltinBrowser, Dir: dir, Timeout: 2 * time.Minute})
}

func exists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}

func fileContains(root, name, needle string) bool {
	data, err := os.ReadFile(filepath.Join(root, name))
	return err == nil && strings.Contains(string(data), needle)
}

func globHTML(root string) []string {
	matches, _ := filepath.Glob(filepath.Join(root, "*.html"))
	sort.Strings(matches)
	return matches
}

// Available reports whether a stage command can run on this machine.
func Available(s Stage) bool {
	if len(s.Command) == 0 {
		return s.Builtin != ""
	}
	_, err := exec.LookPath(s.Command[0])
	return err == nil
}

// TimeoutOf returns the stage timeout, or the default.
func TimeoutOf(s Stage) time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return defaultStageTimeout
}

// FromConfig converts configured verify stages into pipeline stages.
func FromConfig(cfg config.VerifyConfig) []Stage {
	var stages []Stage
	for _, s := range cfg.Stages {
		var env []string
		for k, v := range s.Env {
			env = append(env, k+"="+v)
		}
		sort.Strings(env)
		stages = append(stages, Stage{
			Name: s.Name, Command: append([]string{}, s.Run...), Env: env,
			Timeout: time.Duration(s.TimeoutSec) * time.Second,
		})
	}
	return stages
}
