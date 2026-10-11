package toolsets_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
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
		"../tools/schema.json", "../connection/schema.json", "schema.json",
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
		{name: "mappings on explicit source", path: []string{"spec", "definition"}, field: "permissionMappings", value: []any{}},
		{name: "missing explicit source", path: []string{"spec", "definition", "source"}, field: "explicit", remove: true},
		{name: "null explicit source", path: []string{"spec", "definition", "source"}, field: "explicit", value: nil},
		{name: "unknown source variant", path: []string{"spec", "definition", "source"}, field: "graphql", value: map[string]any{}},
		{name: "competing MCP source", path: []string{"spec", "definition", "source"}, field: "mcp", value: map[string]any{"endpoint": "https://example.com/mcp", "transport": "streamableHttp"}},
		{name: "null MCP beside explicit source", path: []string{"spec", "definition", "source"}, field: "mcp", value: nil},
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

// toolSetPatchSchemaFixture reuses complete replacement values while omitting
// server-observed status, which is never authored in an update envelope.
func toolSetPatchSchemaFixture(t *testing.T) map[string]any {
	t.Helper()
	document := toolSetSchemaFixture(t)
	delete(document, "status")
	return document
}

// TestToolSetPatchSchemaRoundTrip checks offline and embedded compilation, and
// preserves empty no-op objects and complete replacements across both codecs.
func TestToolSetPatchSchemaRoundTrip(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/ToolSetPatch")
	embedded, err := schema.CompileSchema(toolsets.SchemaIDToolSetPatch)
	require.NoError(t, err)
	for _, test := range []struct {
		name string
		spec map[string]any
	}{
		{name: "complete replacements"},
		{name: "empty patch", spec: map[string]any{}},
		{name: "release no-op", spec: map[string]any{"release": map[string]any{}}},
		{name: "request draft", spec: map[string]any{"release": map[string]any{"desiredState": "draft"}}},
		{name: "request primary", spec: map[string]any{"release": map[string]any{"desiredState": "primary"}}},
		{name: "selector replacement", spec: map[string]any{"connectionSelector": map[string]any{"matchLabels": map[string]any{}}}},
		{name: "empty inventory replacement", spec: map[string]any{"definition": map[string]any{"source": map[string]any{"explicit": map[string]any{"tools": []any{}}}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := toolSetPatchSchemaFixture(t)
			if test.spec != nil {
				document["metadata"] = map[string]any{}
				document["spec"] = test.spec
			}
			require.NoError(t, compiled.Validate(document))
			require.NoError(t, embedded.Validate(document))
			encoded, err := json.Marshal(document)
			require.NoError(t, err)
			var patch toolsets.ToolSetPatch
			require.NoError(t, util.DecodeJSONStrict(encoded, &patch))
			roundTrip, err := json.Marshal(patch)
			require.NoError(t, err)
			require.JSONEq(t, string(encoded), string(roundTrip))
			asYAML, err := yaml.Marshal(patch)
			require.NoError(t, err)
			var fromYAML toolsets.ToolSetPatch
			require.NoError(t, util.DecodeYAMLStrict(asYAML, &fromYAML))
			roundTrip, err = json.Marshal(fromYAML)
			require.NoError(t, err)
			require.JSONEq(t, string(encoded), string(roundTrip))
			var decoded any
			require.NoError(t, json.Unmarshal(roundTrip, &decoded))
			require.NoError(t, compiled.Validate(decoded))
		})
	}
}

// TestToolSetPatchSchemaRejectsInvalidFields distinguishes full replacements
// from partial release edits and rejects nulls, status, and unknown fields.
func TestToolSetPatchSchemaRejectsInvalidFields(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/ToolSetPatch")
	for _, test := range []struct {
		name, field string
		path        []string
		value       any
		remove      bool
	}{
		{name: "missing version", field: "apiVersion", remove: true},
		{name: "wrong version", field: "apiVersion", value: "authproxy.net/v2"},
		{name: "missing kind", field: "kind", remove: true},
		{name: "wrong kind", field: "kind", value: "Tool"},
		{name: "missing metadata", field: "metadata", remove: true},
		{name: "null metadata", field: "metadata", value: nil},
		{name: "missing spec", field: "spec", remove: true},
		{name: "null spec", field: "spec", value: nil},
		{name: "unknown root field", field: "definition", value: map[string]any{}},
		{name: "observed status", field: "status", value: map[string]any{"release": map[string]any{"state": "primary"}}},
		{name: "null status", field: "status", value: nil},
		{name: "wrong ID prefix", path: []string{"metadata"}, field: "id", value: "tol_example"},
		{name: "invalid namespace", path: []string{"metadata"}, field: "namespace", value: "root.product.**"},
		{name: "zero generation", path: []string{"metadata"}, field: "generation", value: 0},
		{name: "fractional generation", path: []string{"metadata"}, field: "generation", value: 1.5},
		{name: "server creation time", path: []string{"metadata"}, field: "createdAt", value: "2026-10-08T12:00:00Z"},
		{name: "server update time", path: []string{"metadata"}, field: "updatedAt", value: "2026-10-08T12:00:00Z"},
		{name: "unknown metadata field", path: []string{"metadata"}, field: "revision", value: 1},
		{name: "null selector", path: []string{"spec"}, field: "connectionSelector", value: nil},
		{name: "partial selector", path: []string{"spec", "connectionSelector"}, field: "matchLabels", remove: true},
		{name: "null selector labels", path: []string{"spec", "connectionSelector"}, field: "matchLabels", value: nil},
		{name: "null selector namespace", path: []string{"spec", "connectionSelector"}, field: "namespace", value: nil},
		{name: "unknown selector field", path: []string{"spec", "connectionSelector"}, field: "matchExpressions", value: []any{}},
		{name: "null definition", path: []string{"spec"}, field: "definition", value: nil},
		{name: "partial definition", path: []string{"spec", "definition"}, field: "source", remove: true},
		{name: "missing replacement tools", path: []string{"spec", "definition", "source", "explicit"}, field: "tools", remove: true},
		{name: "null replacement tools", path: []string{"spec", "definition", "source", "explicit"}, field: "tools", value: nil},
		{name: "unknown source field", path: []string{"spec", "definition", "source"}, field: "graphql", value: map[string]any{}},
		{name: "unknown spec field", path: []string{"spec"}, field: "source", value: map[string]any{}},
		{name: "null release", path: []string{"spec"}, field: "release", value: nil},
		{name: "null desired state", path: []string{"spec", "release"}, field: "desiredState", value: nil},
		{name: "empty desired state", path: []string{"spec", "release"}, field: "desiredState", value: ""},
		{name: "active desired state", path: []string{"spec", "release"}, field: "desiredState", value: "active"},
		{name: "archived desired state", path: []string{"spec", "release"}, field: "desiredState", value: "archived"},
		{name: "unknown desired state", path: []string{"spec", "release"}, field: "desiredState", value: "published"},
		{name: "unknown release field", path: []string{"spec", "release"}, field: "state", value: "primary"},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := toolSetPatchSchemaFixture(t)
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

// toolSetMCPDefinitionSchemaFixture includes exact names, fractional refresh
// timing, and both permission matcher forms without tying them to a live catalog.
func toolSetMCPDefinitionSchemaFixture(t *testing.T) map[string]any {
	t.Helper()
	var definition map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
		"source":{"mcp":{
			"endpoint":"https://{{cfg.host}}/mcp","transport":"streamableHttp","refreshInterval":"1.5s",
			"tools":{"includeNames":["list_records"," exact name "],"excludeNames":[]}
		}},
		"defaults":{
			"labels":{"capability":"records","empty":""},
			"annotations":{"docs.example.com/path":"/records","note":"Read records\nwith pagination."},
			"limits":{"timeoutMillis":1500,"maxRequests":3,"maxInputBytes":4096,"maxOutputBytes":8192,"maxResponseBytes":16384,"maxEncodedResultBytes":24576,"maxStackDepth":50}
		},
		"permissionMappings":[{
			"match":{"sourceKeys":["list_records"],"sourceKeyPatterns":["read_.*"]},
			"addVerbs":["tool:records.read","custom verb"]
		}]
	}`), &definition))
	return definition
}

// TestToolSetMCPSchemaRoundTrip shares one source contract across standalone
// definitions, full resources, and whole-definition patches through both codecs.
func TestToolSetMCPSchemaRoundTrip(t *testing.T) {
	for _, test := range []struct {
		name, fragment string
		newValue       func() any
	}{
		{"definition", "#/$defs/ToolSetDefinition", func() any { return &toolsets.ToolSetDefinition{} }},
		{"resource", "", func() any { return &toolsets.ToolSet{} }},
		{"patch", "#/$defs/ToolSetPatch", func() any { return &toolsets.ToolSetPatch{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			compiled := compileToolSetSchema(t, test.fragment)
			embedded, err := schema.CompileSchema(toolsets.SchemaIDToolSets + test.fragment)
			require.NoError(t, err)
			for _, shape := range []string{"complete", "empty mappings", "minimal"} {
				t.Run(shape, func(t *testing.T) {
					definition := toolSetMCPDefinitionSchemaFixture(t)
					if shape == "empty mappings" {
						definition["permissionMappings"] = []any{}
					}
					if shape == "minimal" {
						delete(definition, "permissionMappings")
						delete(definition, "defaults")
						mcp := schemaObject(t, definition, "source", "mcp")
						delete(mcp, "refreshInterval")
						delete(mcp, "tools")
					}
					document := definition
					if test.name != "definition" {
						document = toolSetSchemaFixture(t)
						schemaObject(t, document, "spec")["definition"] = definition
						if test.name == "patch" {
							delete(document, "status")
						}
					}
					require.NoError(t, compiled.Validate(document))
					require.NoError(t, embedded.Validate(document))
					encoded, err := json.Marshal(document)
					require.NoError(t, err)
					value := test.newValue()
					require.NoError(t, util.DecodeJSONStrict(encoded, value))
					roundTrip, err := json.Marshal(value)
					require.NoError(t, err)
					require.JSONEq(t, string(encoded), string(roundTrip))
					asYAML, err := yaml.Marshal(value)
					require.NoError(t, err)
					fromYAML := test.newValue()
					require.NoError(t, util.DecodeYAMLStrict(asYAML, fromYAML))
					roundTrip, err = json.Marshal(fromYAML)
					require.NoError(t, err)
					require.JSONEq(t, string(encoded), string(roundTrip))
				})
			}
		})
	}
}

// TestToolSetMCPSchemaRejectsInvalidFields exercises schema constraints directly,
// including null presence that pointer decoding could otherwise erase.
func TestToolSetMCPSchemaRejectsInvalidFields(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/ToolSetDefinition")
	patchSchema := compileToolSetSchema(t, "#/$defs/ToolSetPatch")
	for _, test := range []struct {
		name, field string
		path        []string
		value       any
		remove      bool
	}{
		{name: "missing source", field: "source", remove: true},
		{name: "no source variant", field: "source", value: map[string]any{}},
		{name: "null MCP", path: []string{"source"}, field: "mcp", value: nil},
		{name: "both sources", path: []string{"source"}, field: "explicit", value: map[string]any{"tools": []any{}}},
		{name: "null competing source", path: []string{"source"}, field: "explicit", value: nil},
		{name: "null MCP beside explicit", field: "source", value: map[string]any{"explicit": map[string]any{"tools": []any{}}, "mcp": nil}},
		{name: "unknown source variant", path: []string{"source"}, field: "graphql", value: map[string]any{}},
		{name: "missing endpoint", path: []string{"source", "mcp"}, field: "endpoint", remove: true},
		{name: "null endpoint", path: []string{"source", "mcp"}, field: "endpoint", value: nil},
		{name: "blank endpoint", path: []string{"source", "mcp"}, field: "endpoint", value: " \t"},
		{name: "padded endpoint", path: []string{"source", "mcp"}, field: "endpoint", value: " https://example.com/mcp"},
		{name: "multiline endpoint", path: []string{"source", "mcp"}, field: "endpoint", value: "https://example.com/\nmcp"},
		{name: "missing transport", path: []string{"source", "mcp"}, field: "transport", remove: true},
		{name: "null transport", path: []string{"source", "mcp"}, field: "transport", value: nil},
		{name: "stdio transport", path: []string{"source", "mcp"}, field: "transport", value: "stdio"},
		{name: "SSE transport", path: []string{"source", "mcp"}, field: "transport", value: "sse"},
		{name: "source credentials", path: []string{"source", "mcp"}, field: "auth", value: map[string]any{}},
		{name: "null refresh interval", path: []string{"source", "mcp"}, field: "refreshInterval", value: nil},
		{name: "numeric refresh interval", path: []string{"source", "mcp"}, field: "refreshInterval", value: 1500},
		{name: "invalid duration unit", path: []string{"source", "mcp"}, field: "refreshInterval", value: "1d"},
		{name: "duration missing digits", path: []string{"source", "mcp"}, field: "refreshInterval", value: ".s"},
		{name: "null filter", path: []string{"source", "mcp"}, field: "tools", value: nil},
		{name: "unknown filter", path: []string{"source", "mcp", "tools"}, field: "includePatterns", value: []any{".*"}},
		{name: "empty include list", path: []string{"source", "mcp", "tools"}, field: "includeNames", value: []any{}},
		{name: "null include list", path: []string{"source", "mcp", "tools"}, field: "includeNames", value: nil},
		{name: "null include name", path: []string{"source", "mcp", "tools"}, field: "includeNames", value: []any{nil}},
		{name: "blank include name", path: []string{"source", "mcp", "tools"}, field: "includeNames", value: []any{" "}},
		{name: "duplicate include name", path: []string{"source", "mcp", "tools"}, field: "includeNames", value: []any{"read", "read"}},
		{name: "null exclude list", path: []string{"source", "mcp", "tools"}, field: "excludeNames", value: nil},
		{name: "null exclude name", path: []string{"source", "mcp", "tools"}, field: "excludeNames", value: []any{nil}},
		{name: "duplicate exclude name", path: []string{"source", "mcp", "tools"}, field: "excludeNames", value: []any{"read", "read"}},
		{name: "null mappings", field: "permissionMappings", value: nil},
		{name: "null mapping", field: "permissionMappings", value: []any{nil}},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := toolSetMCPDefinitionSchemaFixture(t)
			object := schemaObject(t, definition, test.path...)
			if test.remove {
				delete(object, test.field)
			} else {
				object[test.field] = test.value
			}
			require.Error(t, compiled.Validate(definition))
			patch := toolSetPatchSchemaFixture(t)
			schemaObject(t, patch, "spec")["definition"] = definition
			require.Error(t, patchSchema.Validate(patch), "patch definitions must use the same complete source contract")
		})
	}
}

// TestImportedToolDefaultsSchemaAndDecoding exercises the complete definition
// boundary so null wrappers and maps cannot disappear during either decoding.
// Empty maps omit cleanly on encoding; an explicit empty limits object remains.
func TestImportedToolDefaultsSchemaAndDecoding(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/ToolSetDefinition")
	for _, test := range []struct {
		name, input, want string
		valid             bool
	}{
		{name: "omitted", valid: true},
		{name: "empty", input: `{}`, valid: true},
		{name: "empty maps", input: `{"labels":{},"annotations":{}}`, want: `{}`, valid: true},
		{name: "empty limits", input: `{"limits":{}}`, valid: true},
		{name: "labels", input: `{"labels":{"capability":"records","empty":""}}`, valid: true},
		{name: "annotations", input: `{"annotations":{"note":" literal {{cfg.value}}\n","empty":""}}`, valid: true},
		{name: "limits", input: `{"limits":{"timeoutMillis":1500,"maxRequests":2,"maxOutputBytes":4096}}`, valid: true},
		{name: "null defaults", input: `null`},
		{name: "array defaults", input: `[]`},
		{name: "null labels", input: `{"labels":null}`},
		{name: "array labels", input: `{"labels":[]}`},
		{name: "null label", input: `{"labels":{"capability":null}}`},
		{name: "numeric label", input: `{"labels":{"capability":1}}`},
		{name: "reserved label", input: `{"labels":{"apxy/connection/-/id":"cxn_example"}}`},
		{name: "invalid label key", input: `{"labels":{"invalid key":"value"}}`},
		{name: "invalid label value", input: `{"labels":{"capability":"invalid value"}}`},
		{name: "null annotations", input: `{"annotations":null}`},
		{name: "array annotations", input: `{"annotations":[]}`},
		{name: "null annotation", input: `{"annotations":{"note":null}}`},
		{name: "boolean annotation", input: `{"annotations":{"note":false}}`},
		{name: "invalid annotation key", input: `{"annotations":{"invalid key":"value"}}`},
		{name: "null limits", input: `{"limits":null}`},
		{name: "array limits", input: `{"limits":[]}`},
		{name: "null limit", input: `{"limits":{"timeoutMillis":null}}`},
		{name: "zero limit", input: `{"limits":{"maxRequests":0}}`},
		{name: "negative limit", input: `{"limits":{"maxInputBytes":-1}}`},
		{name: "fractional limit", input: `{"limits":{"maxOutputBytes":1.5}}`},
		{name: "string limit", input: `{"limits":{"maxResponseBytes":"4096"}}`},
		{name: "miscased limit", input: `{"limits":{"MaxRequests":1}}`},
		{name: "unknown limit", input: `{"limits":{"maxTokens":100}}`},
		{name: "miscased field", input: `{"Labels":{}}`},
		{name: "unsupported hints", input: `{"hints":{}}`},
		{name: "unsupported name", input: `{"name":"records"}`},
		{name: "unsupported description", input: `{"description":"Read records"}`},
		{name: "unsupported input schema", input: `{"inputSchema":{}}`},
		{name: "unsupported output schema", input: `{"outputSchema":{}}`},
		{name: "unsupported verbs", input: `{"verbs":["tool:records.read"]}`},
		{name: "unsupported executor", input: `{"javascript":"return 1;"}`},
		{name: "unsupported source key", input: `{"sourceKey":"records"}`},
		{name: "unsupported connection", input: `{"connectionRef":{}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := `{"source":{"mcp":{"endpoint":"https://example.com/mcp","transport":"streamableHttp"}}`
			if test.input != "" {
				input += `,"defaults":` + test.input
			}
			input += "}"
			var document any
			require.NoError(t, json.Unmarshal([]byte(input), &document))
			schemaErr := compiled.Validate(document)
			asYAML, err := yaml.Marshal(common.RawJSON(input))
			require.NoError(t, err)
			for _, format := range []struct {
				name   string
				data   []byte
				decode func([]byte, any) error
			}{
				{"JSON", []byte(input), util.DecodeJSONStrict},
				{"YAML", asYAML, util.DecodeYAMLStrict},
			} {
				t.Run(format.name, func(t *testing.T) {
					var decoded toolsets.ToolSetDefinition
					err := format.decode(format.data, &decoded)
					if err == nil {
						err = decoded.Validate(nil)
					}
					if !test.valid {
						require.Error(t, schemaErr)
						require.Error(t, err)
						return
					}
					require.NoError(t, schemaErr)
					require.NoError(t, err)
					roundTrip, err := json.Marshal(decoded)
					require.NoError(t, err)
					want := input
					if test.want != "" {
						want = `{"source":{"mcp":{"endpoint":"https://example.com/mcp","transport":"streamableHttp"}},"defaults":` + test.want + `}`
					}
					require.JSONEq(t, want, string(roundTrip))
				})
			}
		})
	}
}

