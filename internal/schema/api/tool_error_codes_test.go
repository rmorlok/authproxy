package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIsReservedToolErrorCode keeps the platform vocabulary shared across
// authored executors without restricting custom code spelling or API results.
func TestIsReservedToolErrorCode(t *testing.T) {
	for _, code := range []string{
		"INPUT_INVALID", "CONNECTION_UNAVAILABLE", "UPSTREAM_AUTH_FAILED",
		"UPSTREAM_FAILED", "UPSTREAM_PROTOCOL_ERROR", "UNSUPPORTED_CAPABILITY",
		"EXECUTION_LIMIT_EXCEEDED", "OUTPUT_INVALID", "OUTPUT_TOO_LARGE", "TOOL_REVISION_CHANGED",
	} {
		t.Run(code, func(t *testing.T) {
			// Authored producers must reject these names, but platform-generated
			// results must continue to satisfy the common public error contract.
			require.True(t, IsReservedToolErrorCode(code))
			failure := &ToolResultError{
				Code: code, Message: "The operation failed.",
				ExecutionState: ToolExecutionStateNotStarted,
			}
			require.NoError(t, failure.Validate(nil))
		})
	}

	for _, code := range []string{
		"RATE_LIMITED", "upstream_failed", "CUSTOM_é", "OUTPUT_INVALID_CUSTOM", " OUTPUT_INVALID ", "", " ",
	} {
		t.Run("unreserved/"+code, func(t *testing.T) {
			// Classification neither normalizes custom identifiers nor replaces
			// separate validation of blank or otherwise unusable authored codes.
			require.False(t, IsReservedToolErrorCode(code))
		})
	}
}
