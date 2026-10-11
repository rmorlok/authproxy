package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// toolResultSchemaDocument preserves exact JSON number tokens when preparing
// result fixtures for schema validation and lossless codec comparisons.
func toolResultSchemaDocument(t *testing.T, input string) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	var document map[string]any
	require.NoError(t, decoder.Decode(&document))
	return document
}

// toolResultSchemaFixture includes every content variant in a deliberate order,
// empty payloads, extensible provider objects, and connection/catalog provenance.
func toolResultSchemaFixture(t *testing.T) map[string]any {
	t.Helper()
	return toolResultSchemaDocument(t, `{
		"toolId":"tol_01example0000001","revision":7,"invocationId":"tiv_01example0000001","isError":false,
		"structuredContent":{"count":9007199254740993,"fraction":0.12345678901234567890123456789},
		"content":[
			{"type":"text","text":"","annotations":{"audience":["assistant","user"],"priority":0,"lastModified":"provider-clock:recent"},"provider":{"text_extra":true}},
			{"type":"image","data":"","mimeType":"image/png"},
			{"type":"audio","data":"AAE=","mimeType":"audio/wav"},
			{"type":"resource_link","uri":"provider://reports/1#preview","name":"","title":"Report","description":"Open the report.","mimeType":"text/plain","size":-1,"icons":[{"src":"data:image/svg+xml;base64,PHN2Zy8+","mimeType":"image/svg+xml","sizes":["any","24x24"],"theme":"dark"}]},
			{"type":"resource","resource":{"uri":"urn:example:text","mimeType":"text/plain","text":"","provider":{"large":9007199254740993}}},
			{"type":"resource","resource":{"uri":"custom:binary#part","blob":""}}
		],
		"metadata":{"connectionId":"cxn_01example0000001","toolSetGeneration":3,"catalogRevision":5},
		"provider":{"large":9007199254740993,"fraction":0.12345678901234567890123456789,"_meta":{"future":true}}
	}`)
}

// TestToolResultSchemaRoundTrip preserves ordered rich content and raw JSON
// precision through both transport codecs, including empty names and payloads.
func TestToolResultSchemaRoundTrip(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ToolResult")
	document := toolResultSchemaFixture(t)
	require.NoError(t, compiled.Validate(document))
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	var result ToolResult
	require.NoError(t, json.Unmarshal(encoded, &result))
	require.NoError(t, result.Validate(nil))
	asYAML, err := yaml.Marshal(result)
	require.NoError(t, err)
	var fromYAML ToolResult
	require.NoError(t, yaml.Unmarshal(asYAML, &fromYAML))
	require.NoError(t, fromYAML.Validate(nil))
	for _, decoded := range []*ToolResult{&result, &fromYAML} {
		roundTrip, err := json.Marshal(decoded)
		require.NoError(t, err)
		require.JSONEq(t, string(encoded), string(roundTrip))
		require.Contains(t, string(roundTrip), "9007199254740993")
		require.Contains(t, string(roundTrip), "0.12345678901234567890123456789")
		require.NoError(t, compiled.Validate(toolResultSchemaDocument(t, string(roundTrip))))
	}
}

// TestToolResultSchemaOptionalJSON distinguishes omitted structured content and
// error details from explicit JSON null without restricting provider value kinds.
func TestToolResultSchemaOptionalJSON(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ToolResult")
	for _, raw := range []string{"", "null", "false", "12.5", `"text"`, `[]`, `{"count":9007199254740993}`} {
		t.Run("structured/"+raw, func(t *testing.T) {
			document := toolResultSchemaFixture(t)
			document["content"] = []any{}
			delete(document, "structuredContent")
			if raw != "" {
				document["structuredContent"] = json.RawMessage(raw)
			}
			encoded, err := json.Marshal(document)
			require.NoError(t, err)
			require.NoError(t, compiled.Validate(toolResultSchemaDocument(t, string(encoded))))
			var result ToolResult
			require.NoError(t, json.Unmarshal(encoded, &result))
			require.NoError(t, result.Validate(nil))
			roundTrip, err := json.Marshal(result)
			require.NoError(t, err)
			require.JSONEq(t, string(encoded), string(roundTrip))
		})
	}
	for _, state := range []string{"notStarted", "mayHaveExecuted", "completed"} {
		for _, details := range []string{"", "null", `{"retryAfter":2}`, `["reason"]`} {
			t.Run("error/"+state+"/"+details, func(t *testing.T) {
				document := toolResultSchemaFixture(t)
				document["isError"] = true
				failure := map[string]any{"code": "provider.failure", "message": "The operation failed.", "retryable": false, "executionState": state}
				if details != "" {
					failure["details"] = json.RawMessage(details)
				}
				document["error"] = failure
				encoded, err := json.Marshal(document)
				require.NoError(t, err)
				require.NoError(t, compiled.Validate(toolResultSchemaDocument(t, string(encoded))))
				var result ToolResult
				require.NoError(t, json.Unmarshal(encoded, &result))
				require.NoError(t, result.Validate(nil))
				roundTrip, err := json.Marshal(result)
				require.NoError(t, err)
				require.JSONEq(t, string(encoded), string(roundTrip))
			})
		}
	}
}

