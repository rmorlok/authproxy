package render

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// templateLimitsForTest allows ordinary fixtures while keeping every dimension
// explicit. Limit tests tighten one dimension independently.
func templateLimitsForTest() Limits {
	return Limits{MaxBytes: 64 * 1024, MaxDepth: 32, MaxSteps: 10000}
}

// templateContextForTest creates detached JSON input through the public API.
func templateContextForTest(t *testing.T, params map[string]any) *Context {
	t.Helper()
	data, err := NewContext(Input{
		Params: params,
		Cfg:    map[string]any{"tenant": "tenant/one"},
	}, templateLimitsForTest())
	require.NoError(t, err)
	return data
}

// TestTemplateScalarEncodings requires explicit encodings and preserves JSON
// scalar spelling without introducing HTML escaping in any interpolation form.
func TestTemplateScalarEncodings(t *testing.T) {
	// Retain an exact large number and values that permissive templating often loses.
	data := templateContextForTest(t, map[string]any{
		"text": `<a&"b>`, "segment": "a/b c?+", "quote": "a\"b",
		"number": json.Number("9007199254740993"), "flag": false, "nothing": nil,
		"inert": "{{cfg.tenant}}",
	})
	for _, test := range []struct{ source, expected string }{
		{`{{params.text}}|{{{params.text}}}|{{& params.text}}`, `<a&"b>|<a&"b>|<a&"b>`},
		{`{{path.params.segment}}|{{query.params.segment}}`, `a%2Fb%20c%3F+|a%2Fb+c%3F%2B`},
		{`{{json.params.quote}}|{{json.params.flag}}|{{json.params.nothing}}`, `"a\"b"|false|null`},
		{`{{params.number}}|{{params.flag}}|{{params.nothing}}`, `9007199254740993|false|null`},
		{`https://{{cfg.tenant}}`, `https://tenant/one`},
		{`{{params.inert}}`, `{{cfg.tenant}}`},
	} {
		t.Run(test.source, func(t *testing.T) {
			// Compile independently of values, then render the captured context.
			compiled, err := Compile(test.source, templateLimitsForTest())
			require.NoError(t, err)
			result, err := compiled.Render(context.Background(), data)
			require.NoError(t, err)
			require.Equal(t, test.expected, result)
		})
	}
}

// TestTemplateSections resolves nested item scopes before their parents while
// explicit root contexts remain available from every level.
func TestTemplateSections(t *testing.T) {
	// The second group exercises an inverted section without a missing reference.
	data := templateContextForTest(t, map[string]any{"groups": []any{
		map[string]any{"name": "first", "group": "outer", "items": []any{
			map[string]any{"name": "one"}, map[string]any{"name": "two"},
		}},
		map[string]any{"name": "second", "items": []any{}},
	}})
	compiled, err := Compile(`{{#params.groups}}{{name}}:{{#items}}{{name}}/{{group}}/{{cfg.tenant}};{{/items}}{{^items}}empty{{/items}}|{{/params.groups}}`, templateLimitsForTest())
	require.NoError(t, err)
	result, err := compiled.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, "first:one/outer/tenant/one;two/outer/tenant/one;|second:empty|", result)

	// Scalar list elements are available as the current item, including false/null.
	data = templateContextForTest(t, map[string]any{"items": []any{false, nil, "", 0}})
	compiled, err = Compile(`{{#params.items}}[{{.}}]{{/params.items}}`, templateLimitsForTest())
	require.NoError(t, err)
	result, err = compiled.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, "[false][null][][0]", result)
}

// TestTemplateSectionTruthiness follows Mustache JSON semantics rather than
// treating zero, blank strings, or empty objects as absent input.
func TestTemplateSectionTruthiness(t *testing.T) {
	compiled, err := Compile(`{{#params.value}}Y{{/params.value}}{{^params.value}}N{{/params.value}}`, templateLimitsForTest())
	require.NoError(t, err)
	for _, test := range []struct {
		name     string
		value    any
		expected string
	}{
		{"false", false, "N"}, {"null", nil, "N"}, {"empty list", []any{}, "N"},
		{"true", true, "Y"}, {"zero", 0, "Y"}, {"empty string", "", "Y"},
		{"blank string", "  ", "Y"}, {"empty object", map[string]any{}, "Y"},
		{"list items", []any{false, nil, ""}, "YYY"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Both section forms inspect the same present value.
			data := templateContextForTest(t, map[string]any{"value": test.value})
			result, err := compiled.Render(context.Background(), data)
			require.NoError(t, err)
			require.Equal(t, test.expected, result)
		})
	}
}

