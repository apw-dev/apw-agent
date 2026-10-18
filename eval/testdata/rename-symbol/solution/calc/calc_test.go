package calc

import "testing"

func TestSum(t *testing.T) {
	if Sum(2, 3) != 5 || Report(1, 2) != "1 + 2 = 3" {
		t.Fatal("wrong result")
	}
}
