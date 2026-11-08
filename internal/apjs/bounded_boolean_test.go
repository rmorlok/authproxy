package apjs

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// boundedPredicateTestLimits leaves room for connector examples while requiring
// explicit finite bounds for every shared-runtime resource.
func boundedPredicateTestLimits() jsruntime.Limits {
	return jsruntime.Limits{
		MaxSourceBytes: 64 << 10, MaxInputBytes: 64 << 10, MaxOutputBytes: 64,
		MaxDepth: 32, MaxNodes: 10000, MaxStackDepth: 128, Timeout: time.Second,
	}
}

// TestBoundedBooleanCompatibility preserves connector helper lookup, statement
// lists, trailing semicolons, and the legacy non-strict execution mode.
func TestBoundedBooleanCompatibility(t *testing.T) {
	library, err := CompileLibrary(`
		function eligible() { return cfg.region === "eu" && labels.env === "prod"; }
	`)
	require.NoError(t, err)
	jsctx := library.NewContext(map[string]any{
		"cfg":         map[string]any{"region": "eu"},
		"labels":      map[string]string{"env": "prod"},
		"annotations": map[string]string{"mode": "advanced"},
	})

	for _, source := range []string{
		`eligible() && annotations.mode === "advanced"`,
		`const selected = eligible(); selected;`,
		`assignedWithoutDeclaration = eligible(); assignedWithoutDeclaration;`,
		`with (cfg) { region === "eu"; }`,
	} {
		t.Run(source, func(t *testing.T) {
			// Existing accepted syntax produces the same primitive Boolean in both APIs.
			legacy, err := jsctx.EvaluateBoolean(source)
			require.NoError(t, err)
			bounded, err := jsctx.EvaluateBooleanWithContext(context.Background(), source, boundedPredicateTestLimits())
			require.NoError(t, err)
			require.True(t, bounded)
			require.Equal(t, legacy, bounded)
		})
	}

	// False is a valid predicate result rather than an evaluation failure.
	result, err := jsctx.EvaluateBooleanWithContext(context.Background(), `cfg.region === "us"`, boundedPredicateTestLimits())
	require.NoError(t, err)
	require.False(t, result)
}

// TestBoundedBooleanBoxedResults preserves the legacy scalar export of native
// Boolean wrappers without invoking authored coercion or property accessors.
func TestBoundedBooleanBoxedResults(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		want   bool
	}{
		"true":  {source: `new Boolean(true)`, want: true},
		"false": {source: `new Boolean(false)`, want: false},
		"coercion changed": {
			source: `Boolean.prototype.valueOf = () => { throw "private coercion"; }; new Boolean(true);`,
			want:   true,
		},
		"own getter": {
			source: `const boxed = new Boolean(false); Object.defineProperty(boxed, "private", {get() { for (;;) {} }}); boxed;`,
			want:   false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			// Native Boolean export reads its internal scalar value rather than invoking valueOf.
			jsctx := NewContext(nil, nil)
			legacy, err := jsctx.EvaluateBoolean(test.source)
			require.NoError(t, err)
			result, err := jsctx.EvaluateBooleanWithContext(context.Background(), test.source, boundedPredicateTestLimits())
			require.NoError(t, err)
			require.Equal(t, test.want, result)
			require.Equal(t, legacy, result)
		})
	}
}

// TestBoundedBooleanCopiesNativeInputs makes authored mutations local to one VM
// and supplies real JavaScript arrays instead of reflected Go containers.
func TestBoundedBooleanCopiesNativeInputs(t *testing.T) {
	cfg := map[string]any{"items": []any{"original"}, "nested": map[string]any{"value": "original"}}
	labels := map[string]string{"env": "prod"}
	jsctx := NewContext(nil, map[string]any{"cfg": cfg, "labels": labels})

	// Native array methods and nested assignments must never write to caller-owned data.
	result, err := jsctx.EvaluateBooleanWithContext(context.Background(), `
		cfg.items.push("local"); cfg.nested.value = "local"; labels.env = "local";
		Array.isArray(cfg.items) && cfg.items.length === 2 && labels.env === "local";
	`, boundedPredicateTestLimits())
	require.NoError(t, err)
	require.True(t, result)
	require.Equal(t, []any{"original"}, cfg["items"])
	require.Equal(t, map[string]any{"value": "original"}, cfg["nested"])
	require.Equal(t, map[string]string{"env": "prod"}, labels)

	// A later evaluation starts from the original snapshot, not the previous VM's edits.
	result, err = jsctx.EvaluateBooleanWithContext(context.Background(), `
		cfg.items.length === 1 && cfg.nested.value === "original" && labels.env === "prod";
	`, boundedPredicateTestLimits())
	require.NoError(t, err)
	require.True(t, result)
}

