package pagination

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

func TestSplitOrderByParam(t *testing.T) {
	t.Run("it works for a single field value with no order", func(t *testing.T) {
		field, order, err := SplitOrderByParam[string]("foo")
		require.NoError(t, err)
		require.Equal(t, "foo", field)
		require.Equal(t, OrderByAsc, order)
	})

	t.Run("it works for a single field value with ascending order", func(t *testing.T) {
		field, order, err := SplitOrderByParam[string]("foo ASC")
		require.NoError(t, err)
		require.Equal(t, "foo", field)
		require.Equal(t, OrderByAsc, order)
	})

	t.Run("it works for a single field value with ascending ignoring capitalization", func(t *testing.T) {
		field, order, err := SplitOrderByParam[string]("foo Asc")
		require.NoError(t, err)
		require.Equal(t, "foo", field)
		require.Equal(t, OrderByAsc, order)
	})

	t.Run("it works for a single field value with descending order", func(t *testing.T) {
		field, order, err := SplitOrderByParam[string]("foo DESC")
		require.NoError(t, err)
		require.Equal(t, "foo", field)
		require.Equal(t, OrderByDesc, order)
	})

	t.Run("it works for a single field value with descending ignoring capitalization", func(t *testing.T) {
		field, order, err := SplitOrderByParam[string]("foo DeSC")
		require.NoError(t, err)
		require.Equal(t, "foo", field)
		require.Equal(t, OrderByDesc, order)
	})

	t.Run("rejects bad order", func(t *testing.T) {
		_, _, err := SplitOrderByParam[string]("foo bad")
		require.Error(t, err)
	})

	t.Run("rejects empty string", func(t *testing.T) {
		_, _, err := SplitOrderByParam[string]("")
		require.Error(t, err)
	})
}

func TestOrderBy_String(t *testing.T) {
	var ob *OrderBy
	require.Equal(t, "ASC", ob.String())

	ob = util.ToPtr(OrderByAsc)
	require.Equal(t, "ASC", ob.String())
	ob = util.ToPtr(OrderByDesc)
	require.Equal(t, "DESC", ob.String())
}

func TestOrderBy_IsDesc(t *testing.T) {
	var ob *OrderBy
	require.False(t, ob.IsDesc())

	ob = util.ToPtr(OrderByAsc)
	require.False(t, ob.IsDesc())
	ob = util.ToPtr(OrderByDesc)
	require.True(t, ob.IsDesc())
}

func TestOrderBy_IsAsc(t *testing.T) {
	var ob *OrderBy
	require.True(t, ob.IsAsc())

	ob = util.ToPtr(OrderByAsc)
	require.True(t, ob.IsAsc())
	ob = util.ToPtr(OrderByDesc)
	require.False(t, ob.IsAsc())
}
