package javascript_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// TestToolURLHTTP resolves a query-bearing path against copied connection
// configuration and sends only the helper's explicit serialized URL string.
func TestToolURLHTTP(t *testing.T) {
	program, err := javascript.CompileTool(`
		const canonicalBase = new URL("HTTPS://PROVIDER.EXAMPLE:443/v1/").href;
		async function execute(params, context) {
			const query = new URLSearchParams("tag=first&tag=second");
			query.append("q", params.term);
			const target = new URL("../items?" + query.toString(), context.connections[0].cfg.baseUrl);
			const response = await context.connections[0].fetch(target.href);
			return [await response.text(), canonicalBase, typeof fetch];
		}`, nil, expressionLimits())
	require.NoError(t, err)
	// URL construction is available during no-I/O definition validation.
	require.NoError(t, program.Validate(context.Background()))
	input := javascript.Input{Context: expressionContext(t, render.Input{
		Params:       map[string]any{"term": "café + news"},
		Cfg:          map[string]any{"baseUrl": "https://Provider.Example:443/v1/"},
		ConnectionID: "cxn_example", ConnectionName: "Example", ConnectionNamespace: "root.product.owner",
	})}
	requests := make(chan string, 1)
	options := fetchOptions(t, context.Background(), func(request *http.Request) (*http.Response, error) {
		requests <- request.URL.String()
		return fetchResponse(http.StatusOK, io.NopCloser(strings.NewReader("ok"))), nil
	}, fetchLimits())
	result, err := program.EvaluateWithHTTP(context.Background(), input, options)

	// Resolution, canonical host/port, and form encoding survive the authenticating host boundary.
	require.NoError(t, err)
	require.JSONEq(t, `["ok","https://provider.example/v1/","undefined"]`, string(result))
	select {
	case request := <-requests:
		require.Equal(t, "https://provider.example/items?tag=first&tag=second&q=caf%C3%A9+%2B+news", request)
	default:
		t.Fatal("connection did not receive the resolved URL")
	}
}

// TestToolURLObjectRequiresExplicitSerialization preserves fetch's primitive
// URL contract and never executes an object's authored coercion hooks.
func TestToolURLObjectRequiresExplicitSerialization(t *testing.T) {
	program, err := javascript.CompileTool(`async function execute(params, context) {
		const target = new URL("https://provider.example/items");
		let coerced = false, rejected = false;
		Object.defineProperty(target, "toString", {get() {coerced = true; throw "private-coercion";}});
		try {await context.connections[0].fetch(target);} catch (_) {rejected = true;}
		const response = await context.connections[0].fetch(target.href);
		return [rejected, coerced, await response.text()];
	}`, nil, expressionLimits())
	require.NoError(t, err)
	var calls atomic.Int32
	options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return fetchResponse(http.StatusOK, io.NopCloser(strings.NewReader("ok"))), nil
	}, fetchLimits())
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)

	// The malformed object attempt is catchable and unsent; only .href causes I/O.
	require.NoError(t, err)
	require.JSONEq(t, `[true,false,"ok"]`, string(result))
	require.Equal(t, int32(1), calls.Load())
}

// TestToolURLExhaustionStopsHTTP preserves previous target-send evidence while
// making URL parsing or getter exhaustion terminal for later connection calls.
func TestToolURLExhaustionStopsHTTP(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		calls  int32
		state  api.ToolExecutionState
	}{
		"initialization": {
			source: `try {new URL("https://provider.example/" + "a".repeat(2048));} catch (_) {}
				async function execute(params, context) {await context.connections[0].fetch("https://provider.example/items"); return true;}`,
			state: api.ToolExecutionStateNotStarted,
		},
		"before first request": {
			source: `async function execute(params, context) {
				try {new URL("https://provider.example/" + "a".repeat(2048));} catch (_) {}
				await context.connections[0].fetch("https://provider.example/items"); return true;}`,
			state: api.ToolExecutionStateNotStarted,
		},
		"after first request": {
			source: `async function execute(params, context) {
				await context.connections[0].fetch("https://provider.example/first");
				try {new URL("https://provider.example/" + "a".repeat(2048));} catch (_) {}
				await context.connections[0].fetch("https://provider.example/second"); return true;}`,
			calls: 1, state: api.ToolExecutionStateMayHaveExecuted,
		},
		"getter after first request": {
			source: `async function execute(params, context) {
				const target = new URL("https://provider.example/items");
				await context.connections[0].fetch("https://provider.example/first");
				try {for (let i = 0; i < 4096; i++) {target.href;}} catch (_) {}
				await context.connections[0].fetch("https://provider.example/second"); return true;}`,
			calls: 1, state: api.ToolExecutionStateMayHaveExecuted,
		},
	} {
		for _, dimension := range []string{"work", "allocation"} {
			t.Run(name+"/"+dimension, func(t *testing.T) {
				// Input preparation and the earlier send fit; later native URL work exhausts one cap.
				limits := expressionLimits()
				if dimension == "work" {
					limits.MaxWorkUnits = 64 << 10
				} else {
					limits.MaxAllocationUnits = 64 << 10
				}
				program, err := javascript.CompileTool(test.source, nil, limits)
				require.NoError(t, err)
				var calls atomic.Int32
				options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
					calls.Add(1)
					return fetchResponse(http.StatusOK, http.NoBody), nil
				}, fetchLimits())
				result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)

				// Caught helper errors cannot permit another send or a successful partial result.
				require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
				require.Nil(t, result)
				require.Equal(t, test.calls, calls.Load())
				require.Equal(t, test.state, options.Session.ExecutionState())
			})
		}
	}
}
