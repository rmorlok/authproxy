package apjs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateBoolean(t *testing.T) {
	vars := map[string]any{
		"cfg": map[string]any{
			"region": "eu",
			"plan":   "enterprise",
		},
		"labels": map[string]string{
			"apxy/cxr/type": "salesforce",
		},
		"annotations": map[string]string{
			"setup-mode": "advanced",
		},
	}

	t.Run("true", func(t *testing.T) {
		result, err := EvaluateBoolean(`cfg.region === "eu" && labels["apxy/cxr/type"] === "salesforce"`, vars)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("false", func(t *testing.T) {
		result, err := EvaluateBoolean(`cfg.region === "us"`, vars)
		require.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("missing data is available to javascript", func(t *testing.T) {
		result, err := EvaluateBoolean(`cfg.missing === undefined && labels.missing === undefined && annotations.missing === undefined`, vars)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("non boolean return rejected", func(t *testing.T) {
		_, err := EvaluateBoolean(`cfg.region`, vars)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must return a boolean")
	})

	t.Run("javascript error rejected", func(t *testing.T) {
		_, err := EvaluateBoolean(`cfg.region ===`, vars)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "JS expression error")
	})
}
