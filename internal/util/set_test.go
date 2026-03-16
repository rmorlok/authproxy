package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSet(t *testing.T) {
	s := NewSet[int]()
	assert.Equal(t, 0, s.Len())
}

func TestNewSetFrom(t *testing.T) {
	s := NewSetFrom([]string{"a", "b", "c", "a"})
	assert.Equal(t, 3, s.Len())
	assert.True(t, s.Contains("a"))
	assert.True(t, s.Contains("b"))
	assert.True(t, s.Contains("c"))
}

func TestNewSetFromEmpty(t *testing.T) {
	s := NewSetFrom([]int{})
	assert.Equal(t, 0, s.Len())
}

func TestSetAdd(t *testing.T) {
	s := NewSet[string]()
	s.Add("a")
	s.Add("b")
	s.Add("a") // duplicate

	assert.Equal(t, 2, s.Len())
	assert.True(t, s.Contains("a"))
	assert.True(t, s.Contains("b"))
}

func TestSetRemove(t *testing.T) {
	s := NewSet[int]()
	s.Add(1)
	s.Add(2)
	s.Add(3)

	s.Remove(2)
	assert.Equal(t, 2, s.Len())
	assert.False(t, s.Contains(2))
	assert.True(t, s.Contains(1))
	assert.True(t, s.Contains(3))

	// Removing a non-existent element is a no-op
	s.Remove(99)
	assert.Equal(t, 2, s.Len())
}

func TestSetContains(t *testing.T) {
	s := NewSet[string]()
	assert.False(t, s.Contains("x"))

	s.Add("x")
	assert.True(t, s.Contains("x"))

	s.Remove("x")
	assert.False(t, s.Contains("x"))
}

func TestSetPop(t *testing.T) {
	s := NewSet[int]()

	// Pop from empty set
	v, ok := s.Pop()
	assert.False(t, ok)
	assert.Equal(t, 0, v)

	// Pop from non-empty set
	s.Add(10)
	s.Add(20)

	v, ok = s.Pop()
	assert.True(t, ok)
	assert.True(t, v == 10 || v == 20)
	assert.Equal(t, 1, s.Len())
	assert.False(t, s.Contains(v))

	v2, ok := s.Pop()
	assert.True(t, ok)
	assert.NotEqual(t, v, v2)
	assert.Equal(t, 0, s.Len())

	// Pop from now-empty set
	_, ok = s.Pop()
	assert.False(t, ok)
}

func TestSetAll(t *testing.T) {
	s := NewSet[int]()
	s.Add(1)
	s.Add(2)
	s.Add(3)

	seen := make(map[int]bool)
	for v := range s.All() {
		seen[v] = true
	}

	assert.Len(t, seen, 3)
	assert.True(t, seen[1])
	assert.True(t, seen[2])
	assert.True(t, seen[3])
}

func TestSetAllEmpty(t *testing.T) {
	s := NewSet[string]()
	count := 0
	for range s.All() {
		count++
	}
	assert.Equal(t, 0, count)
}

func TestSetAllBreakEarly(t *testing.T) {
	s := NewSet[int]()
	for i := 0; i < 100; i++ {
		s.Add(i)
	}

	count := 0
	for range s.All() {
		count++
		if count == 3 {
			break
		}
	}

	assert.Equal(t, 3, count)
	// Set is unchanged after early break
	assert.Equal(t, 100, s.Len())
}

func TestSetRemoveDuringIteration(t *testing.T) {
	s := NewSet[int]()
	s.Add(1)
	s.Add(2)
	s.Add(3)
	s.Add(4)
	s.Add(5)

	// Remove every element we visit during iteration.
	// Go's map range guarantees this is safe.
	var removed []int
	for v := range s.All() {
		s.Remove(v)
		removed = append(removed, v)
	}

	// All elements were visited and removed
	require.Len(t, removed, 5)
	assert.Equal(t, 0, s.Len())
}

func TestSetRemoveOthersDuringIteration(t *testing.T) {
	// When iterating, remove elements that haven't been visited yet.
	// Go map range may or may not yield deleted keys, so we just verify
	// that the set is consistent afterward.
	s := NewSet[int]()
	s.Add(1)
	s.Add(2)
	s.Add(3)

	visited := NewSet[int]()
	for v := range s.All() {
		visited.Add(v)
		// Try to remove all others
		for i := 1; i <= 3; i++ {
			if i != v {
				s.Remove(i)
			}
		}
	}

	// At least the first element was visited
	assert.GreaterOrEqual(t, visited.Len(), 1)
	// The set retains only the last-visited element (since we delete all others each step).
	// Verify that every element remaining in the set was actually visited.
	for v := range s.All() {
		assert.True(t, visited.Contains(v), "set contains unvisited element %d", v)
	}
}

func TestSetAddDuringIteration(t *testing.T) {
	s := NewSet[int]()
	s.Add(1)

	visited := NewSet[int]()
	for v := range s.All() {
		visited.Add(v)
		if v == 1 {
			s.Add(2)
			s.Add(3)
		}
	}

	// Go map range may or may not yield newly added keys.
	// Just verify we visited at least 1 and the set has all 3.
	assert.GreaterOrEqual(t, visited.Len(), 1)
	assert.True(t, visited.Contains(1))
	assert.Equal(t, 3, s.Len())
}
