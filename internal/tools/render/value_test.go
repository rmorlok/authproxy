package render

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

func TestRenderValuePreservesTypes(t *testing.T) {
	// Typed references preserve all JSON kinds and authored numeric precision.
	data, err := NewContext(Input{Params: map[string]any{
		"false": false, "null": nil, "number": json.Number("9007199254740993.0100"),
		"array": []any{false, nil, "x"}, "object": map[string]any{"count": 2},
		"string": `quote" and <&>`,
	}}, testLimits())
	require.NoError(t, err)
	source := common.RawJSON(`{
		"boolean":{"$value":"params.false"}, "null":{"$value":"params.null"},
		"number":{"$value":"params.number"}, "authored":9007199254740993.0100,
		"array":{"$value":"params.array"}, "object":{"$value":"params.object"},
		"string":{"$value":"params.string"}, "template":"prefix {{params.string}}",
		"nested":[{"value":"{{params.false}}"}]
	}`)
	result, err := RenderValue(context.Background(), source, data, testLimits())
	require.NoError(t, err)
	require.JSONEq(t, `{
		"boolean":false,"null":null,"number":9007199254740993.0100,"authored":9007199254740993.0100,
		"array":[false,null,"x"],"object":{"count":2},"string":"quote\" and <&>",
		"template":"prefix quote\" and <&>","nested":[{"value":"false"}]
	}`, string(result))
	require.Equal(t, 2, strings.Count(string(result), "9007199254740993.0100"))
	require.Contains(t, string(result), "<&>")

	// Returned bytes are independent; corrupting them cannot corrupt future renders.
	result[0] = '!'
	again, err := RenderValue(context.Background(), source, data, testLimits())
	require.NoError(t, err)
	require.True(t, json.Valid(again))
}

func TestRenderValueLiteralAndInertData(t *testing.T) {
	// Caller values that look executable are inserted as data without a second pass.
	data, err := NewContext(Input{Params: map[string]any{
		"data": map[string]any{"$value": "cfg.secret", "text": "{{cfg.secret}}"},
	}}, testLimits())
	require.NoError(t, err)
	source := common.RawJSON(`{
		"injected":{"$value":"params.data"},
		"literal":{"$literal":{"$value":"cfg.secret","text":"{{>private-file}}"}},
		"literalScalar":{"$literal":"{{cfg.secret}}"},
		"ordinary":{"$literal":{"$value":"not.a.reference","another":true}},
		"{{cfg.secret}}":"literal key"
	}`)
	result, err := RenderValue(context.Background(), source, data, testLimits())
	require.NoError(t, err)
	require.JSONEq(t, `{
		"injected":{"$value":"cfg.secret","text":"{{cfg.secret}}"},
		"literal":{"$value":"cfg.secret","text":"{{>private-file}}"},
		"literalScalar":"{{cfg.secret}}", "ordinary":{"$value":"not.a.reference","another":true},
		"{{cfg.secret}}":"literal key"
	}`, string(result))
}

func TestRenderValueFailures(t *testing.T) {
	data, err := NewContext(Input{Cfg: map[string]any{"secret": "private-configuration"}}, testLimits())
	require.NoError(t, err)
	for _, source := range []string{
		``, `{"bad":`, `true false`, `{"$value":42}`, `{"$value":""}`,
		`{"$value":"params.absent"}`, `"{{>partial}}"`, `"{{cfg.secret.child}}"`,
		string([]byte{'"', 0xff, '"'}),
	} {
		t.Run(source, func(t *testing.T) {
			// Invalid source, directives, and strict reference errors return no partial body.
			result, err := RenderValue(context.Background(), common.RawJSON(source), data, testLimits())
			require.Error(t, err)
			require.Nil(t, result)
			require.NotContains(t, err.Error(), "private-configuration")
		})
	}
}

func TestRenderValueLimitsAndCancellation(t *testing.T) {
	data, err := NewContext(Input{Params: map[string]any{
		"large": strings.Repeat("x", 60), "escaped": strings.Repeat("\n", 50),
	}}, testLimits())
	require.NoError(t, err)
	cases := []struct {
		name   string
		source string
		limits Limits
	}{
		{"source", `"` + strings.Repeat("x", 100) + `"`, Limits{MaxBytes: 100, MaxDepth: 32, MaxSteps: 10000}},
		{"injected output", `[{"$value":"params.large"},{"$value":"params.large"}]`, Limits{MaxBytes: 100, MaxDepth: 32, MaxSteps: 10000}},
		{"quoted output", `"{{params.escaped}}"`, Limits{MaxBytes: 100, MaxDepth: 32, MaxSteps: 10000}},
		{"depth", `[[[0]]]`, Limits{MaxBytes: 100, MaxDepth: 2, MaxSteps: 10000}},
		{"shared work", `["{{params.large}}","{{params.large}}"]`, Limits{MaxBytes: 1000, MaxDepth: 32, MaxSteps: 4}},
		{"invalid limits", `null`, Limits{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Expansion and quoting must fit final-output limits, not only source limits.
			result, err := RenderValue(context.Background(), common.RawJSON(tc.source), data, tc.limits)
			require.Error(t, err)
			require.Nil(t, result)
		})
	}

	// Cancellation also rejects empty/literal work before it can produce a body.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = RenderValue(ctx, common.RawJSON(`null`), data, testLimits())
	require.ErrorIs(t, err, context.Canceled)
}

