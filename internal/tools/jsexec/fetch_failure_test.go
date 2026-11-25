package jsexec

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/stretchr/testify/require"
)

// TestExecuteClassifiesUncaughtFetchFailures preserves actionable host categories
// through native Promise rejection without exposing transport or credential text.
func TestExecuteClassifiesUncaughtFetchFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Limits)
		client executeClient
		body   string
		code   string
		state  api.ToolExecutionState
	}{
		{
			name: "credentials with uncertain send", code: "CONNECTION_UNAVAILABLE", state: api.ToolExecutionStateMayHaveExecuted,
			client: func(*http.Request) (*http.Response, error) {
				return nil, auth_methods.MarkCredentialsUnavailable(errors.New("private refresh-token detail"))
			},
		},
		{
			name: "credentials before send", code: "CONNECTION_UNAVAILABLE", state: api.ToolExecutionStateNotStarted,
			client: func(request *http.Request) (*http.Response, error) {
				httpf.ReportSendState(request.Context(), httpf.SendStateNotSent)
				return nil, auth_methods.MarkCredentialsUnavailable(errors.New("private stored-key detail"))
			},
		},
		{
			name: "request count", code: "EXECUTION_LIMIT_EXCEEDED", state: api.ToolExecutionStateMayHaveExecuted,
			change: func(l *Limits) { l.HTTP.MaxRequests = 1 },
			body: `await connection.fetch("https://example.test/one");
				await connection.fetch("https://example.test/two"); return true;`,
			client: func(*http.Request) (*http.Response, error) { return executeJSONResponse(`true`), nil },
		},
		{
			name: "response stream", code: "OUTPUT_TOO_LARGE", state: api.ToolExecutionStateMayHaveExecuted,
			change: func(l *Limits) { l.HTTP.MaxStreamBytes = 8 },
			client: func(*http.Request) (*http.Response, error) {
				return executeJSONResponse(strings.Repeat("private response", 10)), nil
			},
		},
		{
			name: "response headers", code: "OUTPUT_TOO_LARGE", state: api.ToolExecutionStateMayHaveExecuted,
			change: func(l *Limits) { l.HTTP.MaxHeaderBytes = 8 },
			client: func(*http.Request) (*http.Response, error) {
				response := executeJSONResponse(`true`)
				response.Header.Set("X-Private", "private header detail")
				return response, nil
			},
		},
		{
			name: "invalid gzip", code: "UPSTREAM_PROTOCOL_ERROR", state: api.ToolExecutionStateMayHaveExecuted,
			client: func(*http.Request) (*http.Response, error) {
				response := executeJSONResponse("private invalid gzip")
				response.Header.Set("Content-Encoding", "gzip")
				return response, nil
			},
		},
		{
			name: "invalid response", code: "UPSTREAM_PROTOCOL_ERROR", state: api.ToolExecutionStateMayHaveExecuted,
			client: func(*http.Request) (*http.Response, error) {
				response := executeJSONResponse("private response body")
				response.StatusCode = 700
				return response, nil
			},
		},
		{
			name: "transport", code: "UPSTREAM_FAILED", state: api.ToolExecutionStateMayHaveExecuted,
			client: func(*http.Request) (*http.Response, error) { return nil, errors.New("private transport detail") },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Exercise the configured-client adapter and lazy body readers, not injected runtime errors.
			limits := executeLimits()
			if test.change != nil {
				test.change(&limits)
			}
			body := test.body
			if body == "" {
				body = `return await (await connection.fetch("https://example.test/private-path")).text();`
			}
			program := compileExecute(t, `async function execute(params, context) {
				const [connection] = context.connections; `+body+` }`, limits)
			var calls atomic.Int32
			client := executeClient(func(request *http.Request) (*http.Response, error) {
				calls.Add(1)
				return test.client(request)
			})
			result, err := program.Execute(context.Background(), client, executeInvocation())
			require.NoError(t, err)
			requireExecuteFailure(t, result, test.code, test.state)
			require.EqualValues(t, 1, calls.Load())

			// Only the fixed public category and evidence survive the native rejection.
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "private")
			require.Nil(t, result.Error.Details)
		})
	}
}

