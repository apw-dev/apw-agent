package reflex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/project"
)

// TestHelperProcess is not a real test: stage tests run the test binary itself as
// a portable fake tool. APW_HELPER_OUT is printed, APW_HELPER_EXIT is the exit code
// and each run appends a line to APW_HELPER_LOG.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("APW_HELPER") != "1" {
		return
	}
	if log := os.Getenv("APW_HELPER_LOG"); log != "" {
		f, _ := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		fmt.Fprintln(f, os.Getenv("APW_HELPER_NAME")+" CI="+os.Getenv("CI"))
		f.Close()
	}
	fmt.Print(os.Getenv("APW_HELPER_OUT"))
	if os.Getenv("APW_HELPER_EXIT") == "1" {
		os.Exit(1)
	}
	os.Exit(0)
}

func helperStage(name, out string, fail bool, log string, extra ...string) project.Stage {
	exit := "0"
	if fail {
		exit = "1"
	}
	env := append([]string{"APW_HELPER=1", "APW_HELPER_NAME=" + name, "APW_HELPER_OUT=" + out, "APW_HELPER_EXIT=" + exit, "APW_HELPER_LOG=" + log}, extra...)
	return project.Stage{Name: name, Command: []string{os.Args[0], "-test.run=^TestHelperProcess$"}, Env: env}
}

func readLog(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Fields(strings.ReplaceAll(string(data), "\n", " "))
}

func TestStagesRunInOrderAndStopAtFirstFailure(t *testing.T) {
	t.Setenv("CI", "") // CI runners export CI=true; the helper logs the inherited value.
	root := t.TempDir()
	log := filepath.Join(t.TempDir(), "log")
	p := project.Profile{Kind: "node", Summary: "fixture", Stages: []project.Stage{
		helperStage("lint", "", false, log),
		helperStage("typecheck", "src/App.tsx(3,7): error TS2322: Type 'number' is not assignable to type 'string'.\n", true, log),
		helperStage("test", "", false, log),
	}}
	res := NewVerifierForProfile(root, p).Verify(context.Background())
	if res.Passed || res.FailedStage != "typecheck" {
		t.Fatalf("result: %+v", res)
	}
	if len(res.Diagnostics) == 0 || res.Diagnostics[0].File != "src/App.tsx" || res.Diagnostics[0].Line != 3 || res.Diagnostics[0].Col != 7 {
		t.Fatalf("tsc diagnostic not parsed: %+v", res.Diagnostics)
	}
	if got := strings.Join(readLog(t, log), " "); got != "lint CI= typecheck CI=" {
		t.Fatalf("stage order/stop: %q", got)
	}
}

func TestStageEnvironmentAndSkipIfMissing(t *testing.T) {
	root := t.TempDir()
	log := filepath.Join(t.TempDir(), "log")
	missing := project.Stage{Name: "fmt", Command: []string{"apw-definitely-not-installed-tool"}, SkipIfMissing: true}
	p := project.Profile{Kind: "custom", Stages: []project.Stage{missing, helperStage("test", "", false, log, "CI=true")}}
	if res := NewVerifierForProfile(root, p).Verify(context.Background()); !res.Passed {
		t.Fatalf("result: %+v", res)
	}
	if got := strings.Join(readLog(t, log), " "); got != "test CI=true" {
		t.Fatalf("env not passed: %q", got)
	}
	required := project.Stage{Name: "lint", Command: []string{"apw-definitely-not-installed-tool"}}
	res := NewVerifierForProfile(root, project.Profile{Stages: []project.Stage{required}}).Verify(context.Background())
	if res.Passed || res.FailedStage != "lint" || !strings.Contains(res.Diagnostics[0].Message, "not installed") {
		t.Fatalf("missing required tool must fail clearly: %+v", res)
	}
}

