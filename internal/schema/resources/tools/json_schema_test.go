package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompileInputSchemaOwnsLocalReferences verifies that compiled constraints
// retain their meaning after the caller reuses the authored document's bytes.
func TestCompileInputSchemaOwnsLocalReferences(t *testing.T) {
	// Compile an input with a local reference and an object-valued constant.
	raw := common.RawJSON(`{
		"type":"object","required":["selection"],
		"properties":{"selection":{"$ref":"#/$defs/selection"}},
		"$defs":{"selection":{"const":{"name":"original"}}}
	}`)
	before := bytes.Clone(raw)
	schema, err := CompileInputSchema(raw)
	require.NoError(t, err)
	assert.Equal(t, before, []byte(raw), "compilation must preserve the authored document")

	// The compiled schema owns decoded values rather than the caller's buffer.
	copy(raw, bytes.Repeat([]byte("x"), len(raw)))
	require.NoError(t, schema.Validate(map[string]any{
		"selection": map[string]any{"name": "original"},
	}))
	require.Error(t, schema.Validate(map[string]any{
		"selection": map[string]any{"name": "changed"},
	}))
	require.Error(t, schema.Validate(map[string]any{}))
}

// TestCompileOutputSchemaChecksValues verifies that output validators retain
// non-object and boolean schema constraints, and require an authored document.
func TestCompileOutputSchemaChecksValues(t *testing.T) {
	// Output may be a scalar even though the input contract requires an object.
	schema, err := CompileOutputSchema(common.RawJSON(`{"type":"integer","minimum":2}`))
	require.NoError(t, err)
	require.NoError(t, schema.Validate(json.Number("2")))
	require.Error(t, schema.Validate(json.Number("1")))
	require.Error(t, schema.Validate("2"))

	// Boolean output schemas allow or reject every value, including null.
	for _, accepted := range []bool{false, true} {
		schema, err = CompileOutputSchema(common.RawJSON(fmt.Sprint(accepted)))
		require.NoError(t, err)
		if accepted {
			require.NoError(t, schema.Validate(nil))
		} else {
			require.Error(t, schema.Validate(nil))
		}
	}

	// Optional output omission is a caller decision, not an accepting schema.
	schema, err = CompileOutputSchema(nil)
	require.ErrorContains(t, err, "invalid JSON schema")
	assert.Nil(t, schema)
}

// TestCompiledSchemaValidationPreservesValues ensures annotations and rejected
// properties never become implicit transformations of invocation data.
func TestCompiledSchemaValidationPreservesValues(t *testing.T) {
	schema, err := CompileInputSchema(common.RawJSON(`{
		"type":"object","required":["name"],"additionalProperties":false,
		"properties":{
			"name":{"type":"string","default":"anonymous"},
			"count":{"type":"integer","default":1}
		}
	}`))
	require.NoError(t, err)

	for name, test := range map[string]struct {
		value map[string]any
		valid bool
	}{
		"optional default stays absent":    {map[string]any{"name": "Alice"}, true},
		"required default is not inserted": {map[string]any{}, false},
		"additional properties are not stripped": {
			map[string]any{"name": "Alice", "extra": map[string]any{"nested": true}}, false,
		},
		"types are not coerced": {map[string]any{"name": json.Number("42")}, false},
	} {
		t.Run(name, func(t *testing.T) {
			// Capture the entire input, including unknown nested properties.
			before, err := json.Marshal(test.value)
			require.NoError(t, err)

			// Both successful and failed validation must leave all data intact.
			err = schema.Validate(test.value)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			after, err := json.Marshal(test.value)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

// TestCompiledSchemaConcurrentReuse shares one compiled schema across validators
// so race-enabled tests check that runtime validation keeps state per call.
func TestCompiledSchemaConcurrentReuse(t *testing.T) {
	// Local recursion exercises repeated traversal of the same compiled nodes.
	schema, err := CompileInputSchema(common.RawJSON(`{
		"type":"object","required":["name"],
		"properties":{
			"name":{"type":"string"},
			"children":{"type":"array","items":{"$ref":"#"}}
		}
	}`))
	require.NoError(t, err)

	for i := range 16 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()

			// Each call has independent values while sharing the compiled graph.
			require.NoError(t, schema.Validate(map[string]any{
				"name": "parent", "children": []any{map[string]any{"name": "child"}},
			}))
			require.Error(t, schema.Validate(map[string]any{
				"name": "parent", "children": []any{map[string]any{}},
			}))
		})
	}
}

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
