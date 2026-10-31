package httpexec_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// TestExecuteAuthoredInputLimit checks raw argument bytes before any request,
// whether the definition or the operator supplies the stricter allowance.
func TestExecuteAuthoredInputLimit(t *testing.T) {
	for _, authoredIsSmaller := range []bool{false, true} {
		t.Run(fmt.Sprint(authoredIsSmaller), func(t *testing.T) {
			definition, limits := executionDefinition(), executionLimits()
			definition.Limits = &tools.ExecutionLimits{MaxInputBytes: util.ToPtr(int64(100))}
			limits.Validation.MaxInputBytes = 2
			if authoredIsSmaller {
				definition.Limits.MaxInputBytes = util.ToPtr(int64(2))
				limits.Validation.MaxInputBytes = 100
			}
			program := compileExecution(t, definition, limits)
			var calls atomic.Int32
			client := executionClient(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return executionResponse(&executionBody{reader: strings.NewReader(`true`)}), nil
			})

			// An object exactly at the resolved allowance reaches the connection.
			invocation := executionInvocation()
			result, err := program.Execute(context.Background(), client, invocation)
			require.NoError(t, err)
			require.False(t, result.IsError)
			require.EqualValues(t, 1, calls.Load())

			// Editing the authored cap later cannot raise this compiled revision's limit.
			*definition.Limits.MaxInputBytes = 10000
			// Whitespace also consumes input bytes, although the parsed object is equal.
			invocation.Arguments = common.RawJSON("{} ")
			result, err = program.Execute(context.Background(), client, invocation)
			require.NoError(t, err)
			requireExecutionFailure(t, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateNotStarted)
			require.EqualValues(t, 1, calls.Load(), "oversized arguments must not send another request")
		})
	}
}

// TestExecuteAuthoredInputLimitOnlyBoundsArguments keeps connection configuration,
// rendered request data, and response projection inputs under their own allowances.
func TestExecuteAuthoredInputLimitOnlyBoundsArguments(t *testing.T) {
	definition := executionDefinition()
	definition.Limits = &tools.ExecutionLimits{MaxInputBytes: util.ToPtr(int64(2))}
	definition.ProxyHTTP.Headers = map[string]string{"X-Tenant": "{{cfg.tenant}}"}
	definition.ProxyHTTP.Response = &tools.HTTPResponse{
		TransformJavascript: util.ToPtr(`({tenant: cfg.tenant, value: response.body.value})`),
	}
	definition.OutputSchema = common.RawJSON(`{"type":"object","required":["tenant","value"]}`)
	program := compileExecution(t, definition, executionLimits())
	invocation := executionInvocation()
	tenant := strings.Repeat("tenant-", 128)
	invocation.Snapshot.Cfg = map[string]any{"tenant": tenant}
	body := &executionBody{reader: strings.NewReader(`{"value":"provider response"}`)}
	client := executionClient(func(request *http.Request) (*http.Response, error) {
		require.Equal(t, tenant, request.Header.Get("X-Tenant"))
		return executionResponse(body), nil
	})

	// The two-byte caller input remains valid while the larger context flows through.
	result, err := program.Execute(context.Background(), client, invocation)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.JSONEq(t, fmt.Sprintf(`{"tenant":%q,"value":"provider response"}`, tenant), string(result.StructuredContent))
	require.EqualValues(t, 1, body.closes.Load())
}

// TestExecuteAuthoredResponseAndOutputLimits enforces per-definition caps after
// the send and retains uncertainty without exposing a partial provider response.
func TestExecuteAuthoredResponseAndOutputLimits(t *testing.T) {
	for _, test := range []struct {
		name       string
		caps       tools.ExecutionLimits
		payload    string
		projection *string
		code       string
	}{
		{
			name:       "response cannot be hidden by projection",
			caps:       tools.ExecutionLimits{MaxResponseBytes: util.ToPtr(int64(8))},
			payload:    `{"private-value":"too large"}`,
			projection: util.ToPtr(`true`),
		},
		{
			name:    "decoded output",
			caps:    tools.ExecutionLimits{MaxOutputBytes: util.ToPtr(int64(8))},
			payload: `{"private-value":"too large"}`,
		},
		{
			name:    "encoded result envelope",
			caps:    tools.ExecutionLimits{MaxEncodedResultBytes: util.ToPtr(int64(600))},
			payload: `"` + strings.Repeat("x", 512) + `"`,
		},
		{
			name:       "projection output guard",
			caps:       tools.ExecutionLimits{MaxOutputBytes: util.ToPtr(int64(8))},
			payload:    `true`,
			projection: util.ToPtr(`"private-output exceeds the guard"`),
			code:       "EXECUTION_LIMIT_EXCEEDED",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := executionDefinition()
			definition.Limits = &test.caps
			definition.ProxyHTTP.Response = &tools.HTTPResponse{TransformJavascript: test.projection}
			program := compileExecution(t, definition, executionLimits())
			body := &executionBody{reader: strings.NewReader(test.payload)}
			var calls atomic.Int32
			client := executionClient(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return executionResponse(body), nil
			})

			// A post-send size failure discards content and releases the stream once.
			result, err := program.Execute(context.Background(), client, executionInvocation())
			require.NoError(t, err)
			code := test.code
			if code == "" {
				code = "OUTPUT_TOO_LARGE"
			}
			requireExecutionFailure(t, result, code, api.ToolExecutionStateMayHaveExecuted)
			require.EqualValues(t, 1, calls.Load())
			require.EqualValues(t, 1, body.closes.Load())
			if test.caps.MaxEncodedResultBytes != nil {
				encoded, err := json.Marshal(result)
				require.NoError(t, err)
				require.LessOrEqual(t, int64(len(encoded)), *test.caps.MaxEncodedResultBytes)
			}
		})
	}
}

