package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"
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

func TestRegisterInterrupt_DoublePress(t *testing.T) {
	s := NewSession(".", "test")
	t0 := time.Now()

	// (1) First press arms the interrupt but does not exit.
	if s.RegisterInterrupt(t0) {
		t.Fatal("first press: expected RegisterInterrupt to return false")
	}
	if !s.InterruptPending(t0) {
		t.Fatal("first press: expected InterruptPending to be true")
	}

	// (2) Second press within the window signals exit and clears pending state.
	if !s.RegisterInterrupt(t0.Add(500 * time.Millisecond)) {
		t.Fatal("second press within window: expected RegisterInterrupt to return true")
	}
	if s.InterruptPending(t0.Add(500 * time.Millisecond)) {
		t.Fatal("after exit press: expected InterruptPending to be false")
	}

	// (3) Window expiry: a press after the window re-arms instead of exiting.
	t1 := t0.Add(10 * time.Second)
	if s.RegisterInterrupt(t1) {
		t.Fatal("window expiry: expected first press at t1 to return false")
	}
	if s.RegisterInterrupt(t1.Add(ctrlCExitWindow + time.Second)) {
		t.Fatal("window expiry: expected press after window to re-arm and return false")
	}

	// (4) ClearInterrupt resets pending state so the next press only re-arms.
	t2 := t1.Add(3*ctrlCExitWindow + time.Minute)
	if s.RegisterInterrupt(t2) {
		t.Fatal("clear: expected first press at t2 to return false")
	}
	s.ClearInterrupt()
	if s.RegisterInterrupt(t2.Add(100 * time.Millisecond)) {
		t.Fatal("clear: expected press after ClearInterrupt to return false")
	}
}
