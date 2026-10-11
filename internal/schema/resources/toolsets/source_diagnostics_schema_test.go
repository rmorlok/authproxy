package toolsets_test

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestSourceDiagnosticSchemaAndValidation checks the shared diagnostic shape
// without assuming that an observed-status codec enforces authoring strictness.
func TestSourceDiagnosticSchemaAndValidation(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/SourceDiagnostic")
	for _, test := range []struct {
		name, input string
		valid       bool
	}{
		{"document-wide warning", `{"severity":"warning","code":"future-code","message":"Import continued."}`, true},
		{"exact source identity", `{"severity":"error","code":"provider.extension/new","message":"Unsupported operation.","sourceKey":" exact key "}`, true},
		{"document root", `{"severity":"warning","code":"root","message":"Root finding.","location":{"jsonPointer":""}}`, true},
		{"escaped pointer", `{"severity":"error","code":"schema","message":"Invalid schema.","location":{"jsonPointer":"/paths/~1records/get/a~0b/~01"}}`, true},
		{"document identity", `{"severity":"error","code":"parse","message":"Could not parse document.","location":{"documentUri":"urn:provider:schema"}}`, true},
		{"file identity and pointer", `{"severity":"warning","code":"schema","message":"Schema finding.","location":{"documentUri":"file:///schemas/shared.json","jsonPointer":"/properties/id"}}`, true},
		{"missing severity", `{"code":"schema","message":"Schema finding."}`, false},
		{"unsupported severity", `{"severity":"info","code":"schema","message":"Schema finding."}`, false},
		{"missing code", `{"severity":"error","message":"Schema finding."}`, false},
		{"blank code", `{"severity":"error","code":" \t","message":"Schema finding."}`, false},
		{"missing message", `{"severity":"error","code":"schema"}`, false},
		{"blank message", `{"severity":"error","code":"schema","message":" \n"}`, false},
		{"blank source key", `{"severity":"error","code":"schema","message":"Schema finding.","sourceKey":" \t"}`, false},
		{"empty location", `{"severity":"error","code":"schema","message":"Schema finding.","location":{}}`, false},
		{"empty document only", `{"severity":"error","code":"schema","message":"Schema finding.","location":{"documentUri":""}}`, false},
		{"relative document", `{"severity":"error","code":"schema","message":"Schema finding.","location":{"documentUri":"shared.json"}}`, false},
		{"document fragment", `{"severity":"error","code":"schema","message":"Schema finding.","location":{"documentUri":"urn:provider:schema#section"}}`, false},
		{"document credentials", `{"severity":"error","code":"schema","message":"Schema finding.","location":{"documentUri":"https://user:secret@example.com/schema"}}`, false},
		{"document template", `{"severity":"error","code":"schema","message":"Schema finding.","location":{"documentUri":"https://{{cfg.host}}/schema"}}`, false},
		{"document whitespace", `{"severity":"error","code":"schema","message":"Schema finding.","location":{"documentUri":"urn:provider: schema"}}`, false},
		{"pointer without slash", `{"severity":"error","code":"schema","message":"Schema finding.","location":{"jsonPointer":"paths"}}`, false},
		{"fragment pointer", `{"severity":"error","code":"schema","message":"Schema finding.","location":{"jsonPointer":"#/paths"}}`, false},
		{"unescaped tilde", `{"severity":"error","code":"schema","message":"Schema finding.","location":{"jsonPointer":"/paths/~"}}`, false},
		{"unknown tilde escape", `{"severity":"error","code":"schema","message":"Schema finding.","location":{"jsonPointer":"/paths/~2"}}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var document any
			require.NoError(t, json.Unmarshal([]byte(test.input), &document))
			var diagnostic toolsets.SourceDiagnostic
			require.NoError(t, json.Unmarshal([]byte(test.input), &diagnostic))
			if test.valid {
				require.NoError(t, compiled.Validate(document))
				require.NoError(t, diagnostic.Validate(nil))
			} else {
				require.Error(t, compiled.Validate(document))
				require.Error(t, diagnostic.Validate(nil))
			}
		})
	}
}

// TestSourceDiagnosticSchemaWireShapes documents the canonical output shape.
// Observed status intentionally uses ordinary Go codecs, so these schema-only
// restrictions do not require custom null, unknown-field, or casing decoders.
func TestSourceDiagnosticSchemaWireShapes(t *testing.T) {
	compiled := compileToolSetSchema(t, "#/$defs/SourceDiagnostic")
	for _, test := range []struct {
		name, field string
		value       any
	}{
		{"null severity", "severity", nil},
		{"null code", "code", nil},
		{"null message", "message", nil},
		{"null source key", "sourceKey", nil},
		{"null location", "location", nil},
		{"numeric source key", "sourceKey", 1},
		{"unknown field", "retry", true},
		{"miscased field", "SourceKey", "records"},
		{"unknown location field", "location", map[string]any{"jsonPointer": "", "line": 1}},
		{"miscased location field", "location", map[string]any{"JSONPointer": ""}},
		{"null document URI", "location", map[string]any{"documentUri": nil, "jsonPointer": ""}},
		{"null pointer", "location", map[string]any{"jsonPointer": nil}},
		{"numeric pointer", "location", map[string]any{"jsonPointer": 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := map[string]any{"severity": "warning", "code": "future", "message": "Import continued."}
			document[test.field] = test.value
			require.Error(t, compiled.Validate(document))
		})
	}
}

// sourceDiagnosticResourceFixture supplies a stored resource with a valid ID
// and the requested minimal source, preserving the shared release-state fixture.
func sourceDiagnosticResourceFixture(t *testing.T, source string) map[string]any {
	t.Helper()
	document := toolSetSchemaFixture(t)
	schemaObject(t, document, "metadata")["id"] = "tls_01example0000001"
	var sourceValue map[string]any
	switch source {
	case "explicit":
		sourceValue = map[string]any{"tools": []any{}}
	case "mcp":
		sourceValue = map[string]any{"endpoint": "https://example.com/mcp", "transport": "streamableHttp"}
	case "openapi":
		sourceValue = map[string]any{"document": map[string]any{"inline": map[string]any{}}}
	default:
		t.Fatalf("unsupported fixture source %q", source)
	}
	schemaObject(t, document, "spec")["definition"] = map[string]any{"source": map[string]any{source: sourceValue}}
	return document
}

// TestToolSetImportDiagnosticsSchemaRoundTrip preserves report order, exact
// source identities, generic document URIs, and a root pointer across JSON/YAML.
func TestToolSetImportDiagnosticsSchemaRoundTrip(t *testing.T) {
	compiled := compileToolSetSchema(t, "")
	embedded, err := schema.CompileSchema(toolsets.SchemaIDToolSets)
	require.NoError(t, err)
	document := sourceDiagnosticResourceFixture(t, "openapi")
	schemaObject(t, document, "status")["importDiagnostics"] = []any{
		map[string]any{"severity": "warning", "code": "future-warning", "message": "Import continued.", "location": map[string]any{"jsonPointer": ""}},
		map[string]any{"severity": "error", "code": "operation.unsupported", "message": "Unsupported operation.", "sourceKey": " exact key ", "location": map[string]any{"documentUri": "urn:provider:shared", "jsonPointer": "/paths/~1records/get"}},
	}
	require.NoError(t, compiled.Validate(document))
	require.NoError(t, embedded.Validate(document))
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	var resource toolsets.ToolSet
	require.NoError(t, json.Unmarshal(encoded, &resource))
	require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))
	asYAML, err := yaml.Marshal(resource)
	require.NoError(t, err)
	var fromYAML toolsets.ToolSet
	require.NoError(t, yaml.Unmarshal(asYAML, &fromYAML))
	require.NoError(t, fromYAML.ValidateFor(meta.ValidationModeResponse, nil))
	for _, decoded := range []*toolsets.ToolSet{&resource, &fromYAML} {
		roundTrip, err := json.Marshal(decoded)
		require.NoError(t, err)
		require.JSONEq(t, string(encoded), string(roundTrip))
	}
	for _, mode := range []meta.ValidationMode{meta.ValidationModeCreate, meta.ValidationModeUpdate, meta.ValidationModeConfig} {
		require.ErrorContains(t, resource.ValidateFor(mode, nil), "status")
	}
	require.Error(t, compileToolSetSchema(t, "#/$defs/ToolSetPatch").Validate(document), "patches cannot author observed reports")
	patch := toolsets.NewToolSetPatch()
	patch.Status = resource.Status
	require.ErrorContains(t, patch.ValidateFor(meta.ValidationModeUpdate, nil), "status")
}

// TestToolSetImportDiagnosticsSchemaSourceRestriction allows an empty report on
// every source but confines nonempty generation reports to OpenAPI imports.
func TestToolSetImportDiagnosticsSchemaSourceRestriction(t *testing.T) {
	compiled := compileToolSetSchema(t, "")
	for _, source := range []string{"explicit", "mcp", "openapi"} {
		for _, shape := range []string{"omitted", "empty", "nonempty"} {
			t.Run(source+"/"+shape, func(t *testing.T) {
				document := sourceDiagnosticResourceFixture(t, source)
				status := schemaObject(t, document, "status")
				if shape == "empty" {
					status["importDiagnostics"] = []any{}
				} else if shape == "nonempty" {
					status["importDiagnostics"] = []any{map[string]any{"severity": "warning", "code": "future", "message": "Import continued."}}
				}
				encoded, err := json.Marshal(document)
				require.NoError(t, err)
				var resource toolsets.ToolSet
				require.NoError(t, json.Unmarshal(encoded, &resource))
				if shape == "nonempty" && source != "openapi" {
					require.Error(t, compiled.Validate(document))
					require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeResponse, nil), "status.importDiagnostics")
				} else {
					require.NoError(t, compiled.Validate(document))
					require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))
				}
			})
		}
	}
}
