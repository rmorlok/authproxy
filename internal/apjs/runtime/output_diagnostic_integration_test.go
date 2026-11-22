package runtime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// requireOutputReason checks the detached reason without weakening the existing
// opaque error category or retaining a source-bearing Goja exception.
func requireOutputReason(t *testing.T, err error, want jsruntime.OutputRejectionReason) {
	t.Helper()
	require.ErrorIs(t, err, jsruntime.ErrInvalidOutput)
	require.EqualError(t, err, jsruntime.ErrInvalidOutput.Error())
	reason, ok := jsruntime.OutputReason(fmt.Errorf("trusted wrapper: %w", err))
	require.True(t, ok)
	require.Equal(t, want, reason)
	require.NotContains(t, fmt.Sprintf("%+v %#v %s", err, err, reason), "private")
	var exception *goja.Exception
	require.False(t, errors.As(err, &exception))
}

// TestOutputDiagnosticExpressionAndAsyncParity reports the same reason through
// pure and async execution without exposing configuration-derived property names.
func TestOutputDiagnosticExpressionAndAsyncParity(t *testing.T) {
	for _, test := range []struct {
		name, expression string
		reason           jsruntime.OutputRejectionReason
	}{
		{"accessor", `({get [secret]() { throw secret; }})`, jsruntime.OutputAccessor},
		{"cycle", `(() => { const result = {}; result[secret] = result; return result; })()`, jsruntime.OutputCycle},
		{"native object", `({[secret]: new Date(0)})`, jsruntime.OutputUnsupportedObject},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The key comes from copied host data, so even a helpful JSON path would leak it.
			expression, err := jsruntime.CompileExpression(test.expression, runtimeLimits())
			require.NoError(t, err)
			result, err := expression.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime) error {
				return vm.Set("secret", "private-key-from-config")
			})
			require.Nil(t, result)
			requireOutputReason(t, err, test.reason)

			// Async result conversion supplies the identical safe classification.
			program := compileAsync(t, "async function execute(secret) { return "+test.expression+"; }", nil)
			result, err = program.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
				invocation.Params, invocation.Context = vm.ToValue("private-key-from-config"), goja.Undefined()
				return nil
			})
			require.Nil(t, result)
			requireOutputReason(t, err, test.reason)
		})
	}
}

// TestOutputDiagnosticExtendedResults preserves reasons for both branded
// successes and recognized root rejections using the shared result encoder.
func TestOutputDiagnosticExtendedResults(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(fmt.Sprintf("rejected=%t", rejected), func(t *testing.T) {
			statement := `return domainResult(value);`
			if rejected {
				statement = `throw domainFailure(value);`
			}
			program, err := jsruntime.CompileExtendedAsync(`async function execute(params) {
				const value = {get [params.private]() {throw params.private;}};
				`+statement+`
			}`, nil, nil, runtimeLimits(), brandedResultExtension())
			require.NoError(t, err)
			result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))

			// No partial domain result escapes, and the rejected getter is never invoked.
			require.Equal(t, extensionOutcome{}, result)
			requireOutputReason(t, err, jsruntime.OutputAccessor)
		})
	}
}

// TestOutputDiagnosticStickyEncoder keeps the first reason even if trusted
// conversion ignores it, retries with another value, or declines recognition.
func TestOutputDiagnosticStickyEncoder(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(fmt.Sprintf("rejected=%t", rejected), func(t *testing.T) {
			var first, second error
			extension := jsruntime.ResultExtension[json.RawMessage]{
				Install: func(_ context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[json.RawMessage], error) {
					convert := func(_ context.Context, value goja.Value, encoder *jsruntime.ResultEncoder) (json.RawMessage, error) {
						// The second attempt is valid, but cannot erase the first conversion's failure.
						_, first = encoder.Encode(value)
						_, second = encoder.String(vm.ToValue("safe"))
						return json.RawMessage(`"private-partial-result"`), nil
					}
					return jsruntime.ResultInterpreter[json.RawMessage]{
						Fulfilled: convert,
						Rejected: func(ctx context.Context, value goja.Value, encoder *jsruntime.ResultEncoder) (json.RawMessage, bool, error) {
							result, err := convert(ctx, value, encoder)
							return result, false, err
						},
					}, nil
				},
			}
			statement := `return {get privateKey() {throw "private-value";}};`
			if rejected {
				statement = `throw {get privateKey() {throw "private-value";}};`
			}
			program, err := jsruntime.CompileExtendedAsync("async function execute() {"+statement+"}", nil, nil, runtimeLimits(), extension)
			require.NoError(t, err)
			result, err := program.Evaluate(context.Background(), asyncInput(runtimeLimits()))

			// Ignoring the failure or declining a rejected root still discards the partial result.
			require.Nil(t, result)
			require.Same(t, first, second)
			requireOutputReason(t, err, jsruntime.OutputAccessor)
		})
	}
}

