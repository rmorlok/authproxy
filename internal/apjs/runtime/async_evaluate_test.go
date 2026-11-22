package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// asyncInput binds native arguments and matching globals under one input allowance.
// The nonempty params fixture also exercises aggregate byte accounting.
func asyncInput(limits jsruntime.Limits) func(context.Context, *goja.Runtime, *jsruntime.AsyncInvocation) error {
	return func(ctx context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		decoder, err := jsruntime.NewJSONDecoder(vm)
		if err != nil {
			return err
		}
		values, err := decoder.Decode(ctx, jsruntime.JSONLimits{
			MaxBytes: limits.MaxInputBytes, MaxDepth: limits.MaxDepth, MaxNodes: limits.MaxNodes,
		}, json.RawMessage(`{"private":"private-input-value"}`), json.RawMessage(`{}`))
		if err != nil {
			return err
		}
		invocation.Params, invocation.Context = values[0], values[1]
		if err := vm.Set("params", invocation.Params); err != nil {
			return jsruntime.ErrInvalidInput
		}
		if err := vm.Set("context", invocation.Context); err != nil {
			return jsruntime.ErrInvalidInput
		}
		return nil
	}
}

// evaluateAsync checks detached JSON from a program with native input and no host I/O.
func evaluateAsync(t *testing.T, program *jsruntime.AsyncProgram) json.RawMessage {
	t.Helper()
	result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
	require.NoError(t, err)
	require.True(t, json.Valid(result))
	return result
}

// TestAsyncEvaluateArguments passes explicit values without inventing matching globals.
func TestAsyncEvaluateArguments(t *testing.T) {
	program := compileAsync(t, `async function execute(args, snapshot) {
		return [args, snapshot, typeof params, typeof context];
	}`, nil)

	// The adapter chooses each argument independently of any global bindings.
	result, err := program.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		invocation.Params, invocation.Context = vm.ToValue("input"), vm.ToValue(7)
		return nil
	})
	require.NoError(t, err)
	require.JSONEq(t, `["input",7,"undefined","undefined"]`, string(result))
}

// TestAsyncEvaluatePromiseJobs drains language-level asynchronous continuations before encoding.
func TestAsyncEvaluatePromiseJobs(t *testing.T) {
	for name, body := range map[string]string{
		"direct return": `return 42;`,
		"await":         `return await Promise.resolve(42);`,
		"then chain":    `return Promise.resolve(20).then(x => x + 1).then(x => x * 2);`,
		"nested jobs": `let resolve; const pending = new Promise(r => { resolve = r; });
			Promise.resolve().then(() => Promise.resolve().then(() => resolve(42)));
			return await pending;`,
		"thenable": `return await {then(resolve) { resolve(42); }};`,
	} {
		t.Run(name, func(t *testing.T) {
			// Await and then jobs are driven by the same VM that owns the entry point.
			program := compileAsync(t, "async function execute() {"+body+"}", nil)
			require.JSONEq(t, `42`, string(evaluateAsync(t, program)))
		})
	}
}

// TestAsyncEvaluateUnhandledRejections detects abandoned failures without exposing their values.
func TestAsyncEvaluateUnhandledRejections(t *testing.T) {
	for name, test := range map[string]struct{ source, library string }{
		"helper initialization":  {`async function execute() { return 1; }`, `Promise.reject("private-rejection");`},
		"program initialization": {`Promise.reject("private-rejection"); async function execute() { return 1; }`, ""},
		"entry point":            {`async function execute() { Promise.reject("private-rejection"); return 1; }`, ""},
		"continuation":           {`async function execute() { Promise.resolve().then(() => { throw "private-rejection"; }); return 1; }`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			// An otherwise successful return cannot conceal an unhandled promise rejection.
			program := compileAsync(t, test.source, asyncLibrary(t, test.library))
			result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
			require.ErrorIs(t, err, jsruntime.ErrEvaluation)
			require.NotContains(t, err.Error(), "private-rejection")
			require.Nil(t, result)
		})
	}

	// A rejection that gains a handler before the job queue settles is not abandoned.
	program := compileAsync(t, `async function execute() {
		const rejected = Promise.reject("private-rejection");
		await 0;
		return rejected.catch(() => 42);
	}`, nil)
	require.JSONEq(t, `42`, string(evaluateAsync(t, program)))
}

