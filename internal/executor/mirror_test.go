package executor

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestQuotedLiterals(t *testing.T) {
	got := quotedLiterals("Host defaults to \"127.0.0.1\", port 8080, body `{\"status\":\"ok\"}`, \"127.0.0.1\" again, \"x\" too short")
	if !reflect.DeepEqual(got, []string{"127.0.0.1", `{"status":"ok"}`}) {
		t.Fatalf("literals = %q", got)
	}
}

func TestIsTestFile(t *testing.T) {
	for path, want := range map[string]bool{
		"conf/conf_test.go": true, "conf/conf.go": false, "src/App.test.tsx": true, "src/a.spec.ts": true,
		"tests/test_api.py": true, "pkg/test_util.py": true, "src/__tests__/x.js": true, "src/App.tsx": false, "tests/it.rs": true,
	} {
		if isTestFile(path) != want {
			t.Fatalf("isTestFile(%q) != %v", path, want)
		}
	}
}

func TestMirrorWarnings(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	instruction := "Host defaults to \"127.0.0.1\" and the error says \"invalid \\\"port\\\"\"."
	write("conf/conf.go", `const defaultHost = "localhost"`)
	write("conf/conf_test.go", `if c.Host != "localhost" { t.Fatal() }`)
	warnings := mirrorWarnings(instruction, root, []string{"conf/conf.go", "conf/conf_test.go"})
	if len(warnings) != 1 || !strings.Contains(warnings[0], `"127.0.0.1"`) || !strings.Contains(warnings[0], "mirroring the implementation") {
		t.Fatalf("warnings = %v", warnings)
	}
	write("conf/conf_test.go", `if c.Host != "127.0.0.1" { t.Fatal() }`)
	if w := mirrorWarnings("Host defaults to \"127.0.0.1\".", root, []string{"conf/conf.go", "conf/conf_test.go"}); w != nil {
		t.Fatalf("test asserting the stated value flagged: %v", w)
	}
	write("conf/msg_test.go", "want := \"invalid \\\"port\\\"\"")
	if w := mirrorWarnings("Error text \"invalid \\\"port\\\"\".", root, []string{"conf/msg_test.go"}); w != nil {
		t.Fatalf("escaped form not recognized: %v", w)
	}
	if w := mirrorWarnings("Host defaults to \"127.0.0.1\".", root, []string{"conf/conf.go"}); w != nil {
		t.Fatalf("warned without changed tests: %v", w)
	}
	if w := mirrorWarnings("no quotes here", root, []string{"conf/conf_test.go"}); w != nil {
		t.Fatalf("warned without quoted values: %v", w)
	}
}
