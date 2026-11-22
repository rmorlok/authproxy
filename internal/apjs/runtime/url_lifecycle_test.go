package runtime_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// TestURLAvailability gives validation, initialization, and ordinary expressions
// the same pure URL helper without adding network or logging capabilities.
func TestURLAvailability(t *testing.T) {
	limits := runtimeLimits()
	for _, source := range []string{
		`const URL = 1; async function execute() {}`,
		`var URL; async function execute() {}`,
	} {
		// Shared compilation reserves the installed name before authored code can run.
		_, err := jsruntime.CompileAsync(source, nil, nil, limits)
		require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
	}
	_, err := jsruntime.CompileAsync(`async function execute() {}`, asyncLibrary(t, `function URL() {}`), nil, limits)
	require.ErrorIs(t, err, jsruntime.ErrInvalidSource)

	// Initialization can resolve a URL during publication validation without I/O hosts.
	program, err := jsruntime.CompileAsync(`const target = new URL("../items", "https://example.test/v1/");
		async function execute() {return [target.href, typeof fetch, typeof console];}`, nil, nil, limits)
	require.NoError(t, err)
	require.NoError(t, program.Validate(context.Background()))
	result, err := program.Evaluate(context.Background(), asyncInput(limits))
	require.NoError(t, err)
	require.JSONEq(t, `["https://example.test/items","undefined","undefined"]`, string(result))

	// Pure expressions receive URL directly, without an async or Tool-specific adapter.
	expression, err := jsruntime.CompileExpression(`new URL("/items", "https://example.test/v1/").href`, limits)
	require.NoError(t, err)
	result, err = expression.Evaluate(context.Background(), nil)
	require.NoError(t, err)
	require.JSONEq(t, `"https://example.test/items"`, string(result))

	// Managed output accepts the explicit string above, not a URL instance or its toJSON hook.
	result, err = evaluateSource(context.Background(), limits, `new URL("https://example.test/items")`)
	require.ErrorIs(t, err, jsruntime.ErrInvalidOutput)
	require.Nil(t, result)
}

// TestURLCumulativeBudgets accounts for parser admission and repeated reads
// even when authored code catches an exhaustion exception.
func TestURLCumulativeBudgets(t *testing.T) {
	for _, test := range []struct{ name, source string }{
		{"parser admission", `new URL("https://example.test/" + "a".repeat(2048));`},
		{"href getter", `const target = new URL("https://example.test/a"); for (let i = 0; i < 4096; i++) {target.href;}`},
		{"toString", `const target = new URL("https://example.test/a"); for (let i = 0; i < 4096; i++) {target.toString();}`},
		{"toJSON", `const target = new URL("https://example.test/a"); for (let i = 0; i < 4096; i++) {target.toJSON();}`},
	} {
		// The scenarios fit an ordinary execution, including all admitted native work.
		result, err := runUtilitySource(context.Background(), runtimeLimits(), test.source+` true;`)
		require.NoError(t, err)
		require.JSONEq(t, `true`, string(result))

		for _, dimension := range []string{"work", "allocation"} {
			t.Run(dimension+"/"+test.name, func(t *testing.T) {
				limits := runtimeLimits()
				if dimension == "work" {
					limits.MaxWorkUnits = 16 << 10
				} else {
					limits.MaxAllocationUnits = 16 << 10
				}

				// Catching a refusal cannot publish the sentinel as a successful result.
				result, err := runUtilitySource(context.Background(), limits,
					`try {`+test.source+`} catch (_) {} "private-sentinel";`)
				require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
				require.Nil(t, result)
				require.NotContains(t, err.Error(), "private-sentinel")
			})
		}
	}
}

// TestURLInitializationExhaustionKeepsIOClosed rejects a definition whose URL
// setup spends its allowance before the async host's I/O gate could open.
func TestURLInitializationExhaustionKeepsIOClosed(t *testing.T) {
	limits := runtimeLimits()
	limits.MaxAllocationUnits = 16 << 10
	program, err := jsruntime.CompileAsync(`
		try {new URL("https://example.test/" + "a".repeat(2048));} catch (_) {}
		async function execute() {return true;}`, nil, nil, limits)
	require.NoError(t, err)
	// Validation initializes the program and must retain the caught budget failure.
	require.ErrorIs(t, program.Validate(context.Background()), jsruntime.ErrLimitExceeded)
	started := false
	result, err := program.Evaluate(context.Background(), func(_ context.Context, _ *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		invocation.Start = func() { started = true }
		invocation.Params, invocation.Context = goja.Undefined(), goja.Undefined()
		return nil
	})

	// Invocation fails before enabling host effects or returning authored output.
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.Nil(t, result)
	require.False(t, started)
}

// TestURLConcurrentIsolation shares compiled code while each evaluation retains
// its own native URL instances, initialization state, and cumulative allowance.
func TestURLConcurrentIsolation(t *testing.T) {
	limits := runtimeLimits()
	// Each invocation can afford these small URLs independently; pooled charges
	// across all concurrent invocations would exceed this cumulative allowance.
	limits.MaxWorkUnits, limits.MaxAllocationUnits = 16<<10, 16<<10
	program, err := jsruntime.CompileAsync(`
		const base = new URL("https://example.test/v1/");
		let calls = 0;
		async function execute(index) {
			const target = new URL(String(index), base.href);
			return [target.href, ++calls];
		}`, nil, nil, limits)
	require.NoError(t, err)
	require.NoError(t, program.Validate(context.Background()))
	errors := make(chan error, 8)
	var workers sync.WaitGroup
	for index := range cap(errors) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			// Supplying a distinct primitive parameter gives every VM a different result.
			result, err := program.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
				invocation.Params, invocation.Context = vm.ToValue(index), goja.Undefined()
				return nil
			})
			if err == nil && string(result) != fmt.Sprintf(`["https://example.test/v1/%d",1]`, index) {
				err = fmt.Errorf("unexpected isolated URL result: %s", result)
			}
			errors <- err
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

// TestURLCancellationDuringNormalization stops native UTF-16 preparation before
// entering the synchronous parser, for either the input or supplied base URL.
func TestURLCancellationDuringNormalization(t *testing.T) {
	text := "https://example.test/" + strings.Repeat("a", 8192)
	for _, useBase := range []bool{false, true} {
		t.Run(fmt.Sprintf("base=%t", useBase), func(t *testing.T) {
			limits := runtimeLimits()
			limits.MaxWorkUnits, limits.MaxAllocationUnits = int(^uint(0)>>1), int(^uint(0)>>1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reads := 0
			result, err := jsruntime.EvaluateJSON(ctx, limits, func(_ context.Context, vm *goja.Runtime) (goja.Value, error) {
				// The wrapped primitive cancels deterministically during code-unit scanning.
				input := &base64CancelString{
					base64PrimitiveString: vm.ToValue(text).(goja.String), cancel: cancel, cancelAfter: 128,
				}
				defer func() { reads = input.reads }()
				constructor, ok := goja.AssertConstructor(vm.Get("URL"))
				require.True(t, ok)
				arguments := []goja.Value{input}
				if useBase {
					arguments = []goja.Value{vm.ToValue("child"), input}
				}
				value, err := constructor(nil, arguments...)
				return value, jsruntime.EvaluationError(err)
			})

			// Cancellation discards partial native state and retains the caller's error.
			require.ErrorIs(t, err, context.Canceled)
			require.Nil(t, result)
			require.GreaterOrEqual(t, reads, 128)
			require.Less(t, reads, len(text))
		})
	}
}
