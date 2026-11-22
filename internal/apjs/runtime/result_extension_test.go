package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// extensionOutcome represents detached adapter data, independently of any Tool contract.
type extensionOutcome struct {
	Kind   string            // Identifies a recognized success, failure, or ordinary JSON value.
	Fields []json.RawMessage // Owns payloads copied under one invocation's output allowance.
}

// brandedResultExtension installs pure helpers with identity held privately by each VM.
// Looking like a helper result never grants an ordinary authored object the same brand.
func brandedResultExtension() jsruntime.ResultExtension[extensionOutcome] {
	return jsruntime.ResultExtension[extensionOutcome]{
		Bindings: []jsruntime.HostBinding{{Name: "domainResult"}, {Name: "domainFailure"}},
		Install: func(_ context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[extensionOutcome], error) {
			results := make(map[*goja.Object][]goja.Value)
			failures := make(map[*goja.Object][]goja.Value)
			for name, registry := range map[string]map[*goja.Object][]goja.Value{
				"domainResult": results, "domainFailure": failures,
			} {
				if err := vm.Set(name, func(call goja.FunctionCall) goja.Value {
					marker := vm.NewObject()
					registry[marker] = append([]goja.Value(nil), call.Arguments...)
					return marker
				}); err != nil {
					return jsruntime.ResultInterpreter[extensionOutcome]{}, jsruntime.ErrInvalidInput
				}
			}

			return jsruntime.ResultInterpreter[extensionOutcome]{
				Fulfilled: func(_ context.Context, value goja.Value, encoder *jsruntime.ResultEncoder) (extensionOutcome, error) {
					if object, ok := value.(*goja.Object); ok {
						if fields, branded := results[object]; branded {
							return encodeExtensionFields("success", fields, encoder)
						}
					}
					return encodeExtensionFields("json", []goja.Value{value}, encoder)
				},
				Rejected: func(_ context.Context, value goja.Value, encoder *jsruntime.ResultEncoder) (extensionOutcome, bool, error) {
					object, ok := value.(*goja.Object)
					if !ok {
						return extensionOutcome{}, false, nil
					}
					fields, branded := failures[object]
					if !branded {
						return extensionOutcome{}, false, nil
					}
					result, err := encodeExtensionFields("failure", fields, encoder)
					return result, true, err
				},
			}, nil
		},
	}
}

// encodeExtensionFields converts each selected field with the same bounded encoder.
func encodeExtensionFields(kind string, fields []goja.Value, encoder *jsruntime.ResultEncoder) (extensionOutcome, error) {
	result := extensionOutcome{Kind: kind}
	for _, field := range fields {
		data, err := encoder.Encode(field)
		if err != nil {
			return extensionOutcome{}, err
		}
		result.Fields = append(result.Fields, data)
	}
	return result, nil
}

// TestResultExtensionInstallation uses the same helpers during validation and real initialization.
func TestResultExtensionInstallation(t *testing.T) {
	extension := brandedResultExtension()
	install := extension.Install
	var installations atomic.Int32
	extension.Install = func(ctx context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[extensionOutcome], error) {
		installations.Add(1)
		return install(ctx, vm)
	}

	// Compilation only reserves names; pure helpers may be used by top-level initialization later.
	program, err := jsruntime.CompileExtendedAsync(`
		const initialized = domainResult("ready");
		async function execute() { return initialized; }
	`, nil, nil, runtimeLimits(), extension)
	require.NoError(t, err)
	require.Zero(t, installations.Load())
	require.NoError(t, program.Validate(context.Background()))
	require.EqualValues(t, 1, installations.Load())

	// An invocation installs its own helpers and returns detached data from that VM alone.
	result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
	require.NoError(t, err)
	require.EqualValues(t, 2, installations.Load())
	require.Equal(t, "success", result.Kind)
	require.Equal(t, []json.RawMessage{json.RawMessage(`"ready"`)}, result.Fields)
}

