package validation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLimits gives ordinary fixtures room while keeping every allowance finite.
func testLimits() validation.Limits {
	return validation.Limits{
		MaxInputBytes:   4096,
		MaxOutputBytes:  4096,
		MaxDepth:        16,
		MaxValues:       128,
		MaxNumberDigits: 64,
	}
}

// TestCompileRejectsInvalidContracts checks that runtime preparation retains the
// native Tool schema rules, including the prohibition on external references.
func TestCompileRejectsInvalidContracts(t *testing.T) {
	for name, test := range map[string]struct {
		input  string
		output string
		want   string
	}{
		"missing input":       {"", "", "input schema"},
		"malformed input":     {`{"type":`, "", "input schema"},
		"boolean input":       {`true`, "", "top-level type object"},
		"scalar input":        {`{"type":"string"}`, "", "top-level type object"},
		"implicit input type": {`{"properties":{}}`, "", "top-level type object"},
		"old input dialect":   {`{"type":"object","$schema":"http://json-schema.org/draft-07/schema#"}`, "", "only JSON Schema 2020-12"},
		"remote input ref":    {`{"type":"object","$ref":"https://example.invalid/private.json"}`, "", "external JSON schema loading is disabled"},
		"file input ref":      {`{"type":"object","$ref":"file:///private.json"}`, "", "external JSON schema loading is disabled"},
		"malformed output":    {`{"type":"object"}`, `{"type":`, "output schema"},
		"null output schema":  {`{"type":"object"}`, `null`, "output schema"},
		"invalid output type": {`{"type":"object"}`, `{"type":"unknown"}`, "output schema"},
		"remote output ref":   {`{"type":"object"}`, `{"$ref":"https://example.invalid/private.json"}`, "external JSON schema loading is disabled"},
	} {
		t.Run(name, func(t *testing.T) {
			// Compilation must fail before any invocation can use the contract.
			program, err := validation.Compile(common.RawJSON(test.input), common.RawJSON(test.output), testLimits())
			require.ErrorContains(t, err, test.want)
			assert.Nil(t, program)
		})
	}
}

// TestCompileRequiresPositiveLimits prevents omitted or negative allowances
// from accidentally enabling unbounded instance-data preparation.
func TestCompileRequiresPositiveLimits(t *testing.T) {
	for name, change := range map[string]func(*validation.Limits, int){
		"input bytes":   func(l *validation.Limits, n int) { l.MaxInputBytes = n },
		"output bytes":  func(l *validation.Limits, n int) { l.MaxOutputBytes = n },
		"depth":         func(l *validation.Limits, n int) { l.MaxDepth = n },
		"values":        func(l *validation.Limits, n int) { l.MaxValues = n },
		"number digits": func(l *validation.Limits, n int) { l.MaxNumberDigits = n },
	} {
		for _, value := range []int{0, -1} {
			t.Run(fmt.Sprintf("%s/%d", name, value), func(t *testing.T) {
				// Disable one allowance at a time; every other field stays valid.
				limits := testLimits()
				change(&limits, value)
				program, err := validation.Compile(common.RawJSON(`{"type":"object"}`), nil, limits)
				require.ErrorContains(t, err, "limits must all be positive")
				assert.Nil(t, program)
			})
		}
	}
}

// TestCompileBoundsNumberAllowance keeps caller policy below the unsupported
// rational exponents that would otherwise panic inside schema validation.
func TestCompileBoundsNumberAllowance(t *testing.T) {
	inputSchema := common.RawJSON(`{"type":"object","properties":{"n":{"type":"integer"}}}`)
	limits := testLimits()

	// A permissive caller cannot enable exponents rejected by math/big.
	limits.MaxNumberDigits = 1_000_002
	program, err := validation.Compile(inputSchema, nil, limits)
	require.ErrorContains(t, err, "must not exceed 4096")
	assert.Nil(t, program)

	// The maximum supported allowance still permits exact integer comparisons.
	limits.MaxNumberDigits = 4096
	program, err = validation.Compile(inputSchema, nil, limits)
	require.NoError(t, err)
	_, err = program.ValidateInput(context.Background(), common.RawJSON(`{"n":1e4095}`))
	require.NoError(t, err)

	// Unsupported expansion is rejected before the library sees the number.
	params, err := program.ValidateInput(context.Background(), common.RawJSON(`{"n":1e1000001}`))
	require.ErrorIs(t, err, validation.ErrLimit)
	assert.Nil(t, params)
}

