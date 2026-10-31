package httpexec_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/httpexec"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// TestCompileRejectsInvalidPrograms checks each component before a reusable
// executable is exposed, including malformed source that would otherwise fail later.
func TestCompileRejectsInvalidPrograms(t *testing.T) {
	for name, change := range map[string]func(*tools.ToolDefinition, *httpexec.Limits){
		"missing executor": func(d *tools.ToolDefinition, _ *httpexec.Limits) { d.ProxyHTTP = nil },
		"two executors": func(d *tools.ToolDefinition, _ *httpexec.Limits) {
			d.Javascript = util.ToPtr("async function execute() {}")
		},
		"input schema":     func(d *tools.ToolDefinition, _ *httpexec.Limits) { d.InputSchema = common.RawJSON(`true`) },
		"output schema":    func(d *tools.ToolDefinition, _ *httpexec.Limits) { d.OutputSchema = common.RawJSON(`null`) },
		"request template": func(d *tools.ToolDefinition, _ *httpexec.Limits) { d.ProxyHTTP.URL = "https://example.test/{{#x}}" },
		"response transform": func(d *tools.ToolDefinition, _ *httpexec.Limits) {
			d.ProxyHTTP.Response = &tools.HTTPResponse{TransformJavascript: util.ToPtr("({")}
		},
		"render limit":     func(_ *tools.ToolDefinition, l *httpexec.Limits) { l.Render.MaxSteps = 0 },
		"validation limit": func(_ *tools.ToolDefinition, l *httpexec.Limits) { l.Validation.MaxValues = 0 },
		"response limit":   func(_ *tools.ToolDefinition, l *httpexec.Limits) { l.Response.MaxEncodedResultBytes = 0 },
		"timeout":          func(_ *tools.ToolDefinition, l *httpexec.Limits) { l.HTTP.Timeout = 0 },
		"requests":         func(_ *tools.ToolDefinition, l *httpexec.Limits) { l.HTTP.MaxRequests = 0 },
		"response bytes":   func(_ *tools.ToolDefinition, l *httpexec.Limits) { l.HTTP.MaxResponseBytes = 0 },
		"stream bytes":     func(_ *tools.ToolDefinition, l *httpexec.Limits) { l.HTTP.MaxStreamBytes = 0 },
		"header bytes":     func(_ *tools.ToolDefinition, l *httpexec.Limits) { l.HTTP.MaxHeaderBytes = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			// Each invalid field must reject compilation without a partial program.
			definition, limits := executionDefinition(), executionLimits()
			change(&definition, &limits)
			program, err := httpexec.Compile(definition, limits)
			require.Error(t, err)
			require.Nil(t, program)
		})
	}
}

// TestExecuteRejectsUninitializedInputs ensures callers cannot accidentally run
// without a compiled program, request context, or configured connection client.
func TestExecuteRejectsUninitializedInputs(t *testing.T) {
	client := executionClient(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid internal inputs must not reach the client")
		return nil, nil
	})
	for _, program := range []*httpexec.Program{nil, {}} {
		result, err := program.Execute(context.Background(), client, executionInvocation())
		require.ErrorIs(t, err, httpexec.ErrInvalidInvocation)
		require.Nil(t, result)
	}
	program := compileExecution(t, executionDefinition(), executionLimits())
	result, err := program.Execute(nil, client, executionInvocation())
	require.ErrorIs(t, err, httpexec.ErrInvalidInvocation)
	require.Nil(t, result)
	result, err = program.Execute(context.Background(), nil, executionInvocation())
	require.ErrorIs(t, err, httpexec.ErrInvalidInvocation)
	require.Nil(t, result)
}