// TestOutputDiagnosticFinalFailurePrecedence drops a preliminary reason when
// cancellation, shared-budget exhaustion, or an unrelated Promise wins later.
func TestOutputDiagnosticFinalFailurePrecedence(t *testing.T) {
	for _, mode := range []string{"cancellation", "budget", "promise"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			limits := runtimeLimits()
			var first error
			extension := jsruntime.ResultExtension[json.RawMessage]{
				Install: func(_ context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[json.RawMessage], error) {
					return jsruntime.ResultInterpreter[json.RawMessage]{
						Fulfilled: func(runCtx context.Context, value goja.Value, encoder *jsruntime.ResultEncoder) (json.RawMessage, error) {
							_, first = encoder.Encode(value)
							// Trigger the later failure deterministically while conversion still owns the VM.
							switch mode {
							case "cancellation":
								cancel()
							case "budget":
								require.ErrorIs(t, jsruntime.ChargeWork(runCtx, limits.MaxWorkUnits), jsruntime.ErrLimitExceeded)
							case "promise":
								_, err := vm.RunString(`Promise.reject("private-unrelated-rejection")`)
								require.NoError(t, err)
							}
							return nil, first
						},
					}, nil
				},
			}
			program, err := jsruntime.CompileExtendedAsync(`async function execute() {
				return {get privateKey() {throw "private-value";}};
			}`, nil, nil, limits, extension)
			require.NoError(t, err)
			result, err := program.Evaluate(ctx, asyncInput(limits))
			requireOutputReason(t, first, jsruntime.OutputAccessor)

			// The selected final failure must not report a stale conversion reason.
			want := jsruntime.ErrEvaluation
			if mode == "cancellation" {
				want = context.Canceled
			} else if mode == "budget" {
				want = jsruntime.ErrLimitExceeded
			}
			require.ErrorIs(t, err, want)
			require.Nil(t, result)
			reason, ok := jsruntime.OutputReason(err)
			require.False(t, ok)
			require.Empty(t, reason)
		})
	}
}

// TestOutputDiagnosticCaughtLogFailure keeps record-validation failures local
// to the console call instead of poisoning later result conversion diagnostics.
func TestOutputDiagnosticCaughtLogFailure(t *testing.T) {
	for _, invalidResult := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalid-result=%t", invalidResult), func(t *testing.T) {
			var buffer bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buffer, nil))
			resultExpression := `true`
			if invalidResult {
				resultExpression = `new Date(0)`
			}
			program := compileAsync(t, `async function execute() {
				try {console.info("private-record", {get privateKey() {throw "private-value";}});} catch (_) {}
				return `+resultExpression+`;
			}`, nil)
			result, err := program.Evaluate(context.Background(), loggingInput(logger))

			// The refused record emits nothing; a later failure describes only the result itself.
			require.Empty(t, buffer.String())
			if invalidResult {
				require.Nil(t, result)
				requireOutputReason(t, err, jsruntime.OutputUnsupportedObject)
			} else {
				require.NoError(t, err)
				require.JSONEq(t, `true`, string(result))
				reason, ok := jsruntime.OutputReason(err)
				require.False(t, ok)
				require.Empty(t, reason)
			}
		})
	}
}

// TestOutputDiagnosticConcurrentIsolation reuses one compiled program while
// each fresh VM reports its own fixed reason independently of private keys.
func TestOutputDiagnosticConcurrentIsolation(t *testing.T) {
	program := compileAsync(t, `async function execute(kind) {
		const key = "private-key-" + kind;
		if (kind % 3 === 0) return {get [key]() {throw key;}};
		if (kind % 3 === 1) {const result = {}; result[key] = result; return result;}
		return {[key]: new Date(0)};
	}`, nil)
	type outcome struct {
		index int
		err   error
	}
	results := make(chan outcome, 12)
	var workers sync.WaitGroup
	for index := range cap(results) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := program.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
				invocation.Params, invocation.Context = vm.ToValue(index), goja.Undefined()
				return nil
			})
			results <- outcome{index: index, err: err}
		}()
	}
	workers.Wait()
	close(results)
	want := []jsruntime.OutputRejectionReason{jsruntime.OutputAccessor, jsruntime.OutputCycle, jsruntime.OutputUnsupportedObject}
	for result := range results {
		requireOutputReason(t, result.err, want[result.index%len(want)])
	}
}