// TestValidateInputPreservesValues checks exact numbers, allowed extra fields,
// and absent defaults without changing the caller's argument bytes.
func TestValidateInputPreservesValues(t *testing.T) {
	// Numeric constraints distinguish adjacent integers beyond float64 precision.
	program, err := validation.Compile(common.RawJSON(`{
		"type":"object",
		"properties":{
			"id":{"type":"integer","const":9007199254740993},
			"decimal":{"type":"integer"},
			"exponent":{"type":"integer"},
			"limit":{"type":"integer","default":25}
		},
		"required":["id"]
	}`), nil, testLimits())
	require.NoError(t, err)
	raw := common.RawJSON(` {"id":9007199254740993,"decimal":10.0,"exponent":1e1,"extra":{"enabled":false,"items":[0,"",null]}} `)
	before := bytes.Clone(raw)

	params, err := program.ValidateInput(context.Background(), raw)
	require.NoError(t, err)

	// Validation preserves source spelling and values instead of normalizing them.
	assert.Equal(t, json.Number("9007199254740993"), params["id"])
	assert.Equal(t, json.Number("10.0"), params["decimal"])
	assert.Equal(t, json.Number("1e1"), params["exponent"])
	assert.NotContains(t, params, "limit")
	assert.Equal(t, map[string]any{
		"enabled": false,
		"items":   []any{json.Number("0"), "", nil},
	}, params["extra"])
	assert.Equal(t, before, []byte(raw))

	// Mutating a returned tree cannot affect bytes or a later validation result.
	params["extra"].(map[string]any)["items"].([]any)[0] = "changed"
	again, err := program.ValidateInput(context.Background(), raw)
	require.NoError(t, err)
	assert.Equal(t, json.Number("0"), again["extra"].(map[string]any)["items"].([]any)[0])
	assert.Equal(t, before, []byte(raw))

	// Returned strings and containers also remain usable after source replacement.
	for i := range raw {
		raw[i] = 'x'
	}
	assert.Equal(t, json.Number("9007199254740993"), again["id"])
	assert.Equal(t, "", again["extra"].(map[string]any)["items"].([]any)[1])
}

// TestValidateInputReturnsSafeErrors prevents decoder or schema diagnostics
// from disclosing private arguments and ensures failures return no partial map.
func TestValidateInputReturnsSafeErrors(t *testing.T) {
	program, err := validation.Compile(common.RawJSON(`{
		"type":"object",
		"properties":{"privateName":{"type":"integer","maximum":9007199254740992}},
		"required":["privateName"],"additionalProperties":false
	}`), nil, testLimits())
	require.NoError(t, err)

	for name, raw := range map[string]string{
		"missing":             "",
		"null":                `null`,
		"array":               `[]`,
		"string":              `"private-value"`,
		"number":              `1`,
		"boolean":             `true`,
		"malformed":           `{"privateName":"private-value"`,
		"trailing value":      `{"privateName":1} "private-value"`,
		"missing required":    `{}`,
		"extra property":      `{"privateName":1,"private-extra":"private-value"}`,
		"no string coercion":  `{"privateName":"123"}`,
		"no boolean coercion": `{"privateName":true}`,
		"exact maximum":       `{"privateName":9007199254740993}`,
		"duplicate property":  `{"privateName":1,"privateName":2}`,
	} {
		t.Run(name, func(t *testing.T) {
			// Public errors identify the failure class without keys, values, or paths.
			params, err := program.ValidateInput(context.Background(), common.RawJSON(raw))
			require.ErrorIs(t, err, validation.ErrInvalidInput)
			assert.EqualError(t, err, validation.ErrInvalidInput.Error())
			assert.Nil(t, params)
		})
	}
}