// TestExecuteBoundsLocalErrorEnvelope includes identities and error metadata in
// the result allowance, even when no upstream request was made.
func TestExecuteBoundsLocalErrorEnvelope(t *testing.T) {
	invocation := executionInvocation()
	invocation.Arguments = common.RawJSON(`null`)
	client := executionClient(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid arguments must not reach the client")
		return nil, nil
	})
	// Measure the actual safe failure, then allow exactly that many encoded bytes.
	limits := executionLimits()
	program := compileExecution(t, executionDefinition(), limits)
	result, err := program.Execute(context.Background(), client, invocation)
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	limits.Response.MaxEncodedResultBytes = len(encoded)
	program = compileExecution(t, executionDefinition(), limits)
	result, err = program.Execute(context.Background(), client, invocation)
	require.NoError(t, err)
	requireExecutionFailure(t, result, "INPUT_INVALID", api.ToolExecutionStateNotStarted)

	// An allowance too small for either failure envelope returns no partial result.
	limits.Response.MaxEncodedResultBytes = 100
	program = compileExecution(t, executionDefinition(), limits)
	result, err = program.Execute(context.Background(), client, invocation)
	require.ErrorIs(t, err, httpcodec.ErrEncodedResultLimit)
	require.Nil(t, result)
	invocation.ID = strings.Repeat("x", 101)
	result, err = program.Execute(context.Background(), client, invocation)
	require.ErrorIs(t, err, httpcodec.ErrEncodedResultLimit)
	require.Nil(t, result)
}

// TestExecuteUsesSmallerResponseReadLimit prevents an overly permissive host
// allowance from buffering data that the response mapper is certain to reject.
func TestExecuteUsesSmallerResponseReadLimit(t *testing.T) {
	limits := executionLimits()
	limits.Response.MaxResponseBytes = 4
	limits.HTTP.MaxResponseBytes = 100000
	program := compileExecution(t, executionDefinition(), limits)
	body := &executionBody{reader: strings.NewReader(strings.Repeat("x", 10000))}
	client := executionClient(func(*http.Request) (*http.Response, error) { return executionResponse(body), nil })

	// The stream is stopped by the host before the oversized body can reach Map.
	result, err := program.Execute(context.Background(), client, executionInvocation())
	require.NoError(t, err)
	requireExecutionFailure(t, result, "OUTPUT_TOO_LARGE", api.ToolExecutionStateMayHaveExecuted)
	require.EqualValues(t, 1, body.closes.Load())
}

// TestExecuteOutputValidationLimits classifies output data limits after a send
// and keeps optional validation absent when no schema exists.
func TestExecuteOutputValidationLimits(t *testing.T) {
	for _, withSchema := range []bool{false, true} {
		definition, limits := executionDefinition(), executionLimits()
		if withSchema {
			definition.OutputSchema = common.RawJSON(`true`)
		}
		limits.Validation.MaxOutputBytes = 4
		program := compileExecution(t, definition, limits)
		body := &executionBody{reader: strings.NewReader(`{"private":"value"}`)}
		client := executionClient(func(*http.Request) (*http.Response, error) { return executionResponse(body), nil })

		// A configured success schema enforces its data allowance after the send.
		result, err := program.Execute(context.Background(), client, executionInvocation())
		require.NoError(t, err)
		if withSchema {
			requireExecutionFailure(t, result, "OUTPUT_TOO_LARGE", api.ToolExecutionStateMayHaveExecuted)
		} else {
			require.False(t, result.IsError)
		}
		require.EqualValues(t, 1, body.closes.Load())
	}
}

