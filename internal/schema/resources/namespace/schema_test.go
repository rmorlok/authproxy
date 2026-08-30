package namespace

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

type schemaId struct {
	Id string `json:"$id"`
}

func loadSchema(t *testing.T, c *jsonschemav5.Compiler, path string) string {
	schemaBytes, err := os.ReadFile(path)
	require.NoError(t, err)

	var schemaId schemaId
	err = json.Unmarshal(schemaBytes, &schemaId)
	require.NoError(t, err)

	err = c.AddResource(schemaId.Id, bytes.NewReader(schemaBytes))
	require.NoError(t, err)

	return schemaId.Id
}

func TestSchema(t *testing.T) {
	entities := []struct {
		name  string
		ref   string
		tests []struct {
			name  string
			valid bool
			data  string
		}
	}{
		{
			name: "NamespaceName",
			ref:  "./schema.json#/$defs/NamespaceName",
			tests: []struct {
				name  string
				valid bool
				data  string
			}{
				{name: "bad value", valid: false, data: `{"test": "bad.name"}`},
				{name: "wrong type", valid: false, data: `{"test": 99}`},
				{name: "empty", valid: false, data: `{"test": ""}`},
				{name: "simple", valid: true, data: `{"test": "billing"}`},
				{name: "leading underscore", valid: true, data: `{"test": "_billing"}`},
				{name: "trailing dash", valid: true, data: `{"test": "billing-"}`},
			},
		},
		{
			name: "NamespacePath",
			ref:  "./schema.json#/$defs/NamespacePath",
			tests: []struct {
				name  string
				valid bool
				data  string
			}{
				{name: "bad value", valid: false, data: `{"test": "bad"}`},
				{name: "wrong type", valid: false, data: `{"test": 99}`},
				{name: "empty", valid: false, data: `{"test": ""}`},
				{name: "not rooted", valid: false, data: `{"test": "other.namespace"}`},
				{name: "trailing dot", valid: false, data: `{"test": "root."}`},
				{name: "case sensitive", valid: false, data: `{"test": "ROOT"}`},
				{name: "nested trailing dot", valid: false, data: `{"test": "root.other."}`},
				{name: "cannot start with dash", valid: false, data: `{"test": "root.-other"}`},
				{name: "root", valid: true, data: `{"test": "root"}`},
				{name: "nested", valid: true, data: `{"test": "root.other"}`},
				{name: "deeply nested", valid: true, data: `{"test": "root.foo.bar.baz"}`},
				{name: "allows underscores", valid: true, data: `{"test": "root.foo_bar"}`},
				{name: "can start with underscore", valid: true, data: `{"test": "root._foo"}`},
				{name: "allows dashes", valid: true, data: `{"test": "root.foo-bar"}`},
				{name: "allows just numbers", valid: true, data: `{"test": "root.1234"}`},
				{name: "allows mixed", valid: true, data: `{"test": "root.foo-1234_bar"}`},
			},
		},
		{
			name: "NamespaceMatcher",
			ref:  "./schema.json#/$defs/NamespaceMatcher",
			tests: []struct {
				name  string
				valid bool
				data  string
			}{
				{name: "bad value", valid: false, data: `{"test": "bad"}`},
				{name: "wrong type", valid: false, data: `{"test": 99}`},
				{name: "empty", valid: false, data: `{"test": ""}`},
				{name: "not rooted", valid: false, data: `{"test": "other.namespace"}`},
				{name: "trailing dot", valid: false, data: `{"test": "root."}`},
				{name: "case sensitive", valid: false, data: `{"test": "ROOT"}`},
				{name: "nested trailing dot", valid: false, data: `{"test": "root.other."}`},
				{name: "cannot start with dash", valid: false, data: `{"test": "root.-other"}`},
				{name: "root", valid: true, data: `{"test": "root"}`},
				{name: "nested", valid: true, data: `{"test": "root.other"}`},
				{name: "deeply nested", valid: true, data: `{"test": "root.foo.bar.baz"}`},
				{name: "allows underscores", valid: true, data: `{"test": "root.foo_bar"}`},
				{name: "can start with underscore", valid: true, data: `{"test": "root._foo"}`},
				{name: "allows dashes", valid: true, data: `{"test": "root.foo-bar"}`},
				{name: "allows just numbers", valid: true, data: `{"test": "root.1234"}`},
				{name: "allows mixed", valid: true, data: `{"test": "root.foo-1234_bar"}`},
				{name: "allows wildcard", valid: true, data: `{"test": "root.**"}`},
				{name: "allows wildcard on nested", valid: true, data: `{"test": "root.child.**"}`},
				{name: "does not allow single *", valid: false, data: `{"test": "root.*"}`},
			},
		},
	}

	for _, entity := range entities {
		t.Run(entity.name, func(t *testing.T) {
			c := jsonschemav5.NewCompiler()
			_ = loadSchema(t, c, "../../common/schema.json")
			_ = loadSchema(t, c, "../meta/schema.json")
			sid := loadSchema(t, c, "./schema.json")
			require.Equal(t, SchemaId, sid, "schema ID should be the same as the one in the schema")

			const testSchemaID = "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/resources/namespace/test.json"
			err := c.AddResource(testSchemaID, strings.NewReader(strings.TrimSpace(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/resources/namespace/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
    "test": {
      "$ref": "`+entity.ref+`"
    }
  }
}`)))
			require.NoError(t, err)

			schema, err := c.Compile(testSchemaID)
			require.NoError(t, err)

			for _, test := range entity.tests {
				t.Run(test.name, func(t *testing.T) {
					var v interface{}
					require.NoError(t, json.Unmarshal([]byte(test.data), &v))

					err = schema.Validate(v)
					if test.valid {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
				})
			}
		})
	}
}

func TestNamespaceResourceSchema(t *testing.T) {
	c := jsonschemav5.NewCompiler()
	_ = loadSchema(t, c, "../../common/schema.json")
	_ = loadSchema(t, c, "../meta/schema.json")
	sid := loadSchema(t, c, "./schema.json")

	schema, err := c.Compile(sid)
	require.NoError(t, err)

	valid := map[string]any{
		"apiVersion": "authproxy.net/v1alpha1",
		"kind":       "Namespace",
		"metadata": map[string]any{
			"id":        "root.acme",
			"name":      "acme",
			"namespace": "root",
		},
		"spec": map[string]any{
			"encryptionKeyRef": map[string]any{
				"apiVersion": "authproxy.net/v1alpha1",
				"kind":       "Key",
				"id":         "key_test550e8400abcd",
			},
		},
		"status": map[string]any{"state": "active"},
	}
	require.NoError(t, schema.Validate(valid))

	invalid := map[string]any{
		"apiVersion": "authproxy.net/v1alpha1",
		"kind":       "Namespace",
		"metadata":   map[string]any{"name": "bad.name"},
		"spec":       map[string]any{},
		"unknown":    true,
	}
	require.Error(t, schema.Validate(invalid))
}

func TestNamespacePatchSchema(t *testing.T) {
	c := jsonschemav5.NewCompiler()
	_ = loadSchema(t, c, "../../common/schema.json")
	_ = loadSchema(t, c, "../meta/schema.json")
	sid := loadSchema(t, c, "./schema.json")

	schema, err := c.Compile(sid + "#/$defs/NamespacePatch")
	require.NoError(t, err)
	require.NoError(t, schema.Validate(map[string]any{
		"apiVersion": "authproxy.net/v1alpha1",
		"kind":       "Namespace",
		"metadata": map[string]any{
			"labels":      map[string]any{},
			"annotations": map[string]any{},
		},
		"spec": map[string]any{},
	}))
	require.Error(t, schema.Validate(map[string]any{
		"apiVersion": "authproxy.net/v1alpha1",
		"kind":       "Namespace",
		"metadata":   map[string]any{"id": "not-rooted"},
		"spec":       map[string]any{},
	}))
}
