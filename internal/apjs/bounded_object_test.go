package apjs

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// boundedObjectTestLimits allows useful patches while requiring finite policy
// for every stage of copied input, JavaScript execution, and JSON conversion.
func boundedObjectTestLimits() jsruntime.Limits {
	limits := boundedPredicateTestLimits()
	limits.MaxOutputBytes = 4096
	return limits
}

// TestBoundedObjectCompatibility preserves non-strict helper and statement-list
// semantics, JSON patch values, and null/undefined as an empty patch.
func TestBoundedObjectCompatibility(t *testing.T) {
	library, err := CompileLibrary(`
		const initializedWithoutConfig = typeof cfg === "undefined";
		function patch() {
			return {config: {set: {name: cfg.name, integer: 9007199254740991, decimal: 1.25}},
				labels: {set: {team: labels.team}}, initializedWithoutConfig};
		}
	`)
	require.NoError(t, err)
	jsctx := library.NewContext(map[string]any{
		"cfg": map[string]any{"name": "tenant"}, "labels": map[string]string{"team": "platform"},
	})

	// Existing helpers initialize without input globals and read them when called.
	source := `assignedWithoutDeclaration = patch(); assignedWithoutDeclaration;`
	got, err := jsctx.EvaluateObjectJSONWithContext(context.Background(), source, boundedObjectTestLimits())
	require.NoError(t, err)
	require.JSONEq(t, `{"config":{"set":{"name":"tenant","integer":9007199254740991,"decimal":1.25}},"labels":{"set":{"team":"platform"}},"initializedWithoutConfig":true}`, string(got))
	require.Contains(t, string(got), "9007199254740991")

	// Omitted results retain the migration contract instead of becoming invalid JSON output.
	for _, source := range []string{"null", "undefined", "void 0", "var unused = 1;", "({})", "Object.create(null)"} {
		got, err := NewContext(nil, nil).EvaluateObjectJSONWithContext(context.Background(), source, boundedObjectTestLimits())
		require.NoError(t, err)
		require.Equal(t, "{}", string(got))
	}
}

// TestBoundedObjectUndefinedProperties preserves optional-field copying by
// omitting undefined object properties at every depth while retaining nulls.
func TestBoundedObjectUndefinedProperties(t *testing.T) {
	// Missing input fields disappear even inside objects nested in arrays.
	got, err := NewContext(nil, map[string]any{"cfg": map[string]any{}}).EvaluateObjectJSONWithContext(
		context.Background(), `({
			missing: cfg.optional,
			nested: {first: undefined, kept: null, last: undefined},
			items: [{optional: cfg.optional, present: 1}],
			empty: {optional: cfg.optional}
		})`, boundedObjectTestLimits(),
	)
	require.NoError(t, err)
	require.JSONEq(t, `{"nested":{"kept":null},"items":[{"present":1}],"empty":{}}`, string(got))
}

// TestBoundedObjectOwnership keeps mutations inside one VM and returns JSON
// whose storage does not alias either later results or connection-owned input.
func TestBoundedObjectOwnership(t *testing.T) {
	cfg := map[string]any{"nested": map[string]any{"name": "original"}, "items": []any{"first"}}
	labels := map[string]string{"team": "platform"}
	jsctx := NewContext(nil, map[string]any{"cfg": cfg, "labels": labels})

	// Authored writes affect native copied containers and appear only in this result.
	got, err := jsctx.EvaluateObjectJSONWithContext(context.Background(), `
		cfg.nested.name = "local"; cfg.items.push("second"); labels.team = "local";
		({cfg, labels, nativeArray: Array.isArray(cfg.items), noFetch: typeof fetch === "undefined"});
	`, boundedObjectTestLimits())
	require.NoError(t, err)
	require.JSONEq(t, `{"cfg":{"nested":{"name":"local"},"items":["first","second"]},"labels":{"team":"local"},"nativeArray":true,"noFetch":true}`, string(got))
	require.Equal(t, map[string]any{"name": "original"}, cfg["nested"])
	require.Equal(t, []any{"first"}, cfg["items"])
	require.Equal(t, "platform", labels["team"])

	// A caller can modify returned bytes without affecting a fresh execution.
	got[0] = '!'
	again, err := jsctx.EvaluateObjectJSONWithContext(context.Background(), `({name: cfg.nested.name, team: labels.team})`, boundedObjectTestLimits())
	require.NoError(t, err)
	require.JSONEq(t, `{"name":"original","team":"platform"}`, string(again))
}

