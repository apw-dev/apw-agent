package mathx

import "testing"

func TestMax(t *testing.T) {
	if Max(2, 5) != 5 || Max(-1, -3) != -1 {
		t.Fatal("Max is wrong")
	}
}
