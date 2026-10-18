package stack

import "testing"

func TestEval(t *testing.T) {
	got, err := Eval([]string{"2", "3", "+", "4", "*"})
	if err != nil || got != 20 {
		t.Fatalf("Eval = %d, %v", got, err)
	}
}
