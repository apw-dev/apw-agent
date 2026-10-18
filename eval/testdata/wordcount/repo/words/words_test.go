package words

import "testing"

func TestCountSimple(t *testing.T) {
	got := Count("a b a")
	if got["a"] != 2 || got["b"] != 1 {
		t.Fatalf("Count = %v", got)
	}
}
