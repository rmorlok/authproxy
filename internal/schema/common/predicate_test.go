package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPredicateValidate(t *testing.T) {
	vars := map[string]any{
		"cfg":         map[string]any{},
		"labels":      map[string]string{},
		"annotations": map[string]string{},
	}

	t.Run("accepts boolean result", func(t *testing.T) {
		p := &Predicate{Javascript: `cfg.enabled === true`}
		require.NoError(t, p.Validate(&ValidationContext{Path: "predicate"}, vars))
	})

	t.Run("rejects blank javascript", func(t *testing.T) {
		p := &Predicate{Javascript: " \n\t "}
		err := p.Validate(&ValidationContext{Path: "predicate"}, vars)
		require.Error(t, err)
		require.Contains(t, err.Error(), "predicate.javascript")
	})

	t.Run("rejects syntax errors", func(t *testing.T) {
		p := &Predicate{Javascript: `cfg.enabled ===`}
		err := p.Validate(&ValidationContext{Path: "predicate"}, vars)
		require.Error(t, err)
		require.Contains(t, err.Error(), "must evaluate to a boolean")
	})

	t.Run("rejects non boolean results", func(t *testing.T) {
		p := &Predicate{Javascript: `cfg.enabled`}
		err := p.Validate(&ValidationContext{Path: "predicate"}, vars)
		require.Error(t, err)
		require.Contains(t, err.Error(), "must evaluate to a boolean")
	})
}
