package render

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// testLimits permits normal fixtures while keeping all renderer limits explicit.
func testLimits() Limits {
	return Limits{MaxBytes: 1 << 20, MaxDepth: 32, MaxSteps: 10000}
}

// renderText exercises the public compiler and renderer against an invocation.
func renderText(t *testing.T, source string, data *Context) string {
	t.Helper()
	template, err := Compile(source, testLimits())
	require.NoError(t, err)
	result, err := template.Render(context.Background(), data)
	require.NoError(t, err)
	return result
}

func TestContextSnapshot(t *testing.T) {
	// Capture nested input, connection metadata, and immutable revision identity.
	generation, catalog := uint64(7), uint64(12)
	input := Input{
		Params: map[string]any{"items": []any{map[string]any{"name": "original"}}},
		Cfg:    map[string]any{"tenant": "acme", "nested": map[string]any{"region": "us"}},
		Labels: map[string]string{"team": "blue"}, Annotations: map[string]string{"note": "original"},
		ConnectionID: "cxn_example", ConnectionNamespace: "root.product.user",
		ToolID: "tol_example", Revision: 9007199254740993,
		ToolSetID: "tls_example", ToolSetGeneration: &generation, CatalogRevision: &catalog,
	}
	data, err := NewContext(input, testLimits())
	require.NoError(t, err)

	// Later edits to every caller-owned container leave the captured values intact.
	input.Params["items"].([]any)[0].(map[string]any)["name"] = "changed"
	input.Cfg["nested"].(map[string]any)["region"] = "eu"
	input.Labels["team"], input.Annotations["note"] = "red", "changed"
	generation, catalog = 8, 13
	result := renderText(t, `{{#params.items}}{{name}}{{/params.items}} {{cfg.nested.region}} {{labels.team}} {{annotations.note}} {{connection.id}} {{connection.namespace}} {{tool.id}} {{tool.revision}} {{tool.toolSetId}} {{tool.toolSetGeneration}} {{tool.catalogRevision}}`, data)
	require.Equal(t, "original us blue original cxn_example root.product.user tol_example 9007199254740993 tls_example 7 12", result)

	// The safe connection projection contains only identity, never runtime handles.
	require.JSONEq(t, `{"id":"cxn_example","namespace":"root.product.user"}`, renderText(t, `{{json.connection}}`, data))
}

func TestContextEncodings(t *testing.T) {
	// Values include delimiters, non-ASCII text, HTML characters, and precise numbers.
	data, err := NewContext(Input{Params: map[string]any{
		"id": "a/b c?&é", "text": `"<&>`, "number": json.Number("9007199254740993.000"),
		"object": map[string]any{"child": "nested"}, "null": nil, "false": false,
	}}, testLimits())
	require.NoError(t, err)
	require.Equal(t, "a%2Fb%20c%3F&%C3%A9", renderText(t, `{{path.params.id}}`, data))
	require.Equal(t, "a%2Fb+c%3F%26%C3%A9", renderText(t, `{{query.params.id}}`, data))
	require.Equal(t, `"<&>`, renderText(t, `{{params.text}}`, data))
	require.Equal(t, "9007199254740993.000 false null", renderText(t, `{{params.number}} {{params.false}} {{params.null}}`, data))
	require.JSONEq(t, `{"child":"nested"}`, renderText(t, `{{json.params.object}}`, data))
	require.JSONEq(t, `"nested"`, renderText(t, `{{json.params.object.child}}`, data))
	require.JSONEq(t, `"\"<&>"`, renderText(t, `{{json.params.text}}`, data))
	require.Equal(t, "9007199254740993.000", renderText(t, `{{json.params.number}}`, data))
}

func TestContextStrictLookup(t *testing.T) {
	data, err := NewContext(Input{Params: map[string]any{"object": map[string]any{}, "null": nil}}, testLimits())
	require.NoError(t, err)
	for _, source := range []string{
		`{{params.missing}}`, `{{params.null.child}}`, `{{params..object}}`,
		`{{path.params.missing}}`, `{{params.object}}`, `{{query.params.object}}`, `{{tool.toolSetId}}`,
	} {
		t.Run(source, func(t *testing.T) {
			// Present null is valid, but absent references and raw objects must fail.
			template, err := Compile(source, testLimits())
			if err == nil {
				_, err = template.Render(context.Background(), data)
			}
			require.Error(t, err)
		})
	}

	// An inner object shadows the whole outer object, not just its present fields.
	data, err = NewContext(Input{Params: map[string]any{
		"person": map[string]any{"name": "outer"},
		"items":  []any{map[string]any{"person": map[string]any{}}},
	}}, testLimits())
	require.NoError(t, err)
	template, err := Compile(`{{#params}}{{#items}}{{person.name}}{{/items}}{{/params}}`, testLimits())
	require.NoError(t, err)
	_, err = template.Render(context.Background(), data)
	require.ErrorContains(t, err, "missing template reference")
}

func TestContextRejectsNonJSONAndBounds(t *testing.T) {
	cycle := map[string]any{}
	cycle["cycle"] = cycle
	for _, value := range []any{
		func() {}, make(chan int), struct{ Secret string }{"private-configuration"},
		new(string), []byte("private-configuration"), math.NaN(), math.Inf(1),
		json.Number(""), json.Number("01"), json.Number("private-configuration"), cycle, string([]byte{0xff}),
	} {
		// Reject arbitrary Go values without invoking marshalers or leaking values.
		_, err := NewContext(Input{Cfg: map[string]any{"value": value}}, testLimits())
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private-configuration")
	}
	for _, limits := range []Limits{
		{}, {MaxBytes: 1, MaxDepth: 32, MaxSteps: 10000},
		{MaxBytes: 1 << 20, MaxDepth: 1, MaxSteps: 10000},
		{MaxBytes: 1 << 20, MaxDepth: 32, MaxSteps: 1},
	} {
		// Limits apply while copying, before a render can retain the snapshot.
		_, err := NewContext(Input{Params: map[string]any{"large": strings.Repeat("x", 100)}}, limits)
		require.Error(t, err)
	}
}

func TestContextNilCollections(t *testing.T) {
	// Go nil collections encode as JSON null; only absent root scopes are empty.
	data, err := NewContext(Input{Params: map[string]any{
		"map": map[string]any(nil), "labels": map[string]string(nil), "list": []any(nil),
	}}, testLimits())
	require.NoError(t, err)
	require.Equal(t, "null|null|null", renderText(t, `{{json.params.map}}|{{json.params.labels}}|{{json.params.list}}`, data))
	require.Equal(t, "empty", renderText(t, `{{^params.map}}empty{{/params.map}}`, data))
	require.Equal(t, "{}|{}|{}", renderText(t, `{{json.cfg}}|{{json.labels}}|{{json.annotations}}`, data))
}
