package csvsum

import (
	"strings"
	"testing"
)

func TestHiddenSum(t *testing.T) {
	if _, err := SumColumn(strings.NewReader("a,b\n1,2\n3\n"), 1); err == nil || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("short row error = %v", err)
	}
	if got, err := SumColumn(strings.NewReader("a,b\n 1 , 2 \n3, 4\n"), 1); err != nil || got != 6 {
		t.Fatalf("trimmed sum = %v, %v", got, err)
	}
	if _, err := SumColumn(strings.NewReader("a\n1\n"), -1); err == nil {
		t.Fatal("negative column accepted")
	}
	if got, err := SumColumn(strings.NewReader("a\n"), 0); err != nil || got != 0 {
		t.Fatalf("header only = %v, %v", got, err)
	}
}
