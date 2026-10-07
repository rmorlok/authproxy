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

// toolResourceSchemaFixture wraps an authored definition in a fully populated
// Tool resource, including revision-aware readiness and generated ownership.
func toolResourceSchemaFixture(t *testing.T) map[string]any {
	t.Helper()
	spec := toolSchemaFixture(t)
	spec["connectionRef"] = map[string]any{
		"apiVersion": "authproxy.net/v1alpha1", "kind": "Connection", "id": "cxn_example",
	}
	return map[string]any{
		"apiVersion": "authproxy.net/v1alpha1", "kind": "Tool",
		"metadata": map[string]any{"id": "tol_example", "name": "create-item", "namespace": "root.acme"},
		"spec":     spec,
		"status": map[string]any{
			"revision": 2,
			"conditions": []any{map[string]any{
				"type": "Ready", "status": "True", "observedRevision": 2,
				"lastTransitionTime": "2026-10-06T12:00:00Z", "reason": "Published", "message": "Ready to invoke",
			}},
			"managedBy": map[string]any{
				"toolSetRef": map[string]any{
					"apiVersion": "authproxy.net/v1alpha1", "kind": "ToolSet", "id": "tls_example",
					"generation": 3, "namespace": "root", "name": "provider-tools",
				},
				"sourceKey": "GET /v1/records/{recordId}",
			},
		},
	}
}

// TestToolResourceSchemaRoundTrip checks real flat spec composition and typed
// serialization without changing the preexisting authored-definition root.
func TestToolResourceSchemaRoundTrip(t *testing.T) {
	compiled, err := schema.CompileSchema(tools.SchemaIDToolResource)
	require.NoError(t, err)
	document := toolResourceSchemaFixture(t)
	require.NoError(t, compiled.Validate(document))
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	var resource tools.Tool
	require.NoError(t, util.DecodeJSONStrict(encoded, &resource))
	roundTrip, err := json.Marshal(resource)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), string(roundTrip))
	asYAML, err := yaml.Marshal(resource)
	require.NoError(t, err)
	var fromYAML tools.Tool
	require.NoError(t, util.DecodeYAMLStrict(asYAML, &fromYAML))
	roundTrip, err = json.Marshal(fromYAML)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), string(roundTrip))
	var decoded any
	require.NoError(t, json.Unmarshal(roundTrip, &decoded))
	require.NoError(t, compiled.Validate(decoded))
	// The authored-definition root still rejects connection identity, even though
	// ToolSpec composes those same definition fields with its reference.
	require.Error(t, compileToolSchema(t).Validate(document["spec"]))

	delete(document, "status")
	delete(schemaObject(t, document, "metadata"), "id")
	delete(schemaObject(t, document, "metadata"), "name")
	schemaObject(t, document, "spec")["connectionRef"] = map[string]any{
		"apiVersion": "authproxy.net/v1alpha1", "kind": "Connection", "namespace": "root.acme", "name": "production",
	}
	require.NoError(t, compiled.Validate(document), "authoring may omit server fields and use a named connection")
	document["status"] = map[string]any{"revision": 1}
	require.NoError(t, compiled.Validate(document), "readiness observations and ownership are optional")
}

