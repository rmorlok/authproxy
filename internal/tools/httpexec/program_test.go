package httpexec_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/tools/httpexec"
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
