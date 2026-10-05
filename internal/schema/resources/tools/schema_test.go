package tools_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// compileToolSchema exercises the production embedded-schema loader, so a
// package-local schema file that was accidentally omitted from embedding fails.
func compileToolSchema(t *testing.T) *jsonschemav5.Schema {
	t.Helper()
	compiled, err := schema.CompileSchema(tools.SchemaIDTools)
	require.NoError(t, err)
	return compiled
}

// TestToolSchemaFieldsCompose allows a future flat Tool spec to add its
// connection reference while both that spec and authored definitions stay closed.
func TestToolSchemaFieldsCompose(t *testing.T) {
	compiler := jsonschemav5.NewCompiler()
	definitionSchema, err := os.ReadFile("schema.json")
	require.NoError(t, err)
	require.NoError(t, compiler.AddResource(tools.SchemaIDTools, bytes.NewReader(definitionSchema)))
	const specID = "https://schema.test/tool-spec"
	require.NoError(t, compiler.AddResource(specID, strings.NewReader(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"allOf":[
			{"$ref":"`+tools.SchemaIDTools+`#/$defs/ToolDefinitionFields"},
			{"type":"object","required":["connectionRef"],"properties":{"connectionRef":{"type":"object"}}}
		],
		"unevaluatedProperties":false
	}`)))
	specSchema, err := compiler.Compile(specID)
	require.NoError(t, err)
	definition := compileToolSchema(t)
	document := toolSchemaFixture(t)
	require.NoError(t, definition.Validate(document))
	require.Error(t, specSchema.Validate(document), "the composed spec requires its added field")
	document["connectionRef"] = map[string]any{"id": "cxn_example"}
	require.NoError(t, specSchema.Validate(document))
	require.Error(t, definition.Validate(document), "the authored root stays closed")
	document["unknown"] = true
	require.Error(t, specSchema.Validate(document), "the composed spec owns its closure")
	delete(document, "unknown")
	delete(document, "description")
	require.Error(t, specSchema.Validate(document), "composition retains required definition fields")
}

// toolSchemaFixture provides a complete definition with ordinary provider
// property names and every non-body HTTP option. Each caller receives fresh maps.
func toolSchemaFixture(t *testing.T) map[string]any {
	t.Helper()
	var document map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
		"description":"Create a provider item",
		"verbs":["tool:items.create","tool:items.write"],
		"inputSchema":{
			"$schema":"https://json-schema.org/draft/2020-12/schema",
			"type":"object",
			"properties":{"provider_name":{"type":"string"},"data":{"const":{"$ref":"ordinary provider data"}}},
			"required":["provider_name"],
			"additionalProperties":false
		},
		"outputSchema":{"type":"array","items":{"type":"string"}},
		"hints":{"readOnly":false,"destructive":true,"idempotent":false},
		"limits":{
			"timeoutMillis":1500,"maxRequests":3,"maxInputBytes":1536,
			"maxOutputBytes":2048,"maxResponseBytes":4096,"maxEncodedResultBytes":8192,"maxStackDepth":64
		},
		"proxyHttp":{
			"method":"POST","url":"https://{{cfg.apiHost}}/items/{{path.params.id}}",
			"headers":{"X-Provider-Header":"{{cfg.tenant}}"},
			"query":{"provider_key":"{{params.provider_name}}","enabled":true,"page":2,"unset":null,"tags":["a",{"$value":"params.tag"}],"ids":{"$value":"params.ids"}},
			"response":{
				"transformJavascript":"response.body",
				"errors":[
					{"statuses":[404,410],"code":"ITEM_NOT_FOUND","message":"Item not found","retryable":false},
					{"statusClass":5,"code":"PROVIDER_FAILURE","message":"Provider failed","retryable":true},
					{"default":true,"code":"PROVIDER_ERROR","message":"Provider returned an error"}
				]
			}
		}
	}`), &document))
	return document
}

// schemaObject returns an owned object within a fixture for targeted mutations.
func schemaObject(t *testing.T, document map[string]any, fields ...string) map[string]any {
	t.Helper()
	for _, field := range fields {
		object, ok := document[field].(map[string]any)
		require.True(t, ok, "fixture field %s must be an object", field)
		document = object
	}
	return document
}

