package common

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/stretchr/testify/require"
)

func TestPredicateValidate(t *testing.T) {
	vars := map[string]any{
		"sounds": map[string]string{
			"dog": "woof",
			"cat": "meow",
		},
	}
	jsctx := apjs.NewContext(nil, vars)

	t.Run("accepts boolean result", func(t *testing.T) {
		p := &Predicate{Javascript: `sounds.dog === 'woof'`}
		require.NoError(t, p.Validate(&ValidationContext{Path: "predicate"}, jsctx))
	})

	t.Run("accepts helper from context library", func(t *testing.T) {
		library, err := apjs.CompileAndValidateLibrary(`function soundsLikeDog() { return sounds.dog === "woof"; }`)
		require.NoError(t, err)

		p := &Predicate{Javascript: `soundsLikeDog()`}
		require.NoError(t, p.Validate(&ValidationContext{Path: "predicate"}, apjs.NewContext(library, vars)))
	})

	t.Run("rejects blank javascript", func(t *testing.T) {
		p := &Predicate{Javascript: " \n\t "}
		err := p.Validate(&ValidationContext{Path: "predicate"}, jsctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "predicate.javascript")
	})

	t.Run("rejects syntax errors", func(t *testing.T) {
		p := &Predicate{Javascript: `sounds.dog ===`}
		err := p.Validate(&ValidationContext{Path: "predicate"}, jsctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "must evaluate to a boolean")
	})

	t.Run("rejects non boolean results", func(t *testing.T) {
		p := &Predicate{Javascript: `sounds.dog`}
		err := p.Validate(&ValidationContext{Path: "predicate"}, jsctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "must evaluate to a boolean")
	})
}

func TestPredicateGetValue(t *testing.T) {
	vars := map[string]any{
		"sounds": map[string]string{
			"dog": "woof",
			"cat": "meow",
		},
	}
	jsctx := apjs.NewContext(nil, vars)

	t.Run("returns true", func(t *testing.T) {
		p := &Predicate{Javascript: `sounds.dog === 'woof'`}
		v, err := p.GetValue(jsctx)
		require.NoError(t, err)
		require.True(t, v)
	})

	t.Run("returns false", func(t *testing.T) {
		p := &Predicate{Javascript: `sounds.dog === 'meow'`}
		v, err := p.GetValue(jsctx)
		require.NoError(t, err)
		require.False(t, v)
	})

	t.Run("errors on blank javascript", func(t *testing.T) {
		p := &Predicate{Javascript: " \n\t "}
		_, err := p.GetValue(jsctx)
		require.Error(t, err)
	})

	t.Run("errors on syntax errors", func(t *testing.T) {
		p := &Predicate{Javascript: `sounds.dog ===`}
		_, err := p.GetValue(jsctx)
		require.Error(t, err)
	})

	t.Run("errors on non boolean results", func(t *testing.T) {
		p := &Predicate{Javascript: `sounds.dog`}
		_, err := p.GetValue(jsctx)
		require.Error(t, err)
	})
}