// TestTemplateStandaloneLines removes only structural-tag lines and preserves
// literal indentation, inline spaces, CRLF handling, and the final newline.
func TestTemplateStandaloneLines(t *testing.T) {
	data := templateContextForTest(t, map[string]any{"yes": true})
	for _, test := range []struct{ source, expected string }{
		{"A\n  {{! discarded}}\r\nB\n{{#params.yes}}\n  C\n{{/params.yes}}\nD\n{{^params.yes}}\n  hidden\n{{/params.yes}}\nE", "A\nB\n  C\nD\nE"},
		{"{{#params.yes}}\nhello\n{{/params.yes}}", "hello\n"},
		{" before {{#params.yes}}inner{{/params.yes}} after", " before inner after"},
		{"A\n \t{{!}}", "A\n"},
		{"{{! first}}{{! second}}\ntext", "\ntext"},
		{strings.Repeat("{{!x}}", 1000) + "\n  {{!y}}\ntext", "\ntext"},
	} {
		// Standalone handling is fixed at compilation and independent of the data.
		compiled, err := Compile(test.source, templateLimitsForTest())
		require.NoError(t, err)
		result, err := compiled.Render(context.Background(), data)
		require.NoError(t, err)
		require.Equal(t, test.expected, result)
	}
}

// TestTemplateDelimiterChanges carries lexical delimiter changes through
// sections and supports both unescaped forms under the active delimiters.
func TestTemplateDelimiterChanges(t *testing.T) {
	data := templateContextForTest(t, map[string]any{"name": "<A>", "yes": true})
	for _, test := range []struct{ source, expected string }{
		{`{{=<% %>=}}<%params.name%>|<%#params.yes%><%{params.name}%><%&params.name%><%/params.yes%><%={{ }}=%>{{params.name}}`, `<A>|<A><A><A>`},
		{"{{=<% %>=}}\n<%#params.yes%>\n<%={{ }}=%>\n{{params.name}}\n{{/params.yes}}", "<A>\n"},
	} {
		// Delimiter declarations do not become output or runtime lookups.
		compiled, err := Compile(test.source, templateLimitsForTest())
		require.NoError(t, err)
		result, err := compiled.Render(context.Background(), data)
		require.NoError(t, err)
		require.Equal(t, test.expected, result)
	}
}

// TestTemplateStrictReferences rejects missing evaluated references and raw
// collections without including invocation data in an error or partial result.
func TestTemplateStrictReferences(t *testing.T) {
	data := templateContextForTest(t, map[string]any{
		"no": false, "object": map[string]any{"secret": "private-sentinel"},
	})
	for _, source := range []string{
		`prefix {{params.missing}}`, `{{#params.missing}}yes{{/params.missing}}`,
		`{{^params.missing}}no{{/params.missing}}`, `{{params.object.missing}}`, `{{params.object}}`,
	} {
		// Valid syntax cannot turn an absent value into a successful empty string.
		compiled, err := Compile(source, templateLimitsForTest())
		require.NoError(t, err)
		result, err := compiled.Render(context.Background(), data)
		require.Error(t, err)
		require.Empty(t, result)
		require.NotContains(t, err.Error(), "private-sentinel")
	}

	// References in a skipped section are not evaluated, as in ordinary Mustache.
	compiled, err := Compile(`{{#params.no}}{{params.missing}}{{/params.no}}safe`, templateLimitsForTest())
	require.NoError(t, err)
	result, err := compiled.Render(context.Background(), data)
	require.NoError(t, err)
	require.Equal(t, "safe", result)
}

// TestTemplateRejectsMalformedAndExternalSyntax rejects malformed structure and
// all partials before rendering, including partials hidden in inactive sections.
func TestTemplateRejectsMalformedAndExternalSyntax(t *testing.T) {
	for _, source := range []string{
		`{{`, `{{{name}}`, `{{}}`, `{{&}}`, `{{#}}`, `{{/name}}`,
		`{{#name}}`, `{{#a}}{{/b}}`, `{{^a}}{{/b}}`,
		`{{> /etc/passwd}}`, `{{#no}}{{> hidden}}{{/no}}`,
		`{{< layout}}{{/layout}}`, `{{$ block}}content{{/block}}`,
		`{{=bad}}`, `{{= one =}}`, `{{= a b c =}}`, `{{= a= b =}}`,
		`{{=<% %>=}}<%>partial%>`,
		string([]byte{'a', 0xff}),
	} {
		// Compilation never touches an invocation context or server-local file.
		compiled, err := Compile(source, templateLimitsForTest())
		require.Error(t, err, source)
		require.Nil(t, compiled, source)
	}
}

