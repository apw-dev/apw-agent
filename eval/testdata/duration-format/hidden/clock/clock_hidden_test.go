package clock

import (
	"testing"
	"time"
)

func TestHiddenFormat(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0:                  "00:00:00",
		time.Second:        "00:00:01",
		3723 * time.Second: "01:02:03",
		27*time.Hour + 3*time.Minute + 9*time.Second: "27:03:09",
		1500 * time.Millisecond:                      "00:00:01",
		-(90 * time.Second):                          "-00:01:30",
		100 * time.Hour:                              "100:00:00",
	} {
		if got := Format(d); got != want {
			t.Fatalf("Format(%v) = %q, want %q", d, got, want)
		}
	}
}