func TestInstallStageRunsOnceUntilInputsChange(t *testing.T) {
	t.Setenv("CI", "")
	root := t.TempDir()
	log := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"a":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	install := helperStage("install", "", false, log)
	install.Once, install.OnceInputs, install.StampDir = true, []string{"package.json"}, "node_modules"
	p := project.Profile{Stages: []project.Stage{install, helperStage("test", "", false, log)}}
	v := NewVerifierForProfile(root, p)
	for i := 0; i < 2; i++ {
		if res := v.Verify(context.Background()); !res.Passed {
			t.Fatalf("run %d: %+v", i, res)
		}
	}
	if got := strings.Join(readLog(t, log), " "); got != "install CI= test CI= test CI=" {
		t.Fatalf("install should run once: %q", got)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"a":2}`), 0644); err != nil {
		t.Fatal(err)
	}
	v.Verify(context.Background())
	if got := readLog(t, log); len(got) != 10 || got[6] != "install" {
		t.Fatalf("changed package.json must reinstall: %v", got)
	}
}

func TestUnknownProjectWithoutStagesFailsWithGuidance(t *testing.T) {
	res := NewVerifierForProfile(t.TempDir(), project.Profile{Kind: "unknown", Summary: "Unrecognized project type"}).Verify(context.Background())
	if res.Passed || res.FailedStage != "verify" || !strings.Contains(res.Diagnostics[0].Message, "verify.stages") {
		t.Fatalf("result: %+v", res)
	}
}

func TestParseDiagnosticsAcrossTools(t *testing.T) {
	raw := strings.Join([]string{
		"\x1b[31msrc/App.tsx(12,5): error TS2322: Type 'number' is not assignable to type 'string'.\x1b[0m",
		"src/main.ts:4:10 - error TS2304: Cannot find name 'foo'.",
		"",
		"C:/proj/src/Button.tsx",
		"  8:3  error  'x' is assigned a value but never used  no-unused-vars",
		"",
		" FAIL  src/App.test.tsx > App > renders title",
		"   ❯ src/App.test.tsx:15:22",
		"error[E0308]: mismatched types",
		"  --> src/main.rs:2:5",
		`  File "app/main.py", line 3, in <module>`,
		"main.go:7:2: undefined: bar",
	}, "\n")
	diags := ParseDiagnostics("test", raw)
	want := map[string][2]int{
		"src/App.tsx":            {12, 5},
		"src/main.ts":            {4, 10},
		"C:/proj/src/Button.tsx": {8, 3},
		"src/App.test.tsx":       {15, 22},
		"src/main.rs":            {2, 5},
		"app/main.py":            {3, 0},
		"main.go":                {7, 2},
	}
	got := map[string][2]int{}
	for _, d := range diags {
		if _, seen := got[d.File]; !seen || d.Line > 0 {
			got[d.File] = [2]int{d.Line, d.Col}
		}
	}
	for file, pos := range want {
		if got[file] != pos {
			t.Fatalf("%s: got %v want %v (all: %+v)", file, got[file], pos, diags)
		}
	}
}

func TestHTMLLinksStage(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"index.html":      `<link href="css/site.css" rel="stylesheet"><script src="/js/app.js"></script><a href="https://example.com">x</a><a href="#top">t</a><img src="img/logo.png?v=2">`,
		"css/site.css":    "body{}",
		"img/logo.png":    "png",
		"about/info.html": `<a href="../index.html">home</a>`,
	}
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	p := project.Detect(root, nil)
	res := NewVerifierForProfile(root, p).Verify(context.Background())
	if res.Passed || res.FailedStage != "links" || len(res.Diagnostics) != 1 || !strings.Contains(res.Diagnostics[0].Message, "/js/app.js") || res.Diagnostics[0].File != "index.html" {
		t.Fatalf("only the missing script should be reported: %+v", res)
	}
	if err := os.MkdirAll(filepath.Join(root, "js"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "js", "app.js"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if res := NewVerifierForProfile(root, p).Verify(context.Background()); !res.Passed {
		t.Fatalf("all references exist: %+v", res)
	}
}

func TestFormatDiagnosticsBoundsRawOutput(t *testing.T) {
	big := strings.Repeat("x", maxPromptOutput) + "ทดสอบ final failure summary"
	text := FormatDiagnostics(&VerificationResult{FailedStage: "test", RawOutput: big})
	if !strings.Contains(text, "final failure summary") || !strings.Contains(text, "[earlier output omitted]") || len(text) > maxPromptOutput+1024 {
		t.Fatalf("raw output not bounded to its tail (len %d)", len(text))
	}
}

func TestInstallFailureSkipsSourceRepair(t *testing.T) {
	root := t.TempDir()
	install := helperStage("install", "ERR_PNPM_EXECUTOR_LIFECYCLE_SCRIPT_FAILED", true, "")
	install.Once, install.StampDir = true, "node_modules"
	engine := NewReflexEngineForProfile(root, 3, project.Profile{Stages: []project.Stage{install}})
	fixed := false
	attempts, err := engine.Run(context.Background(), func(context.Context, string) error {
		fixed = true
		return nil
	})
	if fixed || err == nil || !strings.Contains(err.Error(), "source repair was not attempted") || !strings.Contains(err.Error(), "LIFECYCLE_SCRIPT_FAILED") || len(attempts) != 1 {
		t.Fatalf("install failure must stop without repair: fixed=%v attempts=%d err=%v", fixed, len(attempts), err)
	}
}

func TestLongPathHint(t *testing.T) {
	long := "C:/" + strings.Repeat("d", windowsPathBudget)
	if runtimeIsWindows() {
		if longPathHint(long, "spawnSync esbuild.exe ENOENT") == "" {
			t.Fatal("expected a hint for a long worktree path with ENOENT")
		}
	}
	if longPathHint("C:/short", "ENOENT") != "" || longPathHint(long, "some other failure") != "" {
		t.Fatal("hint must only appear for long paths with ENOENT")
	}
}

func runtimeIsWindows() bool { return runtime.GOOS == "windows" }
