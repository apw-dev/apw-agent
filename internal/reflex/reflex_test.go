package reflex

import (
	"context"
	"testing"
)

func TestParseGoErrors(t *testing.T) {
	raw := `internal\core\models.go:12:5: undefined: TaskStatus
internal\foo\bar.go:45: missing return at end of function`

	diags := ParseGoErrors("build", raw)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}

	if diags[0].Line != 12 || diags[0].Col != 5 || diags[0].Message != "undefined: TaskStatus" {
		t.Errorf("diag[0] parsed incorrectly: %+v", diags[0])
	}
	if diags[1].Line != 45 || diags[1].Col != 0 || diags[1].Message != "missing return at end of function" {
		t.Errorf("diag[1] parsed incorrectly: %+v", diags[1])
	}
}

func TestParseGoErrors_Prefixes(t *testing.T) {
	raw := "vet.exe: internal\\a\\b.go:92:25: undefined: X\n" +
		"    foo_test.go:12: expected 1, got 2"
	diags := ParseGoErrors("vet", raw)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if diags[0].File != `internal\a\b.go` {
		t.Errorf("diag[0].File = %q", diags[0].File)
	}
	if diags[1].File != "foo_test.go" || diags[1].Line != 12 {
		t.Errorf("diag[1] = %+v", diags[1])
	}
}

func TestFormatDiagnostics(t *testing.T) {
	res := &VerificationResult{
		Passed:      false,
		FailedStage: "build",
		Diagnostics: []Diagnostic{
			{
				Stage:   "build",
				File:    "main.go",
				Line:    10,
				Col:     2,
				Message: "undefined: Foo",
			},
		},
		RawOutput: "main.go:10:2: undefined: Foo\n",
	}

	msg := FormatDiagnostics(res)
	if msg == "" || len(msg) < 20 {
		t.Fatalf("formatted diagnostics message is too short or empty")
	}
}

func TestVerifier(t *testing.T) {
	v := NewVerifier(".")
	out, err := v.RunCommand(context.Background(), "go", "version")
	if err != nil || out == "" {
		t.Fatalf("expected go version to succeed, got %v: %s", err, out)
	}
}
