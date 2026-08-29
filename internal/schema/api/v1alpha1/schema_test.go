package v1alpha1

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

func addSchema(t *testing.T, compiler *jsonschemav5.Compiler, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var header struct {
		ID string `json:"$id"`
	}
	require.NoError(t, json.Unmarshal(data, &header))
	require.NoError(t, compiler.AddResource(header.ID, bytes.NewReader(data)))
	return header.ID
}

func compileDefinition(t *testing.T, definition string) *jsonschemav5.Schema {
	t.Helper()
	compiler := jsonschemav5.NewCompiler()
	addSchema(t, compiler, "../../common/schema.json")
	addSchema(t, compiler, "../../resources/meta/schema.json")
	id := addSchema(t, compiler, "schema.json")
	require.Equal(t, SchemaID, id)
	schema, err := compiler.Compile(id + "#/$defs/" + definition)
	require.NoError(t, err)
	return schema
}

func validateJSON(t *testing.T, schema *jsonschemav5.Schema, value string) error {
	t.Helper()
	var decoded any
	require.NoError(t, json.Unmarshal([]byte(value), &decoded))
	return schema.Validate(decoded)
}

func TestSchemaDefinitions(t *testing.T) {
	listSchema := compileDefinition(t, "ResourceList")
	require.NoError(t, validateJSON(t, listSchema, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"WidgetList",
      "metadata":{"continue":"next","remainingItemCount":2},
      "items":[]
    }`))
	require.Error(t, validateJSON(t, listSchema, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"Widget",
      "metadata":{},
      "items":[]
    }`))

	actionSchema := compileDefinition(t, "Action")
	require.NoError(t, validateJSON(t, actionSchema, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"ConnectionDisconnect",
      "metadata":{"target":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","id":"cxn_123"}},
      "spec":{"timeoutSeconds":30}
    }`))
	require.Error(t, validateJSON(t, actionSchema, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"ConnectionDisconnect",
      "metadata":{},
      "spec":{},
      "unexpected":true
	}`))

	actionRequestSchema := compileDefinition(t, "ActionRequest")
	require.NoError(t, validateJSON(t, actionRequestSchema, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"ConnectionDisconnect",
      "metadata":{"target":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","id":"cxn_123"}},
      "spec":{"timeoutSeconds":30}
    }`))
	require.Error(t, validateJSON(t, actionRequestSchema, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"ConnectionDisconnect",
      "metadata":{"target":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","id":"cxn_123"}},
      "spec":{"timeoutSeconds":30},
      "status":{"taskId":"task_1"}
    }`))
}

func TestSchemaDefinitionsHaveDescriptions(t *testing.T) {
	data, err := os.ReadFile("schema.json")
	require.NoError(t, err)

	var schema struct {
		Definitions map[string]struct {
			Description string `json:"description"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(data, &schema))

	for _, definition := range []string{"ListMeta", "ResourceList", "ActionMeta", "Action", "ActionRequest", "ActionResponse"} {
		require.NotEmpty(t, schema.Definitions[definition].Description, "%s should have a description", definition)
	}
}