// TestAsyncEvaluateRejectsFailedInitialization enforces the same boundary as publication validation.
func TestAsyncEvaluateRejectsFailedInitialization(t *testing.T) {
	program := compileAsync(t, `
		const early = Promise.reject("private-initialization-rejection");
		async function execute() {
			early.catch(() => {});
			return true;
		}
	`, nil)

	// Publication rejects an abandoned initialization promise without invoking execute.
	err := program.Validate(context.Background())
	require.ErrorIs(t, err, jsruntime.ErrEvaluation)
	require.Equal(t, jsruntime.ErrEvaluation.Error(), err.Error())

	// Invocation must reject that same initialization before execute can attach a handler.
	result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
	require.ErrorIs(t, err, jsruntime.ErrEvaluation)
	require.Equal(t, jsruntime.ErrEvaluation.Error(), err.Error())
	require.Nil(t, result)
}

// TestAsyncEvaluateSanitizesErrors covers throws and rejections during each authored execution phase.
func TestAsyncEvaluateSanitizesErrors(t *testing.T) {
	for name, test := range map[string]struct{ source, library string }{
		"helper throws":         {`async function execute() { return 1; }`, `throw new Error("private-initialization-value");`},
		"initialization throws": {`throw new Error("private-initialization-value"); async function execute() { return 1; }`, ""},
		"execute throws":        {`async function execute() { throw new Error(params.private); }`, ""},
		"throws string":         {`async function execute() { throw params.private; }`, ""},
		"throws object":         {`async function execute() { throw {toString() { throw params.private; }}; }`, ""},
		"await rejection":       {`async function execute() { return await Promise.reject(params.private); }`, ""},
		"sync replacement":      {`async function execute() {} execute = () => 1;`, ""},
		// Spoof the async prototype so initialization passes and the result guard
		// must reject the non-Promise without traversing its infinite getter.
		"thenable replacement": {`async function execute() {}
			execute = Object.setPrototypeOf(
				() => ({get then() { for (;;) {} }}), Object.getPrototypeOf(execute));`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			// Neither JavaScript stacks nor input-derived exception details leave the VM.
			program := compileAsync(t, test.source, asyncLibrary(t, test.library))
			result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
			require.ErrorIs(t, err, jsruntime.ErrEvaluation)
			require.Equal(t, jsruntime.ErrEvaluation.Error(), err.Error())
			require.Nil(t, result)
		})
	}
}

// TestAsyncEvaluatePendingPromiseFailsPromptly rejects promises that no installed host can settle.
func TestAsyncEvaluatePendingPromiseFailsPromptly(t *testing.T) {
	limits := runtimeLimits()
	limits.Timeout = 5 * time.Second
	program, err := jsruntime.CompileAsync(`async function execute() { return await new Promise(() => {}); }`, nil, asyncHostBindings(), limits)
	require.NoError(t, err)
	input := asyncInput(runtimeLimits())

	// Bound the test externally so a regression cannot leave it waiting for host work.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// With no host work available, waiting for either deadline cannot make progress.
	result, err := program.Evaluate(ctx, input)
	require.ErrorIs(t, err, jsruntime.ErrEvaluation)
	require.Nil(t, result)
	// Waiting for either deadline would return cancellation instead of ErrEvaluation.
	require.NoError(t, ctx.Err())
}

