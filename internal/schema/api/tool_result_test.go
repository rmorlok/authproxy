package api

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// toolResultForTest is a complete successful result with no rich blocks or
// imported-source metadata. Those values are optional additions to this case.
func toolResultForTest() *ToolResult {
	return &ToolResult{
		ToolID: "tol_01example0000001", Revision: 7,
		InvocationID: "tiv_01example0000001",
		Content:      []ToolContentBlock{},
		Metadata:     ToolResultMetadata{ConnectionID: "cxn_01example0000001"},
	}
}

// toolResultErrorForTest records uncertain side effects and safe public details
// independently of any provider-specific error classification.
func toolResultErrorForTest() *ToolResultError {
	return &ToolResultError{
		Code: "provider-future-code", Message: "The provider did not complete its response.",
		Details:        common.RawJSON(`{"attempt":9007199254740993}`),
		ExecutionState: ToolExecutionStateMayHaveExecuted,
	}
}

// TestToolResultValidation requires result identity and consistent error/source
// metadata while preserving arbitrary successful JSON and extensible codes.
func TestToolResultValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ToolResult)
		field  string
	}{
		{"standalone", func(r *ToolResult) {}, ""},
		{"catalog", func(r *ToolResult) {
			r.Metadata.ToolSetGeneration, r.Metadata.CatalogRevision = util.ToPtr(uint64(2)), util.ToPtr(uint64(9))
		}, ""},
		{"error preserves content", func(r *ToolResult) {
			r.IsError, r.Error = true, toolResultErrorForTest()
			r.StructuredContent = common.RawJSON(`null`)
		}, ""},
		{"missing tool", func(r *ToolResult) { r.ToolID = "" }, "toolId"},
		{"wrong tool", func(r *ToolResult) { r.ToolID = r.Metadata.ConnectionID }, "toolId"},
		{"zero revision", func(r *ToolResult) { r.Revision = 0 }, "revision"},
		{"missing invocation", func(r *ToolResult) { r.InvocationID = "" }, "invocationId"},
		{"wrong invocation", func(r *ToolResult) { r.InvocationID = r.ToolID }, "invocationId"},
		{"unknown invocation", func(r *ToolResult) { r.InvocationID = "unknown" }, "invocationId"},
		{"missing connection", func(r *ToolResult) { r.Metadata.ConnectionID = "" }, "metadata.connectionId"},
		{"wrong connection", func(r *ToolResult) { r.Metadata.ConnectionID = r.ToolID }, "metadata.connectionId"},
		{"zero generation", func(r *ToolResult) { r.Metadata.ToolSetGeneration = util.ToPtr(uint64(0)) }, "metadata.toolSetGeneration"},
		{"catalog without generation", func(r *ToolResult) { r.Metadata.CatalogRevision = util.ToPtr(uint64(9)) }, "metadata.toolSetGeneration"},
		{"zero catalog", func(r *ToolResult) {
			r.Metadata.ToolSetGeneration, r.Metadata.CatalogRevision = util.ToPtr(uint64(2)), util.ToPtr(uint64(0))
		}, "metadata.catalogRevision"},
		{"absent content", func(r *ToolResult) { r.Content = nil }, "content"},
		{"invalid block", func(r *ToolResult) { r.Content = []ToolContentBlock{{}} }, "content[0]"},
		{"error absent", func(r *ToolResult) { r.IsError = true }, "error"},
		{"error on success", func(r *ToolResult) { r.Error = toolResultErrorForTest() }, "error"},
		{"invalid JSON", func(r *ToolResult) { r.StructuredContent = common.RawJSON(`NaN`) }, "structuredContent"},
		{"multiple JSON values", func(r *ToolResult) { r.StructuredContent = common.RawJSON(`{} {}`) }, "structuredContent"},
		{"invalid provider", func(r *ToolResult) { r.Provider = common.RawJSON(`{"incomplete":`) }, "provider"},
		{"null provider", func(r *ToolResult) { r.Provider = common.RawJSON(`null`) }, "provider"},
		{"scalar provider", func(r *ToolResult) { r.Provider = common.RawJSON(`false`) }, "provider"},
		{"array provider", func(r *ToolResult) { r.Provider = common.RawJSON(`[]`) }, "provider"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := toolResultForTest()
			test.change(result)
			before := result.Clone()
			err := result.Validate(&common.ValidationContext{Path: "response"})
			if test.field == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "response."+test.field)
			}
			require.Equal(t, before, result, "validation must not rewrite an outcome")
		})
	}
	require.Error(t, (*ToolResult)(nil).Validate(nil))
}

// TestToolResultErrorValidation distinguishes all execution states without
// deriving retry policy or limiting the set of public diagnostic codes.
func TestToolResultErrorValidation(t *testing.T) {
	require.NoError(t, (*ToolResultError)(nil).Validate(nil))
	for _, state := range []ToolExecutionState{ToolExecutionStateNotStarted, ToolExecutionStateMayHaveExecuted, ToolExecutionStateCompleted} {
		failure := toolResultErrorForTest()
		failure.ExecutionState = state
		failure.Retryable = true
		require.NoError(t, failure.Validate(nil))
	}
	for _, test := range []struct {
		change func(*ToolResultError)
		field  string
	}{
		{func(e *ToolResultError) { e.Code = " \n" }, "code"},
		{func(e *ToolResultError) { e.Message = "\t" }, "message"},
		{func(e *ToolResultError) { e.ExecutionState = "" }, "executionState"},
		{func(e *ToolResultError) { e.ExecutionState = "failed" }, "executionState"},
		{func(e *ToolResultError) { e.Details = common.RawJSON(`undefined`) }, "details"},
	} {
		result := toolResultForTest()
		result.IsError, result.Error = true, toolResultErrorForTest()
		test.change(result.Error)
		require.ErrorContains(t, result.Validate(nil), "$.error."+test.field)
	}
}