// TestCompileRejectsInvalidAuthoredAndOperatorLimits checks every authored cap
// through the public compiler and prevents valid caps from repairing bad policy.
func TestCompileRejectsInvalidAuthoredAndOperatorLimits(t *testing.T) {
	for _, test := range []struct {
		name     string
		cap      func(*tools.ExecutionLimits, int64)
		operator func(*httpexec.Limits, int64)
	}{
		{"timeout", func(c *tools.ExecutionLimits, v int64) { c.TimeoutMillis = &v }, func(l *httpexec.Limits, v int64) { l.HTTP.Timeout = time.Duration(v) }},
		{"requests", func(c *tools.ExecutionLimits, v int64) { c.MaxRequests = util.ToPtr(int(v)) }, func(l *httpexec.Limits, v int64) { l.HTTP.MaxRequests = int(v) }},
		{"input", func(c *tools.ExecutionLimits, v int64) { c.MaxInputBytes = &v }, func(l *httpexec.Limits, v int64) { l.Validation.MaxInputBytes = int(v) }},
		{"output", func(c *tools.ExecutionLimits, v int64) { c.MaxOutputBytes = &v }, func(l *httpexec.Limits, v int64) { l.Response.MaxOutputBytes = int(v) }},
		{"response", func(c *tools.ExecutionLimits, v int64) { c.MaxResponseBytes = &v }, func(l *httpexec.Limits, v int64) { l.HTTP.MaxResponseBytes = v }},
		{"envelope", func(c *tools.ExecutionLimits, v int64) { c.MaxEncodedResultBytes = &v }, func(l *httpexec.Limits, v int64) { l.Response.MaxEncodedResultBytes = int(v) }},
		{"stack", func(c *tools.ExecutionLimits, v int64) { c.MaxStackDepth = util.ToPtr(int(v)) }, func(l *httpexec.Limits, v int64) { l.Response.Transform.MaxStackDepth = int(v) }},
	} {
		for _, value := range []int64{0, -1} {
			t.Run(fmt.Sprintf("%s/%d", test.name, value), func(t *testing.T) {
				definition := executionDefinition()
				definition.ProxyHTTP.Response = &tools.HTTPResponse{TransformJavascript: util.ToPtr(`true`)}
				definition.Limits = &tools.ExecutionLimits{}
				// Invalid authored values fail even when the operator policy is usable.
				test.cap(definition.Limits, value)
				program, err := httpexec.Compile(definition, executionLimits())
				require.Error(t, err)
				require.Nil(t, program)
				// A positive cap must not replace a required zero or negative ceiling.
				test.cap(definition.Limits, 1)
				limits := executionLimits()
				test.operator(&limits, value)
				program, err = httpexec.Compile(definition, limits)
				require.Error(t, err)
				require.Nil(t, program)
			})
		}
	}
}

// TestCompileAuthoredCapsLeaveUnusedTransformLimitsOptional permits a plain HTTP
// Tool to omit pure-JavaScript policy even when its caps also affect projections.
func TestCompileAuthoredCapsLeaveUnusedTransformLimitsOptional(t *testing.T) {
	definition, limits := executionDefinition(), executionLimits()
	definition.Limits = &tools.ExecutionLimits{
		TimeoutMillis: util.ToPtr(int64(1000)), MaxStackDepth: util.ToPtr(12),
		MaxOutputBytes: util.ToPtr(int64(512)),
	}
	limits.Response.Transform = javascript.Limits{}

	// Authored caps constrain active components without enabling a transform.
	program, err := httpexec.Compile(definition, limits)
	require.NoError(t, err)
	require.NotNil(t, program)
}