// TestResultExtensionBindings prevents source and libraries from claiming adapter-owned helper names.
func TestResultExtensionBindings(t *testing.T) {
	for name, test := range map[string]struct{ source, library string }{
		"source lexical": {`const domainResult = 1; async function execute() {}`, ""},
		"source var":     {`var domainFailure; async function execute() {}`, ""},
		"library":        {`async function execute() {}`, `function domainResult() {}`},
	} {
		t.Run(name, func(t *testing.T) {
			// Neither direct declarations nor library declarations can capture a reserved helper.
			_, err := jsruntime.CompileExtendedAsync(test.source, asyncLibrary(t, test.library), nil, runtimeLimits(), brandedResultExtension())
			require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
		})
	}

	// Changing the caller's descriptor slice after compilation cannot alter the frozen binding.
	extension := brandedResultExtension()
	program, err := jsruntime.CompileExtendedAsync(`async function execute() {
		const descriptor = Object.getOwnPropertyDescriptor(globalThis, "domainResult");
		return [descriptor.writable, descriptor.configurable, descriptor.enumerable];
	}`, nil, nil, runtimeLimits(), extension)
	require.NoError(t, err)
	extension.Bindings[0].Name = "changedAfterCompilation"
	result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
	require.NoError(t, err)
	require.JSONEq(t, `[false,false,false]`, string(result.Fields[0]))

	// Duplicate ownership is rejected rather than choosing one binding policy silently.
	_, err = jsruntime.CompileExtendedAsync(`async function execute() {}`, nil,
		[]jsruntime.HostBinding{{Name: "domainResult"}}, runtimeLimits(), brandedResultExtension())
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
}

// TestResultExtensionInvalidInstallation fails safely before initialization when no usable converter exists.
func TestResultExtensionInvalidInstallation(t *testing.T) {
	// A missing installer is rejected during compilation, without evaluating the source.
	_, err := jsruntime.CompileExtendedAsync(`async function execute() {}`, nil, nil, runtimeLimits(), jsruntime.ResultExtension[extensionOutcome]{})
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)

	safe := errors.New("safe installation failure")
	for name, installErr := range map[string]error{"installer error": safe, "missing fulfilled callback": nil} {
		t.Run(name, func(t *testing.T) {
			extension := jsruntime.ResultExtension[extensionOutcome]{
				Install: func(context.Context, *goja.Runtime) (jsruntime.ResultInterpreter[extensionOutcome], error) {
					return jsruntime.ResultInterpreter[extensionOutcome]{}, installErr
				},
			}
			program, err := jsruntime.CompileExtendedAsync(`async function execute() {}`, nil, nil, runtimeLimits(), extension)
			require.NoError(t, err)
			want := installErr
			if want == nil {
				want = jsruntime.ErrInvalidInput
			}

			// Validation and invocation enforce the same interpreter contract.
			require.ErrorIs(t, program.Validate(context.Background()), want)
			result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
			require.ErrorIs(t, err, want)
			require.Equal(t, extensionOutcome{}, result)
		})
	}
}