// TestImportedToolDefaultsSchemaSourceRestrictions preserves the imported-only
// presence rule for defaults and permission mappings in every resource envelope.
// Even empty settings are invalid on an explicit source.
func TestImportedToolDefaultsSchemaSourceRestrictions(t *testing.T) {
	for _, envelope := range []struct{ name, fragment string }{
		{"definition", "#/$defs/ToolSetDefinition"},
		{"resource", ""},
		{"patch", "#/$defs/ToolSetPatch"},
	} {
		t.Run(envelope.name, func(t *testing.T) {
			compiled := compileToolSetSchema(t, envelope.fragment)
			for _, source := range []struct {
				name  string
				value map[string]any
			}{
				{"explicit", map[string]any{"tools": []any{}}},
				{"mcp", map[string]any{"endpoint": "https://example.com/mcp", "transport": "streamableHttp"}},
				{"openapi", map[string]any{"document": map[string]any{"inline": map[string]any{}}}},
			} {
				for _, settings := range []string{"omitted", "defaults", "mappings", "both"} {
					t.Run(source.name+"/"+settings, func(t *testing.T) {
						definition := map[string]any{"source": map[string]any{source.name: source.value}}
						if settings == "defaults" || settings == "both" {
							definition["defaults"] = map[string]any{}
						}
						if settings == "mappings" || settings == "both" {
							definition["permissionMappings"] = []any{}
						}
						document := definition
						if envelope.name != "definition" {
							document = toolSetSchemaFixture(t)
							schemaObject(t, document, "spec")["definition"] = definition
							if envelope.name == "patch" {
								delete(document, "status")
							}
						}
						err := compiled.Validate(document)
						if source.name == "explicit" && settings != "omitted" {
							require.Error(t, err)
						} else {
							require.NoError(t, err)
						}
					})
				}
			}
		})
	}
}

