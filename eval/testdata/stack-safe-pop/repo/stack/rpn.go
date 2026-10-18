package stack

import (
	"fmt"
	"strconv"
)

// Eval evaluates a reverse Polish expression of integers with + - *.
func Eval(tokens []string) (int, error) {
	var s Stack[int]
	for _, tok := range tokens {
		switch tok {
		case "+", "-", "*":
			b := s.Pop()
			a := s.Pop()
			switch tok {
			case "+":
				s.Push(a + b)
			case "-":
				s.Push(a - b)
			default:
				s.Push(a * b)
			}
		default:
			n, err := strconv.Atoi(tok)
			if err != nil {
				return 0, fmt.Errorf("bad token %q", tok)
			}
			s.Push(n)
		}
	}
	if s.Len() != 1 {
		return 0, fmt.Errorf("invalid expression")
	}
	return s.Pop(), nil
}