// TestResultExtensionRecognizesOnlyBrandedRoots preserves ordinary results and safe rejection failures.
func TestResultExtensionRecognizesOnlyBrandedRoots(t *testing.T) {
	for name, test := range map[string]struct {
		body string
		kind string
		json string
		err  error
	}{
		"success":          {body: `return domainResult({value: 42});`, kind: "success", json: `{"value":42}`},
		"failure":          {body: `throw domainFailure({code: "denied"});`, kind: "failure", json: `{"code":"denied"}`},
		"ordinary result":  {body: `return {kind:"success", fields:[42]};`, kind: "json", json: `{"kind":"success","fields":[42]}`},
		"ordinary failure": {body: `throw {kind:"failure", fields:["private"]};`, err: jsruntime.ErrEvaluation},
		"string failure":   {body: `throw "private rejection";`, err: jsruntime.ErrEvaluation},
		"error failure":    {body: `throw new Error("private rejection");`, err: jsruntime.ErrEvaluation},
		"wrong brand":      {body: `throw domainResult("private rejection");`, err: jsruntime.ErrEvaluation},
		"invalid payload":  {body: `throw domainFailure({get code() { throw "private getter"; }});`, err: jsruntime.ErrInvalidOutput},
	} {
		t.Run(name, func(t *testing.T) {
			program, err := jsruntime.CompileExtendedAsync("async function execute() {"+test.body+"}", nil, nil, runtimeLimits(), brandedResultExtension())
			require.NoError(t, err)

			// Only the helper's private identity grants domain interpretation; ordinary throws stay private.
			result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
				require.NotContains(t, err.Error(), "private")
				require.Equal(t, extensionOutcome{}, result)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.kind, result.Kind)
				require.Len(t, result.Fields, 1)
				require.JSONEq(t, test.json, string(result.Fields[0]))
			}
		})
	}
}

// TestResultExtensionUnhandledRejections excludes precisely the root, including already-handled roots.
func TestResultExtensionUnhandledRejections(t *testing.T) {
	for name, source := range map[string]string{
		"detached with rejected root": `async function execute() {
			Promise.reject("private detached failure"); throw domainFailure("safe");
		}`,
		"detached with fulfilled root": `async function execute() {
			Promise.reject("private detached failure"); return domainResult("safe");
		}`,
		"already handled rejected root": `async function execute() { throw domainFailure("safe"); }
			const original = execute;
			execute = Object.setPrototypeOf(() => {
				const root = original(); root.catch(() => {});
				Promise.reject("private detached failure"); return root;
			}, Object.getPrototypeOf(original));`,
		"initialization": `Promise.reject(domainFailure("safe"));
			async function execute() { return domainResult("safe"); }`,
	} {
		t.Run(name, func(t *testing.T) {
			extension := brandedResultExtension()
			install := extension.Install
			conversions := 0
			extension.Install = func(ctx context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[extensionOutcome], error) {
				handler, err := install(ctx, vm)
				handler.Fulfilled = func(context.Context, goja.Value, *jsruntime.ResultEncoder) (extensionOutcome, error) {
					conversions++
					return extensionOutcome{Kind: "must not escape"}, nil
				}
				handler.Rejected = func(context.Context, goja.Value, *jsruntime.ResultEncoder) (extensionOutcome, bool, error) {
					conversions++
					return extensionOutcome{Kind: "must not escape"}, true, nil
				}
				return handler, err
			}
			program, err := jsruntime.CompileExtendedAsync(source, nil, nil, runtimeLimits(), extension)
			require.NoError(t, err)

			// Unrelated failures block all result interpretation instead of being consumed with the root.
			result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
			require.ErrorIs(t, err, jsruntime.ErrEvaluation)
			require.Equal(t, extensionOutcome{}, result)
			require.Zero(t, conversions)
		})
	}
}

// TestResultExtensionChecksRejectionsAfterConversion rejects failures introduced by trusted conversion work.
func TestResultExtensionChecksRejectionsAfterConversion(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(fmt.Sprintf("rejected=%v", rejected), func(t *testing.T) {
			extension := brandedResultExtension()
			install := extension.Install
			extension.Install = func(ctx context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[extensionOutcome], error) {
				handler, err := install(ctx, vm)
				handler.Fulfilled = func(context.Context, goja.Value, *jsruntime.ResultEncoder) (extensionOutcome, error) {
					_, runErr := vm.RunString(`Promise.reject("private converter failure")`)
					require.NoError(t, runErr)
					return extensionOutcome{Kind: "converted"}, nil
				}
				handler.Rejected = func(context.Context, goja.Value, *jsruntime.ResultEncoder) (extensionOutcome, bool, error) {
					_, runErr := vm.RunString(`Promise.reject("private converter failure")`)
					require.NoError(t, runErr)
					return extensionOutcome{Kind: "converted"}, true, errors.New("safe domain failure")
				}
				return handler, err
			}
			body := `return domainResult("safe");`
			if rejected {
				body = `throw domainFailure("safe");`
			}
			program, err := jsruntime.CompileExtendedAsync("async function execute() {"+body+"}", nil, nil, runtimeLimits(), extension)
			require.NoError(t, err)

			// Draining microtasks in a converter cannot bypass the final rejection check, even with a domain error.
			result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
			require.Equal(t, jsruntime.ErrEvaluation, err)
			require.Equal(t, extensionOutcome{}, result)
		})
	}
}