// TestValidateUsesBundledReferences verifies that both instance paths use the
// native compiled reference graph rather than treating references as annotations.
func TestValidateUsesBundledReferences(t *testing.T) {
	program, err := validation.Compile(common.RawJSON(`{
		"$id":"https://example.invalid/input", "type":["object"],
		"properties":{"name":{"$ref":"name"}}, "required":["name"],
		"$defs":{"name":{"$id":"name","type":"string","minLength":2}}
	}`), common.RawJSON(`{
		"$ref":"#items",
		"$defs":{"items":{"$anchor":"items","type":"array","items":{"type":"integer"}}}
	}`), testLimits())
	require.NoError(t, err)

	// Bundled resource IDs and anchors resolve entirely within their documents.
	_, err = program.ValidateInput(context.Background(), common.RawJSON(`{"name":"ok"}`))
	require.NoError(t, err)
	require.NoError(t, program.ValidateOutput(context.Background(), common.RawJSON(`[1,2]`)))

	// Referenced constraints still reject instances with the wrong shape or values.
	_, err = program.ValidateInput(context.Background(), common.RawJSON(`{"name":"x"}`))
	require.ErrorIs(t, err, validation.ErrInvalidInput)
	require.ErrorIs(t, program.ValidateOutput(context.Background(), common.RawJSON(`["private-value"]`)), validation.ErrInvalidOutput)
}

// TestValidateOutputDistinguishesOmissionAndJSON checks schema omission, boolean
// schemas, and every JSON result shape without inventing an object wrapper.
func TestValidateOutputDistinguishesOmissionAndJSON(t *testing.T) {
	for name, test := range map[string]struct {
		schema string
		raw    string
		valid  bool
	}{
		"omitted schema and absent output": {"", "", true},
		"omitted schema skips decoding":    {"", "not JSON", true},
		"true requires present output":     {`true`, "", false},
		"true rejects whitespace only":     {`true`, " \n ", false},
		"true accepts explicit null":       {`true`, `null`, true},
		"true accepts object":              {`true`, `{}`, true},
		"true accepts array":               {`true`, `[]`, true},
		"true accepts scalar":              {`true`, `false`, true},
		"true rejects malformed JSON":      {`true`, `{"private":"value"`, false},
		"true rejects trailing JSON":       {`true`, `null "private-value"`, false},
		"false rejects null":               {`false`, `null`, false},
		"false rejects object":             {`false`, `{}`, false},
		"false rejects array":              {`false`, `[]`, false},
		"false rejects scalar":             {`false`, `42`, false},
		"null schema type accepts null":    {`{"type":"null"}`, `null`, true},
		"object schema rejects null":       {`{"type":"object"}`, `null`, false},
		"string schema accepts string":     {`{"type":"string"}`, `"value"`, true},
		"number schema accepts number":     {`{"type":"number"}`, `1.25`, true},
		"number schema rejects coercion":   {`{"type":"number"}`, `"1.25"`, false},
		"array schema accepts array":       {`{"type":"array","items":{"type":"boolean"}}`, `[true,false]`, true},
		"array schema checks items":        {`{"type":"array","items":{"type":"boolean"}}`, `["private-value"]`, false},
		"output preserves exact integers":  {`{"const":9007199254740993}`, `9007199254740993`, true},
		"output rejects adjacent integers": {`{"const":9007199254740993}`, `9007199254740992`, false},
	} {
		t.Run(name, func(t *testing.T) {
			program, err := validation.Compile(common.RawJSON(`{"type":"object"}`), common.RawJSON(test.schema), testLimits())
			require.NoError(t, err)
			raw := common.RawJSON(test.raw)
			before := bytes.Clone(raw)

			// Successful and rejected output both leave the original JSON untouched.
			err = program.ValidateOutput(context.Background(), raw)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, validation.ErrInvalidOutput)
				assert.EqualError(t, err, validation.ErrInvalidOutput.Error())
			}
			assert.Equal(t, before, []byte(raw))
		})
	}

	// Nil and an empty byte slice both denote absent content to the validator.
	program, err := validation.Compile(common.RawJSON(`{"type":"object"}`), common.RawJSON(`true`), testLimits())
	require.NoError(t, err)
	require.ErrorIs(t, program.ValidateOutput(context.Background(), nil), validation.ErrInvalidOutput)
	require.ErrorIs(t, program.ValidateOutput(context.Background(), common.RawJSON{}), validation.ErrInvalidOutput)
}

