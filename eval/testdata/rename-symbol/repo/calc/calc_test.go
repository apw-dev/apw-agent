package calc

import "testing"

func TestCalc(t *testing.T) {
	if Calc(2, 3) != 5 || Report(1, 2) != "1 + 2 = 3" {
		t.Fatal("wrong result")
	}
}