// TestExecuteCredentialFailureRetainsEarlierSend prevents a later known-unsent
// credential failure from erasing effects of an earlier request in the same Tool.
func TestExecuteCredentialFailureRetainsEarlierSend(t *testing.T) {
	program := compileExecute(t, `async function execute(params, context) {
		const [connection] = context.connections;
		await connection.fetch("https://example.test/update", {method:"POST"});
		await connection.fetch("https://example.test/audit"); return true;
	}`, executeLimits())
	var calls atomic.Int32
	client := executeClient(func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			httpf.ReportSendState(request.Context(), httpf.SendStateSent)
			return executeJSONResponse(`true`), nil
		}
		httpf.ReportSendState(request.Context(), httpf.SendStateNotSent)
		return nil, auth_methods.MarkCredentialsUnavailable(errors.New("private expired credentials"))
	})

	// Credential remediation is actionable, but it cannot make retrying the whole action safe.
	result, err := program.Execute(context.Background(), client, executeInvocation())
	require.NoError(t, err)
	requireExecuteFailure(t, result, "CONNECTION_UNAVAILABLE", api.ToolExecutionStateMayHaveExecuted)
	require.EqualValues(t, 2, calls.Load())
}

// TestExecuteFetchFailureProvenance classifies only the genuine uncaught host
// error, preserving authored recovery and unrelated failures after a caught error.
func TestExecuteFetchFailureProvenance(t *testing.T) {
	for _, test := range []struct {
		name    string
		handler string
		code    string
	}{
		{"caught success", `return {recovered:true};`, ""},
		{"unrelated throw", `throw new Error("private unrelated failure");`, "OUTPUT_INVALID"},
		{"explicit ToolError", `throw new ToolError("CUSTOMER_UNAVAILABLE", "Reconnect the customer service.", {action:"reconnect"});`, "CUSTOMER_UNAVAILABLE"},
		{"genuine rethrow", `throw failure;`, "CONNECTION_UNAVAILABLE"},
		{"mutated genuine error", `failure.message = "private replacement"; failure.code = "invented"; throw failure;`, "CONNECTION_UNAVAILABLE"},
		{"copied error", `throw {name:failure.name, message:failure.message};`, "OUTPUT_INVALID"},
		{"new matching error", `throw new TypeError(failure.message);`, "OUTPUT_INVALID"},
		{"wrapped error", `throw new Proxy(failure, {});`, "OUTPUT_INVALID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			program := compileExecute(t, `async function execute(params, context) {
				try { await context.connections[0].fetch("https://example.test/private-path"); }
				catch (failure) { `+test.handler+` }
			}`, executeLimits())
			client := executeClient(func(*http.Request) (*http.Response, error) {
				return nil, auth_methods.MarkCredentialsUnavailable(errors.New("private refresh-token detail"))
			})

			// The script chooses whether to recover, rethrow, or report a different failure.
			result, err := program.Execute(context.Background(), client, executeInvocation())
			require.NoError(t, err)
			if test.code == "" {
				require.False(t, result.IsError)
				require.JSONEq(t, `{"recovered":true}`, string(result.StructuredContent))
			} else {
				requireExecuteFailure(t, result, test.code, api.ToolExecutionStateMayHaveExecuted)
				if test.code == "CUSTOMER_UNAVAILABLE" {
					require.JSONEq(t, `{"action":"reconnect"}`, string(result.Error.Details))
				}
			}
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "private")
		})
	}
}

// TestExecuteLaterFetchFailureWins follows the rejected root's error identity,
// rather than reporting the first HTTP failure seen anywhere in the invocation.
func TestExecuteLaterFetchFailureWins(t *testing.T) {
	program := compileExecute(t, `async function execute(params, context) {
		const [connection] = context.connections;
		try { await connection.fetch("https://example.test/credentials"); } catch (_) {}
		await connection.fetch("https://example.test/transport"); return true;
	}`, executeLimits())
	var calls atomic.Int32
	client := executeClient(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, auth_methods.MarkCredentialsUnavailable(errors.New("private credential detail"))
		}
		return nil, errors.New("private later transport detail")
	})

	// The handled credential failure cannot replace the later uncaught transport category.
	result, err := program.Execute(context.Background(), client, executeInvocation())
	require.NoError(t, err)
	requireExecuteFailure(t, result, "UPSTREAM_FAILED", api.ToolExecutionStateMayHaveExecuted)
	require.EqualValues(t, 2, calls.Load())
}
