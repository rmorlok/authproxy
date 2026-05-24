package common

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringValueEnvVarBase64_HasValue(t *testing.T) {
	ctx := context.Background()

	t.Run("env var set", func(t *testing.T) {
		t.Setenv("TEST_SV_EVB_HAS", base64.StdEncoding.EncodeToString([]byte("hi")))
		v := &StringValueEnvVarBase64{EnvVar: "TEST_SV_EVB_HAS"}
		assert.True(t, v.HasValue(ctx))
	})

	t.Run("env var unset, no default", func(t *testing.T) {
		v := &StringValueEnvVarBase64{EnvVar: "TEST_SV_EVB_UNSET_NO_DEFAULT"}
		assert.False(t, v.HasValue(ctx))
	})

	t.Run("env var unset, with default", func(t *testing.T) {
		v := &StringValueEnvVarBase64{
			EnvVar:  "TEST_SV_EVB_UNSET_WITH_DEFAULT",
			Default: util.ToPtr(base64.StdEncoding.EncodeToString([]byte("default"))),
		}
		assert.True(t, v.HasValue(ctx))
	})
}

func TestStringValueEnvVarBase64_GetValue(t *testing.T) {
	ctx := context.Background()

	t.Run("env var set returns decoded value", func(t *testing.T) {
		t.Setenv("TEST_SV_EVB_GET", base64.StdEncoding.EncodeToString([]byte("decoded")))
		v := &StringValueEnvVarBase64{EnvVar: "TEST_SV_EVB_GET"}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "decoded", got)
	})

	t.Run("env var unset with default returns decoded default", func(t *testing.T) {
		v := &StringValueEnvVarBase64{
			EnvVar:  "TEST_SV_EVB_GET_UNSET",
			Default: util.ToPtr(base64.StdEncoding.EncodeToString([]byte("default-val"))),
		}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "default-val", got)
	})

	t.Run("env var unset without default returns error", func(t *testing.T) {
		v := &StringValueEnvVarBase64{EnvVar: "TEST_SV_EVB_GET_UNSET_NO_DEFAULT"}
		_, err := v.GetValue(ctx)
		require.Error(t, err)
	})

	t.Run("invalid base64 returns error", func(t *testing.T) {
		t.Setenv("TEST_SV_EVB_INVALID", "not!valid!base64!")
		v := &StringValueEnvVarBase64{EnvVar: "TEST_SV_EVB_INVALID"}
		_, err := v.GetValue(ctx)
		require.Error(t, err)
	})

	t.Run("env var takes precedence over default", func(t *testing.T) {
		t.Setenv("TEST_SV_EVB_PRECEDENCE", base64.StdEncoding.EncodeToString([]byte("from-env")))
		v := &StringValueEnvVarBase64{
			EnvVar:  "TEST_SV_EVB_PRECEDENCE",
			Default: util.ToPtr(base64.StdEncoding.EncodeToString([]byte("default-val"))),
		}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "from-env", got)
	})
}

func TestStringValueEnvVarBase64_Clone(t *testing.T) {
	t.Run("nil receiver returns nil", func(t *testing.T) {
		var v *StringValueEnvVarBase64
		assert.Nil(t, v.Clone())
	})

	t.Run("clone is independent copy", func(t *testing.T) {
		orig := &StringValueEnvVarBase64{
			EnvVar:  "FOO",
			Default: util.ToPtr("aGVsbG8="),
		}
		clone := orig.Clone().(*StringValueEnvVarBase64)
		assert.Equal(t, orig, clone)
		clone.EnvVar = "CHANGED"
		assert.NotEqual(t, orig.EnvVar, clone.EnvVar)
	})
}