// TestValidateOutputPreservesFormatting keeps validation independent from
// response serialization, including whitespace and numeric source spelling.
func TestValidateOutputPreservesFormatting(t *testing.T) {
	program, err := validation.Compile(common.RawJSON(`{"type":"object"}`), common.RawJSON(`{"type":"array"}`), testLimits())
	require.NoError(t, err)
	raw := common.RawJSON("\n [ 9007199254740993, 10.0, 1e1, null, {\"escaped\":\"\\u0061\"} ]\t")
	before := bytes.Clone(raw)

	// Validation must not decode and re-encode the caller's eventual result bytes.
	require.NoError(t, program.ValidateOutput(context.Background(), raw))
	assert.Equal(t, before, []byte(raw))
}

// TestValidateHonorsCanceledContexts checks cancellation before parsing or
// schema evaluation, including the output-schema omission fast path.
func TestValidateHonorsCanceledContexts(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, expire := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer expire()

	for name, test := range map[string]struct {
		ctx  context.Context
		want error
	}{
		"canceled": {canceled, context.Canceled},
		"expired":  {expired, context.DeadlineExceeded},
	} {
		t.Run(name, func(t *testing.T) {
			for _, output := range []common.RawJSON{nil, common.RawJSON(`true`)} {
				program, err := validation.Compile(common.RawJSON(`{"type":"object"}`), output, testLimits())
				require.NoError(t, err)

				// Cancellation takes precedence over malformed or absent instance data.
				params, err := program.ValidateInput(test.ctx, common.RawJSON(`not JSON`))
				require.ErrorIs(t, err, test.want)
				assert.Nil(t, params)
				require.ErrorIs(t, program.ValidateOutput(test.ctx, nil), test.want)
			}
		})
	}
}

// TestProgramOwnsSchemasAndSupportsConcurrentReuse checks that source storage,
// invocation maps, and validation state do not leak across callers.
func TestProgramOwnsSchemasAndSupportsConcurrentReuse(t *testing.T) {
	input := common.RawJSON(`{"type":"object","properties":{"values":{"type":"array","items":{"type":"integer"}}},"required":["values"]}`)
	output := common.RawJSON(`{"type":"integer","minimum":2}`)
	inputBefore, outputBefore := bytes.Clone(input), bytes.Clone(output)
	limits := testLimits()
	program, err := validation.Compile(input, output, limits)
	require.NoError(t, err)
	assert.Equal(t, inputBefore, []byte(input))
	assert.Equal(t, outputBefore, []byte(output))

	// Replacing all caller-owned source and policy fields must not change the program.
	for i := range input {
		input[i] = 'x'
	}
	for i := range output {
		output[i] = 'x'
	}
	limits.MaxInputBytes, limits.MaxOutputBytes = 1, 1

	for i := 0; i < 16; i++ {
		t.Run(fmt.Sprintf("caller-%d", i), func(t *testing.T) {
			t.Parallel()
			for attempt := 0; attempt < 8; attempt++ {
				// Each caller receives a private tree while sharing the compiled schemas.
				params, err := program.ValidateInput(context.Background(), common.RawJSON(`{"values":[2,3]}`))
				require.NoError(t, err)
				values := params["values"].([]any)
				require.Equal(t, []any{json.Number("2"), json.Number("3")}, values)
				values[0] = "changed"
				require.NoError(t, program.ValidateOutput(context.Background(), common.RawJSON(`2`)))

				// Rejections also remain local and cannot poison another validation call.
				params, err = program.ValidateInput(context.Background(), common.RawJSON(`{"values":["private-value"]}`))
				require.ErrorIs(t, err, validation.ErrInvalidInput)
				assert.Nil(t, params)
				require.ErrorIs(t, program.ValidateOutput(context.Background(), common.RawJSON(`1`)), validation.ErrInvalidOutput)
			}
		})
	}
}
