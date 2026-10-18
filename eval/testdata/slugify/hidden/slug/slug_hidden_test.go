package slug

import "testing"

func TestHiddenMake(t *testing.T) {
	for in, want := range map[string]string{
		"Hello, World!":           "hello-world",
		"  Go 1.27 -- release ":   "go-1-27-release",
		"Ünïcode & spaces":        "n-code-spaces",
		"---":                     "",
		"already-slug":            "already-slug",
		"ภาษาไทย then English 42": "then-english-42",
	} {
		if got := Make(in); got != want {
			t.Fatalf("Make(%q) = %q, want %q", in, got, want)
		}
	}
}
