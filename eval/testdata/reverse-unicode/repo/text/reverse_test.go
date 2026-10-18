package text

import "testing"

func TestReverseASCII(t *testing.T) {
	if got := Reverse("abc"); got != "cba" {
		t.Fatalf("Reverse(abc) = %q", got)
	}
}
