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

// runtimeLimits supplies explicit policy while leaving room for ordinary examples.
func runtimeLimits() jsruntime.Limits {
	return jsruntime.Limits{
		MaxSourceBytes: 64 << 10, MaxInputBytes: 64 << 10, MaxOutputBytes: 64 << 10,
		MaxDepth: 32, MaxNodes: 4096, MaxStackDepth: 128, Timeout: 2 * time.Second,
	}
}

// evaluateSource runs authored code through the same safe-error boundary as a domain adapter.
func evaluateSource(ctx context.Context, limits jsruntime.Limits, source string) (json.RawMessage, error) {
	return jsruntime.EvaluateJSON(ctx, limits, func(_ context.Context, vm *goja.Runtime) (goja.Value, error) {
		value, err := vm.RunString(source)
		return value, jsruntime.EvaluationError(err)
	})
}

// TestRuntimeInvalidOptions rejects missing policy and callbacks before invoking trusted work.
func TestRuntimeInvalidOptions(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*jsruntime.Limits)
	}{
		{"source", func(l *jsruntime.Limits) { l.MaxSourceBytes = 0 }},
		{"input", func(l *jsruntime.Limits) { l.MaxInputBytes = -1 }},
		{"output", func(l *jsruntime.Limits) { l.MaxOutputBytes = 0 }},
		{"depth", func(l *jsruntime.Limits) { l.MaxDepth = 0 }},
		{"nodes", func(l *jsruntime.Limits) { l.MaxNodes = 0 }},
		{"stack", func(l *jsruntime.Limits) { l.MaxStackDepth = 0 }},
		{"timeout", func(l *jsruntime.Limits) { l.Timeout = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := runtimeLimits()
			test.change(&limits)
			called := false

			// Both entry points enforce policy before allocating an evaluation for the callback.
			err := jsruntime.Run(context.Background(), limits, func(context.Context, *goja.Runtime) error {
				called = true
				return nil
			})
			require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
			result, err := jsruntime.EvaluateJSON(context.Background(), limits, func(context.Context, *goja.Runtime) (goja.Value, error) {
				called = true
				return nil, nil
			})
			require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
			require.Nil(t, result)
			require.False(t, called)
		})
	}

	// Absent contexts and callback functions are invalid API arguments, not panics.
	require.ErrorIs(t, jsruntime.Run(nil, runtimeLimits(), func(context.Context, *goja.Runtime) error {
		t.Fatal("nil-context callback ran")
		return nil
	}), jsruntime.ErrInvalidInput)
	require.ErrorIs(t, jsruntime.Run(context.Background(), runtimeLimits(), nil), jsruntime.ErrInvalidInput)
	result, err := jsruntime.EvaluateJSON(nil, runtimeLimits(), func(context.Context, *goja.Runtime) (goja.Value, error) {
		t.Fatal("nil-context callback ran")
		return nil, nil
	})
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, result)
	result, err = jsruntime.EvaluateJSON(context.Background(), runtimeLimits(), nil)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, result)
}

// TestRuntimeFailureBoundary preserves safe host classifications while removing authored diagnostics.
func TestRuntimeFailureBoundary(t *testing.T) {
	safe := errors.New("safe adapter classification")
	err := jsruntime.Run(context.Background(), runtimeLimits(), func(context.Context, *goja.Runtime) error { return safe })
	require.ErrorIs(t, err, safe)
	result, err := jsruntime.EvaluateJSON(context.Background(), runtimeLimits(), func(_ context.Context, vm *goja.Runtime) (goja.Value, error) {
		return vm.ToValue("discarded"), safe
	})
	require.ErrorIs(t, err, safe)
	require.Nil(t, result)

	// Trusted adapters classify both runtime exceptions and compilation diagnostics before returning them.
	for _, source := range []string{`throw new Error("private-runtime-marker")`, `const private_source_marker = ;`} {
		result, err := evaluateSource(context.Background(), runtimeLimits(), source)
		require.ErrorIs(t, err, jsruntime.ErrEvaluation)
		require.NotContains(t, err.Error(), "private")
		require.Nil(t, result)
	}
	require.NoError(t, jsruntime.EvaluationError(nil))

	// Stack exhaustion retains a limit classification instead of becoming an ordinary authored error.
	result, err = evaluateSource(context.Background(), runtimeLimits(), `function recurse() { return recurse(); } recurse();`)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.Nil(t, result)
}

// TestRuntimeOutputConversion captures pristine intrinsics before callbacks and charges the final JSON.
func TestRuntimeOutputConversion(t *testing.T) {
	result, err := evaluateSource(context.Background(), runtimeLimits(), `
		Object.getOwnPropertyDescriptor = () => ({value: "forged"});
		Reflect.ownKeys = () => [];
		JSON.stringify = () => "forged";
		({actual: [42, null]});
	`)
	require.NoError(t, err)
	require.JSONEq(t, `{"actual":[42,null]}`, string(result))

	// Escaping and quotes count toward the output allowance; failures never return a prefix.
	limits := runtimeLimits()
	limits.MaxOutputBytes = len(`"\n"`)
	result, err = evaluateSource(context.Background(), limits, `"\n"`)
	require.NoError(t, err)
	require.Equal(t, `"\n"`, string(result))
	limits.MaxOutputBytes--
	result, err = evaluateSource(context.Background(), limits, `"\n"`)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.Nil(t, result)
}

