package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnionBoolMaps(t *testing.T) {
	t.Parallel()

	t.Run("no inputs returns empty map", func(t *testing.T) {
		got := UnionBoolMaps[string]()
		assert.Equal(t, map[string]bool{}, got)
	})

	t.Run("single map is copied", func(t *testing.T) {
		in := map[string]bool{"a": true, "b": true}
		got := UnionBoolMaps(in)
		assert.Equal(t, in, got)
		// Mutating the result must not affect the input.
		got["c"] = true
		assert.NotContains(t, in, "c")
	})

	t.Run("two disjoint maps", func(t *testing.T) {
		got := UnionBoolMaps(
			map[string]bool{"a": true},
			map[string]bool{"b": true},
		)
		assert.Equal(t, map[string]bool{"a": true, "b": true}, got)
	})

	t.Run("overlapping keys deduplicate", func(t *testing.T) {
		got := UnionBoolMaps(
			map[string]bool{"a": true, "b": true},
			map[string]bool{"b": true, "c": true},
		)
		assert.Equal(t, map[string]bool{"a": true, "b": true, "c": true}, got)
	})

	t.Run("false values are normalized to true", func(t *testing.T) {
		got := UnionBoolMaps(map[string]bool{"a": false})
		assert.Equal(t, map[string]bool{"a": true}, got)
	})

	t.Run("nil entries are skipped", func(t *testing.T) {
		got := UnionBoolMaps[string](nil, map[string]bool{"a": true}, nil)
		assert.Equal(t, map[string]bool{"a": true}, got)
	})

	t.Run("works with non-string key types", func(t *testing.T) {
		got := UnionBoolMaps(
			map[int]bool{1: true, 2: true},
			map[int]bool{2: true, 3: true},
		)
		assert.Equal(t, map[int]bool{1: true, 2: true, 3: true}, got)
	})

	t.Run("variadic with three or more maps", func(t *testing.T) {
		got := UnionBoolMaps(
			map[string]bool{"a": true},
			map[string]bool{"b": true},
			map[string]bool{"c": true},
			map[string]bool{"a": true, "d": true},
		)
		assert.Equal(t, map[string]bool{"a": true, "b": true, "c": true, "d": true}, got)
	})
}
