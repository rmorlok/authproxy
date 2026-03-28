package aptmpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderMustache(t *testing.T) {
	t.Run("simple variable substitution", func(t *testing.T) {
		result, err := RenderMustache("https://{{tenant}}.example.com/api", map[string]any{
			"tenant": "acme",
		})
		require.NoError(t, err)
		assert.Equal(t, "https://acme.example.com/api", result)
	})

	t.Run("nested variable substitution", func(t *testing.T) {
		result, err := RenderMustache("https://{{configuration.tenant}}.example.com/oauth/authorize", map[string]any{
			"configuration": map[string]any{
				"tenant": "acme-corp",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "https://acme-corp.example.com/oauth/authorize", result)
	})

	t.Run("multiple variables", func(t *testing.T) {
		result, err := RenderMustache("https://{{configuration.tenant}}.example.com/{{configuration.version}}/api", map[string]any{
			"configuration": map[string]any{
				"tenant":  "acme",
				"version": "v2",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "https://acme.example.com/v2/api", result)
	})

	t.Run("no mustache syntax returns string unchanged", func(t *testing.T) {
		result, err := RenderMustache("https://example.com/api", map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/api", result)
	})

	t.Run("missing variable renders empty", func(t *testing.T) {
		result, err := RenderMustache("https://{{tenant}}.example.com", map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, "https://.example.com", result)
	})

	t.Run("nil data", func(t *testing.T) {
		result, err := RenderMustache("https://example.com", nil)
		require.NoError(t, err)
		assert.Equal(t, "https://example.com", result)
	})

	t.Run("empty template", func(t *testing.T) {
		result, err := RenderMustache("", map[string]any{"key": "value"})
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})
}

func TestContainsMustache(t *testing.T) {
	assert.True(t, ContainsMustache("https://{{tenant}}.example.com"))
	assert.True(t, ContainsMustache("{{foo}}"))
	assert.True(t, ContainsMustache("prefix{{bar}}suffix"))
	assert.False(t, ContainsMustache("https://example.com"))
	assert.False(t, ContainsMustache(""))
	assert.False(t, ContainsMustache("{not mustache}"))
	assert.False(t, ContainsMustache("{ {spaced} }"))
}
