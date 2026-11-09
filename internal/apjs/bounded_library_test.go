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

// TestBoundedLibrarySourceLimits checks the raw source before parsing or dropping
// whitespace and retains it for a later combined library/predicate allowance.
func TestBoundedLibrarySourceLimits(t *testing.T) {
	for name, source := range map[string]string{
		"program":    `const enabled = true;`,
		"whitespace": " \n\t ",
	} {
		t.Run(name, func(t *testing.T) {
			limits := boundedPredicateTestLimits()
			limits.MaxSourceBytes = len(source)

			// An exact fit compiles, including an empty program with raw whitespace.
			library, err := CompileAndValidateLibraryWithContext(context.Background(), source, limits)
			require.NoError(t, err)
			require.Equal(t, source, library.Source())

			// The subsequent predicate shares the source allowance with that raw source.
			_, err = library.NewContext(nil).EvaluateBooleanWithContext(context.Background(), "true", limits)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)

			// One extra byte fails before compilation, even when it is whitespace.
			library, err = CompileAndValidateLibraryWithContext(context.Background(), source+" ", limits)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, library)
		})
	}
}

// TestBoundedLibraryFailures rejects private authored failures and finite-policy
// violations without returning a partially validated reusable program.
func TestBoundedLibraryFailures(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		want   error
		loop   bool
	}{
		"syntax":           {source: `const privateLibraryDetail = ;`, want: jsruntime.ErrInvalidSource},
		"invalid utf8":     {source: "//\xff", want: jsruntime.ErrInvalidSource},
		"throw":            {source: `throw new Error("privateLibraryDetail")`, want: jsruntime.ErrEvaluation},
		"runtime variable": {source: `cfg.privateLibraryDetail`, want: jsruntime.ErrEvaluation},
		"reserved binding": {source: `const cfg = "privateLibraryDetail"`, want: jsruntime.ErrInvalidSource},
		"reserved getter": {
			source: `Object.defineProperty(globalThis, "cfg", {get() { throw "privateLibraryDetail"; }});`,
			want:   jsruntime.ErrEvaluation,
		},
		"reserved getter loop": {
			source: `Object.defineProperty(globalThis, "cfg", {get() { for (;;) {} }});`,
			want:   jsruntime.ErrLimitExceeded,
			loop:   true,
		},
		"initialization loop": {source: `for (;;) {}`, want: jsruntime.ErrLimitExceeded, loop: true},
		"recursive stack":     {source: `function recur() { recur(); } recur();`, want: jsruntime.ErrLimitExceeded},
	} {
		t.Run(name, func(t *testing.T) {
			// Only endless loops need a short timeout; other classifications keep ample time.
			limits := boundedPredicateTestLimits()
			if test.loop {
				limits.Timeout = 50 * time.Millisecond
			}
			library, err := CompileAndValidateLibraryWithContext(context.Background(), test.source, limits)

			// Only safe shared-runtime classifications cross the adapter boundary.
			require.ErrorIs(t, err, test.want)
			require.NotContains(t, err.Error(), "privateLibraryDetail")
			require.Nil(t, library)
		})
	}
}

// TestBoundedLibraryContextAndPolicy checks API misuse and distinguishes caller
// cancellation from the adapter's own timeout, including before source parsing.
func TestBoundedLibraryContextAndPolicy(t *testing.T) {
	limits := boundedPredicateTestLimits()

	// A context and a fully finite policy are required even for an empty library.
	library, err := CompileAndValidateLibraryWithContext(nil, "", limits)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, library)
	invalid := limits
	invalid.Timeout = 0
	library, err = CompileAndValidateLibraryWithContext(context.Background(), "", invalid)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, library)

	// An already canceled caller wins over both invalid policy and authored syntax.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	library, err = CompileAndValidateLibraryWithContext(ctx, `privateLibraryDetail = ;`, invalid)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, library)

	// A caller deadline interrupts library work before the one-second policy expires.
	ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	library, err = CompileAndValidateLibraryWithContext(ctx, `for (;;) {}`, limits)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, library)
}

// TestBoundedLibraryCompatibility preserves non-strict helper programs without
// injecting runtime data or capabilities and disables static/dynamic source maps.
func TestBoundedLibraryCompatibility(t *testing.T) {
	const source = `
		if (typeof cfg !== "undefined" || typeof labels !== "undefined" ||
			typeof annotations !== "undefined" || typeof data !== "undefined" ||
			typeof fetch !== "undefined" || typeof require !== "undefined" ||
			typeof process !== "undefined") { throw "unexpected host binding"; }
		initializedWithoutDeclaration = true;
		eval("1\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json");
		function eligible() { return initializedWithoutDeclaration && cfg.enabled; }
		//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json
	`
	limits := boundedPredicateTestLimits()

	// Validation runs the library without data; the reusable helper reads data only when called.
	library, err := CompileAndValidateLibraryWithContext(context.Background(), source, limits)
	require.NoError(t, err)
	value, err := library.NewContext(map[string]any{"cfg": map[string]any{"enabled": true}}).
		EvaluateBooleanWithContext(context.Background(), `var result = eligible(); new Boolean(result);`, limits)
	require.NoError(t, err)
	require.True(t, value)

	// Omitted and whitespace-only libraries remain valid no-op helper programs.
	for _, source := range []string{"", strings.Repeat(" \n", 3)} {
		library, err = CompileAndValidateLibraryWithContext(context.Background(), source, limits)
		require.NoError(t, err)
		require.NotNil(t, library)
	}
}

// TestBoundedLibraryConcurrentReuse checks that validation retains no VM state
// and its compiled program remains reusable across simultaneous evaluations.
func TestBoundedLibraryConcurrentReuse(t *testing.T) {
	limits := boundedPredicateTestLimits()
	library, err := CompileAndValidateLibraryWithContext(context.Background(), `
		let calls = 0;
		function firstCall() { return ++calls === 1; }
	`, limits)
	require.NoError(t, err)

	// Each invocation initializes its own globals, independent of validation and other calls.
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for range cap(results) {
		wg.Go(func() {
			value, err := library.NewContext(nil).EvaluateBooleanWithContext(context.Background(), `firstCall()`, limits)
			if err == nil && !value {
				err = fmt.Errorf("evaluation reused mutable state")
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