// TestMCPSourceSchemaDurationAndFilterShapes accepts the fractional, compound,
// and microsecond spellings supported by Go while leaving range checks to Go.
func TestMCPSourceSchemaDurationAndFilterShapes(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/MCPSource")
	for _, interval := range []string{"1.5s", "+5m", ".5s", "1.s", "1m30.5s", "1ns", "1us", "1µs", "1μs", "100ms", "1h"} {
		source := map[string]any{"endpoint": "https://{{ cfg.host }}/mcp", "transport": "streamableHttp", "refreshInterval": interval}
		require.NoError(t, compiled.Validate(source), interval)
	}
	for _, filter := range []map[string]any{
		{}, {"excludeNames": []any{}},
		{"includeNames": []any{" exact "}, "excludeNames": []any{" exact "}},
	} {
		require.NoError(t, compiled.Validate(map[string]any{
			"endpoint": "https://example.com/mcp", "transport": "streamableHttp", "tools": filter,
		}))
	}
}

// TestPermissionMappingSchema verifies matcher alternatives, exact alias shape,
// and strict list/object boundaries without duplicating Go's regexp parser.
func TestPermissionMappingSchema(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/PermissionMapping")
	for _, match := range []map[string]any{
		{"sourceKeys": []any{" exact "}},
		{"sourceKeyPatterns": []any{"read_.*"}},
		{"sourceKeys": []any{}, "sourceKeyPatterns": []any{"read_.*"}},
		{"sourceKeys": []any{"read"}, "sourceKeyPatterns": []any{}},
	} {
		require.NoError(t, compiled.Validate(map[string]any{"match": match, "addVerbs": []any{"custom verb"}}))
	}
	for _, test := range []struct {
		name, field string
		match       bool
		value       any
		remove      bool
	}{
		{name: "missing matcher", field: "match", remove: true},
		{name: "null matcher", field: "match", value: nil},
		{name: "empty matcher", field: "match", value: map[string]any{}},
		{name: "empty matcher lists", field: "match", value: map[string]any{"sourceKeys": []any{}, "sourceKeyPatterns": []any{}}},
		{name: "unknown matcher field", match: true, field: "operationIds", value: []any{"read"}},
		{name: "null keys", match: true, field: "sourceKeys", value: nil},
		{name: "null key entry", match: true, field: "sourceKeys", value: []any{nil}},
		{name: "blank key", match: true, field: "sourceKeys", value: []any{" \t"}},
		{name: "duplicate keys", match: true, field: "sourceKeys", value: []any{"read", "read"}},
		{name: "null patterns", match: true, field: "sourceKeyPatterns", value: nil},
		{name: "null pattern entry", match: true, field: "sourceKeyPatterns", value: []any{nil}},
		{name: "blank pattern", match: true, field: "sourceKeyPatterns", value: []any{" \t"}},
		{name: "duplicate patterns", match: true, field: "sourceKeyPatterns", value: []any{".*", ".*"}},
		{name: "missing verbs", field: "addVerbs", remove: true},
		{name: "null verbs", field: "addVerbs", value: nil},
		{name: "empty verbs", field: "addVerbs", value: []any{}},
		{name: "null verb", field: "addVerbs", value: []any{nil}},
		{name: "blank verb", field: "addVerbs", value: []any{" \t"}},
		{name: "leading verb whitespace", field: "addVerbs", value: []any{" tool:read"}},
		{name: "trailing verb whitespace", field: "addVerbs", value: []any{"tool:read\n"}},
		{name: "duplicate verbs", field: "addVerbs", value: []any{"tool:read", "tool:read"}},
		{name: "unknown mapping field", field: "replaceVerbs", value: []any{"tool:read"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := toolSetMCPDefinitionSchemaFixture(t)
			mapping := definition["permissionMappings"].([]any)[0].(map[string]any)
			object := mapping
			if test.match {
				object = schemaObject(t, mapping, "match")
			}
			if test.remove {
				delete(object, test.field)
			} else {
				object[test.field] = test.value
			}
			require.Error(t, compiled.Validate(mapping))
		})
	}
}

