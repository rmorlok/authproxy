package common

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringValueBase64_HasValue(t *testing.T) {
	ctx := context.Background()

	t.Run("non-empty base64", func(t *testing.T) {
		v := &StringValueBase64{Base64: base64.StdEncoding.EncodeToString([]byte("hello"))}
		assert.True(t, v.HasValue(ctx))
	})

	t.Run("empty base64", func(t *testing.T) {
		v := &StringValueBase64{Base64: ""}
		assert.False(t, v.HasValue(ctx))
	})
}

func TestStringValueBase64_GetValue(t *testing.T) {
	ctx := context.Background()

	t.Run("decodes valid base64", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte("hello world"))
		v := &StringValueBase64{Base64: encoded}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "hello world", got)
	})

	t.Run("invalid base64 returns error", func(t *testing.T) {
		v := &StringValueBase64{Base64: "not!valid!base64!!"}
		_, err := v.GetValue(ctx)
		require.Error(t, err)
	})

	t.Run("empty base64 returns empty string", func(t *testing.T) {
		v := &StringValueBase64{Base64: ""}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "", got)
	})
}

func TestStringValueBase64_Clone(t *testing.T) {
	t.Run("nil receiver returns nil", func(t *testing.T) {
		var v *StringValueBase64
		assert.Nil(t, v.Clone())
	})

	t.Run("clone is independent copy", func(t *testing.T) {
		orig := &StringValueBase64{Base64: "aGVsbG8="}
		clone := orig.Clone().(*StringValueBase64)
		assert.Equal(t, orig, clone)
		clone.Base64 = "Y2hhbmdlZA=="
		assert.NotEqual(t, orig.Base64, clone.Base64)
	})
}