// TestAsyncEvaluateJSONContract reuses strict output semantics after the entry-point promise settles.
func TestAsyncEvaluateJSONContract(t *testing.T) {
	for name, value := range map[string]string{
		"undefined": `undefined`, "function": `() => 1`, "nonfinite": `NaN`,
		"bigint": `1n`, "nested promise": `({value: Promise.resolve(1)})`,
		"date": `new Date(0)`, "getter": `({get value() { throw "private"; }})`,
		"proxy": `new Proxy({}, {ownKeys() { throw "private"; }})`,
		"cycle": `(() => { const value = {}; value.self = value; return value; })()`,
	} {
		t.Run(name, func(t *testing.T) {
			// Unsupported values are rejected without coercion or authored property access.
			program := compileAsync(t, "async function execute() { return "+value+"; }", nil)
			result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
			require.ErrorIs(t, err, jsruntime.ErrInvalidOutput)
			require.Equal(t, jsruntime.ErrInvalidOutput.Error(), err.Error())
			require.Nil(t, result)
		})
	}

	// Capture conversion functions before helper initialization can poison the globals.
	library := asyncLibrary(t, `
		Object.getOwnPropertyDescriptor = () => { throw "private"; };
		Reflect.ownKeys = () => { throw "private"; };
		JSON.stringify = () => { throw "private"; };
	`)
	program := compileAsync(t, `async function execute() { return {values: [null, false, "雪", 42]}; }`, library)
	require.JSONEq(t, `{"values":[null,false,"雪",42]}`, string(evaluateAsync(t, program)))
}

// TestAsyncEvaluateLimits keeps execution, initialization, and result conversion under one policy.
func TestAsyncEvaluateLimits(t *testing.T) {
	for name, test := range map[string]struct {
		source, library string
		change          func(*jsruntime.Limits)
	}{
		"execute loop": {`async function execute() { for (;;) {} }`, "", func(l *jsruntime.Limits) { l.Timeout = 100 * time.Millisecond }},
		"job loop":     {`async function execute() { return Promise.resolve().then(() => { for (;;) {} }); }`, "", func(l *jsruntime.Limits) { l.Timeout = 100 * time.Millisecond }},
		"endless microtasks": {`async function execute() {
			const again = () => Promise.resolve().then(again);
			return again();
		}`, "", func(l *jsruntime.Limits) { l.Timeout = 100 * time.Millisecond }},
		"helper loop":  {`async function execute() { return 1; }`, `for (;;) {}`, func(l *jsruntime.Limits) { l.Timeout = 100 * time.Millisecond }},
		"stack":        {`function recurse() { return recurse(); } async function execute() { return recurse(); }`, "", func(l *jsruntime.Limits) { l.MaxStackDepth = 16 }},
		"input bytes":  {`async function execute() { return 1; }`, "", func(l *jsruntime.Limits) { l.MaxInputBytes = 16 }},
		"output bytes": {`async function execute() { return "x".repeat(100); }`, "", func(l *jsruntime.Limits) { l.MaxOutputBytes = 32 }},
		"output nodes": {`async function execute() { return new Array(100).fill(1); }`, "", func(l *jsruntime.Limits) { l.MaxNodes = 32 }},
		"output depth": {`async function execute() { return [[[[[[1]]]]]]; }`, "", func(l *jsruntime.Limits) { l.MaxDepth = 4 }},
	} {
		t.Run(name, func(t *testing.T) {
			// Lower one cap while leaving the other limits sufficient for this scenario.
			limits := runtimeLimits()
			test.change(&limits)
			program, err := jsruntime.CompileAsync(test.source, asyncLibrary(t, test.library), asyncHostBindings(), limits)
			require.NoError(t, err)
			result, err := program.Evaluate(context.Background(), asyncInput(limits))

			// All phases discard partial output on a policy failure.
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, result)
		})
	}
}

// TestAsyncEvaluateCallerCancellation retains caller errors instead of reclassifying them as policy failures.
func TestAsyncEvaluateCallerCancellation(t *testing.T) {
	input := asyncInput(runtimeLimits())
	program := compileAsync(t, `async function execute() { for (;;) {} }`, nil)

	// A canceled caller takes precedence before authored code begins.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := program.Evaluate(ctx, input)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)

	// A caller deadline also interrupts executing JavaScript and retains its classification.
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result, err = program.Evaluate(ctx, input)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, result)
}

// TestAsyncEvaluateNoHostIO keeps the async runner free of host capabilities.
func TestAsyncEvaluateNoHostIO(t *testing.T) {
	program := compileAsync(t, `async function execute() {
		return {fetch: typeof fetch, require: typeof require, process: typeof process,
			setTimeout: typeof setTimeout, WebSocket: typeof WebSocket,
			sourceMaps: await eval("7\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json"),
			functionSourceMaps: (new Function("return 8\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json"))()};
	}`, nil)

	// Dynamic parsing stays local even after await, and no network/timer/module API is installed.
	require.JSONEq(t, `{"fetch":"undefined","require":"undefined","process":"undefined","setTimeout":"undefined","WebSocket":"undefined","sourceMaps":7,"functionSourceMaps":8}`, string(evaluateAsync(t, program)))
}

