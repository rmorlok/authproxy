package pagination

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitOrderByParam(t *testing.T) {
	t.Run("it works for a single field value with no order", func(t *testing.T) {
		field, order, err := SplitOrderByParam("foo")
		assert.NoError(t, err)
		assert.Equal(t, "foo", field)
		assert.Equal(t, OrderByAsc, order)
	})

	t.Run("it works for a single field value with ascending order", func(t *testing.T) {
		field, order, err := SplitOrderByParam("foo ASC")
		assert.NoError(t, err)
		assert.Equal(t, "foo", field)
		assert.Equal(t, OrderByAsc, order)
	})

	t.Run("it works for a single field value with ascending ignoring capitalization", func(t *testing.T) {
		field, order, err := SplitOrderByParam("foo Asc")
		assert.NoError(t, err)
		assert.Equal(t, "foo", field)
		assert.Equal(t, OrderByAsc, order)
	})

	t.Run("it works for a single field value with descending order", func(t *testing.T) {
		field, order, err := SplitOrderByParam("foo DESC")
		assert.NoError(t, err)
		assert.Equal(t, "foo", field)
		assert.Equal(t, OrderByDesc, order)
	})

	t.Run("it works for a single field value with descending ignoring capitalization", func(t *testing.T) {
		field, order, err := SplitOrderByParam("foo DeSC")
		assert.NoError(t, err)
		assert.Equal(t, "foo", field)
		assert.Equal(t, OrderByDesc, order)
	})

	t.Run("rejects bad order", func(t *testing.T) {
		_, _, err := SplitOrderByParam("foo bad")
		assert.Error(t, err)
	})

	t.Run("rejects empty string", func(t *testing.T) {
		_, _, err := SplitOrderByParam("")
		assert.Error(t, err)
	})
}
