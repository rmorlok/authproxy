package api

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// toolInvokeTargetForTest addresses the same immutable Tool as the result fixture.
func toolInvokeTargetForTest() meta.ObjectReference {
	return toolschema.NewToolReference(apid.ID(toolResultForTest().ToolID))
}

// TestToolInvokeArguments validates only object shape and the optional revision
// precondition, leaving values untouched for later captured-schema validation.
func TestToolInvokeArguments(t *testing.T) {
	for _, test := range []struct {
		name      string
		arguments common.RawJSON
		valid     bool
	}{
		{"absent", nil, false},
		{"empty bytes", common.RawJSON{}, false},
		{"null", common.RawJSON(`null`), false},
		{"array", common.RawJSON(`[]`), false},
		{"string", common.RawJSON(`"value"`), false},
		{"number", common.RawJSON(`7`), false},
		{"boolean", common.RawJSON(`true`), false},
		{"invalid", common.RawJSON(`{"unfinished":`), false},
		{"trailing value", common.RawJSON(`{} {}`), false},
		{"no inputs", common.RawJSON(`{}`), true},
		{"preserved inputs", common.RawJSON(` { "extra": null, "integer": 9007199254740993, "string": "7" } `), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := NewToolInvokeRequest(toolInvokeTargetForTest(), ToolInvokeSpec{Arguments: test.arguments})
			before := string(request.Spec.Arguments)
			err := request.ValidateRequest(ToolInvokeActionKind)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "$.spec.arguments")
			}
			require.Equal(t, before, string(request.Spec.Arguments))
		})
	}

	for _, revision := range []*uint64{nil, util.ToPtr(uint64(7)), util.ToPtr(uint64(0))} {
		request := NewToolInvokeRequest(toolInvokeTargetForTest(), ToolInvokeSpec{
			Arguments: common.RawJSON(`{}`), ExpectedRevision: revision,
		})
		err := request.ValidateRequest(ToolInvokeActionKind)
		if revision != nil && *revision == 0 {
			require.ErrorContains(t, err, "$.spec.expectedRevision")
		} else {
			require.NoError(t, err)
		}
	}
}

// TestToolInvokeTargets applies the same ID-only convention to requests and
// responses; neither named references nor generations select an invocation.
func TestToolInvokeTargets(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*meta.ObjectReference)
		field  string
	}{
		{"missing ID", func(r *meta.ObjectReference) { r.ID = "" }, "id"},
		{"named", func(r *meta.ObjectReference) { r.ID, r.Namespace, r.Name = "", "root.product", "read" }, "id"},
		{"ID and name", func(r *meta.ObjectReference) { r.Name = "read" }, "support id only"},
		{"ID and namespace", func(r *meta.ObjectReference) { r.Namespace = "root.product" }, "support id only"},
		{"wrong ID", func(r *meta.ObjectReference) { r.ID = "cxn_01example0000001" }, "id"},
		{"wrong kind", func(r *meta.ObjectReference) { r.Kind = "Connection" }, "kind"},
		{"wrong API version", func(r *meta.ObjectReference) { r.APIVersion = "authproxy.net/v2" }, "apiVersion"},
		{"generation", func(r *meta.ObjectReference) { r.Generation = 7 }, "generation"},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := toolInvokeTargetForTest()
			test.change(&target)
			request := NewToolInvokeRequest(target, ToolInvokeSpec{Arguments: common.RawJSON(`{}`)})
			response := NewToolInvokeResponse(target, *toolResultForTest())
			for _, err := range []error{request.ValidateRequest(ToolInvokeActionKind), response.ValidateResponse(ToolInvokeActionKind)} {
				require.ErrorContains(t, err, "$.metadata.target")
				require.ErrorContains(t, err, test.field)
			}
		})
	}
}

