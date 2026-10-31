package httpexec

import (
	"context"
	"encoding/json"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/util"
)

// ErrorResult builds a bounded failure from captured invocation identities without
// compiling or executing a Tool. Admission uses it for failures such as an
// unavailable connection. Arguments and template data are not copied or evaluated.
// Callers must supply fixed safe codes and messages, never raw dependency errors
// or upstream content. The result owns its metadata and contains no partial output.
//
// A failure too large for maxEncodedResultBytes falls back to OUTPUT_TOO_LARGE;
// if that envelope also cannot fit, httpcodec.ErrEncodedResultLimit is returned.
// Nonpositive limits cannot produce a result. Caller cancellation takes precedence
// over a result or construction error; nil context and invalid identities or error
// fields return ErrInvalidInvocation.
func ErrorResult(
	ctx context.Context,
	invocation Invocation,
	maxEncodedResultBytes int,
	state api.ToolExecutionState,
	code, message string,
) (*api.ToolResult, error) {
	if ctx == nil {
		return nil, ErrInvalidInvocation
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	base, err := resultIdentity(invocation, maxEncodedResultBytes)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		return nil, err
	}
	return errorResult(ctx, base, maxEncodedResultBytes, state, code, message)
}

// resultIdentity applies the compiled result allowance to the captured identities.
func (p *Program) resultIdentity(invocation Invocation) (*api.ToolResult, error) {
	return resultIdentity(invocation, p.limits.Response.MaxEncodedResultBytes)
}

// resultIdentity detaches and validates the correlation data before any request.
// The same captured identities feed rendering, mapped responses, and local errors.
func resultIdentity(invocation Invocation, maxEncodedResultBytes int) (*api.ToolResult, error) {
	if maxEncodedResultBytes <= 0 {
		return nil, httpcodec.ErrEncodedResultLimit
	}
	input := invocation.Snapshot
	remaining := maxEncodedResultBytes

	for _, id := range []string{
		input.ToolID,
		invocation.ID,
		input.ConnectionID,
	} {
		if len(id) > remaining {
			return nil, httpcodec.ErrEncodedResultLimit
		}
		remaining -= len(id)
		if !utf8.ValidString(id) {
			return nil, ErrInvalidInvocation
		}
	}

	result := &api.ToolResult{
		ToolID:       input.ToolID,
		Revision:     input.Revision,
		InvocationID: invocation.ID,
		Content:      []api.ToolContentBlock{},
		Metadata: api.ToolResultMetadata{
			ConnectionID:      input.ConnectionID,
			ToolSetGeneration: util.CloneValue(input.ToolSetGeneration),
			CatalogRevision:   util.CloneValue(input.CatalogRevision),
		},
	}

	if err := result.Validate(nil); err != nil {
		return nil, ErrInvalidInvocation
	}

	return result, nil
}

// errorResult applies the compiled result allowance to a local execution failure.
func (p *Program) errorResult(
	ctx context.Context,
	base *api.ToolResult,
	state api.ToolExecutionState,
	code, message string,
) (*api.ToolResult, error) {
	return errorResult(ctx, base, p.limits.Response.MaxEncodedResultBytes, state, code, message)
}

// errorResult starts from empty captured metadata, never from partial mapped
// output. It bounds platform-generated errors just as the mapper bounds status
// errors. Neither construction nor retry advice can trigger another request.
func errorResult(
	ctx context.Context,
	base *api.ToolResult,
	maxEncodedResultBytes int,
	state api.ToolExecutionState,
	code, message string,
) (*api.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := *base
	result.IsError = true
	result.Error = &api.ToolResultError{
		Code:           code,
		Message:        message,
		ExecutionState: state,
	}
	if err := result.Error.Validate(nil); err != nil {
		return nil, ErrInvalidInvocation
	}

	encoded, err := json.Marshal(result)
	if err == nil && len(encoded) > maxEncodedResultBytes {
		result.Error.Code = "OUTPUT_TOO_LARGE"
		result.Error.Message = "The encoded Tool result exceeds the result byte limit."
		encoded, err = json.Marshal(result)
	}

	// Serialization is synchronous work; observe cancellation before
	// choosing either the result or its construction error.
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		return nil, ErrInvalidInvocation
	}
	if len(encoded) > maxEncodedResultBytes {
		return nil, httpcodec.ErrEncodedResultLimit
	}

	return &result, nil
}
