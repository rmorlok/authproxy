package runtime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// TestLoggingInitializationGate covers both synchronous setup and its drained
// Promise jobs. Only execute may emit after all initialization checks succeed.
func TestLoggingInitializationGate(t *testing.T) {
	for _, uncaught := range []bool{false, true} {
		t.Run(fmt.Sprintf("uncaught=%t", uncaught), func(t *testing.T) {
			var buffer bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buffer, nil))
			source := `let blocked = 0;
				try {console.info("private-init");} catch (_) {blocked++;}
				Promise.resolve().then(() => {
					try {console.info("private-init-job");} catch (_) {blocked++;}
				});
				async function execute() {console.info("execute"); return blocked;}`
			if uncaught {
				source = `Promise.resolve().then(() => console.info("private-init-job"));
					async function execute() {console.info("execute"); return true;}`
			}
			program := compileAsync(t, source, nil)
			result, err := program.Evaluate(context.Background(), loggingInput(logger))

			// Caught attempts allow execution; an abandoned rejection keeps the gate closed.
			if uncaught {
				require.ErrorIs(t, err, jsruntime.ErrEvaluation)
				require.Nil(t, result)
				require.Empty(t, buffer.String())
			} else {
				require.NoError(t, err)
				require.JSONEq(t, `2`, string(result))
				records := loggingRecords(t, &buffer)
				require.Len(t, records, 1)
				require.Equal(t, "execute", records[0]["javascript"].(map[string]any)["message"])
			}
		})
	}
}

// TestLoggingClosesBeforeCleanupAndInterpretation exercises the extended result
// path and verifies neither trusted phase can invoke an authored logger late.
func TestLoggingClosesBeforeCleanupAndInterpretation(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	var cleanupBlocked, resultBlocked bool
	program, err := jsruntime.CompileExtendedAsync(`async function execute() {
		console.info("execute"); return true;
	}`, nil, nil, runtimeLimits(), jsruntime.ResultExtension[json.RawMessage]{
		Install: func(_ context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[json.RawMessage], error) {
			return jsruntime.ResultInterpreter[json.RawMessage]{
				Fulfilled: func(_ context.Context, value goja.Value, encoder *jsruntime.ResultEncoder) (json.RawMessage, error) {
					// Interpretation runs with the deadline active, but emission is already closed.
					_, callErr := vm.RunString(`console.info("private-interpretation")`)
					resultBlocked = callErr != nil
					return encoder.Encode(value)
				},
			}, nil
		},
	})
	require.NoError(t, err)
	result, err := program.Evaluate(context.Background(), func(ctx context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		if err := loggingInput(logger)(ctx, vm, invocation); err != nil {
			return err
		}
		invocation.Close = func() {
			_, callErr := vm.RunString(`console.info("private-cleanup")`)
			cleanupBlocked = callErr != nil
		}
		return nil
	})

	// Cleanup and interpretation cannot create records after the root has settled.
	require.NoError(t, err)
	require.JSONEq(t, `true`, string(result))
	require.True(t, cleanupBlocked)
	require.True(t, resultBlocked)
	records := loggingRecords(t, &buffer)
	require.Len(t, records, 1)
	require.Equal(t, "execute", records[0]["javascript"].(map[string]any)["message"])
}

// TestLoggingCancellationDuringConversion cancels deterministically while the
// native encoder scans a primitive message, before any sink receives data.
func TestLoggingCancellationDuringConversion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	program := compileAsync(t, `async function execute(message) {
		try {console.info(message);} catch (_) {}
		return "private-partial-result";
	}`, nil)
	var input *base64CancelString
	result, err := program.Evaluate(ctx, func(_ context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		// This wrapped primitive triggers cancellation after a known number of code units.
		input = &base64CancelString{
			base64PrimitiveString: vm.ToValue(strings.Repeat("x", 8192)).(goja.String),
			cancel:                cancel, cancelAfter: 128,
		}
		invocation.Params, invocation.Context = input, goja.Undefined()
		invocation.Logger = logger
		return nil
	})

	// Even a caught interruption cannot emit partial data or publish the sentinel result.
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
	require.Empty(t, buffer.String())
	require.GreaterOrEqual(t, input.reads, 128)
	require.Less(t, input.reads, 8192)
}

// TestLoggingConcurrentIsolation reuses compiled source while each evaluation
// captures a distinct sink and fixed attribution with its own execution budget.
func TestLoggingConcurrentIsolation(t *testing.T) {
	program := compileAsync(t, `async function execute(index) {
		const fields = {index}; console.info("item", fields);
		fields.index = "changed"; return index;
	}`, nil)
	const count = 8
	var buffers [count]bytes.Buffer
	errors := make(chan error, count)
	var workers sync.WaitGroup
	for index := range count {
		workers.Add(1)
		go func() {
			defer workers.Done()
			logger := slog.New(slog.NewJSONHandler(&buffers[index], nil)).With("invocation", index)
			result, err := program.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
				invocation.Params, invocation.Context = vm.ToValue(index), goja.Undefined()
				invocation.Logger = logger
				return nil
			})
			if err == nil && string(result) != fmt.Sprint(index) {
				err = fmt.Errorf("unexpected isolated result: %s", result)
			}
			errors <- err
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}

	// Detached log fields retain their original values despite later authored mutation.
	for index := range count {
		records := loggingRecords(t, &buffers[index])
		require.Len(t, records, 1)
		require.Equal(t, float64(index), records[0]["invocation"])
		fields := records[0]["javascript"].(map[string]any)["fields"].(map[string]any)
		require.Equal(t, float64(index), fields["index"])
	}
}