// TestTemplateLimits bounds source, nesting, total output, and execution work.
// Empty sections still consume work so large lists cannot evade the step cap.
func TestTemplateLimits(t *testing.T) {
	data := templateContextForTest(t, map[string]any{
		"text": strings.Repeat("x", 100), "items": make([]any, 100), "yes": true,
	})
	limits := templateLimitsForTest()

	// Reject invalid policies and source that cannot fit the compilation budget.
	_, err := Compile("literal", Limits{})
	require.Error(t, err)
	limits.MaxBytes = 4
	_, err = Compile("literal", limits)
	require.Error(t, err)

	// Every syntactic nesting level counts even when its section would be false.
	limits = templateLimitsForTest()
	limits.MaxDepth = 1
	_, err = Compile(`{{#params.yes}}{{#params.yes}}x{{/params.yes}}{{/params.yes}}`, limits)
	require.Error(t, err)

	// Output is bounded after rendering, not merely by the source length.
	limits = templateLimitsForTest()
	limits.MaxBytes = 32
	compiled, err := Compile(`{{params.text}}`, limits)
	require.NoError(t, err)
	result, err := compiled.Render(context.Background(), data)
	require.Error(t, err)
	require.Empty(t, result)

	// A section with no output must still stop at the bounded iteration count.
	limits = templateLimitsForTest()
	limits.MaxSteps = 4
	compiled, err = Compile(`{{#params.items}}{{/params.items}}`, limits)
	require.NoError(t, err)
	result, err = compiled.Render(context.Background(), data)
	require.Error(t, err)
	require.Empty(t, result)

	// Compilation also bounds otherwise output-free syntax such as comments.
	_, err = Compile(strings.Repeat(`{{!comment}}`, 5), limits)
	require.Error(t, err)
}

// TestTemplateCancellationAndSharedBudget rejects canceled empty renders and
// shares emitted-byte accounting across embedded structured-value strings.
func TestTemplateCancellationAndSharedBudget(t *testing.T) {
	data := templateContextForTest(t, map[string]any{"text": "123456"})
	compiled, err := Compile("", templateLimitsForTest())
	require.NoError(t, err)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	// Even an empty template observes cancellation without returning success.
	_, err = compiled.Render(canceled, data)
	require.ErrorIs(t, err, context.Canceled)
	_, err = compiled.Render(nil, data)
	require.Error(t, err)
	_, err = compiled.Render(context.Background(), nil)
	require.Error(t, err)
	var absent *Template
	_, err = absent.Render(context.Background(), data)
	require.Error(t, err)

	// Several string templates consume one shared budget in a structured body.
	compiled, err = Compile(`{{params.text}}`, templateLimitsForTest())
	require.NoError(t, err)
	limits := templateLimitsForTest()
	limits.MaxBytes = 10
	shared := newBudget(context.Background(), limits)
	result, err := compiled.renderWithBudget(data, shared)
	require.NoError(t, err)
	require.Equal(t, "123456", result)
	result, err = compiled.renderWithBudget(data, shared)
	require.Error(t, err)
	require.Empty(t, result)
}

// TestTemplateConcurrentReuse keeps the compiled program and captured context
// independent of each call's output buffers, scopes, and limits.
func TestTemplateConcurrentReuse(t *testing.T) {
	data := templateContextForTest(t, map[string]any{"items": []any{"a", "b"}})
	compiled, err := Compile(`{{#params.items}}{{.}}{{/params.items}}`, templateLimitsForTest())
	require.NoError(t, err)
	results := make(chan string, 20)
	var workers sync.WaitGroup

	// Share only immutable inputs; each invocation owns its rendering budget.
	for range cap(results) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, err := compiled.Render(context.Background(), data)
			if err != nil {
				results <- err.Error()
				return
			}
			results <- result
		}()
	}
	workers.Wait()
	close(results)
	for result := range results {
		require.Equal(t, "ab", result)
	}
}

// FuzzTemplateRender exercises arbitrary tag structure under small fixed
// budgets. Successful renders remain valid, bounded text; errors return no data.
func FuzzTemplateRender(f *testing.F) {
	limits := Limits{MaxBytes: 4096, MaxDepth: 8, MaxSteps: 128}
	data, err := NewContext(Input{Params: map[string]any{
		"value": "<text>", "yes": true, "no": false, "items": []any{"a", "b"},
	}}, limits)
	require.NoError(f, err)
	for _, source := range []string{
		"", "plain text", "{{params.value}}", "{{{params.value}}}",
		"{{#params.items}}[{{.}}]{{/params.items}}", "{{^params.no}}yes{{/params.no}}",
		"  {{!comment}}\r\n{{=<% %>=}}<%params.value%>",
		"{{#params.no}}{{>partial}}{{/params.no}}", "{{#a}}{{/b}}", "{{",
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		// Compilation must safely reject syntax and source beyond its bounds.
		compiled, err := Compile(source, limits)
		if err != nil {
			require.Nil(t, compiled)
			return
		}

		// Rendering either returns bounded UTF-8 or discards all partial output.
		result, err := compiled.Render(context.Background(), data)
		if err != nil {
			require.Empty(t, result)
			return
		}
		require.LessOrEqual(t, len(result), limits.MaxBytes)
		require.True(t, utf8.ValidString(result))
	})
}