// toolSetOpenAPIDefinitionSchemaFixture reuses import permission mappings while
// keeping the provider's document wholly separate from AuthProxy-owned fields.
func toolSetOpenAPIDefinitionSchemaFixture(t *testing.T, document map[string]any) map[string]any {
	t.Helper()
	definition := toolSetMCPDefinitionSchemaFixture(t)
	definition["source"] = map[string]any{"openapi": map[string]any{
		"document": document,
		"operations": map[string]any{
			"includeOperationIds": []any{"list_records", " exact ID "},
			"excludeOperationIds": []any{},
		},
		"sourceKeyOverrides": []any{
			map[string]any{"method": "get", "path": "/records/{recordId}", "sourceKey": " record key "},
			map[string]any{"method": "QUERY", "path": "/records/search", "sourceKey": "records.query"},
		},
		"unsupportedOperations": "skip",
		"server":                map[string]any{"url": "https://{{ cfg.host }}/v1"},
		"security": map[string]any{"connectionSchemes": []any{
			"providerOAuth", " exact identifier ", "https://schemas.example.com/security.json#/OAuth",
		}},
		"references": map[string]any{
			"baseUri": "https://schemas.example.com/provider/openapi.json",
			"bundle": map[string]any{
				"https://schemas.example.com/provider/record.json": map[string]any{
					"$schema": "provider-dialect", "$id": "urn:provider:record", "$ref": "./other.json",
					"x-provider": map[string]any{"large": json.Number("9007199254740993")},
				},
				"urn:provider:boolean": false,
				"urn:provider:null":    nil,
				"urn:provider:string":  "literal {{cfg.value}}",
				"urn:provider:number":  json.Number("0.12345678901234567890123456789"),
				"file:///schemas/items.json": []any{
					map[string]any{"type": "string"}, true, nil,
				},
			},
			"fetch": []any{
				map[string]any{"url": "https://schemas.example.com/public.json"},
				map[string]any{
					"url": "https://schemas.example.com/private.json",
					"fetchConnectionRef": map[string]any{
						"apiVersion": "authproxy.net/v1alpha1", "kind": "Connection", "id": "cxn_0123456789abcdef",
					},
				},
				map[string]any{
					"url": "https://schemas.example.com/shared.json",
					"fetchConnectionRef": map[string]any{
						"apiVersion": "authproxy.net/v1alpha1", "kind": "Connection", "namespace": "root.docs", "name": "document-reader",
					},
				},
			},
		},
	}}
	return definition
}

// TestToolSetOpenAPISchemaRoundTrip preserves opaque inline documents and both
// fetch reference forms in full resources, definitions, and replacement patches.
// External fetches retain their own credentials regardless of the root source.
// Security scheme mappings remain exact identifiers independent of acquisition.
// Overrides and unsupported-operation policy coexist with operationId filters.
func TestToolSetOpenAPISchemaRoundTrip(t *testing.T) {
	for _, envelope := range []struct {
		name, fragment string
		newValue       func() any
	}{
		{"definition", "#/$defs/ToolSetDefinition", func() any { return &toolsets.ToolSetDefinition{} }},
		{"resource", "", func() any { return &toolsets.ToolSet{} }},
		{"patch", "#/$defs/ToolSetPatch", func() any { return &toolsets.ToolSetPatch{} }},
	} {
		t.Run(envelope.name, func(t *testing.T) {
			compiled := compileToolSetSchema(t, envelope.fragment)
			embedded, err := schema.CompileSchema(toolsets.SchemaIDToolSets + envelope.fragment)
			require.NoError(t, err)
			for _, test := range []struct {
				name     string
				document map[string]any
			}{
				{"opaque inline", map[string]any{"inline": map[string]any{
					"openapi": "future-version", "unknown_provider_field": []any{nil, true, 7.25},
					"components": map[string]any{"schemas": map[string]any{"Example": map[string]any{
						"$schema": "provider-dialect", "$ref": "https://must-not-fetch.invalid/schema.json",
					}}},
				}}},
				{"empty inline", map[string]any{"inline": map[string]any{}}},
				{"public URL", map[string]any{"url": "HTTP://docs.example.com/schema.json?revision=2"}},
				{"fetch by ID", map[string]any{"url": "https://docs.example.com/schema.json", "fetchConnectionRef": map[string]any{
					"apiVersion": "authproxy.net/v1alpha1", "kind": "Connection", "id": "cxn_0123456789abcdef",
				}}},
				{"fetch by namespaced name", map[string]any{"url": "https://docs.example.com/schema.json", "fetchConnectionRef": map[string]any{
					"apiVersion": "authproxy.net/v1alpha1", "kind": "Connection", "namespace": "root.docs", "name": "document-reader",
				}}},
			} {
				t.Run(test.name, func(t *testing.T) {
					definition := toolSetOpenAPIDefinitionSchemaFixture(t, test.document)
					if test.name == "empty inline" {
						definition["permissionMappings"] = []any{}
					}
					document := definition
					if envelope.name != "definition" {
						document = toolSetSchemaFixture(t)
						schemaObject(t, document, "spec")["definition"] = definition
						if envelope.name == "patch" {
							delete(document, "status")
						}
					}
					require.NoError(t, compiled.Validate(document))
					require.NoError(t, embedded.Validate(document))
					encoded, err := json.Marshal(document)
					require.NoError(t, err)
					value := envelope.newValue()
					require.NoError(t, util.DecodeJSONStrict(encoded, value))
					roundTrip, err := json.Marshal(value)
					require.NoError(t, err)
					require.JSONEq(t, string(encoded), string(roundTrip))
					require.Contains(t, string(roundTrip), "9007199254740993")
					require.Contains(t, string(roundTrip), "0.12345678901234567890123456789")
					asYAML, err := yaml.Marshal(value)
					require.NoError(t, err)
					fromYAML := envelope.newValue()
					require.NoError(t, util.DecodeYAMLStrict(asYAML, fromYAML))
					roundTrip, err = json.Marshal(fromYAML)
					require.NoError(t, err)
					require.JSONEq(t, string(encoded), string(roundTrip))
					require.Contains(t, string(roundTrip), "9007199254740993")
					require.Contains(t, string(roundTrip), "0.12345678901234567890123456789")
				})
			}
		})
	}
}

