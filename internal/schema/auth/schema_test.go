package auth

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
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
	c := jsonschemav5.NewCompiler()

	nsid := loadSchema(t, c, "../resources/namespace/schema.json")
	require.Equal(t, nschema.SchemaIdNamespace, nsid)

	sid := loadSchema(t, c, "./schema.json")
	require.Equal(t, SchemaIdAuth, sid, "schema ID should be the same as the one in the schema")

	const testSchemaID = "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/auth/test-permission.json"
	err := c.AddResource(testSchemaID, strings.NewReader(strings.TrimSpace(`
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/auth/test-permission.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
    "test": {
      "$ref": "./schema.json#/$defs/Permission"
    }
  }
}`)))
	require.NoError(t, err)

	schema, err := c.Compile(testSchemaID)
	require.NoError(t, err)

	tests := []struct {
		name  string
		valid bool
		data  string
	}{
		{
			name:  "valid permission",
			valid: true,
			data:  `{"test": {"namespace": "root.prod", "resources": ["connector"], "verbs": ["read"]}}`,
		},
		{
			name:  "valid permission with resource_ids",
			valid: true,
			data:  `{"test": {"namespace": "root.prod", "resources": ["connector"], "resource_ids": ["conn-1"], "verbs": ["read"]}}`,
		},
		{
			name:  "missing namespace",
			valid: false,
			data:  `{"test": {"resources": ["connector"], "verbs": ["read"]}}`,
		},
		{
			name:  "missing resources",
			valid: false,
			data:  `{"test": {"namespace": "root.prod", "verbs": ["read"]}}`,
		},
		{
			name:  "missing verbs",
			valid: false,
			data:  `{"test": {"namespace": "root.prod", "resources": ["connector"]}}`,
		},
		{
			name:  "additional properties not allowed",
			valid: false,
			data:  `{"test": {"namespace": "root.prod", "resources": ["connector"], "verbs": ["read"], "extra": "foo"}}`,
		},
	}

	for _, test := range tests {
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
}
