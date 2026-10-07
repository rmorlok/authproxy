package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// cloneTestJSON exercises named byte slices without coupling utility tests to
// schema packages that themselves depend on util.
type cloneTestJSON []byte

// TestCloneRawJSONMapPreservesRepresentation ensures cloning does not parse or
// normalize JSON, and retains distinctions needed by presence-aware patches.
func TestCloneRawJSONMapPreservesRepresentation(t *testing.T) {
	t.Run("nil map", func(t *testing.T) {
		var original map[string]cloneTestJSON
		require.Nil(t, CloneRawJSONMap(original))
	})

	t.Run("empty map", func(t *testing.T) {
		original := map[string]cloneTestJSON{}
		clone := CloneRawJSONMap(original)
		require.NotNil(t, clone)
		require.Empty(t, clone)
		clone["added"] = cloneTestJSON("null")
		require.Empty(t, original)
	})

	t.Run("raw values", func(t *testing.T) {
		original := map[string]cloneTestJSON{
			"nil":       nil,
			"empty":     {},
			"null":      cloneTestJSON("null"),
			"malformed": cloneTestJSON(`{"unfinished":`),
			"verbatim":  cloneTestJSON(" \n{\"value\":9007199254740993,\"decimal\":1.2300}\t"),
		}
		clone := CloneRawJSONMap(original)
		require.Equal(t, original, clone)
		require.Contains(t, clone, "nil")
		require.Nil(t, clone["nil"])
		require.NotNil(t, clone["empty"])
		require.Empty(t, clone["empty"])
	})

	t.Run("unnamed byte slices", func(t *testing.T) {
		original := map[string][]byte{"value": []byte("null")}
		clone := CloneRawJSONMap(original)
		require.Equal(t, original, clone)
	})
}

// TestCloneRawJSONMapOwnsMapAndBytes checks independence in both directions,
// including entries whose source slices share the same backing array.
func TestCloneRawJSONMapOwnsMapAndBytes(t *testing.T) {
	shared := cloneTestJSON("null")
	original := map[string]cloneTestJSON{
		"first":  shared,
		"second": shared,
		"remove": cloneTestJSON("{}"),
	}
	clone := CloneRawJSONMap(original)

	clone["added"] = cloneTestJSON("[]")
	delete(clone, "remove")
	require.NotContains(t, original, "added")
	require.Contains(t, original, "remove")

	clone["first"][0] = 'N'
	require.Equal(t, cloneTestJSON("null"), original["first"])
	require.Equal(t, cloneTestJSON("null"), clone["second"])

	original["second"][1] = 'U'
	require.Equal(t, cloneTestJSON("Null"), clone["first"])
	require.Equal(t, cloneTestJSON("null"), clone["second"])
	original["second"] = cloneTestJSON("false")
	require.Equal(t, cloneTestJSON("null"), clone["second"])
}

// TestCloneValueScalar checks that nil stays nil while even a pointer to a
// scalar's zero value receives distinct, independently mutable storage.
func TestCloneValueScalar(t *testing.T) {
	require.Nil(t, CloneValue[int](nil))
	value := 0
	clone := CloneValue(&value)
	require.NotNil(t, clone)
	require.NotSame(t, &value, clone)
	require.Zero(t, *clone)

	*clone = 7
	require.Zero(t, value)
	value = 9
	require.Equal(t, 7, *clone)
}

// TestCloneValueStructIsShallow documents the ownership boundary: the outer
// struct is copied, but its nested pointer, map, and slice are deliberately
// shared until callers clone those fields separately.
func TestCloneValueStructIsShallow(t *testing.T) {
	value := 1
	original := struct {
		Name    string
		Pointer *int
		Labels  map[string]string
		Bytes   []byte
	}{
		Name: "original", Pointer: &value,
		Labels: map[string]string{"state": "old"}, Bytes: []byte("old"),
	}
	clone := CloneValue(&original)
	require.NotSame(t, &original, clone)
	require.Equal(t, original, *clone)

	clone.Name = "copy"
	require.Equal(t, "original", original.Name)
	require.Same(t, original.Pointer, clone.Pointer)
	*clone.Pointer = 2
	clone.Labels["state"] = "new"
	clone.Bytes[0] = 'O'
	require.Equal(t, 2, *original.Pointer)
	require.Equal(t, "new", original.Labels["state"])
	require.Equal(t, []byte("Old"), original.Bytes)

	// Replacing an outer field affects only the copied struct, even though
	// mutations through the previous shared field reached the original.
	replacement := 3
	clone.Pointer = &replacement
	clone.Labels = map[string]string{"state": "replacement"}
	clone.Bytes = []byte("replacement")
	require.Equal(t, 2, *original.Pointer)
	require.Equal(t, "new", original.Labels["state"])
	require.Equal(t, []byte("Old"), original.Bytes)
}
