package csvsum

import (
	"strings"
	"testing"
)

func TestSum(t *testing.T) {
	got, err := SumColumn(strings.NewReader("a,b\n1,2\n3,4\n"), 1)
	if err != nil || got != 6 {
		t.Fatalf("SumColumn = %v, %v", got, err)
	}
}
