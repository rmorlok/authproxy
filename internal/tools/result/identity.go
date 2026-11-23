// Package result builds and bounds Tool result envelopes shared by executors.
// Callers own admission, schema validation, and safe public error contents.
package result

import (
	"errors"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/util"
)

// ErrInvalidResult identifies malformed internal result inputs or data that
// cannot be serialized. It never includes identities, payloads, or diagnostics.
var ErrInvalidResult = errors.New("invalid tool result")

// ErrEncodedResultLimit means the encoded-result allowance cannot hold even a
// safe failure envelope. No partial or oversized ToolResult accompanies it.
var ErrEncodedResultLimit = errors.New("encoded result byte limit cannot hold a tool error")

// New captures valid invocation identities in an empty successful result.
// Metadata provenance pointers are copied; later caller edits cannot change the
// result. No arguments, connection configuration, or authored output enter it.
//
// The required identifiers share maxBytes before UTF-8 and identity validation.
// This admission check is not the complete encoded-envelope check: callers must
// still use Finish after adding output or an error. Nonpositive limits return
// ErrEncodedResultLimit; invalid identities or provenance return ErrInvalidResult.
func New(
	toolID string,
	revision uint64,
	invocationID string,
	metadata api.ToolResultMetadata,
	maxBytes int,
) (*api.ToolResult, error) {
	if maxBytes <= 0 {
		return nil, ErrEncodedResultLimit
	}

	remaining := maxBytes
	for _, identity := range []string{toolID, invocationID, metadata.ConnectionID} {
		if len(identity) > remaining {
			return nil, ErrEncodedResultLimit
		}
		remaining -= len(identity)
		if !utf8.ValidString(identity) {
			return nil, ErrInvalidResult
		}
	}

	result := &api.ToolResult{
		ToolID:       toolID,
		Revision:     revision,
		InvocationID: invocationID,
		Content:      []api.ToolContentBlock{},
		Metadata: api.ToolResultMetadata{
			ConnectionID:      metadata.ConnectionID,
			ToolSetGeneration: util.CloneValue(metadata.ToolSetGeneration),
			CatalogRevision:   util.CloneValue(metadata.CatalogRevision),
		},
	}
	if err := result.Validate(nil); err != nil {
		return nil, ErrInvalidResult
	}
	return result, nil
}
