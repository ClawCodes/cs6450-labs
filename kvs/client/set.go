package main

// Implementation of simple Set data structure which uses a map under the hood
type Set[T comparable] struct {
	values map[T]struct{} // empty struct is 0 byte (i.e. map serves as a set)
}

func NewSet[T comparable]() *Set[T] {
	return &Set[T]{values: make(map[T]struct{})}
}

func (s *Set[T]) Add(val T) {
	s.values[val] = struct{}{}
}

func (s *Set[T]) Remove(val T) {
	delete(s.values, val)
}

func (s *Set[T]) Contains(val T) bool {
	_, exists := s.values[val]
	return exists
}

func (s *Set[T]) Size() int {
	return len(s.values)
}
