package auth

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

type test struct {
	Name  string
	Valid bool
	Data  string
}

type entities struct {
	Name   string
	Schema string
	Tests  []test
}

func TestSchema(t *testing.T) {
	entities := []entities{
		{
			Name: "NamespacePath",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/auth/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/NamespacePath"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "bad value",
					Valid: false,
					Data:  `{"test": "bad"}`,
				},
				{
					Name:  "wrong type",
					Valid: false,
					Data:  `{"test": 99}`,
				},
				{
					Name:  "empty",
					Valid: false,
					Data:  `{"test": ""}`,
				},
				{
					Name:  "not rooted",
					Valid: false,
					Data:  `{"test": "other.namespace"}`,
				},
				{
					Name:  "trailing dot",
					Valid: false,
					Data:  `{"test": "root."}`,
				},
				{
					Name:  "case sensitive",
					Valid: false,
					Data:  `{"test": "ROOT"}`,
				},
				{
					Name:  "nested trailing dot",
					Valid: false,
					Data:  `{"test": "root.other."}`,
				},
				{
					Name:  "cannot start with dash",
					Valid: false,
					Data:  `{"test": "root.-other"}`,
				},
				{
					Name:  "root",
					Valid: true,
					Data:  `{"test": "root"}`,
				},
				{
					Name:  "nested",
					Valid: true,
					Data:  `{"test": "root.other"}`,
				},
				{
					Name:  "deeply nested",
					Valid: true,
					Data:  `{"test": "root.foo.bar.baz"}`,
				},
				{
					Name:  "allows underscores",
					Valid: true,
					Data:  `{"test": "root.foo_bar"}`,
				},
				{
					Name:  "can start with underscore",
					Valid: true,
					Data:  `{"test": "root._foo"}`,
				},
				{
					Name:  "allows dashes",
					Valid: true,
					Data:  `{"test": "root.foo-bar"}`,
				},
				{
					Name:  "allows just numbers",
					Valid: true,
					Data:  `{"test": "root.1234"}`,
				},
				{
					Name:  "allows mixed",
					Valid: true,
					Data:  `{"test": "root.foo-1234_bar"}`,
				},
			},
		},
		{
			Name: "NamespaceMatcher",
			Schema: `
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/auth/test.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
	"test": {
		"$ref": "./schema.json#/$defs/NamespaceMatcher"
    }
  }
}`,
			Tests: []test{
				{
					Name:  "bad value",
					Valid: false,
					Data:  `{"test": "bad"}`,
				},
				{
					Name:  "wrong type",
					Valid: false,
					Data:  `{"test": 99}`,
				},
				{
					Name:  "empty",
					Valid: false,
					Data:  `{"test": ""}`,
				},
				{
					Name:  "not rooted",
					Valid: false,
					Data:  `{"test": "other.namespace"}`,
				},
				{
					Name:  "trailing dot",
					Valid: false,
					Data:  `{"test": "root."}`,
				},
				{
					Name:  "case sensitive",
					Valid: false,
					Data:  `{"test": "ROOT"}`,
				},
				{
					Name:  "nested trailing dot",
					Valid: false,
					Data:  `{"test": "root.other."}`,
				},
				{
					Name:  "cannot start with dash",
					Valid: false,
					Data:  `{"test": "root.-other"}`,
				},
				{
					Name:  "root",
					Valid: true,
					Data:  `{"test": "root"}`,
				},
				{
					Name:  "nested",
					Valid: true,
					Data:  `{"test": "root.other"}`,
				},
				{
					Name:  "deeply nested",
					Valid: true,
					Data:  `{"test": "root.foo.bar.baz"}`,
				},
				{
					Name:  "allows underscores",
					Valid: true,
					Data:  `{"test": "root.foo_bar"}`,
				},
				{
					Name:  "can start with underscore",
					Valid: true,
					Data:  `{"test": "root._foo"}`,
				},
				{
					Name:  "allows dashes",
					Valid: true,
					Data:  `{"test": "root.foo-bar"}`,
				},
				{
					Name:  "allows just numbers",
					Valid: true,
					Data:  `{"test": "root.1234"}`,
				},
				{
					Name:  "allows mixed",
					Valid: true,
					Data:  `{"test": "root.foo-1234_bar"}`,
				},
				{
					Name:  "allows wildcard",
					Valid: true,
					Data:  `{"test": "root.**"}`,
				},
				{
					Name:  "allows wildcard on nested",
					Valid: true,
					Data:  `{"test": "root.child.**"}`,
				},
				{
					Name:  "does not allow single *",
					Valid: false,
					Data:  `{"test": "root.*"}`,
				},
			},
		},
		{
			Name: "Permission",
			Schema: `
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
}`,
			Tests: []test{
				{
					Name:  "valid permission",
					Valid: true,
					Data:  `{"test": {"namespace": "root.prod", "resources": ["connector"], "verbs": ["read"]}}`,
				},
				{
					Name:  "valid permission with resource_ids",
					Valid: true,
					Data:  `{"test": {"namespace": "root.prod", "resources": ["connector"], "resource_ids": ["conn-1"], "verbs": ["read"]}}`,
				},
				{
					Name:  "missing namespace",
					Valid: false,
					Data:  `{"test": {"resources": ["connector"], "verbs": ["read"]}}`,
				},
				{
					Name:  "missing resources",
					Valid: false,
					Data:  `{"test": {"namespace": "root.prod", "verbs": ["read"]}}`,
				},
				{
					Name:  "missing verbs",
					Valid: false,
					Data:  `{"test": {"namespace": "root.prod", "resources": ["connector"]}}`,
				},
				{
					Name:  "additional properties not allowed",
					Valid: false,
					Data:  `{"test": {"namespace": "root.prod", "resources": ["connector"], "verbs": ["read"], "extra": "foo"}}`,
				},
			},
		},
	}

	for _, entity := range entities {
		t.Run(entity.Name, func(t *testing.T) {
			c := jsonschemav5.NewCompiler()

			sid := loadSchema(t, c, "./schema.json")

			require.Equal(t, sid, SchemaIdAuth, "schema ID should be the same as the one in the schema")

			err := c.AddResource(
				"https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/auth/test.json",
				strings.NewReader(strings.TrimSpace(entity.Schema)),
			)
			require.NoError(t, err)

			// Compile the test schema
			schema, err := c.Compile("https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/auth/test.json")
			require.NoError(t, err)

			for _, test := range entity.Tests {
				t.Run(test.Name, func(t *testing.T) {
					var v interface{}
					if err := json.Unmarshal([]byte(test.Data), &v); err != nil {
						t.Fatalf("failed to unmarshal JSON: %v", err)
					}

					err = schema.Validate(v)
					if test.Valid {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
				})
			}
		})
	}
}
