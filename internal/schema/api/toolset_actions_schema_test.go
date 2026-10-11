package api

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// toolSetActionSchemaFixture builds the wire envelope independently of the Go
// constructors, including a deliberately opaque task token in responses.
func toolSetActionSchemaFixture(migration, response, named bool) map[string]any {
	connection := map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": "Connection", "id": "cxn_01example0000001"}
	toolSet := map[string]any{"apiVersion": "authproxy.net/v1alpha1", "kind": "ToolSet", "id": "tls_01example0000001"}
	if named {
		delete(connection, "id")
		connection["namespace"], connection["name"] = "root.product.user-1", "production"
		delete(toolSet, "id")
		toolSet["namespace"], toolSet["name"] = "root.product", "catalog-tools"
	}
	kind := "ConnectionToolSetRefresh"
	if migration {
		kind = "ConnectionToolSetGenerationMigration"
		toolSet["generation"] = 4
	}
	document := map[string]any{
		"apiVersion": "authproxy.net/v1alpha1",
		"kind":       kind,
		"metadata":   map[string]any{"target": connection},
		"spec":       map[string]any{"toolSetRef": toolSet},
	}
	if named {
		document["spec"].(map[string]any)["timeoutSeconds"] = 30
	}
	if response {
		document["status"] = map[string]any{"taskId": "opaque-task/accepted:7"}
	}
	return document
}

// toolSetSchemaAction is the common validation boundary used to exercise both
// action types through their ordinary JSON and YAML decoding paths.
type toolSetSchemaAction interface {
	ValidateRequest(meta.Kind) error
	ValidateResponse(meta.Kind) error
}

// newToolSetSchemaAction returns an empty action of the requested concrete type.
func newToolSetSchemaAction(migration bool) toolSetSchemaAction {
	if migration {
		return &ConnectionToolSetGenerationMigrationAction{}
	}
	return &ConnectionToolSetRefreshAction{}
}

// TestConnectionToolSetActionsSchemaRoundTrip checks both selector forms,
// optional timeouts, and the response's retained spec and task tracking token.
func TestConnectionToolSetActionsSchemaRoundTrip(t *testing.T) {
	for _, migration := range []bool{false, true} {
		for _, response := range []bool{false, true} {
			for _, named := range []bool{false, true} {
				document := toolSetActionSchemaFixture(migration, response, named)
				kind := document["kind"].(string)
				direction := "Request"
				if response {
					direction = "Response"
				}
				selector := "id"
				if named {
					selector = "namespace/name"
				}
				t.Run(kind+direction+"/"+selector, func(t *testing.T) {
					compiled := compileRefSchema(t, "./schema.json#/$defs/"+kind+"Action"+direction)
					require.NoError(t, compiled.Validate(document))
					encoded, err := json.Marshal(document)
					require.NoError(t, err)
					fromJSON := newToolSetSchemaAction(migration)
					require.NoError(t, json.Unmarshal(encoded, fromJSON))
					asYAML, err := yaml.Marshal(fromJSON)
					require.NoError(t, err)
					fromYAML := newToolSetSchemaAction(migration)
					require.NoError(t, yaml.Unmarshal(asYAML, fromYAML))
					for _, decoded := range []toolSetSchemaAction{fromJSON, fromYAML} {
						if response {
							require.NoError(t, decoded.ValidateResponse(meta.Kind(kind)))
						} else {
							require.NoError(t, decoded.ValidateRequest(meta.Kind(kind)))
						}
						roundTrip, err := json.Marshal(decoded)
						require.NoError(t, err)
						require.JSONEq(t, string(encoded), string(roundTrip))
						require.NoError(t, compiled.Validate(toolResultSchemaDocument(t, string(roundTrip))))
					}
				})
			}
		}
	}
}

