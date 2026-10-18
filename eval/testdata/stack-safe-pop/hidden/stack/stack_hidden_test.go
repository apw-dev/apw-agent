package stack

import "testing"

func TestHiddenSafeStack(t *testing.T) {
	var s Stack[string]
	if v, ok := s.Pop(); ok || v != "" {
		t.Fatal("Pop on empty stack")
	}
	if _, ok := s.Peek(); ok {
		t.Fatal("Peek on empty stack")
	}
	s.Push("a")
	s.Push("b")
	if v, ok := s.Peek(); !ok || v != "b" || s.Len() != 2 {
		t.Fatal("Peek removed or returned the wrong element")
	}
	if v, ok := s.Pop(); !ok || v != "b" || s.Len() != 1 {
		t.Fatal("Pop wrong")
	}
	for _, expr := range [][]string{{"+"}, {"1", "+"}, {"1", "2", "3", "-", "-", "*"}} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Eval(%v) panicked: %v", expr, r)
				}
			}()
			if _, err := Eval(expr); err == nil {
				t.Fatalf("Eval(%v) accepted too few operands", expr)
			}
		}()
	}
	if got, err := Eval([]string{"5", "1", "2", "+", "4", "*", "+", "3", "-"}); err != nil || got != 14 {
		t.Fatalf("Eval = %d, %v", got, err)
	}
}