// TestRuntimeFreshVMHasNoDomainHosts leaves capability and input binding to the trusted caller.
func TestRuntimeFreshVMHasNoDomainHosts(t *testing.T) {
	for range 2 {
		// Mutations from the previous call cannot appear in this runtime's globals or prototypes.
		result, err := evaluateSource(context.Background(), runtimeLimits(), `
			const observed = [typeof fetch, typeof params, typeof context, typeof cfg,
				typeof toolResult, typeof process, typeof require,
				typeof globalThis.previous, typeof Object.prototype.previous];
			globalThis.previous = true;
			Object.prototype.previous = true;
			observed;
		`)
		require.NoError(t, err)
		require.JSONEq(t, `["undefined","undefined","undefined","undefined","undefined","undefined","undefined","undefined","undefined"]`, string(result))
	}

	// Dynamic compilation follows the same disabled-source-map policy as the owned VM.
	for _, source := range []string{
		`eval("7\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json")`,
		`(new Function("return 7\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json"))()`,
	} {
		result, err := evaluateSource(context.Background(), runtimeLimits(), source)
		require.NoError(t, err)
		require.Equal(t, "7", string(result))
	}
}

// TestRuntimeCancellationDiscardsSuccessfulWork keeps cancellation active beyond the callback's result.
func TestRuntimeCancellationDiscardsSuccessfulWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := jsruntime.EvaluateJSON(ctx, runtimeLimits(), func(_ context.Context, vm *goja.Runtime) (goja.Value, error) {
		value := vm.ToValue("finished callback")
		cancel()
		return value, nil
	})
	// The callback's success cannot outrun cancellation during conversion or final cleanup.
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	err = jsruntime.Run(ctx, runtimeLimits(), func(context.Context, *goja.Runtime) error {
		cancel()
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)

	// Even when policy expiry happened first, a subsequently canceled caller keeps its own error.
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	limits := runtimeLimits()
	limits.Timeout = 10 * time.Millisecond
	err = jsruntime.Run(ctx, limits, func(runCtx context.Context, _ *goja.Runtime) error {
		<-runCtx.Done()
		cancel()
		return jsruntime.ErrEvaluation
	})
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, jsruntime.ErrLimitExceeded)
}

// TestRuntimeInterruptsBusyJavaScript distinguishes host policy expiry from caller cancellation.
func TestRuntimeInterruptsBusyJavaScript(t *testing.T) {
	for _, policyTimeout := range []bool{false, true} {
		t.Run(fmt.Sprint("policy=", policyTimeout), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			limits := runtimeLimits()
			if policyTimeout {
				limits.Timeout = 100 * time.Millisecond
			}
			started := make(chan struct{})
			type outcome struct {
				result json.RawMessage
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := jsruntime.EvaluateJSON(ctx, limits, func(_ context.Context, vm *goja.Runtime) (goja.Value, error) {
					close(started)
					value, err := vm.RunString(`while (true) {}`)
					return value, jsruntime.EvaluationError(err)
				})
				done <- outcome{result, err}
			}()

			// Let the callback begin before canceling, so this exercises active VM interruption.
			select {
			case <-started:
			case observed := <-done:
				// A heavily delayed admission may exhaust policy before reaching the callback.
				require.True(t, policyTimeout)
				require.ErrorIs(t, observed.err, jsruntime.ErrLimitExceeded)
				require.Nil(t, observed.result)
				return
			case <-time.After(5 * time.Second):
				cancel()
				t.Fatal("runtime did not admit the callback before the watchdog expired")
			}
			if !policyTimeout {
				cancel()
			}
			select {
			case observed := <-done:
				require.Nil(t, observed.result)
				if policyTimeout {
					require.ErrorIs(t, observed.err, jsruntime.ErrLimitExceeded)
					require.NoError(t, ctx.Err())
				} else {
					require.ErrorIs(t, observed.err, context.Canceled)
				}
			case <-time.After(5 * time.Second):
				cancel()
				t.Fatal("busy JavaScript did not stop before the watchdog expired")
			}
		})
	}
}

// TestRuntimeConcurrentEvaluations reuses compiled code without sharing authored globals or prototypes.
func TestRuntimeConcurrentEvaluations(t *testing.T) {
	program, err := goja.Compile("example.js", `
		globalThis.calls = (globalThis.calls || 0) + 1;
		Object.prototype.calls = (Object.prototype.calls || 0) + 1;
		({id: id, calls: globalThis.calls, inherited: ({}).calls});
	`, true)
	require.NoError(t, err)
	const count = 12
	type outcome struct {
		id     int
		result json.RawMessage
		err    error
	}
	results := make(chan outcome, count)
	var workers sync.WaitGroup
	for id := range count {
		workers.Go(func() {
			result, err := jsruntime.EvaluateJSON(context.Background(), runtimeLimits(), func(_ context.Context, vm *goja.Runtime) (goja.Value, error) {
				if err := vm.Set("id", id); err != nil {
					return nil, jsruntime.EvaluationError(err)
				}
				value, err := vm.RunProgram(program)
				return value, jsruntime.EvaluationError(err)
			})
			results <- outcome{id, result, err}
		})
	}
	workers.Wait()
	close(results)

	// Each callback and encoder stay with one fresh VM even when the program is shared.
	for observed := range results {
		require.NoError(t, observed.err)
		require.JSONEq(t, fmt.Sprintf(`{"id":%d,"calls":1,"inherited":1}`, observed.id), string(observed.result))
	}
}