// TestToolInvokeEnvelopes keeps client input and observed results separate and
// checks result identity and nested error paths in the response context.
func TestToolInvokeEnvelopes(t *testing.T) {
	require.Error(t, (*ToolInvokeRequest)(nil).ValidateRequest(ToolInvokeActionKind))
	require.Error(t, (*ToolInvokeResponse)(nil).ValidateResponse(ToolInvokeActionKind))

	for _, change := range []func(*meta.TypeMeta){
		func(m *meta.TypeMeta) { m.Kind = "OtherAction" },
		func(m *meta.TypeMeta) { m.APIVersion = "authproxy.net/v2" },
	} {
		request := NewToolInvokeRequest(toolInvokeTargetForTest(), ToolInvokeSpec{Arguments: common.RawJSON(`{}`)})
		response := NewToolInvokeResponse(toolInvokeTargetForTest(), *toolResultForTest())
		change(&request.TypeMeta)
		change(&response.TypeMeta)
		require.Error(t, request.ValidateRequest(ToolInvokeActionKind))
		require.Error(t, response.ValidateResponse(ToolInvokeActionKind))
	}

	request := NewToolInvokeRequest(toolInvokeTargetForTest(), ToolInvokeSpec{Arguments: common.RawJSON(`{}`)})
	request.Status = &struct{}{}
	require.ErrorContains(t, request.ValidateRequest(ToolInvokeActionKind), "status")

	for _, test := range []struct {
		change func(*ToolInvokeResponse)
		field  string
	}{
		{func(r *ToolInvokeResponse) { r.Status = nil }, "$.status"},
		{func(r *ToolInvokeResponse) { r.Status.Result = ToolResult{} }, "$.status.result.toolId"},
		{func(r *ToolInvokeResponse) { r.Status.Result.ToolID = "tol_02example0000001" }, "must match metadata.target.id"},
		{func(r *ToolInvokeResponse) {
			r.Status.Result.IsError, r.Status.Result.Error = true, toolResultErrorForTest()
			r.Status.Result.Error.ExecutionState = "unknown"
		}, "$.status.result.error.executionState"},
	} {
		response := NewToolInvokeResponse(toolInvokeTargetForTest(), *toolResultForTest())
		test.change(&response)
		require.ErrorContains(t, response.ValidateResponse(ToolInvokeActionKind), test.field)
	}
}

// TestToolInvokeRoundTrip preserves caller arguments and rich error results in
// both formats while the response's empty spec cannot echo the invocation input.
func TestToolInvokeRoundTrip(t *testing.T) {
	for _, format := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"JSON", json.Marshal, util.DecodeJSONStrict},
		{"YAML", yaml.Marshal, util.DecodeYAMLStrict},
	} {
		t.Run(format.name, func(t *testing.T) {
			request := NewToolInvokeRequest(toolInvokeTargetForTest(), ToolInvokeSpec{
				Arguments:        common.RawJSON(`{"count":9007199254740993,"nested":{"nothing":null},"secret":"request-only"}`),
				ExpectedRevision: util.ToPtr(uint64(7)),
			})
			require.NoError(t, request.ValidateRequest(ToolInvokeActionKind))
			data, err := format.encode(request)
			require.NoError(t, err)
			var decodedRequest ToolInvokeRequest
			require.NoError(t, format.decode(data, &decodedRequest))
			require.Equal(t, request, decodedRequest)
			require.NoError(t, decodedRequest.ValidateRequest(ToolInvokeActionKind))

			result := toolResultForTest()
			result.IsError, result.Error = true, toolResultErrorForTest()
			result.Error.ExecutionState = ToolExecutionStateNotStarted
			result.StructuredContent = common.RawJSON(`null`)
			result.Content = []ToolContentBlock{{Type: ToolContentTypeText, Text: util.ToPtr("Safe error explanation")}}
			response := NewToolInvokeResponse(toolInvokeTargetForTest(), *result)
			require.NoError(t, response.ValidateResponse(ToolInvokeActionKind))
			data, err = format.encode(response)
			require.NoError(t, err)
			require.NotContains(t, string(data), "request-only")
			require.NotContains(t, string(data), "arguments")
			require.NotContains(t, string(data), "expectedRevision")
			var decodedResponse ToolInvokeResponse
			require.NoError(t, format.decode(data, &decodedResponse))
			require.Equal(t, response, decodedResponse)
			require.NoError(t, decodedResponse.ValidateResponse(ToolInvokeActionKind))
			raw, err := json.Marshal(decodedResponse.Spec)
			require.NoError(t, err)
			require.JSONEq(t, `{}`, string(raw))
		})
	}
}

// TestToolInvokeRequestDecoding follows the shared action pointer semantics:
// null optional revision/status decode as absent, while an authored status does not.
func TestToolInvokeRequestDecoding(t *testing.T) {
	const prefix = `{"apiVersion":"authproxy.net/v1alpha1","kind":"ToolInvoke","metadata":{"target":{"apiVersion":"authproxy.net/v1alpha1","kind":"Tool","id":"tol_01example0000001"}},`

	for _, decode := range []func([]byte, any) error{util.DecodeJSONStrict, util.DecodeYAMLStrict} {
		var request ToolInvokeRequest
		require.NoError(t, decode([]byte(prefix+`"spec":{"arguments":{},"expectedRevision":null},"status":null}`), &request))
		require.Nil(t, request.Spec.ExpectedRevision)
		require.Nil(t, request.Status)
		require.NoError(t, request.ValidateRequest(ToolInvokeActionKind))
		require.NoError(t, decode([]byte(prefix+`"spec":{"arguments":{}},"status":{}}`), &request))
		require.ErrorContains(t, request.ValidateRequest(ToolInvokeActionKind), "status")
		for _, spec := range []string{`{"arguments":{},"unknown":true}`, `{"arguments":{},"expectedRevision":-1}`} {
			var invalid ToolInvokeRequest
			require.Error(t, decode([]byte(prefix+`"spec":`+spec+`}`), &invalid))
		}
	}
}