// TestToolResultSchemaEnvelopeRules checks identity, provenance, required empty
// content, and error presence without relying on zero-value Go decoding.
func TestToolResultSchemaEnvelopeRules(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ToolResult")
	for _, test := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"wrong tool ID", func(v map[string]any) { v["toolId"] = "cxn_example" }},
		{"zero revision", func(v map[string]any) { v["revision"] = 0 }},
		{"wrong invocation ID", func(v map[string]any) { v["invocationId"] = "req_example" }},
		{"missing outcome", func(v map[string]any) { delete(v, "isError") }},
		{"missing content", func(v map[string]any) { delete(v, "content") }},
		{"null content", func(v map[string]any) { v["content"] = nil }},
		{"null block", func(v map[string]any) { v["content"] = []any{nil} }},
		{"missing metadata", func(v map[string]any) { delete(v, "metadata") }},
		{"missing connection", func(v map[string]any) { v["metadata"] = map[string]any{} }},
		{"wrong connection ID", func(v map[string]any) { v["metadata"] = map[string]any{"connectionId": "tol_example"} }},
		{"zero generation", func(v map[string]any) { v["metadata"].(map[string]any)["toolSetGeneration"] = 0 }},
		{"zero catalog revision", func(v map[string]any) { v["metadata"].(map[string]any)["catalogRevision"] = 0 }},
		{"catalog without generation", func(v map[string]any) { delete(v["metadata"].(map[string]any), "toolSetGeneration") }},
		{"missing failure", func(v map[string]any) { v["isError"] = true }},
		{"null failure", func(v map[string]any) { v["isError"] = true; v["error"] = nil }},
		{"failure on success", func(v map[string]any) {
			v["error"] = map[string]any{"code": "failure", "message": "Failed.", "retryable": false, "executionState": "completed"}
		}},
		{"incomplete failure", func(v map[string]any) {
			v["isError"] = true
			v["error"] = map[string]any{"code": "failure", "message": "Failed."}
		}},
		{"unknown execution state", func(v map[string]any) {
			v["isError"] = true
			v["error"] = map[string]any{"code": "failure", "message": "Failed.", "retryable": true, "executionState": "unknown"}
		}},
		{"null provider", func(v map[string]any) { v["provider"] = nil }},
		{"array provider", func(v map[string]any) { v["provider"] = []any{} }},
		{"raw MCP metadata", func(v map[string]any) { v["_meta"] = map[string]any{} }},
		{"independent version field", func(v map[string]any) { v["version"] = 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := toolResultSchemaFixture(t)
			test.change(document)
			require.Error(t, compiled.Validate(document))
		})
	}
}

// TestToolContentBlockSchemaUnionRules prevents data from another variant from
// leaking into a block and preserves the embedded resource text/blob choice.
func TestToolContentBlockSchemaUnionRules(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ToolContentBlock")
	for _, test := range []struct{ name, input string }{
		{"unknown variant", `{"type":"video","data":"AAE=","mimeType":"video/mp4"}`},
		{"missing text", `{"type":"text"}`},
		{"null text", `{"type":"text","text":null}`},
		{"mixed text and image", `{"type":"text","text":"ok","data":"AAE=","mimeType":"image/png"}`},
		{"missing binary data", `{"type":"image","mimeType":"image/png"}`},
		{"missing MIME type", `{"type":"audio","data":"AAE="}`},
		{"blank MIME type", `{"type":"image","data":"","mimeType":" "}`},
		{"missing link name", `{"type":"resource_link","uri":"urn:example:report"}`},
		{"relative resource URI", `{"type":"resource_link","uri":"report.txt","name":"report"}`},
		{"embedded data on link", `{"type":"resource_link","uri":"urn:example:report","name":"report","text":"report"}`},
		{"missing embedded payload", `{"type":"resource","resource":{"uri":"urn:example:report"}}`},
		{"competing embedded payloads", `{"type":"resource","resource":{"uri":"urn:example:report","text":"","blob":""}}`},
		{"null embedded payload", `{"type":"resource","resource":{"uri":"urn:example:report","text":null}}`},
		{"unknown audience", `{"type":"text","text":"ok","annotations":{"audience":["system"]}}`},
		{"priority below range", `{"type":"text","text":"ok","annotations":{"priority":-0.1}}`},
		{"priority above range", `{"type":"text","text":"ok","annotations":{"priority":1.1}}`},
		{"relative icon URI", `{"type":"resource_link","uri":"urn:example:report","name":"report","icons":[{"src":"icon.png"}]}`},
		{"unknown icon theme", `{"type":"resource_link","uri":"urn:example:report","name":"report","icons":[{"src":"data:image/png;base64,","theme":"blue"}]}`},
		{"icons on text", `{"type":"text","text":"ok","icons":[]}`},
		{"scalar provider metadata", `{"type":"text","text":"ok","provider":false}`},
		{"raw MCP metadata", `{"type":"text","text":"ok","_meta":{}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, compiled.Validate(toolResultSchemaDocument(t, test.input)))
		})
	}
}
