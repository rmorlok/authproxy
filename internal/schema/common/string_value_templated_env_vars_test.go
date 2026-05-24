package common

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringValueTemplatedEnvVars_HasValue(t *testing.T) {
	ctx := context.Background()

	t.Run("all env vars set", func(t *testing.T) {
		t.Setenv("TEST_SV_TPL_HOST", "api")
		t.Setenv("TEST_SV_TPL_VERSION", "v1")
		v := &StringValueTemplatedEnvVars{
			Template: "https://{{TEST_SV_TPL_HOST}}.example.com/{{TEST_SV_TPL_VERSION}}",
		}
		assert.True(t, v.HasValue(ctx))
	})

	t.Run("one env var missing, no default", func(t *testing.T) {
		t.Setenv("TEST_SV_TPL_HOST2", "api")
		v := &StringValueTemplatedEnvVars{
			Template: "https://{{TEST_SV_TPL_HOST2}}.example.com/{{TEST_SV_TPL_VERSION_MISSING}}",
		}
		assert.False(t, v.HasValue(ctx))
	})

	t.Run("env var missing, with default", func(t *testing.T) {
		v := &StringValueTemplatedEnvVars{
			Template: "https://{{TEST_SV_TPL_VAR_MISSING_A}}.example.com",
			Default:  util.ToPtr("https://default.example.com"),
		}
		assert.True(t, v.HasValue(ctx))
	})

	t.Run("env var missing with empty default", func(t *testing.T) {
		v := &StringValueTemplatedEnvVars{
			Template: "https://{{TEST_SV_TPL_VAR_MISSING_B}}.example.com",
			Default:  util.ToPtr(""),
		}
		assert.False(t, v.HasValue(ctx))
	})

	t.Run("env var set to empty string treated as missing", func(t *testing.T) {
		t.Setenv("TEST_SV_TPL_EMPTY", "")
		v := &StringValueTemplatedEnvVars{
			Template: "https://{{TEST_SV_TPL_EMPTY}}.example.com",
			Default:  util.ToPtr("https://default.example.com"),
		}
		assert.True(t, v.HasValue(ctx))
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "https://default.example.com", got)
	})

	t.Run("template with no variables", func(t *testing.T) {
		v := &StringValueTemplatedEnvVars{
			Template: "https://example.com/api",
		}
		assert.True(t, v.HasValue(ctx))
	})

	t.Run("empty template with no default", func(t *testing.T) {
		v := &StringValueTemplatedEnvVars{Template: ""}
		assert.False(t, v.HasValue(ctx))
	})

	t.Run("empty template with default", func(t *testing.T) {
		v := &StringValueTemplatedEnvVars{
			Template: "",
			Default:  util.ToPtr("a default"),
		}
		assert.True(t, v.HasValue(ctx))
	})
}

func TestStringValueTemplatedEnvVars_GetValue(t *testing.T) {
	ctx := context.Background()

	t.Run("all env vars set, renders template", func(t *testing.T) {
		t.Setenv("TEST_SV_TPL_GET_HOST", "api")
		t.Setenv("TEST_SV_TPL_GET_VERSION", "v2")
		v := &StringValueTemplatedEnvVars{
			Template: "https://{{TEST_SV_TPL_GET_HOST}}.example.com/{{TEST_SV_TPL_GET_VERSION}}",
		}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "https://api.example.com/v2", got)
	})

	t.Run("missing env var falls back to default", func(t *testing.T) {
		v := &StringValueTemplatedEnvVars{
			Template: "https://{{TEST_SV_TPL_GET_MISSING_A}}.example.com",
			Default:  util.ToPtr("https://default.example.com"),
		}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "https://default.example.com", got)
	})

	t.Run("missing env var with no default returns error", func(t *testing.T) {
		v := &StringValueTemplatedEnvVars{
			Template: "https://{{TEST_SV_TPL_GET_MISSING_B}}.example.com",
		}
		_, err := v.GetValue(ctx)
		require.Error(t, err)
	})

	t.Run("partial set falls back to default", func(t *testing.T) {
		t.Setenv("TEST_SV_TPL_GET_PARTIAL_A", "set")
		v := &StringValueTemplatedEnvVars{
			Template: "{{TEST_SV_TPL_GET_PARTIAL_A}}/{{TEST_SV_TPL_GET_PARTIAL_MISSING}}",
			Default:  util.ToPtr("the-default"),
		}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "the-default", got)
	})

	t.Run("template with no variables returns template directly", func(t *testing.T) {
		v := &StringValueTemplatedEnvVars{
			Template: "https://example.com/api",
		}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/api", got)
	})

	t.Run("repeated variable resolves once", func(t *testing.T) {
		t.Setenv("TEST_SV_TPL_GET_REPEAT", "abc")
		v := &StringValueTemplatedEnvVars{
			Template: "{{TEST_SV_TPL_GET_REPEAT}}-{{TEST_SV_TPL_GET_REPEAT}}",
		}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "abc-abc", got)
	})

	t.Run("env var takes precedence over default when set", func(t *testing.T) {
		t.Setenv("TEST_SV_TPL_GET_PRECEDENCE", "from-env")
		v := &StringValueTemplatedEnvVars{
			Template: "{{TEST_SV_TPL_GET_PRECEDENCE}}",
			Default:  util.ToPtr("default-val"),
		}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "from-env", got)
	})

	t.Run("malformed template returns error", func(t *testing.T) {
		v := &StringValueTemplatedEnvVars{
			Template: "{{unclosed",
		}
		_, err := v.GetValue(ctx)
		require.Error(t, err)
	})
}

func TestStringValueTemplatedEnvVars_Clone(t *testing.T) {
	t.Run("nil receiver returns nil", func(t *testing.T) {
		var v *StringValueTemplatedEnvVars
		assert.Nil(t, v.Clone())
	})

	t.Run("clone is independent copy", func(t *testing.T) {
		orig := &StringValueTemplatedEnvVars{
			Template: "https://{{HOST}}.example.com",
			Default:  util.ToPtr("https://default.example.com"),
		}
		clone := orig.Clone().(*StringValueTemplatedEnvVars)
		assert.Equal(t, orig, clone)
		clone.Template = "changed"
		assert.NotEqual(t, orig.Template, clone.Template)
	})
}

func TestStringValueTemplatedEnvVars_StringValueIntegration(t *testing.T) {
	ctx := context.Background()

	t.Run("HasValue routes through StringValue wrapper", func(t *testing.T) {
		t.Setenv("TEST_SV_TPL_INT_A", "x")
		sv := &StringValue{InnerVal: &StringValueTemplatedEnvVars{
			Template: "{{TEST_SV_TPL_INT_A}}",
		}}
		assert.True(t, sv.HasValue(ctx))

		got, err := sv.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "x", got)
	})

	t.Run("CloneValue produces equal wrapper", func(t *testing.T) {
		sv := &StringValue{InnerVal: &StringValueTemplatedEnvVars{
			Template: "{{HOST}}",
			Default:  util.ToPtr("default"),
		}}
		clone := sv.CloneValue()
		require.NotNil(t, clone)
		assert.Equal(t, sv, clone)
	})
}
