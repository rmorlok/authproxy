package util

import "iter"

type Set[T comparable] struct {
	m map[T]struct{}
}

func NewSet[T comparable]() *Set[T] {
	return &Set[T]{m: make(map[T]struct{})}
}

func NewSetFrom[T comparable](vals []T) *Set[T] {
	s := &Set[T]{m: make(map[T]struct{}, len(vals))}
	for _, v := range vals {
		s.m[v] = struct{}{}
	}
	return s
}

func (s *Set[T]) Add(v T) {
	s.m[v] = struct{}{}
}

func (s *Set[T]) Remove(v T) {
	delete(s.m, v)
}

func (s *Set[T]) Contains(v T) bool {
	_, ok := s.m[v]
	return ok
}

func (s *Set[T]) Len() int {
	return len(s.m)
}

func (s *Set[T]) Pop() (T, bool) {
	for v := range s.m {
		delete(s.m, v)
		return v, true
	}
	var zero T
	return zero, false
}

// All returns an iterator over all elements in the set.
// It is safe to remove elements from the set during iteration.
func (s *Set[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range s.m {
			if !yield(v) {
				return
			}
		}
	}
}
