package render

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
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
		Params: map[string]any{
			"items": []any{
				map[string]any{
					"name": "original",
				},
			},
		},
		Cfg: map[string]any{
			"tenant": "acme",
			"nested": map[string]any{
				"region": "us",
			},
		},
		Labels: map[string]string{
			"team": "blue",
		},
		Annotations: map[string]string{
			"note": "original",
		},
		ConnectionID:        "cxn_example",
		ConnectionNamespace: "root.product.user",
		ToolID:              "tol_example",
		Revision:            9007199254740993,
		ToolSetID:           "tls_example",
		ToolSetGeneration:   &generation,
		CatalogRevision:     &catalog,
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

func TestContextRejectsEmptyAndDotPathSegments(t *testing.T) {
	for _, segment := range []string{"", ".", ".."} {
		t.Run("segment="+segment, func(t *testing.T) {
			// Empty and dot segments can change URL routing even after escaping.
			data, err := NewContext(Input{Params: map[string]any{"id": segment}}, testLimits())
			require.NoError(t, err)
			template, err := Compile(`https://example.com/items/{{path.params.id}}/details`, testLimits())
			require.NoError(t, err)

			// Fail rendering before an HTTP caller can send the ambiguous path.
			_, err = template.Render(context.Background(), data)
			require.ErrorContains(t, err, "nonempty, non-dot path segment")
		})
	}

	// Dots within an identifier and slashes within one segment remain supported.
	data, err := NewContext(Input{Params: map[string]any{"id": "../v1.2"}}, testLimits())
	require.NoError(t, err)
	require.Equal(t, "/items/..%2Fv1.2", renderText(t, `/items/{{path.params.id}}`, data))
}

func TestContextEncodingNamesInSectionScopes(t *testing.T) {
	// Ordinary JSON data may use the same names as the three encoding views.
	data, err := NewContext(Input{Params: map[string]any{
		"query": map[string]any{"text": "find & keep"},
		"path":  map[string]any{"id": "a/b"},
		"json":  map[string]any{"value": "plain"},
	}}, testLimits())
	require.NoError(t, err)

	// Unqualified nested fields remain ordinary references inside the section.
	source := `{{#params}}{{query.text}}|{{path.id}}|{{json.value}}{{/params}}`
	require.Equal(t, "find & keep|a/b|plain", renderText(t, source, data))
	sections := `{{#params}}{{#query.text}}{{.}}{{/query.text}}|{{#path.id}}{{.}}{{/path.id}}|{{#json.value}}{{.}}{{/json.value}}{{/params}}`
	require.Equal(t, "find & keep|a/b|plain", renderText(t, sections, data))
}

func TestContextEncodedReferencesUseInvocationRoots(t *testing.T) {
	// A section can shadow ordinary roots, but encoded references have explicit
	// invocation-root names and must retain those roots' captured values.
	data, err := NewContext(Input{
		Params: map[string]any{
			"id": "root/id",
			"item": map[string]any{
				"params": map[string]any{"id": "inner/id"},
				"cfg":    map[string]any{"tenant": "inner tenant"},
			},
		},
		Cfg: map[string]any{"tenant": "root tenant"},
	}, testLimits())
	require.NoError(t, err)
	source := `{{#params.item}}{{params.id}}|{{path.params.id}}|{{query.cfg.tenant}}|{{json.cfg.tenant}}{{/params.item}}`
	require.Equal(t, `inner/id|root%2Fid|root+tenant|"root tenant"`, renderText(t, source, data))

	// Bare invocation-root references also select the corresponding JSON view.
	require.JSONEq(t, `{"tenant":"root tenant"}`, renderText(t, `{{#params.item}}{{json.cfg}}{{/params.item}}`, data))
}

func TestContextMissingIdentity(t *testing.T) {
	// Constructing a partial context is allowed, but absent identity stays absent.
	data, err := NewContext(Input{}, testLimits())
	require.NoError(t, err)
	require.Equal(t, "{}|{}", renderText(t, `{{json.connection}}|{{json.tool}}`, data))
	for _, name := range []string{"connection.id", "connection.namespace", "tool.id", "tool.revision"} {
		t.Run(name, func(t *testing.T) {
			// A required identity must fail lookup rather than silently render empty.
			template, err := Compile("{{"+name+"}}", testLimits())
			require.NoError(t, err)
			_, err = template.Render(context.Background(), data)
			require.ErrorContains(t, err, "missing template reference")
		})
	}
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
		new(string), []byte("private-configuration"), []string{"private-configuration"}, math.NaN(), math.Inf(1),
		json.Number(""), json.Number("01"), json.Number("private-configuration"), cycle, string([]byte{0xff}),
	} {
		// Reject arbitrary Go values without invoking marshalers or leaking values.
		_, err := NewContext(Input{Cfg: map[string]any{"private-field": value}}, testLimits())
		require.Error(t, err)
		require.ErrorContains(t, err, "context field cfg:")
		require.NotContains(t, err.Error(), "private-configuration")
		require.NotContains(t, err.Error(), "private-field")
	}
	// Invalid input identifies the params scope using only the fixed root name.
	_, err := NewContext(Input{Params: map[string]any{"private-field": func() {}}}, testLimits())
	require.ErrorContains(t, err, "context field params:")
	require.NotContains(t, err.Error(), "private-field")
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

