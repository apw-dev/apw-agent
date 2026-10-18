package words

import (
	"reflect"
	"testing"
)

func TestHiddenCount(t *testing.T) {
	got := Count("Hello, hello\tWORLD!\n  world... ?? Go;")
	want := map[string]int{"hello": 2, "world": 2, "go": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Count = %v, want %v", got, want)
	}
	if got := Count("   "); len(got) != 0 {
		t.Fatalf("Count(spaces) = %v", got)
	}
}