// TestBoundedBooleanLibraryInitialization preserves the legacy ordering while
// protecting native JSON parsing from later library changes to global helpers.
func TestBoundedBooleanLibraryInitialization(t *testing.T) {
	library, err := CompileLibrary(`
		const initiallyMissing = typeof cfg === "undefined";
		JSON.parse = () => { throw "private parser replacement"; };
		function allowed() { return initiallyMissing && cfg.enabled; }
	`)
	require.NoError(t, err)

	// Library initialization sees no injected variables; helper calls later see native input.
	jsctx := library.NewContext(map[string]any{"cfg": map[string]any{"enabled": true}})
	result, err := jsctx.EvaluateBooleanWithContext(context.Background(), `allowed()`, boundedPredicateTestLimits())
	require.NoError(t, err)
	require.True(t, result)

	// Libraries still cannot claim connector-reserved input names.
	library, err = CompileLibrary(`const cfg = {private: "library value"};`)
	require.NoError(t, err)
	result, err = library.NewContext(nil).EvaluateBooleanWithContext(context.Background(), `true`, boundedPredicateTestLimits())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "library value")
	require.False(t, result)
}

// TestBoundedBooleanSafeFailures keeps source, stacks, and thrown input data
// private, and rejects non-Boolean results without invoking conversion hooks.
func TestBoundedBooleanSafeFailures(t *testing.T) {
	for name, test := range map[string]struct {
		library string
		source  string
		err     error
	}{
		"syntax":                  {source: `const private_source = ;`, err: jsruntime.ErrInvalidSource},
		"throw":                   {source: `throw new Error("private thrown value")`, err: jsruntime.ErrEvaluation},
		"library throw":           {library: `throw new Error("private library value")`, source: `true`, err: jsruntime.ErrEvaluation},
		"undefined":               {source: `undefined`, err: jsruntime.ErrInvalidOutput},
		"null":                    {source: `null`, err: jsruntime.ErrInvalidOutput},
		"string":                  {source: `"private string"`, err: jsruntime.ErrInvalidOutput},
		"number":                  {source: `1`, err: jsruntime.ErrInvalidOutput},
		"Promise":                 {source: `Promise.resolve(true)`, err: jsruntime.ErrInvalidOutput},
		"object getter":           {source: `({get value() { for (;;) {} }, valueOf() { throw "private"; }})`, err: jsruntime.ErrInvalidOutput},
		"valueOf spoof":           {source: `({valueOf() { return true; }})`, err: jsruntime.ErrInvalidOutput},
		"Boolean prototype spoof": {source: `Object.create(Boolean.prototype)`, err: jsruntime.ErrInvalidOutput},
		"Boolean proxy":           {source: `new Proxy(new Boolean(true), {get() { throw "private trap"; }, getPrototypeOf() { throw "private trap"; }})`, err: jsruntime.ErrInvalidOutput},
	} {
		t.Run(name, func(t *testing.T) {
			library, err := CompileLibrary(test.library)
			require.NoError(t, err)

			// Unsupported output is classified without recursively exporting its object graph.
			result, err := library.NewContext(nil).EvaluateBooleanWithContext(context.Background(), test.source, boundedPredicateTestLimits())
			require.ErrorIs(t, err, test.err)
			require.NotContains(t, err.Error(), "private")
			require.False(t, result)
		})
	}
}

// TestBoundedBooleanLibraryAccessors keeps authored exceptions raised by host
// property access inside the same safe error boundary as ordinary execution.
func TestBoundedBooleanLibraryAccessors(t *testing.T) {
	for name, test := range map[string]struct {
		library string
		vars    map[string]any
	}{
		"reserved getter": {
			library: `Object.defineProperty(globalThis, "cfg", {
				get() { throw "private reserved getter"; }
			});`,
		},
		"input setter": {
			library: `Object.defineProperty(globalThis, "custom", {
				set(value) { throw "private input setter"; }
			});`,
			vars: map[string]any{"custom": true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			library, err := CompileLibrary(test.library)
			require.NoError(t, err)

			// Reserved-name checks and input binding may execute accessors installed by the library.
			var result bool
			require.NotPanics(t, func() {
				result, err = library.NewContext(test.vars).EvaluateBooleanWithContext(context.Background(), `true`, boundedPredicateTestLimits())
			})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private")
			require.False(t, result)
		})
	}
}

