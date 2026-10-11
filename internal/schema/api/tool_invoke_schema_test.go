package api

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// toolInvokeSchemaFixture builds either wire envelope independently of the Go
// constructors so required fields and request/response ownership can be tested.
func toolInvokeSchemaFixture(t *testing.T, response bool) map[string]any {
	t.Helper()
	document := map[string]any{
		"apiVersion": "authproxy.net/v1alpha1",
		"kind":       "ToolInvoke",
		"metadata": map[string]any{"target": map[string]any{
			"apiVersion": "authproxy.net/v1alpha1", "kind": "Tool", "id": "tol_01example0000001",
		}},
		"spec": map[string]any{"arguments": map[string]any{}},
	}
	if response {
		document["spec"] = map[string]any{}
		document["status"] = map[string]any{"result": toolResultSchemaFixture(t)}
	}
	return document
}

// TestToolInvokeRequestSchemaRoundTrip accepts an empty input object or arbitrary
// nested JSON while preserving revision preconditions and exact number tokens.
func TestToolInvokeRequestSchemaRoundTrip(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ToolInvokeActionRequest")
	target := meta.ObjectReference{APIVersion: meta.APIVersionV1Alpha1, Kind: "Tool", ID: "tol_01example0000001"}
	revision := uint64(7)
	for _, test := range []struct {
		name      string
		arguments common.RawJSON
		revision  *uint64
	}{
		{"empty arguments", common.RawJSON(`{}`), nil},
		{"revision precondition", common.RawJSON(`{"provider_field":{"large":9007199254740993,"fraction":0.12345678901234567890123456789},"values":[null,true,"text"]}`), &revision},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := NewToolInvokeRequest(target, ToolInvokeSpec{Arguments: test.arguments, ExpectedRevision: test.revision})
			require.NoError(t, request.ValidateRequest("ToolInvoke"))
			encoded, err := json.Marshal(request)
			require.NoError(t, err)
			require.NoError(t, compiled.Validate(toolResultSchemaDocument(t, string(encoded))))
			asYAML, err := yaml.Marshal(request)
			require.NoError(t, err)
			var fromJSON, fromYAML ToolInvokeRequest
			require.NoError(t, json.Unmarshal(encoded, &fromJSON))
			require.NoError(t, yaml.Unmarshal(asYAML, &fromYAML))
			for _, decoded := range []*ToolInvokeRequest{&fromJSON, &fromYAML} {
				require.NoError(t, decoded.ValidateRequest("ToolInvoke"))
				roundTrip, err := json.Marshal(decoded)
				require.NoError(t, err)
				require.JSONEq(t, string(encoded), string(roundTrip))
				require.JSONEq(t, string(test.arguments), string(decoded.Spec.Arguments))
				if test.revision != nil {
					require.Contains(t, string(roundTrip), "9007199254740993")
					require.Contains(t, string(roundTrip), "0.12345678901234567890123456789")
				}
			}
		})
	}
}

// TestToolInvokeResponseSchemaRoundTrip retains rich results through the typed
// action envelope and emits an empty spec rather than invocation arguments.
func TestToolInvokeResponseSchemaRoundTrip(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ToolInvokeActionResponse")
	target := meta.ObjectReference{APIVersion: meta.APIVersionV1Alpha1, Kind: "Tool", ID: "tol_01example0000001"}
	for _, failed := range []bool{false, true} {
		name := "success"
		if failed {
			name = "tool failure"
		}
		t.Run(name, func(t *testing.T) {
			document := toolResultSchemaFixture(t)
			if failed {
				document["isError"] = true
				document["error"] = map[string]any{
					"code": "provider.failure", "message": "The operation failed.", "details": nil,
					"retryable": false, "executionState": "mayHaveExecuted",
				}
			}
			resultJSON, err := json.Marshal(document)
			require.NoError(t, err)
			var result ToolResult
			require.NoError(t, json.Unmarshal(resultJSON, &result))
			response := NewToolInvokeResponse(target, result)
			require.NoError(t, response.ValidateResponse("ToolInvoke"))
			encoded, err := json.Marshal(response)
			require.NoError(t, err)
			output := toolResultSchemaDocument(t, string(encoded))
			require.NoError(t, compiled.Validate(output))
			require.Equal(t, map[string]any{}, output["spec"])
			asYAML, err := yaml.Marshal(response)
			require.NoError(t, err)
			var fromJSON, fromYAML ToolInvokeResponse
			require.NoError(t, json.Unmarshal(encoded, &fromJSON))
			require.NoError(t, yaml.Unmarshal(asYAML, &fromYAML))
			for _, decoded := range []*ToolInvokeResponse{&fromJSON, &fromYAML} {
				require.NoError(t, decoded.ValidateResponse("ToolInvoke"))
				roundTrip, err := json.Marshal(decoded)
				require.NoError(t, err)
				require.JSONEq(t, string(encoded), string(roundTrip))
				require.Contains(t, string(roundTrip), "9007199254740993")
				require.Contains(t, string(roundTrip), "0.12345678901234567890123456789")
			}
		})
	}
}