// TestToolResourceSchemaRejectsInvalidFields covers schema-owned resource and
// reference constraints; lifecycle and cross-field comparisons remain in Go.
func TestToolResourceSchemaRejectsInvalidFields(t *testing.T) {
	compiled, err := schema.CompileSchema(tools.SchemaIDToolResource)
	require.NoError(t, err)
	for _, test := range []struct {
		name   string
		path   []string
		field  string
		value  any
		remove bool
	}{
		{name: "missing version", field: "apiVersion", remove: true},
		{name: "wrong version", field: "apiVersion", value: "authproxy.net/v2"},
		{name: "wrong kind", field: "kind", value: "ToolSet"},
		{name: "missing metadata", field: "metadata", remove: true},
		{name: "null metadata", field: "metadata", value: nil},
		{name: "missing spec", field: "spec", remove: true},
		{name: "unknown resource field", field: "definition", value: map[string]any{}},
		{name: "wrong ID prefix", path: []string{"metadata"}, field: "id", value: "cxn_example"},
		{name: "missing namespace", path: []string{"metadata"}, field: "namespace", remove: true},
		{name: "invalid namespace", path: []string{"metadata"}, field: "namespace", value: "outside.acme"},
		{name: "resource generation", path: []string{"metadata"}, field: "generation", value: 1},
		{name: "unknown metadata", path: []string{"metadata"}, field: "revision", value: 1},
		{name: "missing connection", path: []string{"spec"}, field: "connectionRef", remove: true},
		{name: "missing description", path: []string{"spec"}, field: "description", remove: true},
		{name: "nested definition", path: []string{"spec"}, field: "definition", value: map[string]any{}},
		{name: "internal executor", path: []string{"spec"}, field: "mcpCall", value: map[string]any{}},
		{name: "connection version", path: []string{"spec", "connectionRef"}, field: "apiVersion", value: "authproxy.net/v2"},
		{name: "connection kind", path: []string{"spec", "connectionRef"}, field: "kind", value: "Connector"},
		{name: "connection prefix", path: []string{"spec", "connectionRef"}, field: "id", value: "cxr_example"},
		{name: "connection identity", path: []string{"spec", "connectionRef"}, field: "id", remove: true},
		{name: "connection namespace", path: []string{"spec", "connectionRef"}, field: "namespace", value: "outside"},
		{name: "connection generation", path: []string{"spec", "connectionRef"}, field: "generation", value: 1},
		{name: "unknown connection field", path: []string{"spec", "connectionRef"}, field: "revision", value: 1},
		{name: "missing revision", path: []string{"status"}, field: "revision", remove: true},
		{name: "zero revision", path: []string{"status"}, field: "revision", value: 0},
		{name: "fractional revision", path: []string{"status"}, field: "revision", value: 1.5},
		{name: "unknown status field", path: []string{"status"}, field: "generation", value: 1},
		{name: "missing source key", path: []string{"status", "managedBy"}, field: "sourceKey", remove: true},
		{name: "blank source key", path: []string{"status", "managedBy"}, field: "sourceKey", value: " \t"},
		{name: "unknown owner field", path: []string{"status", "managedBy"}, field: "generation", value: 1},
		{name: "owner prefix", path: []string{"status", "managedBy", "toolSetRef"}, field: "id", value: "tol_example"},
		{name: "owner kind", path: []string{"status", "managedBy", "toolSetRef"}, field: "kind", value: "Tool"},
		{name: "canonical owner ID required", path: []string{"status", "managedBy", "toolSetRef"}, field: "id", remove: true},
		{name: "owner generation required", path: []string{"status", "managedBy", "toolSetRef"}, field: "generation", remove: true},
		{name: "owner generation positive", path: []string{"status", "managedBy", "toolSetRef"}, field: "generation", value: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := toolResourceSchemaFixture(t)
			object := schemaObject(t, document, test.path...)
			if test.remove {
				delete(object, test.field)
			} else {
				object[test.field] = test.value
			}
			require.Error(t, compiled.Validate(document))
		})
	}
}

// TestToolConditionAndReferenceSchemas checks the revision-based condition
// vocabulary and generation-free Tool reference independently of an envelope.
func TestToolConditionAndReferenceSchemas(t *testing.T) {
	conditionSchema, err := schema.CompileSchema(tools.SchemaIDToolResource + "#/$defs/ToolCondition")
	require.NoError(t, err)
	for _, test := range []struct {
		field string
		value any
		valid bool
	}{
		{"status", "Unknown", true}, {"observedRevision", 0, true}, {"observedRevision", 1, true},
		{"observedRevision", -1, false}, {"observedGeneration", 1, false}, {"status", "ready", false},
		{"type", "", false}, {"lastTransitionTime", nil, false}, {"unknown", true, false},
	} {
		condition := map[string]any{"type": "Ready", "status": "True", "lastTransitionTime": "2026-10-06T12:00:00Z"}
		condition[test.field] = test.value
		err := conditionSchema.Validate(condition)
		if test.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err, "%s=%v", test.field, test.value)
		}
	}
	referenceSchema, err := schema.CompileSchema(tools.SchemaIDToolResource + "#/$defs/ToolReference")
	require.NoError(t, err)
	reference := map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": "Tool", "id": "tol_example"}
	require.NoError(t, referenceSchema.Validate(reference))
	reference["generation"] = 1
	require.Error(t, referenceSchema.Validate(reference))
	delete(reference, "generation")
	reference["id"] = "tls_example"
	require.Error(t, referenceSchema.Validate(reference))
	delete(reference, "id")
	reference["name"], reference["namespace"] = "create-item", "root.acme"
	require.NoError(t, referenceSchema.Validate(reference))
}

