package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// compileExpression prepares authored source without evaluating it.
func compileExpression(t *testing.T, source string) *jsruntime.Expression {
	t.Helper()
	compiled, err := jsruntime.CompileExpression(source, runtimeLimits())
	require.NoError(t, err)
	return compiled
}

// evaluateExpression runs an optional trusted input binder and checks detached JSON output.
func evaluateExpression(t *testing.T, compiled *jsruntime.Expression, prepare func(context.Context, *goja.Runtime) error) json.RawMessage {
	t.Helper()
	result, err := compiled.Evaluate(context.Background(), prepare)
	require.NoError(t, err)
	require.True(t, json.Valid(result))
	return result
}

// expressionInput supplies a generic JSON value under one explicit native-input allowance.
func expressionInput(input json.RawMessage, limits jsruntime.Limits) func(context.Context, *goja.Runtime) error {
	return func(ctx context.Context, vm *goja.Runtime) error {
		decoder, err := jsruntime.NewJSONDecoder(vm)
		if err != nil {
			return err
		}
		values, err := decoder.Decode(ctx, jsruntime.JSONLimits{
			MaxBytes: limits.MaxInputBytes, MaxDepth: limits.MaxDepth, MaxNodes: limits.MaxNodes,
		}, input)
		if err != nil {
			return err
		}
		if err := vm.Set("params", values[0]); err != nil {
			return jsruntime.ErrInvalidInput
		}
		return nil
	}
}

// evaluateWithWatchdog cancels a broken interruption path before it can block the suite.
func evaluateWithWatchdog(t *testing.T, ctx context.Context, cancel context.CancelFunc, compiled *jsruntime.Expression, prepare func(context.Context, *goja.Runtime) error) (json.RawMessage, error) {
	t.Helper()
	type outcome struct {
		result json.RawMessage
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := compiled.Evaluate(ctx, prepare)
		done <- outcome{result, err}
	}()
	t.Cleanup(cancel)
	select {
	case result := <-done:
		return result.result, result.err
	case <-time.After(5 * time.Second):
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		t.Fatal("expression evaluation failed to stop before watchdog expired")
		return nil, nil
	}
}

// TestExpressionDisablesDynamicSourceMaps keeps authored source-map paths out of compilation and dynamic parsing.
func TestExpressionDisablesDynamicSourceMaps(t *testing.T) {
	// Compilation and dynamic parsing must not read authored source-map URLs from disk.
	for _, source := range []string{
		"7\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json",
		`eval("7\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json")`,
		`(new Function("return 7\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json"))()`,
	} {
		compiled := compileExpression(t, source)

		// All parsing paths finish normally without invoking a source-map loader.
		result := evaluateExpression(t, compiled, nil)
		require.JSONEq(t, `7`, string(result))
	}
}

// TestExpressionCompileValidation requires one strict expression without executing authored code.
func TestExpressionCompileValidation(t *testing.T) {
	for _, source := range []string{
		"", " ", "({", "const value = 1; value", "1; 2", "return 1",
		`1); (() => { throw new Error("private"); })(); (2`,
		string([]byte{'"', 0xff, '"'}),
	} {
		t.Run(source, func(t *testing.T) {
			// Invalid syntax and multiple statements must never become an executable plan.
			compiled, err := jsruntime.CompileExpression(source, runtimeLimits())
			require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
			require.Nil(t, compiled)
		})
	}

	// Compiling valid but throwing code must not execute any authored JavaScript.
	compiled := compileExpression(t, `(() => { throw new Error("private-source-marker"); })()`)
	result, err := compiled.Evaluate(context.Background(), nil)
	require.ErrorIs(t, err, jsruntime.ErrEvaluation)
	require.NotContains(t, err.Error(), "private-source-marker")
	require.Nil(t, result)
}

