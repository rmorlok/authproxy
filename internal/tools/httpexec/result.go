package httpexec

import (
	"context"
	"errors"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools"
	toolresult "github.com/rmorlok/authproxy/internal/tools/result"
)

// ErrorResult builds a bounded failure from captured invocation identities without
// compiling or executing a Tool. Admission uses it for failures such as an
// unavailable connection. Arguments and template data are not copied or evaluated.
// Callers must supply fixed safe codes and messages, never raw dependency errors
// or upstream content. The result owns its metadata and contains no partial output.
//
// A failure too large for maxEncodedResultBytes falls back to OUTPUT_TOO_LARGE;
// if that envelope also cannot fit, toolresult.ErrEncodedResultLimit is returned.
// Nonpositive limits cannot produce a result. Caller cancellation takes precedence
// over a result or construction error; nil context and invalid identities or error
// fields return ErrInvalidInvocation.
func ErrorResult(
	ctx context.Context,
	invocation tools.Invocation,
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
func (p *Program) resultIdentity(invocation tools.Invocation) (*api.ToolResult, error) {
	return resultIdentity(invocation, p.limits.Response.MaxEncodedResultBytes)
}

// resultIdentity detaches and validates the correlation data before any request.
// The same captured identities feed rendering, mapped responses, and local errors.
func resultIdentity(invocation tools.Invocation, maxEncodedResultBytes int) (*api.ToolResult, error) {
	input := invocation.Snapshot
	result, err := toolresult.New(
		input.ToolID, input.Revision, invocation.ID,
		api.ToolResultMetadata{
			ConnectionID:      input.ConnectionID,
			ToolSetGeneration: input.ToolSetGeneration,
			CatalogRevision:   input.CatalogRevision,
		},
		maxEncodedResultBytes,
	)
	if errors.Is(err, toolresult.ErrInvalidResult) {
		return nil, ErrInvalidInvocation
	}
	return result, err
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
	toolresult.SetFailure(&result, &api.ToolResultError{
		Code:    code,
		Message: message,
	}, state)
	if err := result.Error.Validate(nil); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, ErrInvalidInvocation
	}

	finished, err := toolresult.Finish(ctx, &result, state, maxEncodedResultBytes)
	if errors.Is(err, toolresult.ErrInvalidResult) {
		return nil, ErrInvalidInvocation
	}
	return finished, err
}
