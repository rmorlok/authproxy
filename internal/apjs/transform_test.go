package apjs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformJSON(t *testing.T) {
	t.Run("simple identity transform", func(t *testing.T) {
		data := []any{
			map[string]any{"value": "a", "label": "Alpha"},
			map[string]any{"value": "b", "label": "Beta"},
		}

		result, err := TransformJSON("data", data)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, "a", result[0].Value)
		assert.Equal(t, "Alpha", result[0].Label)
		assert.Equal(t, "b", result[1].Value)
		assert.Equal(t, "Beta", result[1].Label)
	})

	t.Run("map transform", func(t *testing.T) {
		data := []any{
			map[string]any{"id": "ws-1", "name": "Workspace One"},
			map[string]any{"id": "ws-2", "name": "Workspace Two"},
		}

		result, err := TransformJSON(`data.map(function(w) { return {value: w.id, label: w.name}; })`, data)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, "ws-1", result[0].Value)
		assert.Equal(t, "Workspace One", result[0].Label)
	})

	t.Run("arrow function map transform", func(t *testing.T) {
		data := []any{
			map[string]any{"id": "1", "title": "First"},
		}

		result, err := TransformJSON(`data.map(w => ({value: w.id, label: w.title}))`, data)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "1", result[0].Value)
		assert.Equal(t, "First", result[0].Label)
	})

	t.Run("nested data access", func(t *testing.T) {
		data := map[string]any{
			"items": []any{
				map[string]any{"id": "x", "name": "Ex"},
			},
		}

		result, err := TransformJSON(`data.items.map(i => ({value: i.id, label: i.name}))`, data)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "x", result[0].Value)
	})

	t.Run("numeric values coerced to string", func(t *testing.T) {
		data := []any{
			map[string]any{"value": 42, "label": "Forty Two"},
		}

		result, err := TransformJSON("data", data)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "42", result[0].Value)
	})

	t.Run("empty array returns empty slice", func(t *testing.T) {
		result, err := TransformJSON("data", []any{})
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("syntax error returns error", func(t *testing.T) {
		_, err := TransformJSON("invalid(((", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "JS expression error")
	})

	t.Run("returns undefined error", func(t *testing.T) {
		_, err := TransformJSON("undefined", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "returned undefined")
	})

	t.Run("returns null error", func(t *testing.T) {
		_, err := TransformJSON("null", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "returned null")
	})

	t.Run("non-array result returns error", func(t *testing.T) {
		_, err := TransformJSON(`"hello"`, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must return an array")
	})

	t.Run("missing value field returns error", func(t *testing.T) {
		data := []any{
			map[string]any{"label": "only label"},
		}

		_, err := TransformJSON("data", data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required 'value' or 'label'")
	})

	t.Run("missing label field returns error", func(t *testing.T) {
		data := []any{
			map[string]any{"value": "only value"},
		}

		_, err := TransformJSON("data", data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required 'value' or 'label'")
	})

	t.Run("non-object array element returns error", func(t *testing.T) {
		data := []any{"not an object"}

		_, err := TransformJSON("data", data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not an object")
	})

	t.Run("filter and map", func(t *testing.T) {
		data := []any{
			map[string]any{"id": "1", "name": "Active", "active": true},
			map[string]any{"id": "2", "name": "Inactive", "active": false},
			map[string]any{"id": "3", "name": "Also Active", "active": true},
		}

		result, err := TransformJSON(`data.filter(d => d.active).map(d => ({value: d.id, label: d.name}))`, data)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, "1", result[0].Value)
		assert.Equal(t, "3", result[1].Value)
	})
}