// TestExpressionLimitsMustBePositive keeps source and execution policy mandatory.
func TestExpressionLimitsMustBePositive(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*jsruntime.Limits)
	}{
		{"source bytes", func(l *jsruntime.Limits) { l.MaxSourceBytes = 0 }},
		{"input bytes", func(l *jsruntime.Limits) { l.MaxInputBytes = 0 }},
		{"output bytes", func(l *jsruntime.Limits) { l.MaxOutputBytes = 0 }},
		{"depth", func(l *jsruntime.Limits) { l.MaxDepth = 0 }},
		{"nodes", func(l *jsruntime.Limits) { l.MaxNodes = 0 }},
		{"stack depth", func(l *jsruntime.Limits) { l.MaxStackDepth = 0 }},
		{"timeout", func(l *jsruntime.Limits) { l.Timeout = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A missing policy value cannot accidentally disable an execution guard.
			limits := runtimeLimits()
			test.change(&limits)
			compiled, err := jsruntime.CompileExpression(`1`, limits)
			require.Error(t, err)
			require.Nil(t, compiled)
		})
	}

	// Source accounting is byte-based and accepts the exact configured boundary.
	source := `"雪"`
	limits := runtimeLimits()
	limits.MaxSourceBytes = len(source)
	_, err := jsruntime.CompileExpression(source, limits)
	require.NoError(t, err)
	limits.MaxSourceBytes--
	compiled, err := jsruntime.CompileExpression(source, limits)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.Nil(t, compiled)
}

// TestExpressionHostFeaturesAreAbsent provides language intrinsics without domain capabilities.
func TestExpressionHostFeaturesAreAbsent(t *testing.T) {
	// Pure expressions have ordinary language intrinsics but no injected host capability.
	compiled := compileExpression(t, `({
		fetch: typeof fetch, require: typeof require, process: typeof process,
        params: typeof params, context: typeof context, cfg: typeof cfg, toolResult: typeof toolResult,
		Deno: typeof Deno, Bun: typeof Bun, console: typeof console,
		setTimeout: typeof setTimeout, XMLHttpRequest: typeof XMLHttpRequest,
		WebSocket: typeof WebSocket, Buffer: typeof Buffer,
		math: Math.max(3, 5), json: JSON.parse('{"answer":42}').answer
	})`)
	result := evaluateExpression(t, compiled, nil)
	require.JSONEq(t, `{
		"fetch":"undefined","require":"undefined","process":"undefined",
        "params":"undefined","context":"undefined","cfg":"undefined","toolResult":"undefined",
		"Deno":"undefined","Bun":"undefined","console":"undefined",
		"setTimeout":"undefined","XMLHttpRequest":"undefined",
		"WebSocket":"undefined","Buffer":"undefined","math":5,"json":42
	}`, string(result))
}

// TestExpressionRejectsNonJSONResults rejects unsupported values without invoking their hooks.
func TestExpressionRejectsNonJSONResults(t *testing.T) {
	for _, source := range []string{
		`undefined`, `(() => {})`, `NaN`, `Infinity`, `-Infinity`,
		`1n`, `Symbol("value")`, `({[Symbol("key")]: 1})`,
		`({value: undefined})`, `[undefined]`, `({value() { return 1; }})`,
		`(() => { const value = {}; value.self = value; return value; })()`,
		`new Date(0)`, `/example/`, `new Map([["value", 1]])`, `new Set([1])`,
		`new Uint8Array([1, 2])`, `Promise.resolve(1)`, `(async () => 1)()`,
		`({ get answer() { throw new Error("private-getter-value"); } })`,
		`({ get answer() { while (true) {} } })`,
		`new Proxy({}, {get() { throw new Error("private-proxy-value"); }})`,
	} {
		t.Run(source, func(t *testing.T) {
			// Returning a non-JSON value must fail instead of silently dropping or coercing it.
			ctx, cancel := context.WithCancel(context.Background())
			result, err := evaluateWithWatchdog(t, ctx, cancel, compileExpression(t, source), nil)
			require.ErrorIs(t, err, jsruntime.ErrInvalidOutput)
			require.NotContains(t, err.Error(), "private-")
			require.Nil(t, result)
		})
	}
}