func TestContextSnapshotJSON(t *testing.T) {
	// Template-shaped strings and objects are captured as ordinary data.
	data, err := NewContext(Input{
		Params: map[string]any{
			"template":  "{{cfg.private}}",
			"directive": map[string]any{"$value": "cfg.private"},
			"number":    json.Number("9007199254740993.000"),
			"null":      nil,
		},
		Cfg: map[string]any{"private": "captured"},
	}, testLimits())
	require.NoError(t, err)

	// Snapshot encoding preserves source data and exact number tokens.
	encoded, err := data.SnapshotJSON(context.Background(), testLimits())
	require.NoError(t, err)
	require.JSONEq(t, `{"params":{"template":"{{cfg.private}}","directive":{"$value":"cfg.private"},"number":9007199254740993.000,"null":null},"cfg":{"private":"captured"},"labels":{},"annotations":{},"connection":{},"tool":{}}`, string(encoded))
	require.Contains(t, string(encoded), "9007199254740993.000")

	// Mutating one returned encoding cannot alter future snapshots.
	encoded[0] = '['
	again, err := data.SnapshotJSON(context.Background(), testLimits())
	require.NoError(t, err)
	require.Equal(t, byte('{'), again[0])
}

func TestContextSnapshotJSONLimits(t *testing.T) {
	// Encode a small known tree to exercise exact serialized-byte boundaries.
	data, err := NewContext(Input{Params: map[string]any{"value": []any{"quoted\nvalue"}}}, testLimits())
	require.NoError(t, err)
	encoded, err := data.SnapshotJSON(context.Background(), testLimits())
	require.NoError(t, err)
	limits := testLimits()
	limits.MaxBytes = len(encoded)
	exact, err := data.SnapshotJSON(context.Background(), limits)
	require.NoError(t, err)
	require.Equal(t, encoded, exact)

	// Each boundary fails without returning a partial serialized snapshot.
	for _, limit := range []Limits{
		{MaxBytes: len(encoded) - 1, MaxDepth: 32, MaxSteps: 10000},
		{MaxBytes: 1 << 20, MaxDepth: 2, MaxSteps: 10000},
		{MaxBytes: 1 << 20, MaxDepth: 32, MaxSteps: 8},
		{},
	} {
		partial, err := data.SnapshotJSON(context.Background(), limit)
		require.Error(t, err)
		require.Nil(t, partial)
	}

	// Cancellation and missing required arguments are also ordinary errors.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	partial, err := data.SnapshotJSON(canceled, testLimits())
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, partial)
	_, err = data.SnapshotJSON(nil, testLimits())
	require.Error(t, err)
	_, err = (*Context)(nil).SnapshotJSON(context.Background(), testLimits())
	require.Error(t, err)
}

// TestContextSnapshotJSONSmallerLimits applies a tighter consumer policy to a
// previously captured snapshot, including large strings and wide objects.
func TestContextSnapshotJSONSmallerLimits(t *testing.T) {
	wide := make(map[string]any, 1024)
	for i := range 1024 {
		wide[strconv.Itoa(i)] = nil
	}
	for _, test := range []struct {
		name   string
		params map[string]any
		limits Limits
	}{
		{"large string", map[string]any{"text": strings.Repeat("x", 4096)}, Limits{MaxBytes: 256, MaxDepth: 32, MaxSteps: 10000}},
		{"wide map", wide, Limits{MaxBytes: 1 << 20, MaxDepth: 32, MaxSteps: 32}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Initial capture succeeds under the broader host policy.
			data, err := NewContext(Input{Params: test.params}, testLimits())
			require.NoError(t, err)

			// A tighter caller gets no partial JSON and does not alter the snapshot.
			result, err := data.SnapshotJSON(context.Background(), test.limits)
			require.Error(t, err)
			require.Nil(t, result)
			result, err = data.SnapshotJSON(context.Background(), testLimits())
			require.NoError(t, err)
			require.True(t, json.Valid(result))
		})
	}
}
