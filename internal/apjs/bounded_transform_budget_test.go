package apjs

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// TestBoundedOptionsShareBudgetWithDetachedFormatting keeps the final Go option
// conversion under the same budget as native input, JavaScript, and JSON output.
func TestBoundedOptionsShareBudgetWithDetachedFormatting(t *testing.T) {
	const source = `[{value:"a",label:"b"}]`
	limits := boundedObjectTestLimits()
	limits.MaxAllocationUnits = 96
	jsctx := NewContext(nil, nil)

	// Each phase fits this allowance independently: first evaluate to JSON.
	encoded, err := jsctx.evaluateJSONWithContext(
		context.Background(), source, limits, jsruntime.JSONOutputOptions{UndefinedAsNull: true},
		func(_ context.Context, _ *goja.Runtime, value goja.Value) (goja.Value, error) { return value, nil },
	)
	require.NoError(t, err)
	require.JSONEq(t, `[ {"value":"a","label":"b"} ]`, string(encoded))

	// Detached decoding/formatting also fits a fresh allowance on its own.
	formatCtx, err := jsruntime.WithExecutionBudget(context.Background(), limits)
	require.NoError(t, err)
	options, err := dataSourceOptionsFromJSON(formatCtx, encoded, limits)
	require.NoError(t, err)
	require.Equal(t, []DataSourceOption{{Value: "a", Label: "b"}}, options)

	// The public operation must not replenish its allowance when the VM closes.
	options, err = jsctx.TransformOptionsWithContext(context.Background(), source, limits)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.Nil(t, options)
	diagnostic, ok := Diagnostic(err)
	require.True(t, ok)
	require.Equal(t, "output", diagnostic.Phase)

	// Increasing the parent allowance admits the same complete logical action.
	limits.MaxAllocationUnits = 4096
	options, err = jsctx.TransformOptionsWithContext(context.Background(), source, limits)
	require.NoError(t, err)
	require.Equal(t, []DataSourceOption{{Value: "a", Label: "b"}}, options)
}