// TestBoundedObjectFailures rejects unsupported output without traversing live
// Goja graphs or invoking authored getters and never returns partial bytes.
func TestBoundedObjectFailures(t *testing.T) {
	for name, test := range map[string]struct {
		library string
		source  string
		want    error
	}{
		"syntax":           {source: "const privateDetail = ;", want: jsruntime.ErrInvalidSource},
		"throw":            {source: `throw new Error("privateDetail")`, want: jsruntime.ErrEvaluation},
		"library throw":    {library: `throw "privateDetail";`, source: "({})", want: jsruntime.ErrEvaluation},
		"reserved binding": {library: `const cfg = "privateDetail";`, source: "({})", want: jsruntime.ErrInvalidSource},
		"array root":       {source: `[{privateDetail: true}]`, want: jsruntime.ErrInvalidOutput},
		"primitive root":   {source: `"privateDetail"`, want: jsruntime.ErrInvalidOutput},
		"getter":           {source: `({get privateDetail() { for (;;) {} }})`, want: jsruntime.ErrInvalidOutput},
		"proxy":            {source: `new Proxy({}, {ownKeys() { for (;;) {} }})`, want: jsruntime.ErrInvalidOutput},
		"cycle":            {source: `const result = {}; result.privateDetail = result; result;`, want: jsruntime.ErrInvalidOutput},
		"array undefined":  {source: `({privateDetail: [undefined]})`, want: jsruntime.ErrInvalidOutput},
		"custom prototype": {source: `Object.create({privateDetail: true})`, want: jsruntime.ErrInvalidOutput},
	} {
		t.Run(name, func(t *testing.T) {
			library, err := CompileLibrary(test.library)
			require.NoError(t, err)

			// Even deliberate getter/proxy loops must be rejected without executing them.
			got, err := library.NewContext(nil).EvaluateObjectJSONWithContext(context.Background(), test.source, boundedObjectTestLimits())
			require.ErrorIs(t, err, test.want)
			require.NotContains(t, err.Error(), "privateDetail")
			require.Nil(t, got)
		})
	}
}

// TestBoundedObjectLimits verifies that object conversion shares the bounded
// execution policy, including byte-exact output and traversal limits.
func TestBoundedObjectLimits(t *testing.T) {
	limits := boundedObjectTestLimits()
	limits.MaxOutputBytes = len(`{"value":"x"}`)
	got, err := NewContext(nil, nil).EvaluateObjectJSONWithContext(context.Background(), `({value: "x"})`, limits)
	require.NoError(t, err)
	require.Equal(t, `{"value":"x"}`, string(got))

	for name, test := range map[string]struct {
		source string
		vars   map[string]any
		limit  func(*jsruntime.Limits)
	}{
		"output bytes": {source: `({value: "x"})`, limit: func(l *jsruntime.Limits) { l.MaxOutputBytes = len(`{"value":"x"}`) - 1 }},
		"input bytes":  {source: "({})", vars: map[string]any{"padding": strings.Repeat("x", 100)}, limit: func(l *jsruntime.Limits) { l.MaxInputBytes = 100 }},
		"source bytes": {source: "({})", limit: func(l *jsruntime.Limits) { l.MaxSourceBytes = 3 }},
		"depth":        {source: "({a: {b: {c: 1}}})", limit: func(l *jsruntime.Limits) { l.MaxDepth = 1 }},
		"nodes":        {source: "({a: 1, b: 2})", limit: func(l *jsruntime.Limits) { l.MaxNodes = 2 }},
		"stack":        {source: "function recur() { return recur(); } recur();", limit: func(l *jsruntime.Limits) { l.MaxStackDepth = 16 }},
		"execution":    {source: "for (;;) {}", limit: func(l *jsruntime.Limits) { l.Timeout = 50 * time.Millisecond }},
	} {
		t.Run(name, func(t *testing.T) {
			limits := boundedObjectTestLimits()
			test.limit(&limits)

			// Every limit discards the entire prospective patch.
			got, err := NewContext(nil, test.vars).EvaluateObjectJSONWithContext(context.Background(), test.source, limits)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, got)
		})
	}
}

// TestBoundedObjectCancellation retains caller cancellation before parsing and
// during execution while requiring an explicit valid context and finite policy.
func TestBoundedObjectCancellation(t *testing.T) {
	jsctx := NewContext(nil, nil)
	limits := boundedObjectTestLimits()
	got, err := jsctx.EvaluateObjectJSONWithContext(nil, "({})", limits)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, got)

	// Missing output policy is rejected rather than silently becoming unlimited.
	invalid := limits
	invalid.MaxOutputBytes = 0
	got, err = jsctx.EvaluateObjectJSONWithContext(context.Background(), "({})", invalid)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, got)

	// A canceled caller wins over invalid source and input policy.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = jsctx.EvaluateObjectJSONWithContext(ctx, "privateDetail ===", invalid)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got)

	// The request deadline ends authored work before its larger execution timeout.
	ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	got, err = jsctx.EvaluateObjectJSONWithContext(ctx, "for (;;) {}", limits)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, got)
}

// TestBoundedObjectConcurrentReuse checks that compiled helpers can produce
// detached object results concurrently without carrying mutable VM state.
func TestBoundedObjectConcurrentReuse(t *testing.T) {
	library, err := CompileLibrary(`let count = 0; function patch() { return {count: ++count}; }`)
	require.NoError(t, err)
	results := make(chan error, 8)
	var wg sync.WaitGroup

	// Each result should observe the first invocation in its own new VM.
	for range cap(results) {
		wg.Go(func() {
			got, err := library.NewContext(nil).EvaluateObjectJSONWithContext(context.Background(), "patch()", boundedObjectTestLimits())
			if err == nil && string(got) != `{"count":1}` {
				err = fmt.Errorf("unexpected shared state: %s", got)
			}
			results <- err
		})
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
}
