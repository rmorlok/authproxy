package javascript_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	api "github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/stretchr/testify/require"
)

// TestToolSearchParamsHTTP builds an encoded query with shared pure utilities
// while keeping all outbound work on the Tool's captured connection.
func TestToolSearchParamsHTTP(t *testing.T) {
	limits := expressionLimits()
	limits.Utilities = jsruntime.UtilityPolicy{URLSearchParams: true, MaxCalls: 8, MaxInputBytes: 256, MaxOutputBytes: 256}
	program, err := javascript.CompileTool(`
		const query = new URLSearchParams("status=open");
		async function execute(params, context) {
			query.append("q", "café + news");
			query.append("status", "new");
			const response = await context.connections[0].fetch("https://provider.example/items?" + query.toString());
			return [await response.text(), typeof fetch, typeof URL, typeof TextEncoder];
		}`, nil, limits)
	require.NoError(t, err)
	// Query initialization is pure and can run during validation without a connection.
	require.NoError(t, program.Validate(context.Background()))
	queries := make(chan string, 1)
	options := fetchOptions(t, context.Background(), func(request *http.Request) (*http.Response, error) {
		queries <- request.URL.RawQuery
		return fetchResponse(http.StatusOK, io.NopCloser(strings.NewReader("ok"))), nil
	}, fetchLimits())
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)

	// Duplicate parameters and escaped user text arrive intact at the configured client.
	require.NoError(t, err)
	require.JSONEq(t, `["ok","undefined","undefined","undefined"]`, string(result))
	select {
	case query := <-queries:
		require.Equal(t, "status=open&q=caf%C3%A9+%2B+news&status=new", query)
	default:
		t.Fatal("connection did not receive the query request")
	}
}

// TestToolSearchParamsFormBody serializes a genuine instance through the shared
// fetch host, copying its form data before any worker or later mutation runs.
func TestToolSearchParamsFormBody(t *testing.T) {
	limits := expressionLimits()
	limits.Utilities = jsruntime.UtilityPolicy{URLSearchParams: true, MaxCalls: 8, MaxInputBytes: 512, MaxOutputBytes: 512}
	program, err := javascript.CompileTool(`
		const form = new URLSearchParams("grant_type=client_credentials&scope=first&scope=second");
		async function execute(params, context) {
			form.append("audience", "café + news");
			Object.defineProperty(form, "toString", {get() {throw "must not run";}});
			const pending = context.connections[0].fetch("https://provider.example/token", {method: "POST", body: form});
			form.set("scope", "changed after dispatch");
			const response = await pending;
			return [await response.text(), typeof fetch];
		}`, nil, limits)
	require.NoError(t, err)
	require.NoError(t, program.Validate(context.Background()))
	requests := make(chan [4]string, 1)
	options := fetchOptions(t, context.Background(), func(request *http.Request) (*http.Response, error) {
		defer request.Body.Close()
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		replay, err := request.GetBody()
		if err != nil {
			return nil, err
		}
		defer replay.Close()
		repeated, err := io.ReadAll(replay)
		if err != nil {
			return nil, err
		}
		requests <- [4]string{request.Method, request.Header.Get("Content-Type"), string(body), string(repeated)}
		return fetchResponse(http.StatusOK, io.NopCloser(strings.NewReader("ok"))), nil
	}, fetchLimits())
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)

	// Both initial send and replay use the same encoded snapshot and inferred header.
	require.NoError(t, err)
	require.JSONEq(t, `["ok","undefined"]`, string(result))
	select {
	case request := <-requests:
		const body = "grant_type=client_credentials&scope=first&scope=second&audience=caf%C3%A9+%2B+news"
		require.Equal(t, [4]string{"POST", "application/x-www-form-urlencoded;charset=UTF-8", body, body}, request)
	default:
		t.Fatal("connection did not receive the form request")
	}
}