// TestOpenAPISourceSchemaRejectsInvalidFields keeps AuthProxy-owned source
// fields strict while leaving document contents and import compilation separate.
func TestOpenAPISourceSchemaRejectsInvalidFields(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/OpenAPISource")
	patchSchema := compileToolSetSchema(t, "#/$defs/ToolSetPatch")
	for _, test := range []struct {
		name, field string
		path        []string
		value       any
		remove      bool
	}{
		{name: "missing document", field: "document", remove: true},
		{name: "null document", field: "document", value: nil},
		{name: "empty document choice", field: "document", value: map[string]any{}},
		{name: "competing document choices", path: []string{"document"}, field: "inline", value: map[string]any{}},
		{name: "null competing document", path: []string{"document"}, field: "inline", value: nil},
		{name: "null URL", path: []string{"document"}, field: "url", value: nil},
		{name: "relative document URL", path: []string{"document"}, field: "url", value: "schema.json"},
		{name: "file document URL", path: []string{"document"}, field: "url", value: "file:///tmp/schema.json"},
		{name: "missing URL authority", path: []string{"document"}, field: "url", value: "https:///schema.json"},
		{name: "URL port without host", path: []string{"document"}, field: "url", value: "https://:8080/schema.json"},
		{name: "URL control character", path: []string{"document"}, field: "url", value: "https://example.com/schema\tv1.json"},
		{name: "URL credentials", path: []string{"document"}, field: "url", value: "https://reader:secret@example.com/schema.json"},
		{name: "URL fragment", path: []string{"document"}, field: "url", value: "https://example.com/schema.json#"},
		{name: "URL template", path: []string{"document"}, field: "url", value: "https://{{cfg.host}}/schema.json"},
		{name: "URL padding", path: []string{"document"}, field: "url", value: "https://example.com/schema.json "},
		{name: "unsupported base URI", path: []string{"document"}, field: "baseURI", value: "https://example.com/"},
		{name: "null fetch reference", path: []string{"document"}, field: "fetchConnectionRef", value: nil},
		{name: "missing reference kind", path: []string{"document", "fetchConnectionRef"}, field: "kind", remove: true},
		{name: "wrong reference kind", path: []string{"document", "fetchConnectionRef"}, field: "kind", value: "Connector"},
		{name: "wrong reference ID", path: []string{"document", "fetchConnectionRef"}, field: "id", value: "cxr_0123456789abcdef"},
		{name: "null reference ID", path: []string{"document", "fetchConnectionRef"}, field: "id", value: nil},
		{name: "reference without identity", path: []string{"document", "fetchConnectionRef"}, field: "id", remove: true},
		{name: "invalid reference namespace", path: []string{"document", "fetchConnectionRef"}, field: "namespace", value: "root.docs.**"},
		{name: "null reference namespace", path: []string{"document", "fetchConnectionRef"}, field: "namespace", value: nil},
		{name: "reference generation", path: []string{"document", "fetchConnectionRef"}, field: "generation", value: 1},
		{name: "reference zero generation", path: []string{"document", "fetchConnectionRef"}, field: "generation", value: 0},
		{name: "reference null generation", path: []string{"document", "fetchConnectionRef"}, field: "generation", value: nil},
		{name: "unknown reference field", path: []string{"document", "fetchConnectionRef"}, field: "token", value: "secret"},
		{name: "null operation filter", field: "operations", value: nil},
		{name: "empty inclusion", path: []string{"operations"}, field: "includeOperationIds", value: []any{}},
		{name: "null inclusion", path: []string{"operations"}, field: "includeOperationIds", value: nil},
		{name: "null operation ID", path: []string{"operations"}, field: "includeOperationIds", value: []any{nil}},
		{name: "blank operation ID", path: []string{"operations"}, field: "includeOperationIds", value: []any{" \t"}},
		{name: "duplicate operation ID", path: []string{"operations"}, field: "includeOperationIds", value: []any{"read", "read"}},
		{name: "null exclusion", path: []string{"operations"}, field: "excludeOperationIds", value: nil},
		{name: "null excluded ID", path: []string{"operations"}, field: "excludeOperationIds", value: []any{nil}},
		{name: "duplicate excluded ID", path: []string{"operations"}, field: "excludeOperationIds", value: []any{"read", "read"}},
		{name: "unknown filter field", path: []string{"operations"}, field: "tags", value: []any{"records"}},
		{name: "null server", field: "server", value: nil},
		{name: "missing server URL", path: []string{"server"}, field: "url", remove: true},
		{name: "null server URL", path: []string{"server"}, field: "url", value: nil},
		{name: "padded server URL", path: []string{"server"}, field: "url", value: " https://example.com/"},
		{name: "multiline server URL", path: []string{"server"}, field: "url", value: "https://example.com/\nv1"},
		{name: "variables conflict with URL override", path: []string{"server"}, field: "variables", value: map[string]any{}},
		{name: "unknown server field", path: []string{"server"}, field: "name", value: "production"},
		{name: "null references", field: "references", value: nil},
		{name: "object fetch list", path: []string{"references"}, field: "fetch", value: map[string]any{}},
		{name: "unsupported security option", path: []string{"security"}, field: "credentials", value: map[string]any{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := toolSetOpenAPIDefinitionSchemaFixture(t, map[string]any{
				"url": "https://example.com/schema.json", "fetchConnectionRef": map[string]any{
					"apiVersion": "authproxy.net/v1alpha1", "kind": "Connection", "id": "cxn_0123456789abcdef",
				},
			})
			source := schemaObject(t, definition, "source", "openapi")
			object := schemaObject(t, source, test.path...)
			if test.remove {
				delete(object, test.field)
			} else {
				object[test.field] = test.value
			}
			require.Error(t, compiled.Validate(source))
			patch := toolSetPatchSchemaFixture(t)
			schemaObject(t, patch, "spec")["definition"] = definition
			require.Error(t, patchSchema.Validate(patch))
		})
	}
}