// TestErrorResultBoundsCapturedFailure checks admission failures without compiling
// a Tool, including exact encoded boundaries and independently owned provenance.
func TestErrorResultBoundsCapturedFailure(t *testing.T) {
	invocation := executionInvocation()
	invocation.Snapshot.ToolSetGeneration = util.ToPtr(uint64(3))
	invocation.Snapshot.CatalogRevision = util.ToPtr(uint64(2))
	invocation.Arguments = common.RawJSON(`invalid JSON is not evaluated here`)
	// Configuration and parameters cannot leak into a local failure envelope.
	invocation.Snapshot.Cfg = map[string]any{"secret": "configuration-value"}
	invocation.Snapshot.Params = map[string]any{"private": "argument-value"}
	result, err := httpexec.ErrorResult(context.Background(), invocation, 4096,
		api.ToolExecutionStateNotStarted, "CONNECTION_UNAVAILABLE", "The connection is not ready for Tool execution.")
	require.NoError(t, err)
	require.NoError(t, result.Validate(nil))
	requireExecutionFailure(t, result, "CONNECTION_UNAVAILABLE", api.ToolExecutionStateNotStarted)
	require.Equal(t, uint64(3), *result.Metadata.ToolSetGeneration)
	require.Equal(t, uint64(2), *result.Metadata.CatalogRevision)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "configuration-value")
	require.NotContains(t, string(encoded), "argument-value")
	*invocation.Snapshot.ToolSetGeneration = 9
	*invocation.Snapshot.CatalogRevision = 8
	require.Equal(t, uint64(3), *result.Metadata.ToolSetGeneration)
	require.Equal(t, uint64(2), *result.Metadata.CatalogRevision)

	// Measure the compact fallback, then use that exact allowance for a longer error.
	fallback, err := httpexec.ErrorResult(context.Background(), invocation, 4096,
		api.ToolExecutionStateNotStarted, "OUTPUT_TOO_LARGE", "The encoded Tool result exceeds the result byte limit.")
	require.NoError(t, err)
	encoded, err = json.Marshal(fallback)
	require.NoError(t, err)
	result, err = httpexec.ErrorResult(context.Background(), invocation, len(encoded),
		api.ToolExecutionStateNotStarted, "CONNECTION_UNAVAILABLE", strings.Repeat("safe message ", 100))
	require.NoError(t, err)
	require.Equal(t, fallback, result)

	// One byte less cannot fit the fallback; no partial result escapes.
	result, err = httpexec.ErrorResult(context.Background(), invocation, len(encoded)-1,
		api.ToolExecutionStateNotStarted, "CONNECTION_UNAVAILABLE", strings.Repeat("safe message ", 100))
	require.ErrorIs(t, err, httpcodec.ErrEncodedResultLimit)
	require.Nil(t, result)
}

// TestErrorResultRejectsInvalidInputs keeps the standalone error builder subject
// to the same identity contract as execution, with cancellation taking priority.
func TestErrorResultRejectsInvalidInputs(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		name    string
		ctx     context.Context
		change  func(*httpexec.Invocation)
		limit   int
		state   api.ToolExecutionState
		code    string
		message string
		want    error
	}{
		{name: "nil context", limit: 4096, want: httpexec.ErrInvalidInvocation},
		{name: "cancelled invalid input", ctx: cancelled, limit: -1, want: context.Canceled},
		{name: "zero budget", ctx: context.Background(), limit: 0, want: httpcodec.ErrEncodedResultLimit},
		{name: "negative budget", ctx: context.Background(), limit: -1, want: httpcodec.ErrEncodedResultLimit},
		{name: "invalid identity", ctx: context.Background(), limit: 4096, change: func(i *httpexec.Invocation) { i.ID = "invalid" }, want: httpexec.ErrInvalidInvocation},
		{name: "zero revision", ctx: context.Background(), limit: 4096, change: func(i *httpexec.Invocation) { i.Snapshot.Revision = 0 }, want: httpexec.ErrInvalidInvocation},
		{name: "invalid state", ctx: context.Background(), limit: 4096, state: "invalid", code: "FAILED", message: "Safe failure.", want: httpexec.ErrInvalidInvocation},
		{name: "blank code", ctx: context.Background(), limit: 4096, state: api.ToolExecutionStateNotStarted, message: "Safe failure.", want: httpexec.ErrInvalidInvocation},
		{name: "blank message", ctx: context.Background(), limit: 4096, state: api.ToolExecutionStateNotStarted, code: "FAILED", want: httpexec.ErrInvalidInvocation},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Reject the malformed internal input without returning a usable result.
			invocation := executionInvocation()
			if test.change != nil {
				test.change(&invocation)
			}
			result, err := httpexec.ErrorResult(test.ctx, invocation, test.limit, test.state, test.code, test.message)
			require.ErrorIs(t, err, test.want)
			require.Nil(t, result)
		})
	}
}