// TestToolPatchSchemaFields validates partial replacements independently from
// the complete definition that ApplyTo constructs and validates afterward.
func TestToolPatchSchemaFields(t *testing.T) {
	compiled, err := schema.CompileSchema(tools.SchemaIDToolPatch)
	require.NoError(t, err)
	for _, test := range []struct {
		name, spec string
		valid      bool
	}{
		{"empty patch", `{}`, true},
		{"description only", `{"description":"Renamed operation"}`, true},
		{"verbs only", `{"verbs":["tool:read"]}`, true},
		{"input schema only", `{"inputSchema":{"type":"object"}}`, true},
		{"output false", `{"outputSchema":false}`, true},
		{"clear optional fields", `{"outputSchema":null,"hints":null,"limits":null,"proxyHttp":null,"javascript":null}`, true},
		{"executor switch", `{"proxyHttp":null,"javascript":"async function execute() {}"}`, true},
		{"executors checked after merge", `{"proxyHttp":{"method":"GET","url":"https://example.test"},"javascript":"code"}`, true},
		{"connection ID", `{"connectionRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","id":"cxn_example"}}`, true},
		{"connection name", `{"connectionRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","namespace":"root.acme","name":"production"}}`, true},
		{"null connection", `{"connectionRef":null}`, false},
		{"null description", `{"description":null}`, false},
		{"null verbs", `{"verbs":null}`, false},
		{"null input schema", `{"inputSchema":null}`, false},
		{"empty verbs", `{"verbs":[]}`, false},
		{"incomplete executor replacement", `{"proxyHttp":{"url":"https://example.test"}}`, false},
		{"invalid optional field", `{"hints":{"readOnly":"yes"}}`, false},
		{"invalid cleared field replacement", `{"limits":{"maxRequests":0}}`, false},
		{"internal executor", `{"openapiOperation":{}}`, false},
		{"unknown field", `{"definition":{}}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var spec any
			require.NoError(t, json.Unmarshal([]byte(test.spec), &spec))
			document := map[string]any{
				"apiVersion": "authproxy.net/v1alpha1", "kind": "Tool", "metadata": map[string]any{}, "spec": spec,
			}
			err := compiled.Validate(document)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	for _, test := range []struct {
		field  string
		value  any
		remove bool
	}{
		{field: "metadata", remove: true}, {field: "spec", remove: true},
		{field: "metadata", value: nil}, {field: "spec", value: nil},
		{field: "status", value: nil}, {field: "status", value: map[string]any{"revision": 2}},
		{field: "kind", value: "ToolSet"}, {field: "unknown", value: true},
		{field: "metadata", value: map[string]any{"generation": 1}},
		{field: "metadata", value: map[string]any{"id": "cxn_example"}},
		{field: "metadata", value: map[string]any{"createdAt": "2026-10-06T12:00:00Z"}},
		{field: "metadata", value: map[string]any{"updatedAt": "2026-10-06T12:00:00Z"}},
	} {
		document := map[string]any{
			"apiVersion": "authproxy.net/v1alpha1", "kind": "Tool", "metadata": map[string]any{}, "spec": map[string]any{},
		}
		if test.remove {
			delete(document, test.field)
		} else {
			document[test.field] = test.value
		}
		require.Error(t, compiled.Validate(document), "field %s", test.field)
	}
}
