package common

import (
	"context"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
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

// TestPredicateGetValueWithContext forwards finite policy and cancellation to
// the bounded connector adapter while retaining ordinary predicate behavior.
func TestPredicateGetValueWithContext(t *testing.T) {
	limits := jsruntime.Limits{
		MaxSourceBytes: 4096, MaxInputBytes: 4096, MaxOutputBytes: 5,
		MaxDepth: 16, MaxNodes: 100, MaxStackDepth: 64, Timeout: time.Second,
	}
	jsctx := apjs.NewContext(nil, map[string]any{"cfg": map[string]any{"enabled": true}})
	predicate := &Predicate{Javascript: `var enabled = cfg.enabled; enabled;`}

	// Statement-list syntax remains valid and produces a primitive Boolean.
	value, err := predicate.GetValueWithContext(context.Background(), jsctx, limits)
	require.NoError(t, err)
	require.True(t, value)

	// The caller's source cap is enforced instead of falling back to legacy execution.
	tight := limits
	tight.MaxSourceBytes = len(predicate.Javascript) - 1
	value, err = predicate.GetValueWithContext(context.Background(), jsctx, tight)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.False(t, value)

	// A canceled request keeps its context error and never yields a positive match.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	value, err = predicate.GetValueWithContext(ctx, jsctx, limits)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, value)

	// Omission must be handled by the resource caller, not interpreted as a match.
	value, err = (*Predicate)(nil).GetValueWithContext(context.Background(), jsctx, limits)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.False(t, value)
}
