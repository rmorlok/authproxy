package result_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	toolresult "github.com/rmorlok/authproxy/internal/tools/result"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// encodedSize measures the public JSON representation used by the byte policy.
func encodedSize(t *testing.T, result *api.ToolResult) int {
	t.Helper()
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	return len(encoded)
}

// fallbackResult describes the small platform failure expected after an
// oversized result, independently of the production failure replacement helper.
func fallbackResult(t *testing.T, state api.ToolExecutionState) *api.ToolResult {
	t.Helper()
	result := capturedResult(t)
	result.IsError = true
	result.Error = &api.ToolResultError{
		Code: "OUTPUT_TOO_LARGE", Message: "The encoded Tool result exceeds the result byte limit.",
		ExecutionState: state,
	}
	return result
}

// TestFinishPreservesFittingResults checks optional JSON distinctions and rich
// or authored-error payloads without interpreting or rewriting them.
func TestFinishPreservesFittingResults(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*api.ToolResult)
	}{
		{"absent output", func(r *api.ToolResult) {}},
		{"explicit null", func(r *api.ToolResult) { r.StructuredContent = common.RawJSON(`null`) }},
		{"structured value", func(r *api.ToolResult) { r.StructuredContent = common.RawJSON(`{"saved":true,"id":9007199254740993}`) }},
		{"rich provider data", func(r *api.ToolResult) {
			r.Content = []api.ToolContentBlock{{
				Type: api.ToolContentTypeText, Text: util.ToPtr("Saved customer"),
				Annotations: &api.ToolContentAnnotations{Audience: []string{"user"}},
				Provider:    common.RawJSON(`{"format":"summary"}`),
			}}
			r.Provider = common.RawJSON(`{"requestId":"request-42"}`)
		}},
		{"explicit error", func(r *api.ToolResult) {
			r.IsError = true
			r.Error = &api.ToolResultError{
				Code: "CUSTOMER_MISSING", Message: "The customer was not found.",
				Details: common.RawJSON(`null`), Retryable: true,
				ExecutionState: api.ToolExecutionStateMayHaveExecuted,
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Finish receives a payload already validated by its executor.
			input := capturedResult(t)
			test.change(input)
			require.NoError(t, input.Validate(nil))
			expected := input.Clone()
			result, err := toolresult.Finish(context.Background(), input, api.ToolExecutionStateMayHaveExecuted, encodedSize(t, input))
			require.NoError(t, err)
			require.Equal(t, expected, result)
		})
	}
}

// TestFinishCountsEncodedExpansion exercises the exact boundary after HTML/JSON
// escaping and binary base64 encoding, including all trusted envelope fields.
func TestFinishCountsEncodedExpansion(t *testing.T) {
	for _, test := range []struct {
		name  string
		block api.ToolContentBlock
	}{
		{"escaped text", api.ToolContentBlock{Type: api.ToolContentTypeText, Text: util.ToPtr(strings.Repeat("<>&\n", 100))}},
		{"binary image", api.ToolContentBlock{
			Type: api.ToolContentTypeImage, MIMEType: "image/png",
			Data: util.ToPtr(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("image-data", 100)))),
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := capturedResult(t)
			input.Content = []api.ToolContentBlock{test.block}
			require.NoError(t, input.Validate(nil))
			limit := encodedSize(t, input)

			// The exact serialized size fits, while one byte less replaces all output.
			result, err := toolresult.Finish(context.Background(), input.Clone(), api.ToolExecutionStateCompleted, limit)
			require.NoError(t, err)
			require.Equal(t, input, result)
			result, err = toolresult.Finish(context.Background(), input, api.ToolExecutionStateCompleted, limit-1)
			require.NoError(t, err)
			require.Equal(t, fallbackResult(t, api.ToolExecutionStateCompleted), result)
		})
	}
}

// TestFinishFallbackClearsAllPayloads preserves evidence and identities while
// dropping every prior payload when the complete envelope cannot fit.
func TestFinishFallbackClearsAllPayloads(t *testing.T) {
	for _, state := range []api.ToolExecutionState{
		api.ToolExecutionStateNotStarted, api.ToolExecutionStateMayHaveExecuted, api.ToolExecutionStateCompleted,
	} {
		for _, explicitError := range []bool{false, true} {
			name := string(state) + "/provider"
			if explicitError {
				name = string(state) + "/error"
			}
			t.Run(name, func(t *testing.T) {
				// Provider data alone can overflow, including on a small successful result.
				input := capturedResult(t)
				input.Provider = common.RawJSON(`{"private":"` + strings.Repeat("provider-data", 100) + `"}`)
				if explicitError {
					// Here the error details overflow; other partial fields remain small.
					input.Provider = common.RawJSON(`{"requestId":"private"}`)
					input.StructuredContent = common.RawJSON(`{"private":"partial-output"}`)
					input.Content = []api.ToolContentBlock{{Type: api.ToolContentTypeText, Text: util.ToPtr("partial content")}}
					input.IsError = true
					input.Error = &api.ToolResultError{
						Code: "CUSTOMER_FAILED", Message: "Could not read customer.",
						Details:   common.RawJSON(`{"private":"` + strings.Repeat("details", 100) + `"}`),
						Retryable: true, ExecutionState: state,
					}
				}
				require.NoError(t, input.Validate(nil))
				expected := fallbackResult(t, state)
				limit := encodedSize(t, expected)

				// The fallback fits exactly and preserves no provider data or retry advice.
				result, err := toolresult.Finish(context.Background(), input.Clone(), state, limit)
				require.NoError(t, err)
				require.Equal(t, expected, result)
				require.NoError(t, result.Validate(nil))

				// If even that safe failure is too large, no partial result may escape.
				result, err = toolresult.Finish(context.Background(), input, state, limit-1)
				require.ErrorIs(t, err, toolresult.ErrEncodedResultLimit)
				require.Nil(t, result)
			})
		}
	}
}