// TestOpenAPIOperationImportSettingsSchemaAndDecoding checks exact override
// identities and unsupported-operation policy through both source codecs.
// Document lookup and the consequences of skip remain importer decisions.
func TestOpenAPIOperationImportSettingsSchemaAndDecoding(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/OpenAPISource")
	for _, test := range []struct {
		name, fields string
		valid        bool
	}{
		{"omitted settings", "", true},
		{"empty overrides", `"sourceKeyOverrides":[]`, true},
		{"exact override", `"sourceKeyOverrides":[{"method":"get","path":"/records/{id}","sourceKey":" records.read "}]`, true},
		{"custom method", `"sourceKeyOverrides":[{"method":"m-Search","path":"/","sourceKey":"root"}]`, true},
		{"case-sensitive method identities", `"sourceKeyOverrides":[{"method":"GET","path":"/records","sourceKey":"upper"},{"method":"get","path":"/records","sourceKey":"lower"}]`, true},
		{"uninterpreted path", `"sourceKeyOverrides":[{"method":"GET","path":"/literal path?value=1#section","sourceKey":"literal"}]`, true},
		{"explicit error policy", `"unsupportedOperations":"error"`, true},
		{"explicit skip policy", `"unsupportedOperations":"skip"`, true},
		{"skip with explicit inclusion", `"unsupportedOperations":"skip","operations":{"includeOperationIds":["selected"]}`, true},
		{"null overrides", `"sourceKeyOverrides":null`, false},
		{"object overrides", `"sourceKeyOverrides":{}`, false},
		{"null override", `"sourceKeyOverrides":[null]`, false},
		{"scalar override", `"sourceKeyOverrides":["records"]`, false},
		{"missing method", `"sourceKeyOverrides":[{"path":"/records","sourceKey":"records"}]`, false},
		{"missing path", `"sourceKeyOverrides":[{"method":"GET","sourceKey":"records"}]`, false},
		{"missing source key", `"sourceKeyOverrides":[{"method":"GET","path":"/records"}]`, false},
		{"null method", `"sourceKeyOverrides":[{"method":null,"path":"/records","sourceKey":"records"}]`, false},
		{"null path", `"sourceKeyOverrides":[{"method":"GET","path":null,"sourceKey":"records"}]`, false},
		{"null source key", `"sourceKeyOverrides":[{"method":"GET","path":"/records","sourceKey":null}]`, false},
		{"numeric method", `"sourceKeyOverrides":[{"method":1,"path":"/records","sourceKey":"records"}]`, false},
		{"numeric path", `"sourceKeyOverrides":[{"method":"GET","path":1,"sourceKey":"records"}]`, false},
		{"numeric source key", `"sourceKeyOverrides":[{"method":"GET","path":"/records","sourceKey":1}]`, false},
		{"empty method", `"sourceKeyOverrides":[{"method":"","path":"/records","sourceKey":"records"}]`, false},
		{"padded method", `"sourceKeyOverrides":[{"method":" GET","path":"/records","sourceKey":"records"}]`, false},
		{"invalid method token", `"sourceKeyOverrides":[{"method":"GET/POST","path":"/records","sourceKey":"records"}]`, false},
		{"empty path", `"sourceKeyOverrides":[{"method":"GET","path":"","sourceKey":"records"}]`, false},
		{"path without leading slash", `"sourceKeyOverrides":[{"method":"GET","path":"records","sourceKey":"records"}]`, false},
		{"empty source key", `"sourceKeyOverrides":[{"method":"GET","path":"/records","sourceKey":""}]`, false},
		{"blank source key", `"sourceKeyOverrides":[{"method":"GET","path":"/records","sourceKey":" \t\n"}]`, false},
		{"unknown override field", `"sourceKeyOverrides":[{"method":"GET","path":"/records","sourceKey":"records","operationId":"read"}]`, false},
		{"miscased override field", `"sourceKeyOverrides":[{"Method":"GET","path":"/records","sourceKey":"records"}]`, false},
		{"miscased overrides", `"SourceKeyOverrides":[]`, false},
		{"null policy", `"unsupportedOperations":null`, false},
		{"empty policy", `"unsupportedOperations":""`, false},
		{"miscased policy value", `"unsupportedOperations":"Skip"`, false},
		{"unknown policy", `"unsupportedOperations":"warn"`, false},
		{"object policy", `"unsupportedOperations":{}`, false},
		{"miscased policy field", `"UnsupportedOperations":"skip"`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := `{"document":{"inline":{}}`
			if test.fields != "" {
				source += "," + test.fields
			}
			source += "}"
			var document any
			require.NoError(t, json.Unmarshal([]byte(source), &document))
			schemaErr := compiled.Validate(document)
			asYAML, err := yaml.Marshal(common.RawJSON(source))
			require.NoError(t, err)
			for _, format := range []struct {
				name   string
				data   []byte
				decode func([]byte, any) error
			}{
				{"JSON", []byte(source), util.DecodeJSONStrict},
				{"YAML", asYAML, util.DecodeYAMLStrict},
			} {
				t.Run(format.name, func(t *testing.T) {
					var decoded toolsets.OpenAPISource
					err := format.decode(format.data, &decoded)
					if err == nil {
						err = decoded.Validate(nil)
					}
					if !test.valid {
						require.Error(t, schemaErr)
						require.Error(t, err)
						return
					}
					require.NoError(t, schemaErr)
					require.NoError(t, err)
					roundTrip, err := json.Marshal(decoded)
					require.NoError(t, err)
					want := source
					if test.name == "empty overrides" {
						want = `{"document":{"inline":{}}}`
					}
					require.JSONEq(t, want, string(roundTrip))
				})
			}
		})
	}
}