// TestResultExtensionUnrecognizedError cannot expose a converter's rejected fallback payload or error.
func TestResultExtensionUnrecognizedError(t *testing.T) {
	extension := brandedResultExtension()
	install := extension.Install
	extension.Install = func(ctx context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[extensionOutcome], error) {
		handler, err := install(ctx, vm)
		handler.Rejected = func(context.Context, goja.Value, *jsruntime.ResultEncoder) (extensionOutcome, bool, error) {
			return extensionOutcome{Kind: "private result"}, false, errors.New("private diagnostic")
		}
		return handler, err
	}
	program, err := jsruntime.CompileExtendedAsync(`async function execute() { throw "private reason"; }`, nil, nil, runtimeLimits(), extension)
	require.NoError(t, err)

	// A declined root interpretation retains the generic safe failure classification.
	result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
	require.Equal(t, jsruntime.ErrEvaluation, err)
	require.Equal(t, extensionOutcome{}, result)
}

// TestResultExtensionUnrecognizedRootCannotEraseLimit retains encoder limits even if recognition is declined.
func TestResultExtensionUnrecognizedRootCannotEraseLimit(t *testing.T) {
	limits := runtimeLimits()
	limits.MaxOutputBytes = 4
	extension := brandedResultExtension()
	install := extension.Install
	extension.Install = func(ctx context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[extensionOutcome], error) {
		handler, err := install(ctx, vm)
		handler.Rejected = func(_ context.Context, value goja.Value, encoder *jsruntime.ResultEncoder) (extensionOutcome, bool, error) {
			_, _ = encoder.Encode(value)
			return extensionOutcome{Kind: "partial"}, false, nil
		}
		return handler, err
	}
	program, err := jsruntime.CompileExtendedAsync(`async function execute() { throw "too large"; }`, nil, nil, limits, extension)
	require.NoError(t, err)

	// A faulty converter cannot clear an exhausted output allowance by subsequently declining the value.
	result, err := program.Evaluate(context.Background(), asyncInput(limits))
	require.Equal(t, jsruntime.ErrLimitExceeded, err)
	require.Equal(t, extensionOutcome{}, result)
}

// TestResultExtensionOptionalRejectionConverter leaves root failures generic when no handler is installed.
func TestResultExtensionOptionalRejectionConverter(t *testing.T) {
	extension := brandedResultExtension()
	install := extension.Install
	extension.Install = func(ctx context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[extensionOutcome], error) {
		handler, err := install(ctx, vm)
		handler.Rejected = nil
		return handler, err
	}
	program, err := jsruntime.CompileExtendedAsync(`async function execute() { throw domainFailure("private"); }`, nil, nil, runtimeLimits(), extension)
	require.NoError(t, err)

	// Pure helpers alone do not authorize exporting their rejected values.
	result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
	require.Equal(t, jsruntime.ErrEvaluation, err)
	require.Equal(t, extensionOutcome{}, result)
}