// TestConnectionToolSetActionsSchemaBoundaries checks selectors and spec fields
// on both envelopes. Null restrictions describe the recommended wire shape;
// they do not introduce custom decoding for optional Go pointers or integers.
func TestConnectionToolSetActionsSchemaBoundaries(t *testing.T) {
	for _, migration := range []bool{false, true} {
		for _, response := range []bool{false, true} {
			fixture := toolSetActionSchemaFixture(migration, response, false)
			definition := fixture["kind"].(string) + "ActionRequest"
			if response {
				definition = fixture["kind"].(string) + "ActionResponse"
			}
			t.Run(definition, func(t *testing.T) {
				compiled := compileRefSchema(t, "./schema.json#/$defs/"+definition)
				for _, test := range []struct {
					name, field string
					path        []string
					value       any
					remove      bool
				}{
					{name: "wrong kind", field: "kind", value: "ConnectionGenerationMigration"},
					{name: "missing target", path: []string{"metadata"}, field: "target", remove: true},
					{name: "target without selector", path: []string{"metadata", "target"}, field: "id", remove: true},
					{name: "wrong target type", path: []string{"metadata", "target"}, field: "kind", value: "Connector"},
					{name: "wrong target prefix", path: []string{"metadata", "target"}, field: "id", value: "tls_01example0000001"},
					{name: "target generation", path: []string{"metadata", "target"}, field: "generation", value: 2},
					{name: "missing spec", field: "spec", remove: true},
					{name: "missing toolset", path: []string{"spec"}, field: "toolSetRef", remove: true},
					{name: "null toolset", path: []string{"spec"}, field: "toolSetRef", value: nil},
					{name: "toolset without selector", path: []string{"spec", "toolSetRef"}, field: "id", remove: true},
					{name: "wrong toolset type", path: []string{"spec", "toolSetRef"}, field: "kind", value: "Tool"},
					{name: "wrong toolset prefix", path: []string{"spec", "toolSetRef"}, field: "id", value: "tol_01example0000001"},
					{name: "missing toolset version", path: []string{"spec", "toolSetRef"}, field: "apiVersion", remove: true},
					{name: "wrong toolset version", path: []string{"spec", "toolSetRef"}, field: "apiVersion", value: "authproxy.net/v2"},
					{name: "zero generation", path: []string{"spec", "toolSetRef"}, field: "generation", value: 0},
					{name: "negative generation", path: []string{"spec", "toolSetRef"}, field: "generation", value: -1},
					{name: "null generation", path: []string{"spec", "toolSetRef"}, field: "generation", value: nil},
					{name: "zero timeout", path: []string{"spec"}, field: "timeoutSeconds", value: 0},
					{name: "negative timeout", path: []string{"spec"}, field: "timeoutSeconds", value: -1},
					{name: "fractional timeout", path: []string{"spec"}, field: "timeoutSeconds", value: 1.5},
					{name: "null timeout", path: []string{"spec"}, field: "timeoutSeconds", value: nil},
					{name: "unknown spec field", path: []string{"spec"}, field: "connectionId", value: "cxn_01example0000001"},
					{name: "unknown envelope field", field: "taskId", value: "opaque-task"},
				} {
					t.Run(test.name, func(t *testing.T) {
						document := toolSetActionSchemaFixture(migration, response, false)
						object := document
						for _, field := range test.path {
							object = object[field].(map[string]any)
						}
						if test.remove {
							delete(object, test.field)
						} else {
							object[test.field] = test.value
						}
						require.Error(t, compiled.Validate(document))
					})
				}
				t.Run("action generation rule", func(t *testing.T) {
					document := toolSetActionSchemaFixture(migration, response, false)
					reference := document["spec"].(map[string]any)["toolSetRef"].(map[string]any)
					if migration {
						delete(reference, "generation")
					} else {
						reference["generation"] = 4
					}
					require.Error(t, compiled.Validate(document))
				})
				for _, status := range []any{nil, map[string]any{}, map[string]any{"taskId": ""}, map[string]any{"taskId": " \t"}, map[string]any{"taskId": nil}, map[string]any{"taskId": "opaque", "appliedGeneration": 4}} {
					document := toolSetActionSchemaFixture(migration, response, false)
					document["status"] = status
					require.Error(t, compiled.Validate(document), "status: %#v", status)
				}
				document := toolSetActionSchemaFixture(migration, response, false)
				if response {
					delete(document, "status")
				} else {
					document["status"] = map[string]any{"taskId": "opaque-task"}
				}
				require.Error(t, compiled.Validate(document), "status belongs only in responses and is required there")
			})
		}
	}
}
