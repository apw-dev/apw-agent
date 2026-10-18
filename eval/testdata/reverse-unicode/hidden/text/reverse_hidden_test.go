package text

import (
	"testing"
	"unicode/utf8"
)

func TestHiddenReverseRunes(t *testing.T) {
	for in, want := range map[string]string{"": "", "héllo": "olléh", "กขค": "คขก", "日本語": "語本日"} {
		got := Reverse(in)
		if got != want || !utf8.ValidString(got) {
			t.Fatalf("Reverse(%q) = %q, want %q", in, got, want)
		}
	}
	if Reverse(Reverse("สวัสดีครับ")) != "สวัสดีครับ" {
		t.Fatal("Reverse is not an involution")
	}
}