// TestExpressionOutputIgnoresModifiedIntrinsics keeps final conversion independent of authored globals.
func TestExpressionOutputIgnoresModifiedIntrinsics(t *testing.T) {
	// Authored mutations to standard objects must not replace the host's JSON traversal.
	compiled := compileExpression(t, `(() => {
		Object.keys = () => [];
		Object.getOwnPropertyDescriptor = () => ({value: "forged"});
		Object.getOwnPropertyDescriptors = () => ({});
		Array.isArray = () => false;
		JSON.stringify = () => '"forged"';
		return {actual: [42, {name: "Ada"}]};
	})()`)
	require.JSONEq(t, `{"actual":[42,{"name":"Ada"}]}`, string(evaluateExpression(t, compiled, nil)))
}

// TestExpressionTimeout applies policy expiry independently of caller cancellation.
func TestExpressionTimeout(t *testing.T) {
	// The runtime's execution policy remains distinct from the caller's lifetime.
	limits := runtimeLimits()
	limits.Timeout = 100 * time.Millisecond
	compiled, err := jsruntime.CompileExpression(`(() => { if (params.busy) { while (true) {} } return 42; })()`, limits)
	require.NoError(t, err)
	for _, callerDeadline := range []bool{false, true} {
		t.Run(fmt.Sprint("caller deadline=", callerDeadline), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if callerDeadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
			}
			input := expressionInput(json.RawMessage(`{"busy":true}`), limits)
			result, err := evaluateWithWatchdog(t, ctx, cancel, compiled, input)

			// The expression hit its policy limit while the caller remains active.
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.NotErrorIs(t, err, context.DeadlineExceeded)
			require.NoError(t, ctx.Err())
			require.Nil(t, result)

			// A timed-out evaluation cannot poison a later call on the same program.
			input = expressionInput(json.RawMessage(`{"busy":false}`), limits)
			require.JSONEq(t, `42`, string(evaluateExpression(t, compiled, input)))
		})
	}
}

// TestExpressionStackAndOutputLimits bounds recursive execution and escaped result bytes.
func TestExpressionStackAndOutputLimits(t *testing.T) {
	// Recursive authored code trips the configured stack bound before its time budget.
	compiled := compileExpression(t, `(() => { function recurse() { return recurse(); } return recurse(); })()`)
	result, err := compiled.Evaluate(context.Background(), nil)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.Nil(t, result)

	// Byte limits apply to the encoded JSON result, including escaping and quotes.
	limits := runtimeLimits()
	limits.MaxOutputBytes = len(`"\n"`)
	compiled, err = jsruntime.CompileExpression(`"\n"`, limits)
	require.NoError(t, err)
	require.Equal(t, `"\n"`, string(evaluateExpression(t, compiled, nil)))
	limits.MaxOutputBytes--
	compiled, err = jsruntime.CompileExpression(`"\n"`, limits)
	require.NoError(t, err)
	result, err = compiled.Evaluate(context.Background(), nil)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.Nil(t, result)
}

// TestExpressionJSONValues preserves JSON kinds without introducing domain defaults.
func TestExpressionJSONValues(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		want   string
	}{
		{"object", `({name: "Ada", enabled: true, empty: null})`, `{"name":"Ada","enabled":true,"empty":null}`},
		{"array", `[1, "two", false, null]`, `[1,"two",false,null]`},
		{"string", `"line\n雪"`, `"line\n雪"`},
		{"number", `6 * 7`, `42`},
		{"boolean", `false`, `false`},
		{"null", `null`, `null`},
		{"standard number precision", `9007199254740993`, `9007199254740992`},
	} {
		t.Run(test.name, func(t *testing.T) {
			// No preparation callback is necessary for data-independent expressions.
			compiled := compileExpression(t, test.source)
			require.JSONEq(t, test.want, string(evaluateExpression(t, compiled, nil)))
		})
	}
}

