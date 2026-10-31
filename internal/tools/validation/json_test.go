package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLimits supplies small explicit allowances that keep test failures local
// to the one bound each scenario changes.
func testLimits() Limits {
	return Limits{MaxInputBytes: 4096, MaxOutputBytes: 4096, MaxDepth: 8, MaxValues: 100, MaxNumberDigits: 64}
}

// TestJSONDataBounds checks inclusive limits through the public validator,
// including numeric expansion before the schema library can allocate big.Rat.
func TestJSONDataBounds(t *testing.T) {
	for name, test := range map[string]struct {
		raw   string
		limit func(*Limits)
		ok    bool
	}{
		"byte boundary":              {` {"a":0} `, func(l *Limits) { l.MaxInputBytes = 9 }, true},
		"byte overflow":              {` {"a":0} `, func(l *Limits) { l.MaxInputBytes = 8 }, false},
		"root container":             {`{"a":0}`, func(l *Limits) { l.MaxDepth = 1 }, true},
		"nested container":           {`{"a":[]}`, func(l *Limits) { l.MaxDepth = 1 }, false},
		"depth boundary":             {`{"a":[0]}`, func(l *Limits) { l.MaxDepth = 2 }, true},
		"depth overflow":             {`{"a":[[]]}`, func(l *Limits) { l.MaxDepth = 2 }, false},
		"values boundary":            {`{"a":[null,false]}`, func(l *Limits) { l.MaxValues = 4 }, true},
		"values overflow":            {`{"a":[null,false,0]}`, func(l *Limits) { l.MaxValues = 4 }, false},
		"keys are not values":        {`{"a":null,"b":false}`, func(l *Limits) { l.MaxValues = 3 }, true},
		"empty containers count":     {`{"a":[],"b":{}}`, func(l *Limits) { l.MaxValues = 2 }, false},
		"mantissa boundary":          {`{"a":-1.234}`, func(l *Limits) { l.MaxNumberDigits = 4 }, true},
		"mantissa overflow":          {`{"a":-1.2345}`, func(l *Limits) { l.MaxNumberDigits = 4 }, false},
		"positive exponent boundary": {`{"a":12e+2}`, func(l *Limits) { l.MaxNumberDigits = 4 }, true},
		"positive exponent overflow": {`{"a":12e+3}`, func(l *Limits) { l.MaxNumberDigits = 4 }, false},
		"negative exponent boundary": {`{"a":12E-2}`, func(l *Limits) { l.MaxNumberDigits = 4 }, true},
		"negative exponent overflow": {`{"a":12E-3}`, func(l *Limits) { l.MaxNumberDigits = 4 }, false},
		"leading exponent zeros":     {`{"a":1e000000003}`, func(l *Limits) { l.MaxNumberDigits = 4 }, true},
		"huge positive exponent":     {`{"a":1e999999999999999999999999}`, func(l *Limits) {}, false},
		"huge negative exponent":     {`{"a":1e-999999999999999999999999}`, func(l *Limits) {}, false},
	} {
		t.Run(name, func(t *testing.T) {
			// A numeric constraint exercises the library's exact rational path
			// only after the JSON reader has accepted the bounded number.
			limits := testLimits()
			test.limit(&limits)
			program, err := Compile(common.RawJSON(`{"type":"object","properties":{"a":{"minimum":-10000}}}`), nil, limits)
			require.NoError(t, err)
			value, err := program.ValidateInput(context.Background(), common.RawJSON(test.raw))
			if test.ok {
				require.NoError(t, err)
				require.NotNil(t, value)
			} else {
				require.ErrorIs(t, err, ErrLimit)
				assert.Nil(t, value, "rejected data must not produce partial arguments")
			}
		})
	}
}

// TestJSONOutputBounds verifies output has its own byte allowance and shares
// the structural/numeric bounds instead of trusting provider JSON.
func TestJSONOutputBounds(t *testing.T) {
	limits := testLimits()
	limits.MaxOutputBytes = 4
	limits.MaxDepth = 1
	limits.MaxValues = 2
	limits.MaxNumberDigits = 3
	program, err := Compile(common.RawJSON(`{"type":"object"}`), common.RawJSON(`true`), limits)
	require.NoError(t, err)

	// Values exactly at the output limits remain valid, including JSON null.
	for _, raw := range []string{`null`, `[1]`, `1e2`} {
		require.NoError(t, program.ValidateOutput(context.Background(), common.RawJSON(raw)))
	}
	for _, raw := range []string{`null `, `[[]]`, `[1,2]`, `1e3`} {
		require.ErrorIs(t, program.ValidateOutput(context.Background(), common.RawJSON(raw)), ErrLimit)
	}
	// The separate input cap still permits an object larger than four bytes.
	_, err = program.ValidateInput(context.Background(), common.RawJSON(`{"long-name":true}`))
	require.NoError(t, err)
}

// TestJSONRejectsAmbiguousOrMalformedData checks that parser errors cannot
// echo caller text and that duplicate properties are not silently overwritten.
func TestJSONRejectsAmbiguousOrMalformedData(t *testing.T) {
	program, err := Compile(common.RawJSON(`{"type":"object"}`), common.RawJSON(`true`), testLimits())
	require.NoError(t, err)
	for _, raw := range []string{
		``, ` `, `{"secret":"private"`, `{"secret":private}`, `{} null`, `{} private`,
		`{"secret":null,"secret":1}`, `{"nested":{"a":1,"\u0061":2}}`,
		`{"a":[1,]}`, `{"a":]}`, `{"a":[}`, "{\"a\":\"\xff\"}",
	} {
		// Both boundaries discard detailed decoder messages and partial data.
		value, err := program.ValidateInput(context.Background(), common.RawJSON(raw))
		require.ErrorIs(t, err, ErrInvalidInput, "input %q", raw)
		assert.Equal(t, ErrInvalidInput.Error(), err.Error())
		assert.Nil(t, value)
		err = program.ValidateOutput(context.Background(), common.RawJSON(raw))
		require.ErrorIs(t, err, ErrInvalidOutput, "output %q", raw)
		assert.Equal(t, ErrInvalidOutput.Error(), err.Error())
	}
}

// TestJSONDeepInputStopsAtConfiguredDepth exercises nesting well beyond the
// configured allowance without building the whole decoded tree first.
func TestJSONDeepInputStopsAtConfiguredDepth(t *testing.T) {
	limits := testLimits()
	raw := []byte(strings.Repeat("[", 1000) + "null" + strings.Repeat("]", 1000))
	_, err := decode(context.Background(), raw, limits.MaxOutputBytes, limits)
	require.ErrorIs(t, err, ErrLimit)
}

// FuzzJSONDecode compares accepted bounded values with encoding/json so token
// handling cannot silently change arrays, nulls, strings, or numeric spelling.
func FuzzJSONDecode(f *testing.F) {
	for _, raw := range []string{`{}`, `null`, `{"n":9007199254740993}`, `{"a":[false,[],{},null]}`, `1e99999999999`, `{"a":1,"a":2}`, `"\ud800"`, `[{]`} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		limits := testLimits()
		value, err := decode(context.Background(), []byte(raw), limits.MaxInputBytes, limits)
		if err != nil {
			return
		}
		require.True(t, json.Valid([]byte(raw)))
		decoder := json.NewDecoder(bytes.NewBufferString(raw))
		decoder.UseNumber()
		var expected any
		require.NoError(t, decoder.Decode(&expected))
		assert.Equal(t, expected, value)
	})
}