// TestToolResultRoundTrip preserves every JSON value, absent/null distinctions,
// numeric precision, and the complete error envelope through both wire formats.
func TestToolResultRoundTrip(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"JSON", json.Marshal, util.DecodeJSONStrict},
		{"YAML", yaml.Marshal, util.DecodeYAMLStrict},
	} {
		t.Run(format.name, func(t *testing.T) {
			for _, payload := range []string{"", `null`, `false`, `""`, `9007199254740993`, `1.234567890123456789`, `[]`, `{"items":[]}`} {
				result := toolResultForTest()
				result.IsError, result.Error = true, toolResultErrorForTest()
				result.Content = []ToolContentBlock{{Type: ToolContentTypeText, Text: util.ToPtr("Provider error context")}}
				if payload != "" {
					result.StructuredContent, result.Error.Details = common.RawJSON(payload), common.RawJSON(payload)
				} else {
					result.Error.Details = nil
				}
				result.Provider = common.RawJSON(`{"upstream":{"sequence":9007199254740993}}`)
				data, err := format.encode(result)
				require.NoError(t, err)
				var decoded ToolResult
				require.NoError(t, format.decode(data, &decoded))
				require.NoError(t, decoded.Validate(nil))
				require.Equal(t, result, &decoded, payload)
				encoded, err := json.Marshal(&decoded)
				require.NoError(t, err)
				if payload == "" {
					require.NotContains(t, string(encoded), `"structuredContent"`)
					require.NotContains(t, string(encoded), `"details"`)
				} else {
					require.Contains(t, string(encoded), `"structuredContent":`+payload)
				}
			}
			for _, input := range []string{
				`{"unknown":true}`, `{"metadata":{"credentials":"secret"}}`,
				`{"error":{"stack":"private"}}`, `{"revision":-1}`,
			} {
				var decoded ToolResult
				require.Error(t, format.decode([]byte(input), &decoded), input)
			}
		})
	}
}

// TestToolResultYAMLNullAndAliases checks authored YAML nodes as well as emitted
// YAML, including error values decoded outside a result envelope.
func TestToolResultYAMLNullAndAliases(t *testing.T) {
	var result ToolResult
	require.NoError(t, util.DecodeYAMLStrict([]byte(`toolId: tol_01example0000001
revision: 7
invocationId: tiv_01example0000001
isError: true
content: []
metadata: {connectionId: cxn_01example0000001}
structuredContent: &nothing null
error:
  <<: {code: EXAMPLE, message: Example failure, retryable: false, executionState: notStarted}
  details: *nothing
`), &result))
	require.NoError(t, result.Validate(nil))
	require.Equal(t, common.RawJSON(`null`), result.StructuredContent)
	require.Equal(t, common.RawJSON(`null`), result.Error.Details)
	var failure ToolResultError
	require.NoError(t, util.DecodeYAMLStrict([]byte("code: X\nmessage: Error\nretryable: false\nexecutionState: completed\ndetails: null\n"), &failure))
	require.Equal(t, common.RawJSON(`null`), failure.Details)
	before := result.Clone()
	require.Error(t, util.DecodeYAMLStrict([]byte("structuredContent: null\nunknown: value"), &result))
	require.Equal(t, before, &result, "failed YAML conversion preserves the previous result")
	beforeError := failure.Clone()
	require.Error(t, util.DecodeYAMLStrict([]byte("details: null\nunknown: value"), &failure))
	require.Equal(t, beforeError, &failure)
}

// TestToolResultCloneOwnership detaches all outcome storage so adapters may
// project content without mutating the complete result retained as an artifact.
func TestToolResultCloneOwnership(t *testing.T) {
	require.Nil(t, (*ToolResult)(nil).Clone())
	require.Nil(t, (*ToolResultError)(nil).Clone())
	for _, content := range [][]ToolContentBlock{nil, {}} {
		original := toolResultForTest()
		original.Content = content
		require.Equal(t, original, original.Clone())
	}
	original := toolResultForTest()
	original.StructuredContent = common.RawJSON(`{"value":true}`)
	original.Provider = common.RawJSON(`{"example":1}`)
	original.Metadata.ToolSetGeneration, original.Metadata.CatalogRevision = util.ToPtr(uint64(2)), util.ToPtr(uint64(9))
	original.IsError, original.Error = true, toolResultErrorForTest()
	require.NoError(t, util.DecodeJSONStrict([]byte(`[{"type":"text","text":"Original response"}]`), &original.Content))
	copy := original.Clone()
	require.Equal(t, original, copy)
	copy.StructuredContent[2], copy.Provider[2], copy.Error.Details[2] = 'x', 'x', 'x'
	*copy.Metadata.ToolSetGeneration, *copy.Metadata.CatalogRevision = 3, 10
	*copy.Content[0].Text = "Projected response"
	require.Equal(t, "Original response", *original.Content[0].Text)
	copy.Content[0] = ToolContentBlock{}
	require.Equal(t, common.RawJSON(`{"value":true}`), original.StructuredContent)
	require.Equal(t, common.RawJSON(`{"example":1}`), original.Provider)
	require.Equal(t, toolResultErrorForTest(), original.Error)
	require.Equal(t, uint64(2), *original.Metadata.ToolSetGeneration)
	require.Equal(t, uint64(9), *original.Metadata.CatalogRevision)
	require.NoError(t, original.Validate(nil))
}
