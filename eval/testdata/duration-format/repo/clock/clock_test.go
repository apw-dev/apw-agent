package clock

import (
	"testing"
	"time"
)

func TestFormatNotEmpty(t *testing.T) {
	if Format(time.Second) == "" {
		t.Fatal("empty")
	}
}