// TestSetFailureReplacesPartialOutput installs the caller's chosen safe error
// while retaining its explicit details and replacing stale output and advice.
func TestSetFailureReplacesPartialOutput(t *testing.T) {
	input := capturedResult(t)
	expected := input.Clone()
	input.StructuredContent = common.RawJSON(`{"partial":true}`)
	input.Content = []api.ToolContentBlock{{Type: api.ToolContentTypeText, Text: util.ToPtr("partial")}}
	input.Provider = common.RawJSON(`{"requestId":"private"}`)
	input.IsError, input.Error = true, &api.ToolResultError{Code: "OLD", Message: "Old failure.", Retryable: true}
	failure := &api.ToolResultError{Code: "CUSTOMER_FAILED", Message: "Could not read customer.", Details: common.RawJSON(`{"status":404}`)}

	// The incoming error owns its details; trusted evidence supplies its state.
	toolresult.SetFailure(input, failure, api.ToolExecutionStateMayHaveExecuted)
	expected.IsError = true
	expected.Error = &api.ToolResultError{
		Code: "CUSTOMER_FAILED", Message: "Could not read customer.", Details: common.RawJSON(`{"status":404}`),
		ExecutionState: api.ToolExecutionStateMayHaveExecuted,
	}
	require.Equal(t, expected, input)
	require.NoError(t, input.Validate(nil))
}

// TestFinishRejectsInvalidInputs returns no result for invalid host arguments or
// malformed raw JSON, without exposing serializer diagnostics or payload data.
func TestFinishRejectsInvalidInputs(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*api.ToolResult)
		ctx    context.Context
		state  api.ToolExecutionState
		limit  int
		want   error
	}{
		{name: "nil context", state: api.ToolExecutionStateNotStarted, limit: 4096, want: toolresult.ErrInvalidResult},
		{name: "invalid state", ctx: context.Background(), state: "unknown", limit: 4096, want: toolresult.ErrInvalidResult},
		{name: "zero limit", ctx: context.Background(), state: api.ToolExecutionStateNotStarted, want: toolresult.ErrEncodedResultLimit},
		{name: "negative limit", ctx: context.Background(), state: api.ToolExecutionStateNotStarted, limit: -1, want: toolresult.ErrEncodedResultLimit},
		{name: "invalid JSON", ctx: context.Background(), state: api.ToolExecutionStateNotStarted, limit: 4096,
			change: func(r *api.ToolResult) { r.Provider = common.RawJSON(`{"private-invalid-json":`) }, want: toolresult.ErrInvalidResult},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := capturedResult(t)
			if test.change != nil {
				test.change(input)
			}
			result, err := toolresult.Finish(test.ctx, input, test.state, test.limit)
			require.ErrorIs(t, err, test.want)
			require.NotContains(t, err.Error(), "private-invalid-json")
			require.Nil(t, result)
		})
	}

	// A missing result is an invalid host input even when the allowance is valid.
	result, err := toolresult.Finish(context.Background(), nil, api.ToolExecutionStateNotStarted, 4096)
	require.ErrorIs(t, err, toolresult.ErrInvalidResult)
	require.Nil(t, result)
}

// cancelAfterCheck models cancellation immediately after an entry check observes
// a live context, allowing completion-time precedence to be tested without sleeps.
type cancelAfterCheck struct {
	context.Context
	cancel context.CancelFunc // Cancels the embedded context after its first Err read.
}

// Err samples the current state before canceling; subsequent reads see cancellation.
func (c *cancelAfterCheck) Err() error {
	err := c.Context.Err()
	c.cancel()
	return err
}

// TestFinishCancellationWins keeps cancellation ahead of a completed result,
// serialization failure, invalid host inputs, or an exhausted envelope allowance.
func TestFinishCancellationWins(t *testing.T) {
	for _, invalidJSON := range []bool{false, true} {
		// Cancel after the entry check; Finish must observe it again before returning.
		ctx, cancel := context.WithCancel(context.Background())
		input := capturedResult(t)
		if invalidJSON {
			input.StructuredContent = common.RawJSON(`{"private-invalid-json":`)
		}
		result, err := toolresult.Finish(&cancelAfterCheck{Context: ctx, cancel: cancel}, input, api.ToolExecutionStateNotStarted, 4096)
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
	}

	// Already-canceled callers win even when the remaining arguments are invalid.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := toolresult.Finish(canceled, nil, "unknown", -1)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)

	// An elapsed caller deadline retains its context classification as well.
	expired, stop := context.WithDeadline(context.Background(), time.Time{})
	defer stop()
	result, err = toolresult.Finish(expired, capturedResult(t), api.ToolExecutionStateCompleted, 1)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, result)
}