// TestToolSearchParamsExhaustionStopsHTTP preserves earlier send evidence and
// prevents a caught query-output limit from enabling any subsequent requests.
func TestToolSearchParamsExhaustionStopsHTTP(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		calls  int32
		state  api.ToolExecutionState
	}{
		"initialization": {
			source: `const q = new URLSearchParams("x=1"); try {q.toString();} catch (_) {}
				async function execute(params, context) {await context.connections[0].fetch("https://provider.example/items"); return true;}`,
			state: api.ToolExecutionStateNotStarted,
		},
		"before first request": {
			source: `async function execute(params, context) {
				const q = new URLSearchParams("x=1"); try {q.toString();} catch (_) {}
				await context.connections[0].fetch("https://provider.example/items"); return true;}`,
			state: api.ToolExecutionStateNotStarted,
		},
		"after first request": {
			source: `async function execute(params, context) {
				await context.connections[0].fetch("https://provider.example/first");
				const q = new URLSearchParams("x=1"); try {q.toString();} catch (_) {}
				await context.connections[0].fetch("https://provider.example/second"); return true;}`,
			calls: 1, state: api.ToolExecutionStateMayHaveExecuted,
		},
		"form body before first request": {
			source: `async function execute(params, context) {
				const q = new URLSearchParams("x=1");
				try {await context.connections[0].fetch("https://provider.example/form", {method: "POST", body: q});} catch (_) {}
				await context.connections[0].fetch("https://provider.example/later"); return true;}`,
			state: api.ToolExecutionStateNotStarted,
		},
		"form body after first request": {
			source: `async function execute(params, context) {
				await context.connections[0].fetch("https://provider.example/first");
				const q = new URLSearchParams("x=1");
				try {await context.connections[0].fetch("https://provider.example/form", {method: "POST", body: q});} catch (_) {}
				await context.connections[0].fetch("https://provider.example/later"); return true;}`,
			calls: 1, state: api.ToolExecutionStateMayHaveExecuted,
		},
	} {
		t.Run(name, func(t *testing.T) {
			// Parsing uses six output units, leaving too little for the three-byte serialization.
			limits := expressionLimits()
			limits.Utilities = jsruntime.UtilityPolicy{URLSearchParams: true, MaxCalls: 8, MaxInputBytes: 64, MaxOutputBytes: 8}
			program, err := javascript.CompileTool(test.source, nil, limits)
			require.NoError(t, err)
			var calls atomic.Int32
			options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return fetchResponse(http.StatusOK, http.NoBody), nil
			}, fetchLimits())
			result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)

			// The overall invocation fails even though authored code tried to catch the limit.
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, result)
			require.Equal(t, test.calls, calls.Load())
			require.Equal(t, test.state, options.Session.ExecutionState())
		})
	}
}

// TestToolTextCodecHTTP uses the shared codecs with the Tool's sole connection,
// preserving native byte ownership and keeping unrelated capabilities disabled.
func TestToolTextCodecHTTP(t *testing.T) {
	limits := expressionLimits()
	limits.Utilities = jsruntime.UtilityPolicy{TextCodecs: true, MaxCalls: 8, MaxInputBytes: 64, MaxOutputBytes: 64}
	program, err := javascript.CompileTool(`
		const encoder = new TextEncoder(), decoder = new TextDecoder(undefined, {fatal:true});
		async function execute(params, context) {
			const bytes = encoder.encode("café☃");
			const response = await context.connections[0].fetch("https://provider.example/echo", {
				method: "POST", body: bytes.buffer
			});
			return [decoder.decode(await response.arrayBuffer()), typeof fetch, typeof btoa];
		}`, nil, limits)
	require.NoError(t, err)
	// Pure codec construction works during no-I/O validation with fresh budgets.
	require.NoError(t, program.Validate(context.Background()))
	requestBodies := make(chan string, 1)
	options := fetchOptions(t, context.Background(), func(request *http.Request) (*http.Response, error) {
		defer request.Body.Close()
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		requestBodies <- string(body)
		return fetchResponse(http.StatusOK, io.NopCloser(strings.NewReader("café☃"))), nil
	}, fetchLimits())

	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)

	// UTF-8 bytes cross the existing ArrayBuffer HTTP contract without enabling global fetch.
	require.NoError(t, err)
	require.JSONEq(t, `["café☃","undefined","undefined"]`, string(result))
	select {
	case body := <-requestBodies:
		require.Equal(t, "café☃", body)
	default:
		t.Fatal("connection did not receive the encoded request")
	}
}

// TestToolTextCodecExhaustionStopsHTTP preserves send evidence while preventing
// a caught encoding limit from enabling or continuing connection requests.
func TestToolTextCodecExhaustionStopsHTTP(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		calls  int32
		state  api.ToolExecutionState
	}{
		"initialization": {
			source: `const e = new TextEncoder(); try { e.encode("é"); } catch (_) {}
				async function execute(params, context) { await context.connections[0].fetch("https://provider.example/items"); return true; }`,
			state: api.ToolExecutionStateNotStarted,
		},
		"before first request": {
			source: `async function execute(params, context) {
				try { new TextEncoder().encode("é"); } catch (_) {}
				await context.connections[0].fetch("https://provider.example/items"); return true;
			}`,
			state: api.ToolExecutionStateNotStarted,
		},
		"after first request": {
			source: `async function execute(params, context) {
				await context.connections[0].fetch("https://provider.example/first");
				try { new TextEncoder().encode("é"); } catch (_) {}
				await context.connections[0].fetch("https://provider.example/second"); return true;
			}`,
			calls: 1, state: api.ToolExecutionStateMayHaveExecuted,
		},
	} {
		t.Run(name, func(t *testing.T) {
			// One output byte cannot hold the two-byte UTF-8 encoding of this character.
			limits := expressionLimits()
			limits.Utilities = jsruntime.UtilityPolicy{TextCodecs: true, MaxCalls: 8, MaxInputBytes: 16, MaxOutputBytes: 1}
			program, err := javascript.CompileTool(test.source, nil, limits)
			require.NoError(t, err)
			var calls atomic.Int32
			options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return fetchResponse(http.StatusOK, http.NoBody), nil
			}, fetchLimits())

			result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)

			// The invocation fails without further dispatch, retaining any earlier send evidence.
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, result)
			require.Equal(t, test.calls, calls.Load())
			require.Equal(t, test.state, options.Session.ExecutionState())
		})
	}
}

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