// TestResultExtensionSharedOutputAllowance bounds all selected payload fields together.
func TestResultExtensionSharedOutputAllowance(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(fmt.Sprintf("rejected=%v", rejected), func(t *testing.T) {
			limits := runtimeLimits()
			limits.MaxOutputBytes = 7
			body := `return domainResult("ab", "cd");`
			if rejected {
				body = `throw domainFailure("ab", "cd");`
			}
			program, err := jsruntime.CompileExtendedAsync("async function execute() {"+body+"}", nil, nil, limits, brandedResultExtension())
			require.NoError(t, err)

			// Each field fits alone, but its JSON quotes and bytes count toward the same seven-byte cap.
			result, err := program.Evaluate(context.Background(), asyncInput(limits))
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Equal(t, extensionOutcome{}, result)
		})
	}

	// Even an adapter accidentally ignoring a failed encode cannot return partial output.
	extension := brandedResultExtension()
	install := extension.Install
	extension.Install = func(ctx context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[extensionOutcome], error) {
		handler, err := install(ctx, vm)
		handler.Fulfilled = func(_ context.Context, value goja.Value, encoder *jsruntime.ResultEncoder) (extensionOutcome, error) {
			_, _ = encoder.Encode(value)
			return extensionOutcome{Kind: "partial"}, nil
		}
		return handler, err
	}
	program, err := jsruntime.CompileExtendedAsync(`async function execute() { return undefined; }`, nil, nil, runtimeLimits(), extension)
	require.NoError(t, err)
	result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))
	require.ErrorIs(t, err, jsruntime.ErrInvalidOutput)
	require.Equal(t, extensionOutcome{}, result)
}

// TestResultExtensionCleanupAndCancellation closes hosts before conversion and keeps the deadline active.
func TestResultExtensionCleanupAndCancellation(t *testing.T) {
	for _, cancelCaller := range []bool{false, true} {
		t.Run(fmt.Sprintf("caller=%v", cancelCaller), func(t *testing.T) {
			limits := runtimeLimits()
			limits.Timeout = 50 * time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			closed := false
			extension := brandedResultExtension()
			install := extension.Install
			extension.Install = func(ctx context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[extensionOutcome], error) {
				handler, err := install(ctx, vm)
				handler.Fulfilled = func(runCtx context.Context, _ goja.Value, _ *jsruntime.ResultEncoder) (extensionOutcome, error) {
					require.True(t, closed, "host cleanup must precede result interpretation")
					if cancelCaller {
						cancel()
					}
					<-runCtx.Done()
					return extensionOutcome{Kind: "late"}, nil
				}
				return handler, err
			}
			program, err := jsruntime.CompileExtendedAsync(`async function execute() { return 42; }`, nil, nil, limits, extension)
			require.NoError(t, err)

			// Cleanup runs once, while the conversion callback still shares the execution deadline.
			result, err := program.Evaluate(ctx, func(_ context.Context, _ *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
				invocation.Params, invocation.Context = goja.Undefined(), goja.Undefined()
				invocation.Close = func() { closed = true }
				return nil
			})
			if cancelCaller {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			}
			require.Equal(t, extensionOutcome{}, result)
		})
	}
}

// TestResultExtensionConcurrentEvaluations gives each invocation private brands and detached data.
func TestResultExtensionConcurrentEvaluations(t *testing.T) {
	program, err := jsruntime.CompileExtendedAsync(`async function execute(params) {
		await Promise.resolve(); return domainResult(params);
	}`, nil, nil, runtimeLimits(), brandedResultExtension())
	require.NoError(t, err)

	// Sharing compiled code never shares a VM, its helper registry, or the output buffers.
	var work sync.WaitGroup
	for i := range 12 {
		work.Go(func() {
			result, err := program.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
				invocation.Params, invocation.Context = vm.ToValue(i), goja.Undefined()
				return nil
			})
			require.NoError(t, err)
			require.Equal(t, "success", result.Kind)
			require.Equal(t, []json.RawMessage{json.RawMessage(fmt.Sprint(i))}, result.Fields)
		})
	}
	work.Wait()
}
