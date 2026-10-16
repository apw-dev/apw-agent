package tui

import (
	"bytes"
	"strings"
	"testing"
)

func TestFilterCommands(t *testing.T) {
	all := FilterCommands("/")
	if len(all) == 0 {
		t.Fatal("expected commands for '/', got 0")
	}

	mo := FilterCommands("/mo")
	if len(mo) == 0 || mo[0].Name != "/model" {
		t.Fatalf("expected /model first for '/mo', got %+v", mo)
	}

	none := FilterCommands("normal text")
	if len(none) != 0 {
		t.Fatalf("expected no slash commands for normal text, got %d", len(none))
	}
}

func TestRenderDashboard(t *testing.T) {
	var buf bytes.Buffer
	RenderDashboard(&buf, ".", "0.1.0-alpha", "1234abcd")
	out := buf.String()
	for _, want := range []string{"Brain 1 (Reflex)", "Brain 2 (Deep)", "Daemon (apwd)", "Available Skills", "Welcome to Apiwat Agent!"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected dashboard output to contain %q, got:\n%s", want, out)
		}
	}
}