// TestToolInvokeSchemaTargetRules constrains both actions to a versioned Tool
// ID reference. Namespaces, names, and generations are not invocation selectors.
func TestToolInvokeSchemaTargetRules(t *testing.T) {
	for _, direction := range []struct {
		name, definition string
		response         bool
	}{
		{"request", "ToolInvokeActionRequest", false},
		{"response", "ToolInvokeActionResponse", true},
	} {
		t.Run(direction.name, func(t *testing.T) {
			compiled := compileRefSchema(t, "./schema.json#/$defs/"+direction.definition)
			for _, test := range []struct {
				name, field string
				value       any
				remove      bool
			}{
				{name: "missing ID", field: "id", remove: true},
				{name: "wrong ID prefix", field: "id", value: "cxn_01example0000001"},
				{name: "wrong target kind", field: "kind", value: "ToolSet"},
				{name: "missing target version", field: "apiVersion", remove: true},
				{name: "wrong target version", field: "apiVersion", value: "authproxy.net/v2"},
				{name: "name selector", field: "name", value: "read-records"},
				{name: "namespace selector", field: "namespace", value: "root.product"},
				{name: "generation selector", field: "generation", value: 3},
			} {
				t.Run(test.name, func(t *testing.T) {
					document := toolInvokeSchemaFixture(t, direction.response)
					target := document["metadata"].(map[string]any)["target"].(map[string]any)
					if test.remove {
						delete(target, test.field)
					} else {
						target[test.field] = test.value
					}
					require.Error(t, compiled.Validate(document))
				})
			}
		})
	}
}

// TestToolInvokeSchemaRequestAndResponseOwnership checks the canonical wire
// boundary. Schema null restrictions intentionally do not imply custom pointer
// codecs or stronger null-versus-omission rules for ordinary Go decoding.
func TestToolInvokeSchemaRequestAndResponseOwnership(t *testing.T) {
	requestSchema := compileRefSchema(t, "./schema.json#/$defs/ToolInvokeActionRequest")
	responseSchema := compileRefSchema(t, "./schema.json#/$defs/ToolInvokeActionResponse")
	for _, test := range []struct {
		name, field string
		response    bool
		inSpec      bool
		value       any
		remove      bool
	}{
		{name: "wrong action kind", field: "kind", value: "ToolExecute"},
		{name: "missing spec", field: "spec", remove: true},
		{name: "null spec", field: "spec", value: nil},
		{name: "missing arguments", field: "arguments", inSpec: true, remove: true},
		{name: "null arguments", field: "arguments", inSpec: true, value: nil},
		{name: "array arguments", field: "arguments", inSpec: true, value: []any{}},
		{name: "scalar arguments", field: "arguments", inSpec: true, value: "input"},
		{name: "zero expected revision", field: "expectedRevision", inSpec: true, value: 0},
		{name: "negative expected revision", field: "expectedRevision", inSpec: true, value: -1},
		{name: "null expected revision", field: "expectedRevision", inSpec: true, value: nil},
		{name: "fractional expected revision", field: "expectedRevision", inSpec: true, value: 1.5},
		{name: "unknown request option", field: "connectionId", inSpec: true, value: "cxn_01example0000001"},
		{name: "client status", field: "status", value: map[string]any{}},
		{name: "null client status", field: "status", value: nil},
		{name: "missing result status", field: "status", response: true, remove: true},
		{name: "null result status", field: "status", response: true, value: nil},
		{name: "missing result", field: "status", response: true, value: map[string]any{}},
		{name: "null result", field: "status", response: true, value: map[string]any{"result": nil}},
		{name: "invalid result", field: "status", response: true, value: map[string]any{"result": map[string]any{}}},
		{name: "arguments echoed in response", field: "arguments", response: true, inSpec: true, value: map[string]any{"private": "input"}},
		{name: "revision echoed in response", field: "expectedRevision", response: true, inSpec: true, value: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := toolInvokeSchemaFixture(t, test.response)
			object := document
			if test.inSpec {
				object = document["spec"].(map[string]any)
			}
			if test.remove {
				delete(object, test.field)
			} else {
				object[test.field] = test.value
			}
			compiled := requestSchema
			if test.response {
				compiled = responseSchema
			}
			require.Error(t, compiled.Validate(document))
		})
	}
}