// TestCompileValueRejectsMalformedDirectives catches authoring mistakes before
// an invocation supplies values, while $literal explicitly preserves such data.
func TestCompileValueRejectsMalformedDirectives(t *testing.T) {
	for _, source := range []string{
		`{"$value":"params.tags","default":[]}`, `{"$literal":{},"extra":true}`,
		`{"nested":{"$typo":"params.tags"}}`, `{"$unknown":1,"ordinary":2}`,
		`{"$value":42}`, `{"$value":""}`, `{"$value":"params..name"}`,
		`{"$value":"json.params.name"}`, `{"nested":["{{>file}}"]}`,
	} {
		t.Run(source, func(t *testing.T) {
			// Reserved keys and invalid templates are compile errors without data.
			compiled, err := CompileValue(common.RawJSON(source), testLimits())
			require.Error(t, err)
			require.Nil(t, compiled)
		})
	}

	// Explicit literal data can contain reserved keys and uninterpreted templates.
	source := common.RawJSON(`{"$literal":{"$value":"params.tags","$unknown":"{{>file}}","default":[]}}`)
	compiled, err := CompileValue(source, testLimits())
	require.NoError(t, err)
	data, err := NewContext(Input{}, testLimits())
	require.NoError(t, err)
	result, err := compiled.Render(context.Background(), data)
	require.NoError(t, err)
	require.JSONEq(t, `{"$value":"params.tags","$unknown":"{{>file}}","default":[]}`, string(result))
}

// TestCompileValueSharesSyntaxBudget includes all embedded templates in one
// allowance, including comments that produce no rendering work or output.
func TestCompileValueSharesSyntaxBudget(t *testing.T) {
	limits := testLimits()
	limits.MaxSteps = 6
	source := common.RawJSON(`["{{!one}}{{!two}}","{{!three}}{{!four}}"]`)

	// Each string fits alone, but both strings plus their JSON nodes exceed six.
	_, err := Compile(`{{!one}}{{!two}}`, limits)
	require.NoError(t, err)
	compiled, err := CompileValue(source, limits)
	require.ErrorContains(t, err, "step limit")
	require.Nil(t, compiled)

	// Once compiled under a sufficient policy, comment-heavy source renders cheaply.
	limits.MaxSteps = 7
	compiled, err = CompileValue(source, limits)
	require.NoError(t, err)
	data, err := NewContext(Input{}, testLimits())
	require.NoError(t, err)
	result, err := compiled.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, `["",""]`, string(result))

	// The one-off helper counts both phases instead of resetting after compilation.
	result, err = RenderValue(context.Background(), source, data, limits)
	require.ErrorContains(t, err, "step limit")
	require.Nil(t, result)
}

// TestValueTemplateConcurrentReuse proves a compiled body owns its source and
// keeps invocation data and output buffers independent between renders.
func TestValueTemplateConcurrentReuse(t *testing.T) {
	source := common.RawJSON(`{"name":"Hello {{params.name}}","items":{"$value":"params.items"}}`)
	compiled, err := CompileValue(source, testLimits())
	require.NoError(t, err)

	// Destroy the caller's source after compiling; the reusable program owns its data.
	for i := range source {
		source[i] = '!'
	}
	inputs := make([]*Context, 20)
	for i := range inputs {
		inputs[i], err = NewContext(Input{Params: map[string]any{
			"name": fmt.Sprintf("contact-%d", i), "items": []any{i, false, nil},
		}}, testLimits())
		require.NoError(t, err)
	}

	// Concurrent calls share only the compiled program and their own immutable input.
	results := make([]common.RawJSON, len(inputs))
	errors := make([]error, len(inputs))
	var workers sync.WaitGroup
	for i := range inputs {
		workers.Add(1)
		go func() {
			defer workers.Done()
			results[i], errors[i] = compiled.Render(context.Background(), inputs[i])
		}()
	}
	workers.Wait()
	for i, result := range results {
		require.NoError(t, errors[i])
		require.JSONEq(t, fmt.Sprintf(`{"name":"Hello contact-%d","items":[%d,false,null]}`, i, i), string(result))
	}

	// A returned byte slice can be changed without affecting subsequent calls.
	results[0][0] = '!'
	again, err := compiled.Render(context.Background(), inputs[0])
	require.NoError(t, err)
	require.True(t, json.Valid(again))
}