// TestBoundedBooleanLimits covers source accounting, native input conversion,
// Boolean output bytes, and the VM's stack policy.
func TestBoundedBooleanLimits(t *testing.T) {
	const librarySource = `function allowed() { return true; }`
	library, err := CompileLibrary(librarySource)
	require.NoError(t, err)
	limits := boundedPredicateTestLimits()
	limits.MaxSourceBytes = len(librarySource) + len(`allowed()`)

	// The library and predicate fit exactly together, then fail one byte below that total.
	result, err := library.NewContext(nil).EvaluateBooleanWithContext(context.Background(), `allowed()`, limits)
	require.NoError(t, err)
	require.True(t, result)
	limits.MaxSourceBytes--
	result, err = library.NewContext(nil).EvaluateBooleanWithContext(context.Background(), `allowed()`, limits)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.False(t, result)

	for name, change := range map[string]func(*jsruntime.Limits){
		"input bytes": func(l *jsruntime.Limits) { l.MaxInputBytes = 8 },
		"input depth": func(l *jsruntime.Limits) { l.MaxDepth = 1 },
		"input nodes": func(l *jsruntime.Limits) { l.MaxNodes = 3 },
		"output":      func(l *jsruntime.Limits) { l.MaxOutputBytes = 3 },
	} {
		t.Run(name, func(t *testing.T) {
			limits := boundedPredicateTestLimits()
			change(&limits)
			jsctx := NewContext(nil, map[string]any{"cfg": map[string]any{"enabled": true}})

			// Input and output limits both fail closed even when authored work is trivial.
			result, err := jsctx.EvaluateBooleanWithContext(context.Background(), `true`, limits)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.False(t, result)
		})
	}

	// A recursive helper exhausts the configured stack rather than leaking a Goja stack trace.
	result, err = NewContext(nil, nil).EvaluateBooleanWithContext(context.Background(), `function recurse() { return recurse(); } recurse();`, boundedPredicateTestLimits())
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.False(t, result)
}

// TestBoundedBooleanTimeout bounds initialization, synchronous evaluation, and
// Promise microtasks with the same execution deadline.
func TestBoundedBooleanTimeout(t *testing.T) {
	for name, test := range map[string]struct{ library, source string }{
		"library":         {library: `for (;;) {}`, source: `true`},
		"reserved getter": {library: `Object.defineProperty(globalThis, "cfg", {get() { for (;;) {} }});`, source: `true`},
		"predicate":       {source: `for (;;) {}`},
		"microtasks":      {source: `function again() { Promise.resolve().then(again); } again(); true;`},
	} {
		t.Run(name, func(t *testing.T) {
			library, err := CompileLibrary(test.library)
			require.NoError(t, err)
			limits := boundedPredicateTestLimits()
			limits.Timeout = 40 * time.Millisecond

			// No phase can acquire a fresh deadline or return partial success after expiry.
			result, err := library.NewContext(nil).EvaluateBooleanWithContext(context.Background(), test.source, limits)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.False(t, result)
		})
	}
}

// TestBoundedBooleanCancellation preserves real caller cancellation and rejects
// missing execution policy rather than silently reverting to the legacy path.
func TestBoundedBooleanCancellation(t *testing.T) {
	jsctx := NewContext(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// A canceled caller wins even when the policy timeout has also elapsed.
	limits := boundedPredicateTestLimits()
	limits.Timeout = time.Nanosecond
	result, err := jsctx.EvaluateBooleanWithContext(ctx, `true`, limits)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, result)

	// Cancellation during authored work retains the caller's context error.
	ctx, cancel = context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	result, err = jsctx.EvaluateBooleanWithContext(ctx, `for (;;) {}`, boundedPredicateTestLimits())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, result)

	// Invalid API arguments cannot accidentally opt into unbounded evaluation.
	_, err = jsctx.EvaluateBooleanWithContext(nil, `true`, boundedPredicateTestLimits())
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	_, err = jsctx.EvaluateBooleanWithContext(context.Background(), `true`, jsruntime.Limits{})
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
}

// TestBoundedBooleanSourceMapsAndCapabilities retains disabled source-map
// loading for static and dynamic code without exposing new host capabilities.
func TestBoundedBooleanSourceMapsAndCapabilities(t *testing.T) {
	const directive = "\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json"
	library, err := CompileLibrary(`const ready = true;` + directive)
	require.NoError(t, err)
	for _, source := range []string{
		`ready;` + directive,
		`eval(` + strconv.Quote(`ready`+directive) + `)`,
		`Function(` + strconv.Quote(`return true;`+directive) + `)()`,
		`typeof fetch === "undefined" && typeof require === "undefined" && typeof process === "undefined" && typeof toolResult === "undefined"`,
	} {
		// Missing source-map files must not turn otherwise valid code into a parser error.
		result, err := library.NewContext(nil).EvaluateBooleanWithContext(context.Background(), source, boundedPredicateTestLimits())
		require.NoError(t, err)
		require.True(t, result)
	}
}

// TestBoundedBooleanConcurrentLibraryReuse keeps mutable helper state and copied
// inputs isolated when callers share one compiled connector library.
func TestBoundedBooleanConcurrentLibraryReuse(t *testing.T) {
	library, err := CompileLibrary(`let calls = 0; function first() { return ++calls === 1; }`)
	require.NoError(t, err)
	jsctx := library.NewContext(map[string]any{"cfg": map[string]any{"count": 0}})
	results := make(chan error, 12)
	var work sync.WaitGroup

	// Each concurrent call gets its own lexical state and mutable native snapshot.
	for range cap(results) {
		work.Go(func() {
			result, err := jsctx.EvaluateBooleanWithContext(context.Background(), `cfg.count++; first() && cfg.count === 1`, boundedPredicateTestLimits())
			if err == nil && !result {
				err = fmt.Errorf("evaluation reused mutable state")
			}
			results <- err
		})
	}
	work.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
}
