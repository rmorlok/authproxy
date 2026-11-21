package javascript_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	api "github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/stretchr/testify/require"
)

// TestToolBase64Policy verifies that a trusted caller can select shared pure
// helpers without a Tool-specific implementation or additional I/O capability.
func TestToolBase64Policy(t *testing.T) {
	limits := expressionLimits()
	program, err := javascript.CompileTool(`async function execute() {
		return [typeof atob, typeof btoa];
	}`, nil, limits)
	require.NoError(t, err)

	// Existing Tool profiles retain the absent-by-default helper surface.
	result, err := program.Evaluate(context.Background(), fetchInput(t))
	require.NoError(t, err)
	require.JSONEq(t, `["undefined","undefined"]`, string(result))

	// Explicit policy makes the same helpers available in validation and execution.
	limits.Utilities = jsruntime.UtilityPolicy{Base64: true, MaxCalls: 2, MaxInputBytes: 5, MaxOutputBytes: 5}
	program, err = javascript.CompileTool(`const encoded = btoa("\xff");
		async function execute() {
			return [atob(encoded).charCodeAt(0), typeof fetch, typeof URL, typeof TextEncoder];
		}`, nil, limits)
	require.NoError(t, err)
	require.NoError(t, program.Validate(context.Background()))
	result, err = program.Evaluate(context.Background(), fetchInput(t))
	require.NoError(t, err)
	require.JSONEq(t, `[255,"undefined","undefined","undefined"]`, string(result))
}

// TestToolUtilityExhaustionStopsHTTP checks the utility limit at the real Tool
// host boundary. Exhaustion stops further effects even inside authored try/catch;
// already-sent requests retain their evidence when the overall result fails.
func TestToolUtilityExhaustionStopsHTTP(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		calls  int32
		state  api.ToolExecutionState
	}{
		"initialization": {
			source: `btoa("a"); try { btoa("b"); } catch (_) {}
				async function execute(params, context) { await context.connections[0].fetch("https://provider.example/items"); return true; }`,
			state: api.ToolExecutionStateNotStarted,
		},
		"before first request": {
			source: `async function execute(params, context) {
				btoa("a"); try { btoa("b"); } catch (_) {}
				await context.connections[0].fetch("https://provider.example/items"); return true;
			}`,
			state: api.ToolExecutionStateNotStarted,
		},
		"after first request": {
			source: `async function execute(params, context) {
				await context.connections[0].fetch("https://provider.example/first");
				btoa("a"); try { btoa("b"); } catch (_) {}
				await context.connections[0].fetch("https://provider.example/second"); return true;
			}`,
			calls: 1, state: api.ToolExecutionStateMayHaveExecuted,
		},
	} {
		t.Run(name, func(t *testing.T) {
			// One utility call is permitted independently of the HTTP request budget.
			limits := expressionLimits()
			limits.Utilities = jsruntime.UtilityPolicy{Base64: true, MaxCalls: 1, MaxInputBytes: 16, MaxOutputBytes: 16}
			program, err := javascript.CompileTool(test.source, nil, limits)
			require.NoError(t, err)
			var calls atomic.Int32
			options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return fetchResponse(http.StatusOK, http.NoBody), nil
			}, fetchLimits())

			result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)

			// No partial result escapes, no later request starts, and prior effects remain visible.
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, result)
			require.Equal(t, test.calls, calls.Load())
			require.Equal(t, test.state, options.Session.ExecutionState())
		})
	}
}
