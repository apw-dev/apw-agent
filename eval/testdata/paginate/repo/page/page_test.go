package page

import "testing"

func TestPageShortList(t *testing.T) {
	got := Page([]string{"a", "b", "c"}, 1, 10)
	if len(got) != 3 {
		t.Fatalf("Page = %v", got)
	}
}