// TestExpressionPreparationFailure preserves safe adapter failures before executing authored code.
func TestExpressionPreparationFailure(t *testing.T) {
	compiled := compileExpression(t, `record()`)
	safe := errors.New("safe input classification")
	executed := false
	result, err := compiled.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime) error {
		// A selected pure helper makes authored execution observable after binding fails.
		if err := vm.Set("record", func() bool { executed = true; return true }); err != nil {
			return jsruntime.ErrInvalidInput
		}
		return safe
	})
	require.ErrorIs(t, err, safe)
	require.Nil(t, result)
	require.False(t, executed)

	// Preparation can reject malformed input without exposing parser diagnostics.
	result, err = compiled.Evaluate(context.Background(), expressionInput(json.RawMessage(`{"private-input":}`), runtimeLimits()))
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.NotContains(t, err.Error(), "private-input")
	require.Nil(t, result)
}

// TestExpressionPreparationSharesDeadline includes input preparation in the original policy window.
func TestExpressionPreparationSharesDeadline(t *testing.T) {
	limits := runtimeLimits()
	limits.Timeout = 10 * time.Millisecond
	compiled, err := jsruntime.CompileExpression(`42`, limits)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	result, err := evaluateWithWatchdog(t, ctx, cancel, compiled, func(runCtx context.Context, _ *goja.Runtime) error {
		// Deliberately report success after preparation consumes the entire policy.
		<-runCtx.Done()
		return nil
	})
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.NoError(t, ctx.Err())
	require.Nil(t, result)

	// Caller cancellation during preparation retains its distinct classification.
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	executed := false
	result, err = compileExpression(t, `record()`).Evaluate(ctx, func(_ context.Context, vm *goja.Runtime) error {
		if err := vm.Set("record", func() bool { executed = true; return true }); err != nil {
			return jsruntime.ErrInvalidInput
		}
		cancel()
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
	require.False(t, executed, "canceled preparation must not start authored execution")
}

// TestExpressionNativeInputAndConcurrentReuse isolates copied inputs, globals, and returned bytes.
func TestExpressionNativeInputAndConcurrentReuse(t *testing.T) {
	encoded := json.RawMessage(`{"count":41}`)
	prepare := expressionInput(encoded, runtimeLimits())
	compiled := compileExpression(t, `(() => {
        const previous = typeof globalThis.previous;
        globalThis.previous = true;
        params.count++;
        return {value: params.count, previous, native: Object.getPrototypeOf(params) === Object.prototype};
    })()`)
	const count = 12
	type outcome struct {
		result json.RawMessage
		err    error
	}
	results := make(chan outcome, count)
	var workers sync.WaitGroup
	for range count {
		workers.Go(func() {
			result, err := compiled.Evaluate(context.Background(), prepare)
			results <- outcome{result, err}
		})
	}
	workers.Wait()
	close(results)

	// Concurrent calls share the compiled program and input bytes, never native VM state.
	for observed := range results {
		require.NoError(t, observed.err)
		require.JSONEq(t, `{"value":42,"previous":"undefined","native":true}`, string(observed.result))
		observed.result[0] = ' ' // The caller owns each returned byte slice.
	}
	require.Equal(t, `{"count":41}`, string(encoded))
	require.JSONEq(t, `{"value":42,"previous":"undefined","native":true}`, string(evaluateExpression(t, compiled, prepare)))
}

// TestExpressionInvalidArguments rejects absent programs and contexts before calling preparation.
func TestExpressionInvalidArguments(t *testing.T) {
	for _, compiled := range []*jsruntime.Expression{nil, {}} {
		result, err := compiled.Evaluate(context.Background(), func(context.Context, *goja.Runtime) error {
			t.Fatal("invalid program called preparation")
			return nil
		})
		require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
		require.Nil(t, result)
	}
	result, err := compileExpression(t, `1`).Evaluate(nil, nil)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, result)
}
