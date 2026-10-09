package toolsets_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema"
	"github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// compileToolSetSchema loads only repository-owned schemas and forbids any
// fallback resource loading, including network access, while resolving refs.
func compileToolSetSchema(t *testing.T, fragment string) *jsonschemav5.Schema {
	t.Helper()
	compiler := jsonschemav5.NewCompiler()
	compiler.LoadURL = func(url string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("unexpected external schema resource %q", url)
	}
	for _, path := range []string{
		"../../common/schema.json", "../meta/schema.json", "../namespace/schema.json",
		"../tools/schema.json", "schema.json",
	} {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var envelope struct {
			ID string `json:"$id"`
		}
		require.NoError(t, json.Unmarshal(data, &envelope))
		if path == "schema.json" {
			require.Equal(t, toolsets.SchemaIDToolSets, envelope.ID)
		}
		require.NoError(t, compiler.AddResource(envelope.ID, bytes.NewReader(data)))
	}
	compiled, err := compiler.Compile(toolsets.SchemaIDToolSets + fragment)
	require.NoError(t, err)
	return compiled
}

// toolSetSchemaFixture supplies both executor shapes, restricted template
// metadata, inherited selector labels, and a stored release envelope.
func toolSetSchemaFixture(t *testing.T) map[string]any {
	t.Helper()
	var document map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
		"apiVersion":"authproxy.net/v1alpha1","kind":"ToolSet",
		"metadata":{"id":"tls_example","name":"provider-tools","namespace":"root.product","generation":2},
		"spec":{
			"connectionSelector":{"namespace":"root.product.**","matchLabels":{"provider":"example","apxy/connector/-/id":"cxr_example"}},
			"release":{"desiredState":"primary"},
			"definition":{"source":{"explicit":{"tools":[
				{
					"key":"list-records",
					"metadata":{"name":"records","labels":{"capability":"records"},"annotations":{"docs.example.com/path":"/records"}},
					"spec":{
						"description":"List records","verbs":["tool:records.list"],
						"inputSchema":{"type":"object","properties":{"provider_field":{"type":"boolean"}}},
						"outputSchema":false,"hints":{"readOnly":true},
						"proxyHttp":{"method":"POST","url":"https://{{cfg.host}}/records","bodyJson":null}
					}
				},
				{
					"key":"lookup-record",
					"spec":{
						"description":"Look up a record","verbs":["tool:records.lookup"],
						"inputSchema":{"type":"object"},
						"javascript":"async function execute(params, context) { return params; }"
					}
				}
			]}}}
		},
		"status":{"release":{"state":"primary"}}
	}`), &document))
	return document
}

// schemaObject follows an object-only fixture path for a focused mutation.
func schemaObject(t *testing.T, document map[string]any, fields ...string) map[string]any {
	t.Helper()
	for _, field := range fields {
		object, ok := document[field].(map[string]any)
		require.True(t, ok, "%s must be an object in this fixture", field)
		document = object
	}
	return document
}

// TestToolSetSchemaRoundTrip confirms the schema is available offline and in
// the production embed, then preserves both template executors through JSON/YAML.
func TestToolSetSchemaRoundTrip(t *testing.T) {
	compiled := compileToolSetSchema(t, "")
	document := toolSetSchemaFixture(t)
	require.NoError(t, compiled.Validate(document))
	embedded, err := schema.CompileSchema(toolsets.SchemaIDToolSets)
	require.NoError(t, err)
	require.NoError(t, embedded.Validate(document))
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	var resource toolsets.ToolSet
	require.NoError(t, util.DecodeJSONStrict(encoded, &resource))
	roundTrip, err := json.Marshal(resource)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), string(roundTrip))
	asYAML, err := yaml.Marshal(resource)
	require.NoError(t, err)
	var fromYAML toolsets.ToolSet
	require.NoError(t, util.DecodeYAMLStrict(asYAML, &fromYAML))
	roundTrip, err = json.Marshal(fromYAML)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), string(roundTrip))
	var decoded any
	require.NoError(t, json.Unmarshal(roundTrip, &decoded))
	require.NoError(t, compiled.Validate(decoded))
}

// TestToolSetSchemaAuthoringAndReleaseShapes keeps server persistence rules
// outside the generic schema while recognizing every defined release state.
func TestToolSetSchemaAuthoringAndReleaseShapes(t *testing.T) {
	compiled := compileToolSetSchema(t, "")
	document := toolSetSchemaFixture(t)
	delete(document, "status")
	metadata := schemaObject(t, document, "metadata")
	delete(metadata, "id")
	delete(metadata, "name")
	delete(metadata, "generation")
	delete(schemaObject(t, document, "spec"), "release")
	selector := schemaObject(t, document, "spec", "connectionSelector")
	delete(selector, "namespace")
	selector["matchLabels"] = map[string]any{}
	schemaObject(t, document, "spec", "definition", "source", "explicit")["tools"] = []any{}
	require.NoError(t, compiled.Validate(document), "empty inventory and an explicitly broad selector are valid")
	selector["namespace"] = ""
	require.NoError(t, compiled.Validate(document), "empty namespace selects the same default as omission")
	for _, desired := range []string{"draft", "primary"} {
		schemaObject(t, document, "spec")["release"] = map[string]any{"desiredState": desired}
		require.NoError(t, compiled.Validate(document))
	}
	for _, state := range []string{"draft", "primary", "active", "archived"} {
		document["status"] = map[string]any{"release": map[string]any{"state": state}}
		require.NoError(t, compiled.Validate(document), "desired/observed compatibility is checked by Go lifecycle validation")
	}
}

// TestToolSetSchemaRejectsInvalidFields checks the closed resource/source
// shapes and selector requirements without relying on Go validation.
func TestToolSetSchemaRejectsInvalidFields(t *testing.T) {
	compiled := compileToolSetSchema(t, "")
	for _, test := range []struct {
		name, field string
		path        []string
		value       any
		remove      bool
	}{
		{name: "missing version", field: "apiVersion", remove: true},
		{name: "wrong kind", field: "kind", value: "Tool"},
		{name: "wrong prefix", path: []string{"metadata"}, field: "id", value: "tol_example"},
		{name: "missing namespace", path: []string{"metadata"}, field: "namespace", remove: true},
		{name: "zero generation", path: []string{"metadata"}, field: "generation", value: 0},
		{name: "unknown root field", field: "source", value: map[string]any{}},
		{name: "missing selector", path: []string{"spec"}, field: "connectionSelector", remove: true},
		{name: "null selector", path: []string{"spec"}, field: "connectionSelector", value: nil},
		{name: "missing labels", path: []string{"spec", "connectionSelector"}, field: "matchLabels", remove: true},
		{name: "null labels", path: []string{"spec", "connectionSelector"}, field: "matchLabels", value: nil},
		{name: "wrong label type", path: []string{"spec", "connectionSelector"}, field: "matchLabels", value: map[string]any{"provider": true}},
		{name: "null selector namespace", path: []string{"spec", "connectionSelector"}, field: "namespace", value: nil},
		{name: "unsupported wildcard", path: []string{"spec", "connectionSelector"}, field: "namespace", value: "root.product.*"},
		{name: "unsupported selector operator", path: []string{"spec", "connectionSelector"}, field: "matchExpressions", value: []any{}},
		{name: "missing definition", path: []string{"spec"}, field: "definition", remove: true},
		{name: "missing source", path: []string{"spec", "definition"}, field: "source", remove: true},
		{name: "selector inside generation", path: []string{"spec", "definition"}, field: "connectionSelector", value: map[string]any{}},
		{name: "unsupported mappings", path: []string{"spec", "definition"}, field: "permissionMappings", value: []any{}},
		{name: "missing explicit source", path: []string{"spec", "definition", "source"}, field: "explicit", remove: true},
		{name: "null explicit source", path: []string{"spec", "definition", "source"}, field: "explicit", value: nil},
		{name: "deferred OpenAPI source", path: []string{"spec", "definition", "source"}, field: "openapi", value: map[string]any{}},
		{name: "deferred MCP source", path: []string{"spec", "definition", "source"}, field: "mcp", value: map[string]any{}},
		{name: "missing tools", path: []string{"spec", "definition", "source", "explicit"}, field: "tools", remove: true},
		{name: "null tools", path: []string{"spec", "definition", "source", "explicit"}, field: "tools", value: nil},
		{name: "active is observed only", path: []string{"spec", "release"}, field: "desiredState", value: "active"},
		{name: "archived is observed only", path: []string{"spec", "release"}, field: "desiredState", value: "archived"},
		{name: "unknown desired state", path: []string{"spec", "release"}, field: "desiredState", value: "published"},
		{name: "missing release status", path: []string{"status"}, field: "release", remove: true},
		{name: "missing release state", path: []string{"status", "release"}, field: "state", remove: true},
		{name: "unknown observed state", path: []string{"status", "release"}, field: "state", value: "ready"},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := toolSetSchemaFixture(t)
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

// TestToolTemplateSchemaBoundaries restricts metadata to authored template
// fields and reuses the closed definition so connection identity cannot leak in.
func TestToolTemplateSchemaBoundaries(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/ToolTemplate")
	for _, test := range []struct {
		name, field string
		path        []string
		value       any
		remove      bool
	}{
		{name: "missing key", field: "key", remove: true},
		{name: "blank key", field: "key", value: " \t"},
		{name: "missing spec", field: "spec", remove: true},
		{name: "connection binding", path: []string{"spec"}, field: "connectionRef", value: map[string]any{}},
		{name: "missing verbs", path: []string{"spec"}, field: "verbs", remove: true},
		{name: "competing executor", path: []string{"spec"}, field: "javascript", value: "code"},
		{name: "generated executor", path: []string{"spec"}, field: "mcpCall", value: map[string]any{}},
		{name: "metadata ID", path: []string{"metadata"}, field: "id", value: "tol_example"},
		{name: "metadata namespace", path: []string{"metadata"}, field: "namespace", value: "root.product"},
		{name: "metadata generation", path: []string{"metadata"}, field: "generation", value: 1},
		{name: "metadata ownership", path: []string{"metadata"}, field: "managedBy", value: map[string]any{}},
		{name: "metadata timestamp", path: []string{"metadata"}, field: "createdAt", value: "2026-10-07T12:00:00Z"},
		{name: "invalid name", path: []string{"metadata"}, field: "name", value: "not a resource name"},
		{name: "reserved label", path: []string{"metadata"}, field: "labels", value: map[string]any{"apxy/connector/-/id": "cxr_example"}},
		{name: "invalid label key", path: []string{"metadata"}, field: "labels", value: map[string]any{"invalid key": "value"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := schemaObject(t, toolSetSchemaFixture(t), "spec", "definition", "source", "explicit")
			template := source["tools"].([]any)[0].(map[string]any)
			object := schemaObject(t, template, test.path...)
			if test.remove {
				delete(object, test.field)
			} else {
				object[test.field] = test.value
			}
			require.Error(t, compiled.Validate(template))
		})
	}
}

// TestToolSetReferenceSchema accepts logical or generation-addressed typed
// references while enforcing identity, namespace, and positive generation shape.
func TestToolSetReferenceSchema(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/ToolSetReference")
	reference := map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": "ToolSet", "id": "tls_example"}
	require.NoError(t, compiled.Validate(reference))
	reference["generation"] = 2
	require.NoError(t, compiled.Validate(reference))
	reference["generation"] = 0
	require.Error(t, compiled.Validate(reference))
	delete(reference, "generation")
	reference["id"] = "tol_example"
	require.Error(t, compiled.Validate(reference))
	delete(reference, "id")
	reference["namespace"], reference["name"] = "root.product", "provider-tools"
	require.NoError(t, compiled.Validate(reference))
	reference["kind"] = "Tool"
	require.Error(t, compiled.Validate(reference))
}
