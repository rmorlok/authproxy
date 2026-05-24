package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringValueDirect_HasValue(t *testing.T) {
	ctx := context.Background()

	t.Run("non-empty value", func(t *testing.T) {
		v := &StringValueDirect{Value: "hello"}
		assert.True(t, v.HasValue(ctx))
	})

	t.Run("empty value", func(t *testing.T) {
		v := &StringValueDirect{Value: ""}
		assert.False(t, v.HasValue(ctx))
	})
}

func TestStringValueDirect_GetValue(t *testing.T) {
	ctx := context.Background()

	t.Run("returns value", func(t *testing.T) {
		v := &StringValueDirect{Value: "hello"}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "hello", got)
	})

	t.Run("returns empty value without error", func(t *testing.T) {
		v := &StringValueDirect{Value: ""}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "", got)
	})
}

func TestStringValueDirect_Clone(t *testing.T) {
	t.Run("nil receiver returns nil", func(t *testing.T) {
		var v *StringValueDirect
		assert.Nil(t, v.Clone())
	})

	t.Run("clone is independent copy", func(t *testing.T) {
		orig := &StringValueDirect{Value: "hello", IsDirect: true, IsNonString: true}
		clone := orig.Clone().(*StringValueDirect)
		assert.Equal(t, orig, clone)
		clone.Value = "changed"
		assert.NotEqual(t, orig.Value, clone.Value)
	})
}