// TestExecuteAuthoredOutputLimitAllowsShrinkingProjection distinguishes provider
// response bytes and projection input from the smaller final output allowance.
func TestExecuteAuthoredOutputLimitAllowsShrinkingProjection(t *testing.T) {
	definition := executionDefinition()
	definition.Limits = &tools.ExecutionLimits{MaxOutputBytes: util.ToPtr(int64(4))}
	definition.OutputSchema = common.RawJSON(`{"type":"string","const":"ok"}`)
	definition.ProxyHTTP.Response = &tools.HTTPResponse{TransformJavascript: util.ToPtr(`response.body.status`)}
	program := compileExecution(t, definition, executionLimits())
	body := &executionBody{reader: strings.NewReader(`{"status":"ok","unused":"` + strings.Repeat("x", 1024) + `"}`)}
	client := executionClient(func(*http.Request) (*http.Response, error) { return executionResponse(body), nil })

	// The four JSON bytes of "ok" fit all output guards without clipping input.
	result, err := program.Execute(context.Background(), client, executionInvocation())
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Equal(t, `"ok"`, string(result.StructuredContent))
	require.Len(t, result.Content, 1)
	require.Equal(t, "ok", *result.Content[0].Text)
	require.EqualValues(t, 1, body.closes.Load())
}

// TestExecuteAuthoredTimeout interrupts a waiting connection call before the
// looser operator deadline while preserving the caller's still-live context.
func TestExecuteAuthoredTimeout(t *testing.T) {
	definition := executionDefinition()
	definition.Limits = &tools.ExecutionLimits{TimeoutMillis: util.ToPtr(int64(200))}
	limits := executionLimits()
	limits.HTTP.Timeout = 10 * time.Second
	program := compileExecution(t, definition, limits)
	watchdog, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var calls atomic.Int32
	client := executionClient(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})

	// The unknown client has been entered, so timeout cannot claim no execution.
	result, err := program.Execute(watchdog, client, executionInvocation())
	require.NoError(t, err)
	require.NoError(t, watchdog.Err())
	requireExecutionFailure(t, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateMayHaveExecuted)
	require.EqualValues(t, 1, calls.Load())
}

// TestExecuteAuthoredStackLimit constrains response projection without changing
// the request or allowing a stack failure to erase an already possible effect.
func TestExecuteAuthoredStackLimit(t *testing.T) {
	definition := executionDefinition()
	definition.ProxyHTTP.Response = &tools.HTTPResponse{
		TransformJavascript: util.ToPtr(`(() => { function count(n) { return n === 0 ? 0 : count(n - 1) + 1; } return count(40); })()`),
	}
	client := executionClient(func(*http.Request) (*http.Response, error) {
		return executionResponse(&executionBody{reader: strings.NewReader(`true`)}), nil
	})

	// The finite projection succeeds under the operator's ordinary stack allowance.
	program := compileExecution(t, definition, executionLimits())
	result, err := program.Execute(context.Background(), client, executionInvocation())
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Equal(t, `40`, string(result.StructuredContent))

	// Recompiling with a smaller authored cap fails safely after the same request.
	definition.Limits = &tools.ExecutionLimits{MaxStackDepth: util.ToPtr(12)}
	program = compileExecution(t, definition, executionLimits())
	result, err = program.Execute(context.Background(), client, executionInvocation())
	require.NoError(t, err)
	requireExecutionFailure(t, result, "EXECUTION_LIMIT_EXCEEDED", api.ToolExecutionStateMayHaveExecuted)
}
