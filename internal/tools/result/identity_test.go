package result_test

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/api"
	toolresult "github.com/rmorlok/authproxy/internal/tools/result"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// capturedResult supplies valid invocation identities and imported provenance.
func capturedResult(t *testing.T) *api.ToolResult {
	t.Helper()
	result, err := toolresult.New("tol_01example0000001", 7, "tiv_01example0000001",
		api.ToolResultMetadata{
			ConnectionID:      "cxn_01example0000001",
			ToolSetGeneration: util.ToPtr(uint64(3)), CatalogRevision: util.ToPtr(uint64(8)),
		}, 16384)
	require.NoError(t, err)
	return result
}

// TestNewCapturesIndependentIdentity keeps source provenance detached from the
// caller while initializing an empty, valid result for later assembly.
func TestNewCapturesIndependentIdentity(t *testing.T) {
	metadata := api.ToolResultMetadata{
		ConnectionID:      "cxn_01example0000001",
		ToolSetGeneration: util.ToPtr(uint64(3)), CatalogRevision: util.ToPtr(uint64(8)),
	}
	result, err := toolresult.New("tol_01example0000001", 7, "tiv_01example0000001", metadata, 4096)
	require.NoError(t, err)
	require.NoError(t, result.Validate(nil))
	require.Equal(t, "tol_01example0000001", result.ToolID)
	require.Equal(t, uint64(7), result.Revision)
	require.Equal(t, "tiv_01example0000001", result.InvocationID)
	require.Equal(t, metadata, result.Metadata)
	require.NotNil(t, result.Content)
	require.Empty(t, result.Content)
	require.Nil(t, result.StructuredContent)
	require.Nil(t, result.Provider)
	require.Nil(t, result.Error)
	require.False(t, result.IsError)

	// Reusing the caller's metadata cannot change an already captured result.
	*metadata.ToolSetGeneration, *metadata.CatalogRevision = 11, 12
	require.Equal(t, uint64(3), *result.Metadata.ToolSetGeneration)
	require.Equal(t, uint64(8), *result.Metadata.CatalogRevision)
	*result.Metadata.ToolSetGeneration, *result.Metadata.CatalogRevision = 20, 21
	require.Equal(t, uint64(11), *metadata.ToolSetGeneration)
	require.Equal(t, uint64(12), *metadata.CatalogRevision)
}

// TestNewBoundsAggregateIdentity distinguishes preliminary identity admission
// from Finish's later bound on the complete serialized envelope.
func TestNewBoundsAggregateIdentity(t *testing.T) {
	base := capturedResult(t)
	identityBytes := len(base.ToolID) + len(base.InvocationID) + len(base.Metadata.ConnectionID)
	for _, limit := range []int{-1, 0, identityBytes - 1} {
		// Reject disabled bounds and an allowance smaller than the combined IDs.
		result, err := toolresult.New(base.ToolID, base.Revision, base.InvocationID, base.Metadata, limit)
		require.ErrorIs(t, err, toolresult.ErrEncodedResultLimit)
		require.Nil(t, result)
	}

	// Exact aggregate identity size is accepted before envelope construction.
	result, err := toolresult.New(base.ToolID, base.Revision, base.InvocationID, base.Metadata, identityBytes)
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.Greater(t, len(encoded), identityBytes)

	// Standalone Tools retain absent generation/catalog fields as nil.
	result, err = toolresult.New(base.ToolID, base.Revision, base.InvocationID,
		api.ToolResultMetadata{ConnectionID: base.Metadata.ConnectionID}, 4096)
	require.NoError(t, err)
	require.Nil(t, result.Metadata.ToolSetGeneration)
	require.Nil(t, result.Metadata.CatalogRevision)
}

// TestNewRejectsInvalidIdentity keeps malformed correlation data and provenance
// out of results without returning field values through error diagnostics.
func TestNewRejectsInvalidIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*api.ToolResult)
	}{
		{"missing tool", func(r *api.ToolResult) { r.ToolID = "" }},
		{"wrong tool prefix", func(r *api.ToolResult) { r.ToolID = r.Metadata.ConnectionID }},
		{"zero revision", func(r *api.ToolResult) { r.Revision = 0 }},
		{"missing invocation", func(r *api.ToolResult) { r.InvocationID = "" }},
		{"wrong invocation prefix", func(r *api.ToolResult) { r.InvocationID = r.ToolID }},
		{"missing connection", func(r *api.ToolResult) { r.Metadata.ConnectionID = "" }},
		{"wrong connection prefix", func(r *api.ToolResult) { r.Metadata.ConnectionID = r.ToolID }},
		{"tool UTF-8", func(r *api.ToolResult) { r.ToolID += "\xff" }},
		{"invocation UTF-8", func(r *api.ToolResult) { r.InvocationID += "\xff" }},
		{"connection UTF-8", func(r *api.ToolResult) { r.Metadata.ConnectionID += "\xff" }},
		{"zero generation", func(r *api.ToolResult) { r.Metadata.ToolSetGeneration = util.ToPtr(uint64(0)) }},
		{"zero catalog", func(r *api.ToolResult) { r.Metadata.CatalogRevision = util.ToPtr(uint64(0)) }},
		{"catalog without generation", func(r *api.ToolResult) { r.Metadata.ToolSetGeneration = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Introduce one malformed field into otherwise admitted identities.
			base := capturedResult(t)
			test.change(base)
			result, err := toolresult.New(base.ToolID, base.Revision, base.InvocationID, base.Metadata, 4096)
			require.ErrorIs(t, err, toolresult.ErrInvalidResult)
			require.Nil(t, result)
		})
	}
}
