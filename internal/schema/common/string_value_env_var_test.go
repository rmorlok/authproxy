package common

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringValueEnvVar_HasValue(t *testing.T) {
	ctx := context.Background()

	t.Run("env var set", func(t *testing.T) {
		t.Setenv("TEST_SV_EV_HAS", "value")
		v := &StringValueEnvVar{EnvVar: "TEST_SV_EV_HAS"}
		assert.True(t, v.HasValue(ctx))
	})

	t.Run("env var unset, no default", func(t *testing.T) {
		v := &StringValueEnvVar{EnvVar: "TEST_SV_EV_UNSET_NO_DEFAULT"}
		assert.False(t, v.HasValue(ctx))
	})

	t.Run("env var unset, with default", func(t *testing.T) {
		v := &StringValueEnvVar{
			EnvVar:  "TEST_SV_EV_UNSET_WITH_DEFAULT",
			Default: util.ToPtr("default-val"),
		}
		assert.True(t, v.HasValue(ctx))
	})

	t.Run("env var unset, with empty default", func(t *testing.T) {
		v := &StringValueEnvVar{
			EnvVar:  "TEST_SV_EV_UNSET_EMPTY_DEFAULT",
			Default: util.ToPtr(""),
		}
		assert.False(t, v.HasValue(ctx))
	})

	t.Run("env var set to empty string falls back to default", func(t *testing.T) {
		t.Setenv("TEST_SV_EV_EMPTY", "")
		v := &StringValueEnvVar{
			EnvVar:  "TEST_SV_EV_EMPTY",
			Default: util.ToPtr("default-val"),
		}
		assert.True(t, v.HasValue(ctx))
	})
}

func TestStringValueEnvVar_GetValue(t *testing.T) {
	ctx := context.Background()

	t.Run("env var set returns env value", func(t *testing.T) {
		t.Setenv("TEST_SV_EV_GET", "the-value")
		v := &StringValueEnvVar{EnvVar: "TEST_SV_EV_GET"}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "the-value", got)
	})

	t.Run("env var unset with default returns default", func(t *testing.T) {
		v := &StringValueEnvVar{
			EnvVar:  "TEST_SV_EV_GET_UNSET",
			Default: util.ToPtr("default-val"),
		}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "default-val", got)
	})

	t.Run("env var unset without default returns error", func(t *testing.T) {
		v := &StringValueEnvVar{EnvVar: "TEST_SV_EV_GET_UNSET_NO_DEFAULT"}
		_, err := v.GetValue(ctx)
		require.Error(t, err)
	})

	t.Run("env var set to empty string falls back to default", func(t *testing.T) {
		t.Setenv("TEST_SV_EV_GET_EMPTY", "")
		v := &StringValueEnvVar{
			EnvVar:  "TEST_SV_EV_GET_EMPTY",
			Default: util.ToPtr("default-val"),
		}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "default-val", got)
	})

	t.Run("env var takes precedence over default", func(t *testing.T) {
		t.Setenv("TEST_SV_EV_PRECEDENCE", "from-env")
		v := &StringValueEnvVar{
			EnvVar:  "TEST_SV_EV_PRECEDENCE",
			Default: util.ToPtr("default-val"),
		}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "from-env", got)
	})
}

func TestStringValueEnvVar_Clone(t *testing.T) {
	t.Run("nil receiver returns nil", func(t *testing.T) {
		var v *StringValueEnvVar
		assert.Nil(t, v.Clone())
	})

	t.Run("clone is independent copy", func(t *testing.T) {
		orig := &StringValueEnvVar{
			EnvVar:  "FOO",
			Default: util.ToPtr("bar"),
		}
		clone := orig.Clone().(*StringValueEnvVar)
		assert.Equal(t, orig, clone)
		clone.EnvVar = "CHANGED"
		assert.NotEqual(t, orig.EnvVar, clone.EnvVar)
	})
}
