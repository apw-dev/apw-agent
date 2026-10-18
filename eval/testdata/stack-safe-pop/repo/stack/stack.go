package stack

// Stack is a LIFO stack.
type Stack[T any] struct {
	items []T
}

// Push adds v on top.
func (s *Stack[T]) Push(v T) {
	s.items = append(s.items, v)
}

// Pop removes and returns the top element.
func (s *Stack[T]) Pop() T {
	v := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return v
}

// Len reports the number of elements.
func (s *Stack[T]) Len() int {
	return len(s.items)
}
