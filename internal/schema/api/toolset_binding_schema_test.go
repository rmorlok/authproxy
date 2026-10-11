package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// toolSetBindingSchemaFixture retains a complete catalog and a failed migration
// observation. Its attempt starts before completion, and the attempted generation
// exceeds the generation still applied to the connection.
func toolSetBindingSchemaFixture(t *testing.T) map[string]any {
	t.Helper()
	return toolResultSchemaDocument(t, `{
		"apiVersion":"authproxy.net/v1alpha1","kind":"ConnectionToolSetBinding",
		"metadata":{"namespace":"root.product.user-1","labels":{"provider":"example"},"annotations":{"note":"Observed binding."},"createdAt":"2026-10-11T10:00:00Z","updatedAt":"2026-10-11T10:01:00Z"},
		"spec":{
			"connectionRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","id":"cxn_01example0000001","namespace":"root.product.user-1","name":"production"},
			"toolSetRef":{"apiVersion":"authproxy.net/v1alpha1","kind":"ToolSet","id":"tls_01example0000001","namespace":"root.product","name":"catalog-tools"}
		},
		"status":{
			"appliedGeneration":4,
			"conditions":[
				{"type":"Installed","status":"True","observedGeneration":4,"lastTransitionTime":"2026-10-11T10:00:01Z"},
				{"type":"MigrationSucceeded","status":"False","observedGeneration":5,"lastTransitionTime":"2026-10-11T10:01:00Z","reason":"ImportFailed","message":"The previous generation remains installed."}
			],
			"catalog":{
				"lastAttemptAt":"2026-10-11T10:00:00Z",
				"lastComplete":{"revision":7,"sourceDigest":"sha256:opaque-catalog-digest","lastSuccessAt":"2026-10-11T10:00:01Z","freshness":"stale","contextMatches":"Unknown"},
				"diagnostics":[{"severity":"error","code":"catalog.refresh.failed","message":"The last complete catalog was retained.","location":{"jsonPointer":""}}]
			}
		}
	}`)
}

// toolSetBindingSchemaObject follows a known object path in a binding fixture.
func toolSetBindingSchemaObject(t *testing.T, document map[string]any, path ...string) map[string]any {
	t.Helper()
	for _, field := range path {
		object, ok := document[field].(map[string]any)
		require.True(t, ok, "%s must be an object in this fixture", field)
		document = object
	}
	return document
}

// TestConnectionToolSetBindingSchemaRoundTrip uses the canonical metadata,
// references, conditions, and diagnostics through JSON and YAML codecs.
func TestConnectionToolSetBindingSchemaRoundTrip(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ConnectionToolSetBinding")
	document := toolSetBindingSchemaFixture(t)
	require.NoError(t, compiled.Validate(document))
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	var binding ConnectionToolSetBinding
	require.NoError(t, json.Unmarshal(encoded, &binding))
	require.NoError(t, binding.Validate(nil))
	asYAML, err := yaml.Marshal(binding)
	require.NoError(t, err)
	var fromYAML ConnectionToolSetBinding
	require.NoError(t, yaml.Unmarshal(asYAML, &fromYAML))
	require.NoError(t, fromYAML.Validate(nil))
	for _, value := range []*ConnectionToolSetBinding{&binding, &fromYAML} {
		roundTrip, err := json.Marshal(value)
		require.NoError(t, err)
		require.JSONEq(t, string(encoded), string(roundTrip))
		require.NoError(t, compiled.Validate(toolResultSchemaDocument(t, string(roundTrip))))
	}
}

// TestConnectionToolSetBindingSchemaObservationShapes permits unresolved
// installation and partial refresh observations without inventing a snapshot.
func TestConnectionToolSetBindingSchemaObservationShapes(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ConnectionToolSetBinding")
	for _, status := range []string{
		`{}`,
		`{"conditions":[]}`,
		`{"catalog":{}}`,
		`{"catalog":{"lastAttemptAt":"2026-10-11T10:00:00Z","diagnostics":[{"severity":"error","code":"catalog.unavailable","message":"No catalog is available."}]}}`,
		`{"appliedGeneration":4,"conditions":[{"type":"MigrationSucceeded","status":"False","observedGeneration":2,"lastTransitionTime":"2026-10-11T10:00:00Z"}]}`,
	} {
		document := toolSetBindingSchemaFixture(t)
		document["status"] = toolResultSchemaDocument(t, status)
		// Canonical references can retain just the immutable IDs.
		for _, field := range []string{"connectionRef", "toolSetRef"} {
			reference := toolSetBindingSchemaObject(t, document, "spec", field)
			delete(reference, "namespace")
			delete(reference, "name")
		}
		require.NoError(t, compiled.Validate(document), status)
		encoded, err := json.Marshal(document)
		require.NoError(t, err)
		var binding ConnectionToolSetBinding
		require.NoError(t, json.Unmarshal(encoded, &binding))
		require.NoError(t, binding.Validate(nil))
	}
}

