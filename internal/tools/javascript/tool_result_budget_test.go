package javascript_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/stretchr/testify/require"
)

// toolResultBudgetLimits tightens one cumulative allowance while leaving the
// other allowance and ordinary input/output boundaries generous.
func toolResultBudgetLimits(dimension string, units int) javascript.Limits {
	limits := expressionLimits()
	if dimension == "work" {
		limits.MaxWorkUnits = units
	} else {
		limits.MaxAllocationUnits = units
	}
	return limits
}

// TestToolResultInitializationBudget charges discarded helper handles during
// publication and invocation, even when no rich content is ultimately returned.
func TestToolResultInitializationBudget(t *testing.T) {
	for _, dimension := range []string{"work", "allocation"} {
		t.Run(dimension, func(t *testing.T) {
			limits := toolResultBudgetLimits(dimension, 16<<10)
			baseline, err := javascript.CompileTool(`
				toolResult({});
				async function execute() { return true; }
			`, nil, limits)
			require.NoError(t, err)

			// A normal initialization and the captured input fit the same policy.
			require.NoError(t, baseline.Validate(context.Background()))
			result, err := baseline.Evaluate(context.Background(), fetchInput(t))
			require.NoError(t, err)
			require.JSONEq(t, `true`, string(result.StructuredContent))

			program, err := javascript.CompileTool(`
				try { for (let i = 0; i < 65536; i++) { toolResult({}); } } catch (_) {}
				async function execute() { return true; }
			`, nil, limits)
			require.NoError(t, err)

			// Discarding handles cannot refund their cost, and catching exhaustion
			// cannot make either publication or invocation succeed.
			require.ErrorIs(t, program.Validate(context.Background()), apjsruntime.ErrLimitExceeded)
			result, err = program.Evaluate(context.Background(), fetchInput(t))
			require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
			require.Nil(t, result)
		})
	}
}

// TestToolResultHelperExhaustionStopsHTTP prevents caught helper exhaustion from
// permitting a later send while retaining evidence from requests already made.
func TestToolResultHelperExhaustionStopsHTTP(t *testing.T) {
	for _, dimension := range []string{"work", "allocation"} {
		for _, before := range []bool{true, false} {
			phase := "before first request"
			if !before {
				phase = "after first request"
			}
			t.Run(dimension+"/"+phase, func(t *testing.T) {
				limits := toolResultBudgetLimits(dimension, 16<<10)
				prefix := ""
				var expectedCalls int32
				expectedState := api.ToolExecutionStateNotStarted
				if !before {
					prefix = `await context.connections[0].fetch("https://provider.example/first");`
					expectedCalls = 1
					expectedState = api.ToolExecutionStateMayHaveExecuted
				}
				program, err := javascript.CompileTool(`async function execute() {
					`+prefix+`
					try { for (let i = 0; i < 65536; i++) { toolResult({}); } } catch (_) {}
					await context.connections[0].fetch("https://provider.example/later");
					return true;
				}`, nil, limits)
				require.NoError(t, err)
				var calls atomic.Int32
				options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
					calls.Add(1)
					return fetchResponse(http.StatusOK, http.NoBody), nil
				}, fetchLimits())

				// Only helper creation is repeated; the payload and final result are tiny.
				result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)

				// Exhaustion is terminal despite the catch, and earlier side effects remain visible.
				require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
				require.Nil(t, result)
				require.Equal(t, expectedCalls, calls.Load())
				require.Equal(t, expectedState, options.Session.ExecutionState())
			})
		}
	}
}

// TestToolResultNativeConversionSharesBudget makes derived text and typed rich
// decoding spend the same allowance as the JSON traversal that precedes them.
func TestToolResultNativeConversionSharesBudget(t *testing.T) {
	for _, test := range []struct {
		name, ordinary, converted string
	}{
		{
			name:      "ordinary string text block",
			ordinary:  `["x".repeat(16384)]`,
			converted: `"x".repeat(16384)`,
		},
		{
			name:      "typed rich content",
			ordinary:  `{content: [{type: "text", text: "x".repeat(16384)}]}`,
			converted: `toolResult({content: [{type: "text", text: "x".repeat(16384)}]})`,
		},
	} {
		for _, dimension := range []string{"work", "allocation"} {
			t.Run(test.name+"/"+dimension, func(t *testing.T) {
				limits := toolResultBudgetLimits(dimension, 40<<10)
				ordinary, err := javascript.CompileTool("async function execute() { return "+test.ordinary+"; }", nil, limits)
				require.NoError(t, err)
				converted, err := javascript.CompileTool("async function execute() { return "+test.converted+"; }", nil, limits)
				require.NoError(t, err)

				// The payload fits every output boundary and ordinary JSON conversion
				// fits the small shared allowance, without relying on exact counters.
				ordinaryResult, err := ordinary.Evaluate(context.Background(), fetchInput(t))
				require.NoError(t, err)
				require.NotNil(t, ordinaryResult)
				generous := compileTool(t, "async function execute() { return "+test.converted+"; }", nil)
				require.Len(t, evaluateRichResult(t, generous).Content, 1)

				// Additional native conversion cannot reset the work/allocation ledger
				// or publish the JSON produced before the remaining budget runs out.
				result, err := converted.Evaluate(context.Background(), fetchInput(t))
				require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
				require.Nil(t, result)
			})
		}
	}
}
