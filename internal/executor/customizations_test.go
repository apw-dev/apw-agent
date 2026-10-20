package executor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/sandbox"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

func TestProgressiveSkillsRulesAndSecurityBoundary(t *testing.T) {
	dir := t.TempDir()

	// 1. Create sensitive .apw files that must remain unreadable via ReadAPWCustomization
	if err := os.MkdirAll(filepath.Join(dir, ".apw"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".apw", "auth.json"), []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := surgeon.ReadAPWCustomization(dir, ".apw/auth.json"); err == nil {
		t.Fatal("expected ReadAPWCustomization to reject .apw/auth.json")
	}

	// 2. Create two workspace skills (.apw/skills/<slug>/SKILL.md)
	dbSkillDir := filepath.Join(dir, ".apw", "skills", "db-migration")
	uiSkillDir := filepath.Join(dir, ".apw", "skills", "ui-theme")
	if err := os.MkdirAll(dbSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(uiSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbSkillContent := `---
name: db-migration
description: Safe SQLite schema migration procedure with transactional rollback
---
Always wrap schema migrations in an immediate transaction and add a regression test.`
	uiSkillContent := `---
name: ui-theme
description: Tailwind dark theme color tokens and squircle border guidelines
---
Use #11141B background and #E07A5F accent buttons.`
	if err := os.WriteFile(filepath.Join(dbSkillDir, "SKILL.md"), []byte(dbSkillContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uiSkillDir, "SKILL.md"), []byte(uiSkillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	skills := DiscoverWorkspaceSkills(dir)
	if len(skills) != 2 {
		t.Fatalf("expected 2 discovered skills, got %d", len(skills))
	}

	// Progressive disclosure: instruction mentioning @skill:db-migration should expand db-migration body,
	// while keeping ui-theme in the compact index only.
	ctxText := ProgressiveSkillsContext(dir, "Please run @skill:db-migration on store")
	if !strings.Contains(ctxText, "Activated Skill Procedure [db-migration]") || !strings.Contains(ctxText, "immediate transaction") {
		t.Fatalf("expected db-migration skill body to be activated, got:\n%s", ctxText)
	}
	if strings.Contains(ctxText, "Use #11141B background") {
		t.Fatalf("expected unrelated ui-theme skill body to stay unexpanded, got:\n%s", ctxText)
	}

	// 3. Create modular rules (.apw/rules/*.md) including a duplicate to test content-hash deduplication
	rulesDir := filepath.Join(dir, ".apw", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ruleBody := "---\ntrigger: always_on\n---\nNever use eval or innerHTML in desktop UI."
	if err := os.WriteFile(filepath.Join(rulesDir, "01-security.md"), []byte(ruleBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "02-duplicate.md"), []byte(ruleBody), 0o644); err != nil {
		t.Fatal(err)
	}
	rulesCtx := DiscoverModularRules(dir, "update button")
	if strings.Count(rulesCtx, "Never use eval or innerHTML") != 1 {
		t.Fatalf("expected content-hash deduplication to yield 1 modular rule in context, got:\n%s", rulesCtx)
	}
}

func TestLifecycleHooksPrePlanAndPrePatchDeny(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".apw"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Configure .apw/hooks.json using go env / go version or inline stdout JSON via go test helper
	// Wait: let's use a portable Go command or cmd/sh via `go` or `node`? Even simpler: every test environment running `go test` has `go` in PATH, or we can compile a tiny helper binary in t.TempDir()!
	helperSrc := filepath.Join(dir, "hookhelper.go")
	helperBin := filepath.Join(dir, "hookhelper.exe")
	code := `package main
import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)
func main() {
	var p struct {
		Event      string ` + "`json:\"event\"`" + `
		TargetFile string ` + "`json:\"targetFile\"`" + `
	}
	_ = json.NewDecoder(os.Stdin).Decode(&p)
	if p.Event == "PrePatch" && strings.Contains(p.TargetFile, "production.lock") {
		fmt.Println(` + "`" + `{"decision":"deny","reason":"production.lock is protected by PrePatch hook"}` + "`" + `)
		return
	}
	fmt.Println(` + "`" + `{"decision":"allow","additionalContext":"Hook verified policy compliance"}` + "`" + `)
}
`
	if err := os.WriteFile(helperSrc, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	hooksJSON := `{
  "policy-guard": {
    "PrePlan": [
      { "command": "go run hookhelper.go", "timeout": 15 }
    ],
    "PrePatch": [
      { "command": "go run hookhelper.go", "timeout": 15 }
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(dir, ".apw", "hooks.json"), []byte(hooksJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = helperBin

	exec := &Executor{
		Workspace: dir,
		Trusted:   true,
	}

	// 1. PrePlan should succeed and return additionalContext
	extra, err := exec.runLifecycleHooks(context.Background(), HookPrePlan, "", "add feature")
	if err != nil {
		t.Fatalf("PrePlan hook failed unexpectedly: %v", err)
	}
	if !strings.Contains(extra, "Hook verified policy compliance") {
		t.Fatalf("expected additionalContext from PrePlan hook, got %q", extra)
	}

	// 2. PrePatch on a normal file should be allowed
	_, err = exec.runLifecycleHooks(context.Background(), HookPrePatch, "internal/app/app.go", "patch")
	if err != nil {
		t.Fatalf("PrePatch hook on normal file failed: %v", err)
	}

	// 3. PrePatch on production.lock should be denied by hook policy
	_, err = exec.runLifecycleHooks(context.Background(), HookPrePatch, "production.lock", "patch")
	if err == nil || !strings.Contains(err.Error(), "production.lock is protected") {
		t.Fatalf("expected PrePatch hook to deny production.lock, got: %v", err)
	}
}

// Lifecycle hooks are policy gates: every failure to evaluate one must stop the task
// instead of silently allowing it.
func TestLifecycleHooksFailClosed(t *testing.T) {
	sleep := "sleep 5"
	if runtime.GOOS == "windows" {
		sleep = "ping -n 6 127.0.0.1 >nul"
	}
	cases := []struct {
		name    string
		hooks   string
		sandbox sandbox.Policy
		want    string
	}{
		{"non-zero exit", `{"g":{"PrePatch":[{"command":"exit 3"}]}}`, sandbox.Policy{}, "failed"},
		{"timeout", `{"g":{"PrePatch":[{"command":` + strconv.Quote(sleep) + `,"timeout":1}]}}`, sandbox.Policy{}, "timed out"},
		{"malformed json", `{"g":`, sandbox.Policy{}, "invalid .apw/hooks.json"},
		{"bad matcher", `{"g":{"PrePatch":[{"command":"exit 0","matcher":"("}]}}`, sandbox.Policy{}, "invalid matcher"},
		{"sandbox active", `{"g":{"PrePatch":[{"command":"exit 0"}]}}`, sandbox.Policy{Mode: sandbox.ModeDocker}, "refused while sandbox"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, ".apw"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".apw", "hooks.json"), []byte(tc.hooks), 0o644); err != nil {
				t.Fatal(err)
			}
			e := &Executor{Workspace: dir, Trusted: true, Sandbox: tc.sandbox}
			_, err := e.runLifecycleHooks(context.Background(), HookPrePatch, "main.go", "patch")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}

	// No hooks.json, a non-matching hook under sandbox, and an untrusted workspace stay allowed.
	dir := t.TempDir()
	e := &Executor{Workspace: dir, Trusted: true}
	if _, err := e.runLifecycleHooks(context.Background(), HookPrePatch, "main.go", "patch"); err != nil {
		t.Fatalf("missing hooks.json must allow: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".apw"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".apw", "hooks.json"), []byte(`{"g":{"PrePatch":[{"command":"exit 3","matcher":"[.]lock$"}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	e.Sandbox = sandbox.Policy{Mode: sandbox.ModeDocker}
	if _, err := e.runLifecycleHooks(context.Background(), HookPrePatch, "main.go", "patch"); err != nil {
		t.Fatalf("non-matching hook must not block: %v", err)
	}
	e.Trusted = false
	if _, err := e.runLifecycleHooks(context.Background(), HookPrePatch, "x.lock", "patch"); err != nil {
		t.Fatalf("untrusted workspace runs no hooks: %v", err)
	}
}
