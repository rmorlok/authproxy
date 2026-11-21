package javascript

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dop251/goja"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// TestInputSnapshotUsesExecutionBudget keeps repeated bounded serialization from
// resetting the surrounding execution's cumulative work or storage allowance.
func TestInputSnapshotUsesExecutionBudget(t *testing.T) {
	input := Input{Context: inputTestContext(t, render.Input{
		Cfg: map[string]any{"tenant": strings.Repeat("x", 128)},
	})}
	limits := inputTestLimits()
	snapshot, _, err := inputJSON(context.Background(), input, limits)
	require.NoError(t, err)

	// Each local serialization fits, but the parent permits only two copies.
	limits.MaxWorkUnits, limits.MaxAllocationUnits = 2*len(snapshot), 2*len(snapshot)
	ctx, err := apjsruntime.WithExecutionBudget(context.Background(), limits)
	require.NoError(t, err)
	for range 2 {
		encoded, response, err := inputJSON(ctx, input, limits)
		require.NoError(t, err)
		require.JSONEq(t, string(snapshot), string(encoded))
		require.JSONEq(t, `{}`, string(response))
	}

	// Exhaustion returns no partial snapshot or response, despite fresh render caps.
	encoded, response, err := inputJSON(ctx, input, limits)
	require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
	require.Nil(t, encoded)
	require.Nil(t, response)
}

// TestAsyncInputReshapingSharesExecutionBudget makes raw-scope splitting and
// re-encoding spend one allowance rather than hiding intermediate native copies.
func TestAsyncInputReshapingSharesExecutionBudget(t *testing.T) {
	encoded := json.RawMessage(`{"cfg":{"tenant":"private-tenant"}}`)
	limits := inputTestLimits()
	limits.MaxAllocationUnits = 2 * len(encoded)
	ctx, err := apjsruntime.WithExecutionBudget(context.Background(), limits)
	require.NoError(t, err)
	fields, err := decodeAsyncInput(ctx, encoded)
	require.NoError(t, err)

	// Encoding the already-bounded scopes fits a fresh allowance on its own.
	fresh, err := apjsruntime.WithExecutionBudget(context.Background(), limits)
	require.NoError(t, err)
	result, err := encodeAsyncInput(fresh, fields)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), string(result))

	// Combining both native phases exceeds the shared allowance and drops output.
	result, err = encodeAsyncInput(ctx, fields)
	require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
	require.Nil(t, result)
	require.NotContains(t, err.Error(), "private-tenant")
}

// TestPrepareInvocationPreservesReshapeBudgetFailure ensures the Tool adapter
// returns safe budget failures without relabeling them as malformed input.
func TestPrepareInvocationPreservesReshapeBudgetFailure(t *testing.T) {
	input := Input{Context: inputTestContext(t, render.Input{})}
	limits := inputTestLimits()
	snapshot, _, err := inputJSON(context.Background(), input, limits)
	require.NoError(t, err)
	limits.MaxAllocationUnits = 2 * len(snapshot)
	ctx, err := apjsruntime.WithExecutionBudget(context.Background(), limits)
	require.NoError(t, err)
	program := &ToolProgram{limits: limits}
	vm := goja.New()
	invocation := &apjsruntime.AsyncInvocation{}

	// Serialization and scope splitting fit; re-encoding cannot allocate its copy.
	err = program.prepareInvocation(ctx, vm, invocation, input, nil, nil)
	require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
	require.Nil(t, invocation.Params)
	require.Nil(t, invocation.Context)
	require.Nil(t, vm.Get("params"))
}

// TestAsyncInputReshapingKeepsErrorsSafe withholds parser diagnostics while
// preserving caller cancellation before either native JSON operation starts.
func TestAsyncInputReshapingKeepsErrorsSafe(t *testing.T) {
	invalid := json.RawMessage(`{"private-field":`)
	// Malformed raw fields expose only the existing safe classification.
	fields, err := decodeAsyncInput(context.Background(), invalid)
	require.ErrorIs(t, err, apjsruntime.ErrInvalidInput)
	require.Nil(t, fields)
	result, err := encodeAsyncInput(context.Background(), map[string]json.RawMessage{"cfg": invalid})
	require.ErrorIs(t, err, apjsruntime.ErrInvalidInput)
	require.NotContains(t, err.Error(), "private-field")
	require.Nil(t, result)

	// Canceled callers retain their context error even when the bytes are invalid.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = decodeAsyncInput(ctx, invalid)
	require.ErrorIs(t, err, context.Canceled)
	_, err = encodeAsyncInput(ctx, map[string]json.RawMessage{"cfg": invalid})
	require.ErrorIs(t, err, context.Canceled)
}