// TestToolSchemaBodyCodecsRoundTrip covers presence-sensitive body modes through
// both public encodings, including empty bodies and an explicit JSON null.
func TestToolSchemaBodyCodecsRoundTrip(t *testing.T) {
	compiled := compileToolSchema(t)
	for _, tc := range []struct {
		name string
		body string
	}{
		{"no body", `{}`},
		{"JSON object", `{"bodyJson":{"provider_field":{"$value":"params.value"},"literal":{"$literal":{"$value":"literal text"}}}}`},
		{"JSON null", `{"bodyJson":null}`},
		{"JSON scalar", `{"bodyJson":false}`},
		{"JSON empty array", `{"bodyJson":[]}`},
		{"text template", `{"bodyTemplate":{"mediaType":"text/plain","template":""}}`},
		{"binary", `{"bodyRaw":"aGVsbG8="}`},
		{"empty binary", `{"bodyRaw":""}`},
		{"binary template", `{"bodyRaw":"{{params.base64}}"}`},
		{"form", `{"form":{"fields":{"provider_field":"{{params.value}}","repeated":[1,2],"typed":{"$value":"params.tags"}}}}`},
		{"empty form", `{"form":{"fields":{}}}`},
		{"multipart", `{"multipart":{"parts":[{"name":"title","text":""},{"name":"file","filename":"a.bin","mediaType":"application/octet-stream","bodyRaw":""}]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := toolSchemaFixture(t)
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(tc.body), &body))
			for key, value := range body {
				schemaObject(t, document, "proxyHttp")[key] = value
			}
			require.NoError(t, compiled.Validate(document))
			encoded, err := json.Marshal(document)
			require.NoError(t, err)
			var definition tools.ToolDefinition
			require.NoError(t, util.DecodeJSONStrict(encoded, &definition))
			roundTrip, err := json.Marshal(definition)
			require.NoError(t, err)
			require.JSONEq(t, string(encoded), string(roundTrip))

			yamlBytes, err := yaml.Marshal(definition)
			require.NoError(t, err)
			var fromYAML tools.ToolDefinition
			require.NoError(t, util.DecodeYAMLStrict(yamlBytes, &fromYAML))
			roundTrip, err = json.Marshal(fromYAML)
			require.NoError(t, err)
			require.JSONEq(t, string(encoded), string(roundTrip))
			var decoded any
			require.NoError(t, json.Unmarshal(roundTrip, &decoded))
			require.NoError(t, compiled.Validate(decoded))
		})
	}
}

// TestToolSchemaJavaScriptAndSchemaShapes preserves the native schema forms
// independently of HTTP, including a boolean output schema and no output schema.
func TestToolSchemaJavaScriptAndSchemaShapes(t *testing.T) {
	compiled := compileToolSchema(t)
	for _, output := range []any{nil, true, false, map[string]any{"type": "string"}} {
		document := toolSchemaFixture(t)
		delete(document, "proxyHttp")
		document["javascript"] = "async function execute(params, context) { return params; }"
		schemaObject(t, document, "inputSchema")["type"] = []any{"object"}
		if output == nil {
			delete(document, "outputSchema")
		} else {
			document["outputSchema"] = output
		}
		require.NoError(t, compiled.Validate(document))
		encoded, err := json.Marshal(document)
		require.NoError(t, err)
		var definition tools.ToolDefinition
		require.NoError(t, util.DecodeJSONStrict(encoded, &definition))
		roundTrip, err := json.Marshal(definition)
		require.NoError(t, err)
		require.JSONEq(t, string(encoded), string(roundTrip))
	}
}

// TestToolSchemaRejectsInvalidOwnedFields verifies strict authoring boundaries
// while the separate schema compiler owns authored-schema semantic validation.
func TestToolSchemaRejectsInvalidOwnedFields(t *testing.T) {
	compiled := compileToolSchema(t)
	for _, tc := range []struct {
		name   string
		path   []string
		field  string
		value  any
		remove bool
	}{
		{name: "missing description", field: "description", remove: true},
		{name: "blank description", field: "description", value: " \t\n"},
		{name: "missing verbs", field: "verbs", remove: true},
		{name: "empty verbs", field: "verbs", value: []any{}},
		{name: "blank verb", field: "verbs", value: []any{" "}},
		{name: "duplicate verbs", field: "verbs", value: []any{"tool:x", "tool:x"}},
		{name: "missing input schema", field: "inputSchema", remove: true},
		{name: "boolean input schema", field: "inputSchema", value: true},
		{name: "untyped input schema", field: "inputSchema", value: map[string]any{}},
		{name: "array input schema", path: []string{"inputSchema"}, field: "type", value: "array"},
		{name: "nullable input schema", path: []string{"inputSchema"}, field: "type", value: []any{"object", "null"}},
		{name: "null output schema", field: "outputSchema", value: nil},
		{name: "scalar output schema", field: "outputSchema", value: "string"},
		{name: "no executor", field: "proxyHttp", remove: true},
		{name: "two executors", field: "javascript", value: "async function execute() {}"},
		{name: "internal MCP executor", field: "mcpCall", value: map[string]any{}},
		{name: "internal OpenAPI executor", field: "openapiOperation", value: map[string]any{}},
		{name: "resource field", field: "connectionRef", value: map[string]any{}},
		{name: "hint type", path: []string{"hints"}, field: "readOnly", value: "true"},
		{name: "unknown hint", path: []string{"hints"}, field: "safe", value: true},
		{name: "zero timeout", path: []string{"limits"}, field: "timeoutMillis", value: 0},
		{name: "negative byte budget", path: []string{"limits"}, field: "maxInputBytes", value: -1},
		{name: "fractional request budget", path: []string{"limits"}, field: "maxRequests", value: 1.5},
		{name: "unknown limit", path: []string{"limits"}, field: "timeout", value: "1s"},
		{name: "missing HTTP method", path: []string{"proxyHttp"}, field: "method", remove: true},
		{name: "invalid HTTP method", path: []string{"proxyHttp"}, field: "method", value: "GET /"},
		{name: "blank HTTP URL", path: []string{"proxyHttp"}, field: "url", value: " "},
		{name: "header type", path: []string{"proxyHttp"}, field: "headers", value: map[string]any{"X-Provider": 2}},
		{name: "unknown HTTP field", path: []string{"proxyHttp"}, field: "body", value: "legacy"},
		{name: "nested query object", path: []string{"proxyHttp", "query"}, field: "bad", value: map[string]any{"nested": true}},
		{name: "nested query array", path: []string{"proxyHttp", "query"}, field: "bad", value: []any{map[string]any{"nested": true}}},
		{name: "empty directive", path: []string{"proxyHttp", "query"}, field: "bad", value: map[string]any{"$value": ""}},
		{name: "extra directive field", path: []string{"proxyHttp", "query"}, field: "bad", value: map[string]any{"$value": "params.x", "extra": true}},
		{name: "template media type required", path: []string{"proxyHttp"}, field: "bodyTemplate", value: map[string]any{"template": "hello"}},
		{name: "form fields required", path: []string{"proxyHttp"}, field: "form", value: map[string]any{}},
		{name: "multipart parts required", path: []string{"proxyHttp"}, field: "multipart", value: map[string]any{"parts": []any{}}},
		{name: "unknown response field", path: []string{"proxyHttp", "response"}, field: "body", value: "response.body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := toolSchemaFixture(t)
			object := schemaObject(t, document, tc.path...)
			if tc.remove {
				delete(object, tc.field)
			} else {
				object[tc.field] = tc.value
			}
			require.Error(t, compiled.Validate(document))
		})
	}
}

// TestToolSchemaRejectsAmbiguousBodiesAndSelectors catches overlapping union
// members even when the competing JSON body is explicitly null.
func TestToolSchemaRejectsAmbiguousBodiesAndSelectors(t *testing.T) {
	compiled := compileToolSchema(t)
	for _, body := range []string{
		`{"bodyJson":null,"bodyRaw":""}`,
		`{"bodyTemplate":{"mediaType":"text/plain","template":""},"form":{"fields":{}}}`,
		`{"bodyRaw":null}`,
		`{"bodyTemplate":null}`,
		`{"form":null}`,
		`{"multipart":null}`,
		`{"multipart":{"parts":[{"name":"x"}]}}`,
		`{"multipart":{"parts":[{"name":"x","text":"","bodyRaw":""}]}}`,
	} {
		document := toolSchemaFixture(t)
		var fields map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &fields))
		for key, value := range fields {
			schemaObject(t, document, "proxyHttp")[key] = value
		}
		require.Error(t, compiled.Validate(document), body)
	}
	for _, selector := range []string{
		`{}`, `{"statuses":[]}`, `{"statuses":[99]}`, `{"statuses":[600]}`,
		`{"statuses":[404,404]}`, `{"statusClass":0}`, `{"statusClass":6}`,
		`{"default":false}`, `{"statuses":[404],"statusClass":4}`,
		`{"statusClass":4,"default":true}`,
	} {
		document := toolSchemaFixture(t)
		var mapping map[string]any
		require.NoError(t, json.Unmarshal([]byte(selector), &mapping))
		mapping["code"], mapping["message"] = "PROVIDER_ERROR", "Provider failed"
		schemaObject(t, document, "proxyHttp", "response")["errors"] = []any{mapping}
		require.Error(t, compiled.Validate(document), selector)
	}
}