// TestAsyncEvaluateHostPhases gates admission until initialization and its jobs succeed.
func TestAsyncEvaluateHostPhases(t *testing.T) {
	for name, initialization := range map[string]string{
		"success":   `record("initialize"); Promise.resolve().then(() => record("initialize job"));`,
		"throw":     `record("initialize"); throw "private";`,
		"rejection": `record("initialize"); Promise.reject("private");`,
	} {
		t.Run(name, func(t *testing.T) {
			program := compileAsync(t, initialization+` async function execute() { record("execute"); return 42; }`, nil)
			var phases []string

			// Host admission and cleanup run on the same owner as authored initialization.
			result, err := program.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
				phases = append(phases, "prepare")
				invocation.Close = func() { phases = append(phases, "close") }
				invocation.Params, invocation.Context = goja.Undefined(), goja.Undefined()
				invocation.Start = func() { phases = append(phases, "start") }
				invocation.Await = func() error { t.Fatal("settled execute requested host work"); return nil }
				return vm.Set("record", func(phase string) { phases = append(phases, phase) })
			})

			// Initialization failures close the host without enabling it or invoking execute.
			if name == "success" {
				require.NoError(t, err)
				require.Equal(t, "42", string(result))
				require.Equal(t, []string{"prepare", "initialize", "initialize job", "start", "execute", "close"}, phases)
			} else {
				require.ErrorIs(t, err, jsruntime.ErrEvaluation)
				require.Nil(t, result)
				require.Equal(t, []string{"prepare", "initialize", "close"}, phases)
			}
		})
	}
}

// TestAsyncEvaluateHostCompletions permits late catches and stops when the root settles.
func TestAsyncEvaluateHostCompletions(t *testing.T) {
	for name, source := range map[string]string{
		"late catch": `async function execute() {
			const rejected = Promise.reject("private");
			await fetch();
			await fetch();
			return rejected.catch(() => 42);
		}`,
		"winning race": `async function execute() { return Promise.race([fetch(), fetch()]); }`,
	} {
		t.Run(name, func(t *testing.T) {
			program := compileAsync(t, source, nil)
			awaited, abandoned := 0, 0
			closed := false
			result, err := program.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
				var pending []func(any) error
				invocation.Params, invocation.Context = goja.Undefined(), goja.Undefined()
				invocation.Close = func() { closed = true; abandoned = len(pending) }
				invocation.Await = func() error {
					// One owner-thread completion may admit the next host operation.
					if len(pending) == 0 {
						return jsruntime.ErrEvaluation
					}
					resolve := pending[0]
					pending = pending[1:]
					awaited++
					return jsruntime.EvaluationError(resolve(42))
				}
				return vm.Set("fetch", func() goja.Value {
					promise, resolve, _ := vm.NewPromise()
					pending = append(pending, resolve)
					return vm.ToValue(promise)
				})
			})

			// Final rejection checks allow a catch installed after host awaits; a race leaves its loser alone.
			require.NoError(t, err)
			require.Equal(t, "42", string(result))
			require.True(t, closed)
			if name == "late catch" {
				require.Equal(t, 2, awaited)
				require.Zero(t, abandoned)
			} else {
				require.Equal(t, 1, awaited)
				require.Equal(t, 1, abandoned)
			}
		})
	}
}

