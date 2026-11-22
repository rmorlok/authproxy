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

// TestToolErrorPayloadLimits bounds code, message, and details together rather
// than granting each field an independent output allowance.
func TestToolErrorPayloadLimits(t *testing.T) {
	for _, test := range []struct {
		name, arguments string
		change          func(*javascript.Limits)
	}{
		{"bytes", `"C".repeat(40), "m".repeat(40), "d".repeat(40)`, func(l *javascript.Limits) { l.MaxOutputBytes = 100 }},
		{"nodes", `"FAILED", "Failed", Array(100).fill(0)`, func(l *javascript.Limits) { l.MaxNodes = 80 }},
		{"depth", `"FAILED", "Failed", {a:{b:{c:{d:{e:1}}}}}`, func(l *javascript.Limits) { l.MaxDepth = 4 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := expressionLimits()
			test.change(&limits)
			baseline, err := javascript.CompileTool(`async function execute() { throw new ToolError("FAILED", "Failed"); }`, nil, limits)
			require.NoError(t, err)

			// The captured input and an ordinary small error fit the same policy.
			require.Equal(t, "FAILED", evaluateToolError(t, baseline).Code)
			program, err := javascript.CompileTool("async function execute() { throw new ToolError("+test.arguments+"); }", nil, limits)
			require.NoError(t, err)

			// An oversized failure is never published as truncated or partial public data.
			result, err := program.Evaluate(context.Background(), fetchInput(t))
			require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
			require.Nil(t, result)
		})
	}
}

// TestToolErrorInitializationBudget charges every helper construction even if
// the handle is discarded and no explicit error ever reaches the root result.
func TestToolErrorInitializationBudget(t *testing.T) {
	for _, dimension := range []string{"work", "allocation"} {
		t.Run(dimension, func(t *testing.T) {
			limits := toolResultBudgetLimits(dimension, 16<<10)
			baseline, err := javascript.CompileTool(`new ToolError("FAILED", "Failed");
				async function execute() { return true; }`, nil, limits)
			require.NoError(t, err)

			// A single pure constructor is valid during publication and invocation.
			require.NoError(t, baseline.Validate(context.Background()))
			require.JSONEq(t, `true`, string(evaluateRichResult(t, baseline).StructuredContent))
			program, err := javascript.CompileTool(`
				try { for (let i = 0; i < 65536; i++) new ToolError("FAILED", "Failed"); } catch (_) {}
				async function execute() { return true; }
			`, nil, limits)
			require.NoError(t, err)

			// Catching exhaustion or releasing handles cannot restore cumulative policy.
			require.ErrorIs(t, program.Validate(context.Background()), apjsruntime.ErrLimitExceeded)
			result, err := program.Evaluate(context.Background(), fetchInput(t))
			require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
			require.Nil(t, result)
		})
	}
}

// TestToolErrorExhaustionStopsHTTP prevents a caught constructor budget failure
// from admitting later requests and retains evidence from an earlier send.
func TestToolErrorExhaustionStopsHTTP(t *testing.T) {
	for _, dimension := range []string{"work", "allocation"} {
		t.Run(dimension, func(t *testing.T) {
			limits := toolResultBudgetLimits(dimension, 16<<10)
			program, err := javascript.CompileTool(`async function execute() {
				await context.connections[0].fetch("https://provider.example/first");
				try { for (let i = 0; i < 65536; i++) new ToolError("FAILED", "Failed"); } catch (_) {}
				await context.connections[0].fetch("https://provider.example/later");
				return true;
			}`, nil, limits)
			require.NoError(t, err)
			var calls atomic.Int32
			options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return fetchResponse(http.StatusOK, http.NoBody), nil
			}, fetchLimits())

			// An earlier successful HTTP call does not replenish native helper budgets.
			result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
			require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
			require.Nil(t, result)
			require.EqualValues(t, 1, calls.Load())
			require.Equal(t, api.ToolExecutionStateMayHaveExecuted, options.Session.ExecutionState())
		})
	}
}

// TestToolErrorConversionSharesBudget makes typed failure conversion spend the
// allowance already consumed by native JSON output and previous phases.
func TestToolErrorConversionSharesBudget(t *testing.T) {
	for _, dimension := range []string{"work", "allocation"} {
		t.Run(dimension, func(t *testing.T) {
			limits := toolResultBudgetLimits(dimension, 40<<10)
			const payload = `{code: "FAILED", message: "Failed", details: "x".repeat(16384)}`
			ordinary, err := javascript.CompileTool("async function execute() { return "+payload+"; }", nil, limits)
			require.NoError(t, err)
			const source = `async function execute() { throw new ToolError("FAILED", "Failed", "x".repeat(16384)); }`
			converted, err := javascript.CompileTool(source, nil, limits)
			require.NoError(t, err)

			// The JSON fits output boundaries and is valid under a generous policy;
			// ordinary JSON conversion also fits the smaller shared allowance.
			require.NotNil(t, evaluateRichResult(t, ordinary).StructuredContent)
			generous := compileTool(t, source, nil)
			require.Len(t, evaluateToolError(t, generous).Details, 16386)

			// Typed conversion cannot publish the prior JSON after its shared ledger is exhausted.
			result, err := converted.Evaluate(context.Background(), fetchInput(t))
			require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
			require.Nil(t, result)
		})
	}
}
