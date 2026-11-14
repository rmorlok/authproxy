package common

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// TestPredicateValidateBounded checks copied validation inputs, limits, and safe
// error paths at the common entry point shared by connector predicate fields.
func TestPredicateValidateBounded(t *testing.T) {
	vars := map[string]any{"cfg": map[string]any{"enabled": true}}
	jsctx := apjs.NewContext(nil, vars)
	vc := &ValidationContext{Path: "predicate"}

	// Statement completion and boxed Boolean compatibility do not expose Go maps.
	p := &Predicate{Javascript: `cfg.enabled = false; new Boolean(true);`}
	require.NoError(t, p.Validate(vc, jsctx))
	require.Equal(t, true, vars["cfg"].(map[string]any)["enabled"])

	for _, tc := range []struct {
		name   string
		source string
		vars   map[string]any
		want   error
	}{
		{name: "source", source: strings.Repeat(" ", apjs.ConnectorPredicateLimits().MaxSourceBytes) + "true", want: jsruntime.ErrLimitExceeded},
		{name: "input", source: "true", vars: map[string]any{"padding": strings.Repeat("x", apjs.ConnectorPredicateLimits().MaxInputBytes)}, want: jsruntime.ErrLimitExceeded},
		{name: "private throw", source: `throw new Error("privateValidationDetail")`, want: jsruntime.ErrEvaluation},
		{name: "private syntax", source: "privateValidationDetail ===", want: jsruntime.ErrInvalidSource},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Schema errors retain the field path and safe category, not authored details.
			p := &Predicate{Javascript: tc.source}
			err := p.Validate(vc, apjs.NewContext(nil, tc.vars))
			require.ErrorContains(t, err, "predicate.javascript")
			require.ErrorContains(t, err, tc.want.Error())
			require.NotContains(t, err.Error(), "privateValidationDetail")
		})
	}

	// Nested validation observes cancellation before parsing authored source.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	vc = (&ValidationContext{Path: "setup", Ctx: ctx}).PushField("steps").PushIndex(0).PushField("if")
	p = &Predicate{Javascript: "privateValidationDetail ==="}
	err := p.Validate(vc, jsctx)
	require.ErrorContains(t, err, "setup.steps[0].if.javascript")
	require.ErrorContains(t, err, context.Canceled.Error())
	require.NotContains(t, err.Error(), "privateValidationDetail")
}

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
