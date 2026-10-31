package httpexec

import (
	"context"
	"encoding/json"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools/httpcodec"
	"github.com/rmorlok/authproxy/internal/util"
)

// resultIdentity detaches and validates the correlation data before any request.
// The same captured identities feed rendering, mapped responses, and local errors.
func (p *Program) resultIdentity(invocation Invocation) (*api.ToolResult, error) {
	input := invocation.Snapshot
	remaining := p.limits.Response.MaxEncodedResultBytes

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

// errorResult starts from empty captured metadata, never from partial mapped
// output. It bounds executor-generated errors just as the mapper bounds status
// errors. Neither construction nor retry advice can trigger another request.
func (p *Program) errorResult(
	ctx context.Context,
	base *api.ToolResult,
	state api.ToolExecutionState,
	code, message string,
) (*api.ToolResult, error) {
	result := *base
	result.IsError = true
	result.Error = &api.ToolResultError{
		Code:           code,
		Message:        message,
		ExecutionState: state,
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, ErrInvalidInvocation
	}
	if len(encoded) > p.limits.Response.MaxEncodedResultBytes {
		result.Error.Code = "OUTPUT_TOO_LARGE"
		result.Error.Message = "The encoded Tool result exceeds the result byte limit."
		encoded, err = json.Marshal(result)

		if err != nil ||
			len(encoded) > p.limits.Response.MaxEncodedResultBytes {
			return nil, httpcodec.ErrEncodedResultLimit
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &result, nil
}