// TestOpenAPISecurityConfigSchemaAndDecoding keeps scheme mappings exact across
// the source's JSON and YAML boundaries. Empty mappings impose no new shape
// requirements; document and connection compatibility remain importer checks.
func TestOpenAPISecurityConfigSchemaAndDecoding(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/OpenAPISource")
	for _, test := range []struct {
		name, input string
		valid       bool
	}{
		{"omitted configuration", "", true},
		{"empty configuration", `{}`, true},
		{"empty mappings", `{"connectionSchemes":[]}`, true},
		{"one scheme", `{"connectionSchemes":["providerOAuth"]}`, true},
		{"exact identifiers", `{"connectionSchemes":["OAuth","oauth"," OAuth ","scheme name"]}`, true},
		{"URI identifiers", `{"connectionSchemes":["https://schemas.example.com/security.json#/OAuth","./security.json#/ApiKey","urn:example:scheme"]}`, true},
		{"literal pattern characters", `{"connectionSchemes":["^oauth.*$","{{scheme}}"]}`, true},
		{"null configuration", `null`, false},
		{"array configuration", `[]`, false},
		{"scalar configuration", `"oauth"`, false},
		{"null mappings", `{"connectionSchemes":null}`, false},
		{"object mappings", `{"connectionSchemes":{"oauth":true}}`, false},
		{"string mappings", `{"connectionSchemes":"oauth"}`, false},
		{"null mapping", `{"connectionSchemes":[null]}`, false},
		{"numeric mapping", `{"connectionSchemes":[1]}`, false},
		{"empty identifier", `{"connectionSchemes":[""]}`, false},
		{"blank identifier", `{"connectionSchemes":[" \t\n"]}`, false},
		{"duplicate identifier", `{"connectionSchemes":["oauth","oauth"]}`, false},
		{"miscased field", `{"ConnectionSchemes":["oauth"]}`, false},
		{"unknown field", `{"schemes":["oauth"]}`, false},
		{"separate connection", `{"connectionRef":{}}`, false},
		{"credentials", `{"credentials":{"token":"secret"}}`, false},
		{"scopes", `{"scopes":["records:read"]}`, false},
		{"credential placement", `{"placement":"header"}`, false},
		{"auth bypass", `{"disabled":true}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := `{"document":{"inline":{}}}`
			if test.input != "" {
				source = `{"document":{"inline":{}},"security":` + test.input + `}`
			}
			var document any
			require.NoError(t, json.Unmarshal([]byte(source), &document))
			schemaErr := compiled.Validate(document)
			asYAML, err := yaml.Marshal(common.RawJSON(source))
			require.NoError(t, err)
			for _, format := range []struct {
				name   string
				data   []byte
				decode func([]byte, any) error
			}{
				{"JSON", []byte(source), util.DecodeJSONStrict},
				{"YAML", asYAML, util.DecodeYAMLStrict},
			} {
				t.Run(format.name, func(t *testing.T) {
					var decoded toolsets.OpenAPISource
					err := format.decode(format.data, &decoded)
					if err == nil {
						err = decoded.Validate(nil)
					}
					if !test.valid {
						require.Error(t, schemaErr)
						require.Error(t, err)
						return
					}
					require.NoError(t, schemaErr)
					require.NoError(t, err)
					roundTrip, err := json.Marshal(decoded)
					require.NoError(t, err)
					want := source
					if test.name == "empty mappings" {
						want = `{"document":{"inline":{}},"security":{}}`
					}
					require.JSONEq(t, want, string(roundTrip))
				})
			}
		})
	}
}

// TestOpenAPIReferenceConfigSchemaAndDecoding keeps owned reference settings
// strict while accepting arbitrary bundled JSON and explicit external fetches
// through either source codec. Schema compilation and validation remain offline.
func TestOpenAPIReferenceConfigSchemaAndDecoding(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/OpenAPIReferenceConfig")
	tests := []struct {
		name, input string
		valid       bool
	}{
		{"empty configuration", `{}`, true},
		{"empty bundle", `{"bundle":{}}`, true},
		{"HTTP base", `{"baseUri":"https://example.com/spec.json?revision=2"}`, true},
		{"opaque base", `{"baseUri":"urn:example:openapi"}`, true},
		{"file identity", `{"baseUri":"file:///schemas/openapi.json"}`, true},
		{"generic absolute URI", `{"baseUri":"urn:"}`, true},
		{"opaque at sign", `{"baseUri":"urn:example:user@example.com"}`, true},
		{"escaped fragment character", `{"baseUri":"https://example.com/schema%23name.json"}`, true},
		{"arbitrary bundled values", `{"bundle":{"urn:test:object":{"$schema":"unknown","$id":"relative","$ref":"https://never-fetch.invalid/schema","unknown_field":null},"urn:test:false":false,"urn:test:true":true,"urn:test:null":null,"urn:test:array":[1,null,"literal"],"urn:test:number":2.5,"urn:test:string":"{{cfg.literal}}"}}`, true},
		{"empty fetch list", `{"fetch":[]}`, true},
		{"public external fetch", `{"fetch":[{"url":"HTTP://schemas.example.com/shared.json?revision=2"}]}`, true},
		{"external fetch by ID", `{"fetch":[{"url":"https://schemas.example.com/shared.json","fetchConnectionRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","id":"cxn_0123456789abcdef"}}]}`, true},
		{"external fetch by namespaced name", `{"fetch":[{"url":"https://schemas.example.com/shared.json","fetchConnectionRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","namespace":"root.docs","name":"document-reader"}}]}`, true},
		{"bundle and external fetch", `{"bundle":{"urn:example:part":false},"fetch":[{"url":"https://schemas.example.com/shared.json"}]}`, true},
		{"null configuration", `null`, false},
		{"array configuration", `[]`, false},
		{"null base", `{"baseUri":null}`, false},
		{"numeric base", `{"baseUri":1}`, false},
		{"null bundle", `{"bundle":null}`, false},
		{"array bundle", `{"bundle":[]}`, false},
		{"string bundle", `{"bundle":"{}"}`, false},
		{"miscased base", `{"baseURI":"urn:example:root"}`, false},
		{"unknown field", `{"unknown":true}`, false},
		{"object fetch list", `{"fetch":{"urn:example:part":{"url":"https://example.com/part.json"}}}`, false},
		{"null fetch list", `{"fetch":null}`, false},
		{"null fetch entry", `{"fetch":[null]}`, false},
		{"string fetch entry", `{"fetch":["https://example.com/part.json"]}`, false},
		{"missing fetch URL", `{"fetch":[{}]}`, false},
		{"null fetch URL", `{"fetch":[{"url":null}]}`, false},
		{"numeric fetch URL", `{"fetch":[{"url":42}]}`, false},
		{"relative fetch URL", `{"fetch":[{"url":"./part.json"}]}`, false},
		{"file fetch URL", `{"fetch":[{"url":"file:///schemas/part.json"}]}`, false},
		{"opaque fetch URL", `{"fetch":[{"url":"urn:example:part"}]}`, false},
		{"fetch URL fragment", `{"fetch":[{"url":"https://example.com/part.json#/properties/id"}]}`, false},
		{"fetch URL credentials", `{"fetch":[{"url":"https://reader:secret@example.com/part.json"}]}`, false},
		{"fetch URL template", `{"fetch":[{"url":"https://{{cfg.host}}/part.json"}]}`, false},
		{"fetch URL padding", `{"fetch":[{"url":" https://example.com/part.json"}]}`, false},
		{"miscased fetch field", `{"Fetch":[]}`, false},
		{"miscased fetch URL", `{"fetch":[{"URL":"https://example.com/part.json"}]}`, false},
		{"unknown fetch option", `{"fetch":[{"url":"https://example.com/part.json","followRedirects":true}]}`, false},
		{"inline fetch entry", `{"fetch":[{"url":"https://example.com/part.json","inline":{}}]}`, false},
		{"null external fetch reference", `{"fetch":[{"url":"https://example.com/part.json","fetchConnectionRef":null}]}`, false},
		{"missing external fetch identity", `{"fetch":[{"url":"https://example.com/part.json","fetchConnectionRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection"}}]}`, false},
		{"wrong external fetch kind", `{"fetch":[{"url":"https://example.com/part.json","fetchConnectionRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","id":"cxn_0123456789abcdef"}}]}`, false},
		{"external fetch generation", `{"fetch":[{"url":"https://example.com/part.json","fetchConnectionRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","id":"cxn_0123456789abcdef","generation":0}}]}`, false},
		{"external fetch wildcard namespace", `{"fetch":[{"url":"https://example.com/part.json","fetchConnectionRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","namespace":"root.docs.**","name":"document-reader"}}]}`, false},
	}
	for _, uri := range []struct {
		name, value string
	}{
		{"empty", ""},
		{"relative", "./schema.json"},
		{"fragment", "urn:example:schema#part"},
		{"empty fragment", "https://example.com/schema.json#"},
		{"userinfo", "https://reader:secret@example.com/schema.json"},
		{"file userinfo", "file://reader@localhost/schema.json"},
		{"space", "urn:example: schema"},
		{"newline", "urn:example:schema\n"},
		{"control", "urn:example:schema\u007f"},
		{"Unicode whitespace", "urn:example:schema\u2003"},
		{"template", "urn:example:{{cfg.schema}}"},
	} {
		for _, location := range []string{"baseUri", "bundle"} {
			value := map[string]any{"baseUri": uri.value}
			if location == "bundle" {
				value = map[string]any{"bundle": map[string]any{uri.value: false}}
			}
			encoded, err := json.Marshal(value)
			require.NoError(t, err)
			tests = append(tests, struct {
				name, input string
				valid       bool
			}{location + " " + uri.name, string(encoded), false})
		}
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var document any
			require.NoError(t, json.Unmarshal([]byte(test.input), &document))
			schemaErr := compiled.Validate(document)
			// The enclosing source observes null reference wrappers even where
			// yaml.v3 would skip a root value's custom UnmarshalYAML method.
			source := []byte(`{"document":{"inline":{}},"references":` + test.input + `}`)
			asYAML, err := yaml.Marshal(common.RawJSON(source))
			require.NoError(t, err)
			for _, format := range []struct {
				name   string
				data   []byte
				decode func([]byte, any) error
			}{
				{"JSON", source, util.DecodeJSONStrict},
				{"YAML", asYAML, util.DecodeYAMLStrict},
			} {
				t.Run(format.name, func(t *testing.T) {
					var decoded toolsets.OpenAPISource
					err := format.decode(format.data, &decoded)
					if err == nil {
						err = decoded.Validate(nil)
					}
					if !test.valid {
						require.Error(t, schemaErr)
						require.Error(t, err)
						return
					}
					require.NoError(t, schemaErr)
					require.NoError(t, err)
					roundTrip, err := json.Marshal(decoded)
					require.NoError(t, err)
					var output map[string]any
					require.NoError(t, json.Unmarshal(roundTrip, &output))
					require.NoError(t, compiled.Validate(output["references"]))
				})
			}
		})
	}
}

// TestOpenAPIReferenceURISchemaGuideBoundary leaves complete URI parsing to Go
// instead of duplicating net/url's percent-encoding rules in a schema regex.
func TestOpenAPIReferenceURISchemaGuideBoundary(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/OpenAPIReferenceConfig")
	require.NoError(t, compiled.Validate(map[string]any{"baseUri": "https://example.com/%invalid"}))
	var source toolsets.OpenAPISource
	require.NoError(t, util.DecodeJSONStrict([]byte(`{"document":{"inline":{}},"references":{"baseUri":"https://example.com/%invalid"}}`), &source))
	require.Error(t, source.Validate(nil), "Go URI parsing remains authoritative")
}

// TestOpenAPIServerConfigSchemaAndDecoding compares the authoring schema with
// both Go codecs, including source choice, literal bindings, and explicit nulls.
func TestOpenAPIServerConfigSchemaAndDecoding(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/OpenAPIServerConfig")
	for _, test := range []struct {
		name, input string
		valid       bool
	}{
		{"URL override", `{"url":"https://{{cfg.host}}/v1"}`, true},
		{"whole URL template", `{"url":"{{ cfg.baseURL }}"}`, true},
		{"zero index", `{"index":0}`, true},
		{"document bounds deferred", `{"index":1000000}`, true},
		{"empty bindings", `{"index":1,"variables":{}}`, true},
		{"exact literal bindings", `{"index":0,"variables":{" region ":" eu ","empty":"","spaces":" \t\n","single-braces":"{literal}"}}`, true},
		{"template-looking literals", `{"index":0,"variables":{"region":"{{cfg.region}}","opening":"literal{{value","closing":"value}}literal"}}`, true},
		{"null object", `null`, false},
		{"missing choice", `{}`, false},
		{"competing choices", `{"url":"https://example.com","index":0}`, false},
		{"empty URL", `{"url":""}`, false},
		{"null URL", `{"url":null}`, false},
		{"negative index", `{"index":-1}`, false},
		{"fractional index", `{"index":0.5}`, false},
		{"fractional exponent index", `{"index":15e-1}`, false},
		{"string index", `{"index":"0"}`, false},
		{"null index", `{"index":null}`, false},
		{"bindings without choice", `{"variables":{}}`, false},
		{"bindings with URL", `{"url":"https://example.com","variables":{}}`, false},
		{"null bindings", `{"index":0,"variables":null}`, false},
		{"array bindings", `{"index":0,"variables":[]}`, false},
		{"null binding", `{"index":0,"variables":{"region":null}}`, false},
		{"numeric binding", `{"index":0,"variables":{"region":1}}`, false},
		{"empty name", `{"index":0,"variables":{"":"eu"}}`, false},
		{"blank name", `{"index":0,"variables":{" \t\n":"eu"}}`, false},
		{"Unicode blank name", `{"index":0,"variables":{"\u0085\u00a0\u2003":"eu"}}`, false},
		{"vertical tab name", `{"index":0,"variables":{"\u000b":"eu"}}`, false},
		{"miscased index", `{"Index":0}`, false},
		{"unknown option", `{"index":0,"unknown":true}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var document any
			require.NoError(t, json.Unmarshal([]byte(test.input), &document))
			schemaErr := compiled.Validate(document)
			asYAML, err := yaml.Marshal(common.RawJSON(test.input))
			require.NoError(t, err)
			for _, format := range []struct {
				name   string
				decode func(any) error
			}{
				{"JSON", func(value any) error { return util.DecodeJSONStrict([]byte(test.input), value) }},
				{"YAML", func(value any) error { return util.DecodeYAMLStrict(asYAML, value) }},
			} {
				t.Run(format.name, func(t *testing.T) {
					var value toolsets.OpenAPIServerConfig
					err := format.decode(&value)
					if err == nil {
						err = value.Validate(nil)
					}
					if !test.valid {
						require.Error(t, schemaErr)
						require.Error(t, err)
						return
					}
					require.NoError(t, schemaErr)
					require.NoError(t, err)
					roundTrip, err := json.Marshal(value)
					require.NoError(t, err)
					require.JSONEq(t, test.input, string(roundTrip))
				})
			}
		})
	}
}