// TestValueTemplateRenderFailures applies strict lookup and runtime limits on
// every reuse; successful compilation does not grant unlimited execution.
func TestValueTemplateRenderFailures(t *testing.T) {
	limits := testLimits()
	limits.MaxBytes = 100
	data, err := NewContext(Input{Params: map[string]any{"large": strings.Repeat("x", 60)}}, testLimits())
	require.NoError(t, err)
	for _, source := range []string{
		`[{"$value":"params.large"},{"$value":"params.large"}]`,
		`{"$value":"params.missing"}`,
		`["{{params.large}}","{{params.large}}"]`,
	} {
		// Repeated references share the output cap; missing values return no body.
		compiled, err := CompileValue(common.RawJSON(source), limits)
		require.NoError(t, err)
		result, err := compiled.Render(context.Background(), data)
		require.Error(t, err)
		require.Nil(t, result)
	}

	// Cancellation and absent API inputs fail even for an otherwise literal value.
	compiled, err := CompileValue(common.RawJSON(`null`), limits)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = compiled.Render(ctx, data)
	require.ErrorIs(t, err, context.Canceled)
	_, err = compiled.Render(nil, data)
	require.Error(t, err)
	_, err = compiled.Render(context.Background(), nil)
	require.Error(t, err)
	var absent *ValueTemplate
	_, err = absent.Render(context.Background(), data)
	require.Error(t, err)
}

// TestParameterValuesOptionalReferences preserves positional omission markers
// and keeps false, zero, empty strings, and all supplied data distinguishable.
func TestParameterValuesOptionalReferences(t *testing.T) {
	// Capture omitted, null, falsy, and template-looking values in one snapshot.
	data, err := NewContext(Input{Params: map[string]any{
		"null": nil, "false": false, "zero": 0, "blank": "",
		"list":   []any{"first", "{{cfg.secret}}"},
		"object": map[string]any{"$optional": "cfg.secret"},
	}, Cfg: map[string]any{"secret": "private-configuration"}}, testLimits())
	require.NoError(t, err)
	source := common.RawJSON(`[
		{"$optional":"params.missing"}, {"$optional":"params.pagination.cursor"},
		{"$optional":"params.null"}, {"$optional":"params.null.child"},
		{"$optional":"cfg.secret.child"}, {"$optional":"params.false"},
		{"$optional":"params.zero"}, {"$optional":"params.blank"},
		{"$optional":"params.list"}, {"$optional":"params.object"},
		null, {"$value":"params.null"}, {"$literal":{"$optional":"cfg.secret"}}
	]`)
	program, err := CompileParameterValues(source, testLimits())
	require.NoError(t, err)

	// The compiled program owns its source, and injected data is never reinterpreted.
	for i := range source {
		source[i] = '!'
	}
	result, err := program.Render(context.Background(), data)
	require.NoError(t, err)
	require.JSONEq(t, `[[],[],[],[],[],false,0,"",["first","{{cfg.secret}}"],
		{"$optional":"cfg.secret"},null,null,{"$optional":"cfg.secret"}]`, string(result))
	require.NotContains(t, string(result), "private-configuration")

	// Mutating returned bytes leaves later renders and the captured context intact.
	result[0] = '!'
	again, err := program.Render(context.Background(), data)
	require.NoError(t, err)
	require.True(t, json.Valid(again))
}

// TestParameterValuesRestrictOptionalDirectives catches unsupported placements
// and malformed references before any invocation data is available.
func TestParameterValuesRestrictOptionalDirectives(t *testing.T) {
	for _, source := range []string{
		`null`, `{}`, `"text"`, `{"$optional":"params.cursor"}`,
		`[[{"$optional":"params.cursor"}]]`,
		`[{"nested":{"$optional":"params.cursor"}}]`,
		`[{"$optional":"params.cursor","default":[]}]`,
		`[{"$optional":42}]`, `[{"$optional":""}]`,
		`[{"$optional":"params..cursor"}]`, `[{"$optional":"query.params.cursor"}]`,
	} {
		t.Run(source, func(t *testing.T) {
			// Only exact, whole-field directives belong to the parameter language.
			program, err := CompileParameterValues(common.RawJSON(source), testLimits())
			require.Error(t, err)
			require.Nil(t, program)
		})
	}

	// Ordinary JSON bodies still reject optional directives at every nesting level.
	for _, source := range []string{
		`{"$optional":"params.cursor"}`, `[{"$optional":"params.cursor"}]`,
		`{"cursor":{"$optional":"params.cursor"}}`,
	} {
		program, err := CompileValue(common.RawJSON(source), testLimits())
		require.Error(t, err)
		require.Nil(t, program)
	}

	// A valid parameter program may be empty, but it must still have an array root.
	program, err := CompileParameterValues(common.RawJSON(`[]`), testLimits())
	require.NoError(t, err)
	data, err := NewContext(Input{}, testLimits())
	require.NoError(t, err)
	result, err := program.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, common.RawJSON(`[]`), result)
}

