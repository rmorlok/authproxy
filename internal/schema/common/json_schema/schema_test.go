package json_schema

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

func TestSchema(t *testing.T) {
	schemaBytes, err := os.ReadFile("./schema.json")
	require.NoError(t, err)

	var schemaId struct {
		Id string `json:"$id"`
	}
	require.NoError(t, json.Unmarshal(schemaBytes, &schemaId))
	require.Equal(t, SchemaIdJSONSchema, schemaId.Id)

	c := jsonschemav5.NewCompiler()
	require.NoError(t, c.AddResource(schemaId.Id, bytes.NewReader(schemaBytes)))
	_, err = c.Compile(schemaId.Id)
	require.NoError(t, err)
}

func TestMarshaledSchemaMatchesContract(t *testing.T) {
	c := jsonschemav5.NewCompiler()
	schemaBytes, err := os.ReadFile("./schema.json")
	require.NoError(t, err)
	require.NoError(t, c.AddResource(SchemaIdJSONSchema, bytes.NewReader(schemaBytes)))

	schema, err := c.Compile(SchemaIdJSONSchema + "#/$defs/Schema")
	require.NoError(t, err)

	data, err := json.Marshal(Schema{
		Type:                 "object",
		Required:             []string{"client_id"},
		AdditionalProperties: false,
		Properties: map[string]Property{
			"client_id": {
				Type:      "string",
				Title:     "Client ID",
				MinLength: 1,
			},
		},
	})
	require.NoError(t, err)

	var decoded any
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NoError(t, schema.Validate(decoded))
}
