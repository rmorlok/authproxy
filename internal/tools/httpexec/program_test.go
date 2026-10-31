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