// TestParameterValuesKeepRequiredReferencesStrict proves opting one field into
// omission never suppresses errors from other required lookups or templates.
func TestParameterValuesKeepRequiredReferencesStrict(t *testing.T) {
	data, err := NewContext(Input{Cfg: map[string]any{"secret": "private-configuration"}}, testLimits())
	require.NoError(t, err)
	for _, source := range []string{
		`[{"$optional":"params.cursor"},{"$value":"params.required"}]`,
		`[{"$optional":"params.cursor"},"{{params.required}}"]`,
		`[{"$optional":"params.cursor"},{"$value":"cfg.secret.child"}]`,
	} {
		// An omitted field does not turn subsequent missing references into omissions.
		program, err := CompileParameterValues(common.RawJSON(source), testLimits())
		require.NoError(t, err)
		result, err := program.Render(context.Background(), data)
		require.ErrorIs(t, err, errMissingReference)
		require.Nil(t, result)
		require.NotContains(t, err.Error(), "private-configuration")
	}
}

// TestParameterValuesShareLimits applies one work, byte, and depth allowance to
// optional fields, including omitted fields, and honors invocation cancellation.
func TestParameterValuesShareLimits(t *testing.T) {
	data, err := NewContext(Input{Params: map[string]any{
		"large": strings.Repeat("x", 60), "list": []any{1, 2, 3},
	}}, testLimits())
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		source string
		limits Limits
		error  string
	}{
		{"output", `[{"$optional":"params.large"},{"$optional":"params.large"}]`, Limits{MaxBytes: 100, MaxDepth: 32, MaxSteps: 100}, "byte limit"},
		{"work", `[{"$optional":"params.list"},{"$optional":"params.list"}]`, Limits{MaxBytes: 100, MaxDepth: 32, MaxSteps: 6}, "step limit"},
		{"omitted work", `[{"$optional":"params.a"},{"$optional":"params.b"}]`, Limits{MaxBytes: 100, MaxDepth: 32, MaxSteps: 4}, "step limit"},
		{"depth", `[{"$optional":"params.list"}]`, Limits{MaxBytes: 100, MaxDepth: 1, MaxSteps: 100}, "depth limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Source compilation fits, but the complete rendered program exceeds policy.
			program, err := CompileParameterValues(common.RawJSON(tc.source), tc.limits)
			require.NoError(t, err)
			result, err := program.Render(context.Background(), data)
			require.ErrorContains(t, err, tc.error)
			require.Nil(t, result)
		})
	}

	// A cancelled invocation cannot produce even an omission marker.
	program, err := CompileParameterValues(common.RawJSON(`[{"$optional":"params.cursor"}]`), testLimits())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := program.Render(ctx, data)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
}

// FuzzValueTemplateRender exercises JSON structure, directives, and embedded
// templates together. Successful reusable and one-off renders must agree.
func FuzzValueTemplateRender(f *testing.F) {
	limits := Limits{MaxBytes: 4096, MaxDepth: 8, MaxSteps: 128}
	data, err := NewContext(Input{Params: map[string]any{
		"name": "Ada", "flag": false, "items": []any{1, nil, "{{cfg.secret}}"},
	}}, limits)
	require.NoError(f, err)
	for _, source := range []string{
		`null`, `{"name":"{{params.name}}"}`, `{"$value":"params.items"}`,
		`{"$literal":{"$value":"params.missing"}}`, `{"$value":"params.items","default":[]}`,
		`["{{#json.params.flag}}bad{{/json.params.flag}}"]`, `{"bad":`,
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		// Compile arbitrary JSON and Mustache under a fixed, small resource policy.
		compiled, err := CompileValue(common.RawJSON(source), limits)
		if err != nil {
			require.Nil(t, compiled)
			return
		}
		result, err := compiled.Render(context.Background(), data)
		if err != nil {
			require.Nil(t, result)
			return
		}
		require.True(t, json.Valid(result))
		require.LessOrEqual(t, len(result), limits.MaxBytes)

		// The one-off path has a combined work budget; when it fits, bytes agree.
		once, err := RenderValue(context.Background(), common.RawJSON(source), data, limits)
		if err != nil {
			require.Nil(t, once)
			return
		}
		require.Equal(t, result, once)
	})
}
