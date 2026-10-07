package tools

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateSchemaNativeDialectAndLocalReferences covers supported local
// pointers, anchors, bundled resources, and recursive schemas without mutation.
func TestValidateSchemaNativeDialectAndLocalReferences(t *testing.T) {
	for name, schema := range map[string]string{
		"implicit dialect": `{"type":"object"}`,
		"singleton type":   `{"type":["object"]}`,
		"explicit dialect": `{"$schema":"https://json-schema.org/draft/2020-12/schema#","type":"object"}`,
		"root reference": `{
			"type":"object","$ref":"#/$defs/params",
			"$defs":{"params":{"type":"object","properties":{"name":{"type":"string"}}}}
		}`,
		"escaped pointer": `{
			"type":"object","properties":{"name":{"$ref":"#/$defs/a~1b~0c"}},
			"$defs":{"a/b~c":{"type":"string"}}
		}`,
		"local anchor": `{
			"type":"object","properties":{"name":{"$ref":"#name"}},
			"$defs":{"name":{"$anchor":"name","type":"string"}}
		}`,
		"bundled resource": `{
			"$id":"https://example.test/tool","type":"object",
			"properties":{"name":{"$ref":"name"}},
			"$defs":{"name":{"$id":"name","type":"string"}}
		}`,
		"recursive local reference": `{
			"type":"object","properties":{"children":{"type":"array","items":{"$ref":"#"}}}
		}`,
		"native tuple syntax": `{
			"type":"object","properties":{"tuple":{"type":"array","prefixItems":[{"type":"string"}],"items":false}}
		}`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := common.RawJSON(schema)
			before := bytes.Clone(raw)
			require.NoError(t, validateSchema(raw, true, &common.ValidationContext{Path: "inputSchema"}))
			assert.Equal(t, before, []byte(raw), "contract validation must preserve the authored schema")
		})
	}
}

// TestValidateSchemaRejectsInvalidContracts keeps malformed schemas, alternate
// dialects, and external resource loads outside the native contract.
func TestValidateSchemaRejectsInvalidContracts(t *testing.T) {
	for name, test := range map[string]struct {
		schema string
		want   string
	}{
		"missing":               {``, "invalid JSON schema"},
		"malformed":             {`{"type":`, "invalid JSON schema"},
		"trailing value":        {`{"type":"object"} true`, "exactly one JSON value"},
		"trailing garbage":      {`{"type":"object"} invalid`, "exactly one JSON value"},
		"null":                  {`null`, "object or boolean"},
		"array":                 {`[]`, "object or boolean"},
		"string root":           {`{"type":"string"}`, "top-level type object"},
		"mixed types":           {`{"type":["object","null"]}`, "top-level type object"},
		"boolean input":         {`true`, "top-level type object"},
		"implicit object":       {`{"properties":{}}`, "top-level type object"},
		"root ref without type": {`{"$ref":"#/$defs/params","$defs":{"params":{"type":"object"}}}`, "top-level type object"},
		"unsupported dialect":   {`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object"}`, "only JSON Schema 2020-12"},
		"invalid dialect":       {`{"$schema":42,"type":"object"}`, "only JSON Schema 2020-12"},
		"unknown dialect":       {`{"$schema":"https://example.test/custom-schema","type":"object"}`, "only JSON Schema 2020-12"},
		"nested dialect": {`{
			"type":"object","properties":{"name":{"$schema":"http://json-schema.org/draft-07/schema#","type":"string"}}
		}`, "only JSON Schema 2020-12"},
		"referenced dialect": {`{
			"type":"object","$ref":"#/$defs/params",
			"$defs":{"params":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object"}}
		}`, "only JSON Schema 2020-12"},
		"referenced annotation becomes schema": {`{
			"type":"object","$ref":"#/examples/0",
			"examples":[{"$schema":"http://json-schema.org/draft-07/schema#","type":"object"}]
		}`, "only JSON Schema 2020-12"},
		"missing local target":  {`{"type":"object","$ref":"#/$defs/missing"}`, "not found"},
		"remote reference":      {`{"type":"object","$ref":"https://example.test/schema.json"}`, "external JSON schema loading is disabled"},
		"file reference":        {`{"type":"object","$ref":"file:///must-not-read.json"}`, "external JSON schema loading is disabled"},
		"relative reference":    {`{"type":"object","$ref":"other.json"}`, "external JSON schema loading is disabled"},
		"invalid keyword":       {`{"type":"object","properties":{"name":{"minLength":-1}}}`, "invalid JSON schema"},
		"obsolete tuple syntax": {`{"type":"object","properties":{"tuple":{"type":"array","items":[{"type":"string"}]}}}`, "invalid JSON schema"},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateSchema(common.RawJSON(test.schema), true, &common.ValidationContext{Path: "inputSchema"})
			require.ErrorContains(t, err, test.want)
			var validationError *common.ValidationError
			require.ErrorAs(t, err, &validationError)
			assert.Equal(t, "inputSchema", validationError.Path)
		})
	}
}

// TestValidateSchemaOutputAcceptsEverySchemaRoot permits successful output to
// have any JSON type, including schemas that accept or reject every value.
func TestValidateSchemaOutputAcceptsEverySchemaRoot(t *testing.T) {
	for _, schema := range []string{`true`, `false`, `{}`, `{"type":"string"}`, `{"type":["array","null"]}`} {
		t.Run(schema, func(t *testing.T) {
			require.NoError(t, validateSchema(common.RawJSON(schema), false, &common.ValidationContext{Path: "outputSchema"}))
		})
	}
}

// TestValidateSchemaDoesNotInterpretAnnotationData distinguishes actual schema
// locations from values that merely contain keyword-looking keys.
func TestValidateSchemaDoesNotInterpretAnnotationData(t *testing.T) {
	schema := common.RawJSON(`{
		"type":"object",
		"default":{"$schema":"other-dialect","$ref":"file:///not-a-schema"},
		"const":{"$schema":17,"$ref":"https://not-a-schema.test"},
		"examples":[{"$schema":false,"$defs":{"literal":{"$ref":"unloaded.json"}}}],
		"$defs":{"unused":{"$schema":"http://json-schema.org/draft-07/schema#","$ref":"unloaded.json"}}
	}`)
	before := bytes.Clone(schema)
	require.NoError(t, validateSchema(schema, true, &common.ValidationContext{Path: "inputSchema"}))
	assert.Equal(t, before, []byte(schema))
}

// TestValidateSchemaBoundsAuthoredSizeAndDepth exercises both inclusive limits
// and the first rejected value, including deep annotation data.
func TestValidateSchemaBoundsAuthoredSizeAndDepth(t *testing.T) {
	base := `{"type":"object","description":""}`
	for _, size := range []int{maxSchemaBytes, maxSchemaBytes + 1} {
		schema := common.RawJSON(`{"type":"object","description":"` + strings.Repeat("x", size-len(base)) + `"}`)
		err := validateSchema(schema, true, &common.ValidationContext{})
		if size == maxSchemaBytes {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "exceeds")
		}
	}
	for _, depth := range []int{maxSchemaDepth, maxSchemaDepth + 1} {
		// The schema root counts as one container, even though this nested
		// default is annotation data rather than a series of subschemas.
		schema := common.RawJSON(`{"type":"object","default":` + strings.Repeat("[", depth-1) + `0` + strings.Repeat("]", depth-1) + `}`)
		err := validateSchema(schema, true, &common.ValidationContext{})
		if depth == maxSchemaDepth {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "nesting exceeds")
		}
	}
}
