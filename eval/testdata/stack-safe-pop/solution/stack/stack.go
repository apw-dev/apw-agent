package stack

// Stack is a LIFO stack.
type Stack[T any] struct {
	items []T
}

// Push adds v on top.
func (s *Stack[T]) Push(v T) {
	s.items = append(s.items, v)
}

// Pop removes and returns the top element; ok is false on an empty stack.
func (s *Stack[T]) Pop() (T, bool) {
	var zero T
	if len(s.items) == 0 {
		return zero, false
	}
	v := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return v, true
}

// Peek returns the top element without removing it.
func (s *Stack[T]) Peek() (T, bool) {
	var zero T
	if len(s.items) == 0 {
		return zero, false
	}
	return s.items[len(s.items)-1], true
}

// Len reports the number of elements.
func (s *Stack[T]) Len() int {
	return len(s.items)
}
