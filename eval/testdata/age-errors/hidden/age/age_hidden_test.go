package age

import (
	"errors"
	"strings"
	"testing"
)

func TestHiddenSentinel(t *testing.T) {
	for _, in := range []string{"abc", "-1", "151", "1e3", ""} {
		_, err := Parse(in)
		if !errors.Is(err, ErrInvalidAge) || !strings.Contains(err.Error(), in) {
			t.Fatalf("Parse(%q) error = %v", in, err)
		}
	}
	for _, in := range []string{"0", "150", "7"} {
		if _, err := Parse(in); err != nil {
			t.Fatalf("Parse(%q) = %v", in, err)
		}
	}
}