// TestAsyncEvaluatePreparationCleanup releases a partially constructed host on error or panic.
func TestAsyncEvaluatePreparationCleanup(t *testing.T) {
	program := compileAsync(t, `async function execute() { throw "must not run"; }`, nil)
	safe := errors.New("safe preparation classification")
	for name, failure := range map[string]func() error{
		"error":           func() error { return safe },
		"interrupt panic": func() error { panic(&goja.InterruptedError{}) },
		"unknown panic":   func() error { panic("trusted host bug") },
	} {
		t.Run(name, func(t *testing.T) {
			closed := 0
			invoke := func() {
				result, err := program.Evaluate(context.Background(), func(_ context.Context, _ *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
					// Register cleanup as soon as a resource exists, before any later preparation can fail.
					invocation.Close = func() { closed++ }
					return failure()
				})
				require.Nil(t, result)
				if name == "error" {
					require.ErrorIs(t, err, safe)
				} else {
					require.ErrorIs(t, err, jsruntime.ErrEvaluation)
				}
			}

			// Unknown Go panics still propagate after cleanup; VM interruption is a safe error.
			if name == "unknown panic" {
				require.PanicsWithValue(t, "trusted host bug", invoke)
			} else {
				invoke()
			}
			require.Equal(t, 1, closed)
		})
	}
}

// TestAsyncEvaluateClosesBeforeEncoding makes host cleanup observable in the detached result.
func TestAsyncEvaluateClosesBeforeEncoding(t *testing.T) {
	program := compileAsync(t, `async function execute(args) { return args; }`, nil)
	result, err := program.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		// The owner can release resources before output conversion begins, even with an immediate root.
		params := vm.NewObject()
		invocation.Params, invocation.Context = params, goja.Undefined()
		invocation.Close = func() { require.NoError(t, params.Set("closed", true)) }
		invocation.Await = func() error { t.Fatal("fulfilled root requested host work"); return nil }
		return nil
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"closed":true}`, string(result))
}

// TestAsyncEvaluateHostFailure keeps host errors and cancellation classifications without partial output.
func TestAsyncEvaluateHostFailure(t *testing.T) {
	for _, kind := range []string{"no work", "host error", "caller canceled"} {
		t.Run(kind, func(t *testing.T) {
			limits := runtimeLimits()
			program, err := jsruntime.CompileAsync(`async function execute() { return await new Promise(() => {}); }`, nil, nil, limits)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			safe := errors.New("safe host classification")
			closed := false

			result, err := program.Evaluate(ctx, func(runCtx context.Context, _ *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
				invocation.Params, invocation.Context = goja.Undefined(), goja.Undefined()
				invocation.Close = func() { closed = true }
				invocation.Await = func() error {
					// Pending host waits cooperate with the same evaluation deadline.
					switch kind {
					case "no work":
						return jsruntime.ErrEvaluation
					case "host error":
						return safe
					case "caller canceled":
						cancel()
					}
					<-runCtx.Done()
					return runCtx.Err()
				}
				return nil
			})

			// Root failure always closes an admitted host, including during a host wait.
			require.Nil(t, result)
			require.True(t, closed)
			switch kind {
			case "no work":
				require.ErrorIs(t, err, jsruntime.ErrEvaluation)
			case "host error":
				require.ErrorIs(t, err, safe)
			case "caller canceled":
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

// TestAsyncEvaluateInvalidInputs rejects absent host state without entering authored code.
func TestAsyncEvaluateInvalidInputs(t *testing.T) {
	program := compileAsync(t, `async function execute() { throw "must not run"; }`, nil)
	for name, test := range map[string]struct {
		program *jsruntime.AsyncProgram
		ctx     context.Context
		prepare func(context.Context, *goja.Runtime, *jsruntime.AsyncInvocation) error
	}{
		"nil program":     {nil, context.Background(), asyncInput(runtimeLimits())},
		"empty program":   {&jsruntime.AsyncProgram{}, context.Background(), asyncInput(runtimeLimits())},
		"nil context":     {program, nil, asyncInput(runtimeLimits())},
		"nil preparation": {program, context.Background(), nil},
		"missing params": {program, context.Background(), func(_ context.Context, _ *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
			invocation.Context = goja.Undefined()
			return nil
		}},
		"missing context": {program, context.Background(), func(_ context.Context, _ *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
			invocation.Params = goja.Undefined()
			return nil
		}},
	} {
		t.Run(name, func(t *testing.T) {
			// Explicit JavaScript undefined is supported, but absent Go values are adapter errors.
			result, err := test.program.Evaluate(test.ctx, test.prepare)
			require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
			require.Nil(t, result)
		})
	}
}