// TestOpenAPIServerConfigSchemaIntegerSpellingBoundary records the intentional
// difference between JSON Schema's integer-valued numbers and standard Go int
// decoding. YAML preserves these numeric spellings through the JSON boundary.
func TestOpenAPIServerConfigSchemaIntegerSpellingBoundary(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/OpenAPIServerConfig")
	for _, token := range []string{"1.0", "1e0"} {
		t.Run(token, func(t *testing.T) {
			input := []byte(`{"index":` + token + `}`)
			var document any
			require.NoError(t, json.Unmarshal(input, &document))
			require.NoError(t, compiled.Validate(document), "the schema checks numeric value, not spelling")
			var fromJSON, fromYAML toolsets.OpenAPIServerConfig
			require.Error(t, util.DecodeJSONStrict(input, &fromJSON), "standard int decoding requires integer syntax")
			require.Error(t, util.DecodeYAMLStrict([]byte("index: "+token), &fromYAML), "YAML uses the same JSON integer boundary")
		})
	}
}

// TestToolSetOpenAPIServerSelectionSchemaRoundTrip preserves indexed selection
// in full definitions, resources, and replacement patches. Document bounds and
// variable declarations remain importer checks, even for an empty document.
func TestToolSetOpenAPIServerSelectionSchemaRoundTrip(t *testing.T) {
	for _, envelope := range []struct {
		name, fragment string
		newValue       func() any
	}{
		{"definition", "#/$defs/ToolSetDefinition", func() any { return &toolsets.ToolSetDefinition{} }},
		{"resource", "", func() any { return &toolsets.ToolSet{} }},
		{"patch", "#/$defs/ToolSetPatch", func() any { return &toolsets.ToolSetPatch{} }},
	} {
		t.Run(envelope.name, func(t *testing.T) {
			compiled := compileToolSetSchema(t, envelope.fragment)
			for _, server := range []map[string]any{
				{"index": 0},
				{"index": 1, "variables": map[string]any{}},
				{"index": 2, "variables": map[string]any{" region ": " eu ", "suffix": "", "literal": "{{cfg.host}}"}},
			} {
				definition := toolSetOpenAPIDefinitionSchemaFixture(t, map[string]any{"inline": map[string]any{}})
				schemaObject(t, definition, "source", "openapi")["server"] = server
				document := definition
				if envelope.name != "definition" {
					document = toolSetSchemaFixture(t)
					delete(document, "status")
					metadata := schemaObject(t, document, "metadata")
					delete(metadata, "id")
					delete(metadata, "generation")
					schemaObject(t, document, "spec")["definition"] = definition
				}
				require.NoError(t, compiled.Validate(document))
				encoded, err := json.Marshal(document)
				require.NoError(t, err)
				value := envelope.newValue()
				require.NoError(t, util.DecodeJSONStrict(encoded, value))
				asYAML, err := yaml.Marshal(value)
				require.NoError(t, err)
				fromYAML := envelope.newValue()
				require.NoError(t, util.DecodeYAMLStrict(asYAML, fromYAML))
				for _, decoded := range []any{value, fromYAML} {
					switch decoded := decoded.(type) {
					case *toolsets.ToolSetDefinition:
						require.NoError(t, decoded.Validate(nil))
					case *toolsets.ToolSet:
						require.NoError(t, decoded.Validate(nil))
					case *toolsets.ToolSetPatch:
						require.NoError(t, decoded.ValidateFor(meta.ValidationModeUpdate, nil))
					}
					roundTrip, err := json.Marshal(decoded)
					require.NoError(t, err)
					require.JSONEq(t, string(encoded), string(roundTrip))
				}
			}
		})
	}
}

// TestOpenAPIDocumentSchemaInlineBoundary accepts any object while rejecting
// text and scalar encodings, ambiguous document choices, and unusable fetch refs.
func TestOpenAPIDocumentSchemaInlineBoundary(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/OpenAPIDocument")
	for _, value := range []any{nil, true, 3, "openapi: 3.1.0", []any{}} {
		require.Error(t, compiled.Validate(map[string]any{"inline": value}))
	}
	for _, extra := range []map[string]any{
		{"url": nil},
		{"url": "https://example.com/schema.json"},
		{"fetchConnectionRef": map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": "Connection", "id": "cxn_0123456789abcdef"}},
	} {
		extra["inline"] = map[string]any{}
		require.Error(t, compiled.Validate(extra))
	}
	sourceSchema := compileToolSetSchema(t, "#/$defs/OpenAPISource")
	for _, operations := range []map[string]any{
		{}, {"excludeOperationIds": []any{}},
		{"includeOperationIds": []any{" exact ID "}, "excludeOperationIds": []any{" exact ID "}},
	} {
		require.NoError(t, sourceSchema.Validate(map[string]any{
			"document": map[string]any{"inline": map[string]any{}}, "operations": operations,
		}))
	}
	require.NoError(t, sourceSchema.Validate(map[string]any{"document": map[string]any{"inline": map[string]any{}}}),
		"operation filters and server overrides are optional")
}

// TestToolSetSourceSchemaExactlyOneVariant rejects competing sources even when
// the extra field is explicitly null, independently of permission mapping rules.
func TestToolSetSourceSchemaExactlyOneVariant(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/ToolSetSource")
	variants := map[string]any{
		"explicit": map[string]any{"tools": []any{}},
		"mcp":      map[string]any{"endpoint": "https://example.com/mcp", "transport": "streamableHttp"},
		"openapi":  map[string]any{"document": map[string]any{"inline": map[string]any{}}},
	}
	for name, value := range variants {
		require.NoError(t, compiled.Validate(map[string]any{name: value}), name)
		for other, otherValue := range variants {
			if name != other {
				require.Error(t, compiled.Validate(map[string]any{name: value, other: otherValue}), "%s with %s", name, other)
				require.Error(t, compiled.Validate(map[string]any{name: value, other: nil}), "%s with null %s", name, other)
			}
		}
	}
}
