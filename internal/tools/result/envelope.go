package result

import (
	"context"
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/schema/api"
)

// SetFailure replaces all partial output with the supplied public failure while
// preserving captured identities and provenance. Result and failure must be
// nonnil and exclusively owned by the caller; ownership of failure transfers to
// result. The caller validates its fields and supplies trusted execution state.
//
// Provider metadata is output too: it must not survive a failed conversion or an
// oversized-result replacement. Existing error details and retry advice also
// disappear when the caller supplies a fresh platform failure.
func SetFailure(result *api.ToolResult, failure *api.ToolResultError, state api.ToolExecutionState) {
	result.StructuredContent = nil
	result.Content = []api.ToolContentBlock{}
	result.Provider = nil
	result.IsError = true
	failure.ExecutionState = state
	result.Error = failure
}

// Finish bounds the JSON encoding of a complete, exclusively owned Tool result.
// It counts identities, provenance, escaping, rich content, provider metadata,
// and error details. Callers must already bound and validate these inputs and
// perform any output-schema checks; this serialization limit is not a hard
// allocation or process-memory cap.
//
// An existing error must carry the supplied execution state; a mismatch returns
// ErrInvalidResult before any size check or mutation. An oversized result becomes
// a clean OUTPUT_TOO_LARGE failure with that state. If it cannot fit, Finish returns
// ErrEncodedResultLimit and no result; nonpositive maxBytes returns the same error.
// A nil context or result, an unknown execution state, or unserializable content
// returns ErrInvalidResult. Caller cancellation takes precedence over serialization
// outcomes, including failure to encode a fallback.
func Finish(
	ctx context.Context,
	result *api.ToolResult,
	state api.ToolExecutionState,
	maxBytes int,
) (*api.ToolResult, error) {
	if ctx == nil {
		return nil, ErrInvalidResult
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, ErrInvalidResult
	}
	switch state {
	case api.ToolExecutionStateNotStarted, api.ToolExecutionStateMayHaveExecuted, api.ToolExecutionStateCompleted:
	default:
		return nil, ErrInvalidResult
	}
	// Existing errors and size-limit fallbacks must report the same evidence.
	if result.Error != nil && result.Error.ExecutionState != state {
		return nil, ErrInvalidResult
	}
	if maxBytes <= 0 {
		return nil, ErrEncodedResultLimit
	}

	encoded, err := json.Marshal(result)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		return nil, ErrInvalidResult
	}
	if len(encoded) <= maxBytes {
		return result, nil
	}

	// Replace rather than truncate: no partial success, provider fields, authored
	// error details, or retry advice can escape in the compact platform failure.
	SetFailure(result, &api.ToolResultError{
		Code:    "OUTPUT_TOO_LARGE",
		Message: "The encoded Tool result exceeds the result byte limit.",
	}, state)
	encoded, err = json.Marshal(result)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		return nil, ErrInvalidResult
	}
	if len(encoded) > maxBytes {
		return nil, ErrEncodedResultLimit
	}
	return result, nil
}