// TestConnectionToolSetBindingSchemaBoundaries checks logical identity,
// snapshot completeness, and the absence of unsupported mutation/task fields.
// Namespace relationships and duplicate condition types remain Go checks.
func TestConnectionToolSetBindingSchemaBoundaries(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ConnectionToolSetBinding")
	for _, test := range []struct {
		name, field string
		path        []string
		value       any
		remove      bool
	}{
		{name: "wrong kind", field: "kind", value: "ToolSet"},
		{name: "missing namespace", path: []string{"metadata"}, field: "namespace", remove: true},
		{name: "wildcard namespace", path: []string{"metadata"}, field: "namespace", value: "root.product.**"},
		{name: "independent binding ID", path: []string{"metadata"}, field: "id", value: "binding-example"},
		{name: "independent binding name", path: []string{"metadata"}, field: "name", value: "binding-name"},
		{name: "metadata generation", path: []string{"metadata"}, field: "generation", value: 4},
		{name: "missing connection", path: []string{"spec"}, field: "connectionRef", remove: true},
		{name: "missing toolset", path: []string{"spec"}, field: "toolSetRef", remove: true},
		{name: "connection without ID", path: []string{"spec", "connectionRef"}, field: "id", remove: true},
		{name: "toolset without ID", path: []string{"spec", "toolSetRef"}, field: "id", remove: true},
		{name: "wrong connection kind", path: []string{"spec", "connectionRef"}, field: "kind", value: "Connector"},
		{name: "wrong connection ID", path: []string{"spec", "connectionRef"}, field: "id", value: "cxr_01example0000001"},
		{name: "wrong toolset kind", path: []string{"spec", "toolSetRef"}, field: "kind", value: "Tool"},
		{name: "wrong toolset ID", path: []string{"spec", "toolSetRef"}, field: "id", value: "tol_01example0000001"},
		{name: "wrong target version", path: []string{"spec", "toolSetRef"}, field: "apiVersion", value: "authproxy.net/v2"},
		{name: "connection generation", path: []string{"spec", "connectionRef"}, field: "generation", value: 1},
		{name: "pinned toolset reference", path: []string{"spec", "toolSetRef"}, field: "generation", value: 4},
		{name: "missing status", field: "status", remove: true},
		{name: "null status", field: "status", value: nil},
		{name: "zero applied generation", path: []string{"status"}, field: "appliedGeneration", value: 0},
		{name: "snapshot without applied generation", path: []string{"status"}, field: "appliedGeneration", remove: true},
		{name: "snapshot without attempt", path: []string{"status", "catalog"}, field: "lastAttemptAt", remove: true},
		{name: "null catalog", path: []string{"status"}, field: "catalog", value: nil},
		{name: "missing snapshot revision", path: []string{"status", "catalog", "lastComplete"}, field: "revision", remove: true},
		{name: "zero snapshot revision", path: []string{"status", "catalog", "lastComplete"}, field: "revision", value: 0},
		{name: "blank source digest", path: []string{"status", "catalog", "lastComplete"}, field: "sourceDigest", value: " \t"},
		{name: "missing success timestamp", path: []string{"status", "catalog", "lastComplete"}, field: "lastSuccessAt", remove: true},
		{name: "unknown freshness", path: []string{"status", "catalog", "lastComplete"}, field: "freshness", value: "unknown"},
		{name: "boolean context agreement", path: []string{"status", "catalog", "lastComplete"}, field: "contextMatches", value: true},
		{name: "null condition", path: []string{"status"}, field: "conditions", value: []any{nil}},
		{name: "invalid condition status", path: []string{"status"}, field: "conditions", value: []any{map[string]any{"type": "Ready", "status": "yes", "lastTransitionTime": "2026-10-11T10:00:00Z"}}},
		{name: "invalid shared diagnostic", path: []string{"status", "catalog"}, field: "diagnostics", value: []any{map[string]any{"severity": "info", "code": "catalog.note", "message": "Note."}}},
		{name: "task identity", path: []string{"status"}, field: "taskId", value: "task-example"},
		{name: "migration object", path: []string{"status"}, field: "migration", value: map[string]any{}},
		{name: "desired generation", path: []string{"spec"}, field: "desiredGeneration", value: 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := toolSetBindingSchemaFixture(t)
			object := toolSetBindingSchemaObject(t, document, test.path...)
			if test.remove {
				delete(object, test.field)
			} else {
				object[test.field] = test.value
			}
			require.Error(t, compiled.Validate(document))
		})
	}
}

// TestConnectionToolSetBindingSchemaList composes the read-only projection with
// the shared list envelope, including an empty page and opaque continuation.
func TestConnectionToolSetBindingSchemaList(t *testing.T) {
	compiled := compileRefSchema(t, "./schema.json#/$defs/ConnectionToolSetBindingList")
	encoded, err := json.Marshal(toolSetBindingSchemaFixture(t))
	require.NoError(t, err)
	var binding ConnectionToolSetBinding
	require.NoError(t, json.Unmarshal(encoded, &binding))
	for _, items := range [][]ConnectionToolSetBinding{nil, {binding}} {
		list := NewConnectionToolSetBindingList(items, "next-page")
		require.NoError(t, list.Validate())
		data, err := json.Marshal(list)
		require.NoError(t, err)
		document := toolResultSchemaDocument(t, string(data))
		require.NoError(t, compiled.Validate(document))
		require.Equal(t, map[string]any{"continue": "next-page"}, document["metadata"])
		document["items"] = nil
		require.Error(t, compiled.Validate(document))
	}
}
