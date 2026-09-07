package meta

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
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
	tests := []struct {
		definition string
		valid      string
		invalid    string
	}{
		{"TypeMeta", `{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector"}`, `{"apiVersion":"authproxy.net/v1alpha2","kind":"connector"}`},
		{"ObjectMeta", `{"id":"cxr_123","name":"greenhouse","namespace":"root","generation":3,"labels":{"environment":"demo"}}`, `{"name":"-bad","unknown":true}`},
		{"ObjectMetaPatch", `{"labels":{},"annotations":{}}`, `{"generation":0}`},
		{"ObjectReference", `{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","id":"cxr_123","generation":3}`, `{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector"}`},
		{"Condition", `{"type":"Ready","status":"True","observedGeneration":3,"lastTransitionTime":"2026-08-28T10:00:00Z"}`, `{"type":"Ready","status":"yes","lastTransitionTime":"not-a-time"}`},
	}
	for _, test := range tests {
		t.Run(test.definition, func(t *testing.T) {
			schema := compileDefinition(t, test.definition)
			require.NoError(t, validateJSON(t, schema, test.valid))
			require.Error(t, validateJSON(t, schema, test.invalid))
		})
	}
}

func TestObjectReferenceSchemaSupportsNamespacedName(t *testing.T) {
	schema := compileDefinition(t, "ObjectReference")
	require.NoError(t, validateJSON(t, schema, "{\"apiVersion\":\"authproxy.net/v1alpha1\",\"kind\":\"Connector\",\"namespace\":\"root.prod\",\"name\":\"greenhouse\"}"))
	require.Error(t, validateJSON(t, schema, "{\"apiVersion\":\"authproxy.net/v1alpha1\",\"kind\":\"Connector\",\"name\":\"greenhouse\"}"))
}

func TestResourceSchemaComposesWithoutRepeatingTypeMeta(t *testing.T) {
	compiler := jsonschemav5.NewCompiler()
	addSchema(t, compiler, "../../common/schema.json")
	addSchema(t, compiler, "schema.json")
	const concreteID = "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/resources/meta/test-widget.json"
	require.NoError(t, compiler.AddResource(concreteID, strings.NewReader(`{
      "$schema":"https://json-schema.org/draft/2020-12/schema",
      "$id":"`+concreteID+`",
      "allOf":[
        {"$ref":"./schema.json#/$defs/Resource"},
        {
          "type":"object",
          "properties":{
            "kind":{"const":"Widget"},
            "spec":{
              "type":"object",
              "required":["enabled"],
              "properties":{"enabled":{"type":"boolean"}},
              "additionalProperties":false
            }
          }
        }
      ],
      "unevaluatedProperties":false
    }`)))
	schema, err := compiler.Compile(concreteID)
	require.NoError(t, err)
	require.NoError(t, validateJSON(t, schema, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"Widget",
      "metadata":{"name":"example"},
      "spec":{"enabled":true}
    }`))
	require.Error(t, validateJSON(t, schema, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"Widget",
      "metadata":{"name":"example"},
      "spec":{"enabled":true},
      "unknown":true
    }`))
}
