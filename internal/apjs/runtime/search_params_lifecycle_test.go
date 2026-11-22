package runtime_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// TestSearchParamsAvailability exposes query utilities in every managed execution
// without granting effectful capabilities.
func TestSearchParamsAvailability(t *testing.T) {
	for _, source := range []string{
		`const URLSearchParams = 1; async function execute() {}`,
		`var URLSearchParams; async function execute() {}`,
	} {
		_, err := jsruntime.CompileAsync(source, nil, nil, runtimeLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
	}
	_, err := jsruntime.CompileAsync(`async function execute() {}`, asyncLibrary(t, `function URLSearchParams() {}`), nil, runtimeLimits())
	require.ErrorIs(t, err, jsruntime.ErrInvalidSource)

	// Standard pure helpers are present; HTTP and logging remain absent.
	result, err := evaluateSource(context.Background(), runtimeLimits(),
		`[typeof URLSearchParams, typeof URL, typeof TextEncoder, typeof atob, typeof fetch, typeof console]`)
	require.NoError(t, err)
	require.JSONEq(t, `["function","function","function","function","undefined","undefined"]`, string(result))
}

// TestSearchParamsExactBudgets exercises cumulative allocation/work accounting,
// including empty tuples and other helpers sharing the same allowances.
func TestSearchParamsExactBudgets(t *testing.T) {
	for _, test := range []struct {
		name, source, want   string
		calls, input, output int
	}{
		{"parse and serialize", `new URLSearchParams("a=b").toString()`, `"a=b"`, 2, 6, 9},
		{"Unicode mutation and get", `const p = new URLSearchParams(); p.append("é","😀"); p.get("é")`, `"😀"`, 3, 8, 15},
		{"empty returned slot", `new URLSearchParams("=").getAll("")`, `[""]`, 2, 2, 3},
		{"delete does not refund", `const p = new URLSearchParams(); p.append("",""); p.delete(""); p.append("",""); p.size`, `1`, 5, 1, 2},
		{"stable sort work and scratch", `const p = new URLSearchParams("b=1&a=2"); p.sort(); p.toString()`, `"a=2&b=1"`, 3, 23, 22},
		{"base64 shares counters", `btoa(new URLSearchParams("a=b").toString())`, `"YT1i"`, 3, 9, 13},
		{"text codecs share counters", `new TextEncoder().encode(new URLSearchParams("a=b").toString()).length`, `3`, 4, 9, 12},
		{"bad arguments keep call charge", `const p = new URLSearchParams(); try {p.get(9);} catch (_) {} p.append("a","b"); p.toString()`, `"a=b"`, 4, 5, 8},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := searchParamsLimits()
			limits.MaxWorkUnits = test.calls + test.input
			limits.MaxAllocationUnits = test.output + 1 // Retained query instance metadata.
			if test.name == "text codecs share counters" {
				limits.MaxAllocationUnits++
			}
			// Exact independent totals include native state traversal and all temporary storage.
			result, err := runUtilitySource(context.Background(), limits, test.source)
			require.NoError(t, err)
			require.JSONEq(t, test.want, string(result))
			for _, field := range []string{"work", "allocation"} {
				tight := limits
				switch field {
				case "work":
					tight.MaxWorkUnits--
				case "allocation":
					tight.MaxAllocationUnits--
				}
				// Catching a failed reservation cannot publish a successful result.
				result, err = runUtilitySource(context.Background(), tight,
					`try {`+test.source+`;} catch (_) {} "privateDetail";`)
				require.ErrorIs(t, err, jsruntime.ErrLimitExceeded, field)
				require.Nil(t, result)
				require.NotContains(t, err.Error(), "privateDetail")
			}
		})
	}
}

// TestSearchParamsInitializationBudget keeps the same counters and mutable
// query state across initialization and execution, but validation is isolated.
func TestSearchParamsInitializationBudget(t *testing.T) {
	for _, failInitialization := range []bool{false, true} {
		limits := searchParamsLimits()
		limits.MaxWorkUnits = 7
		source := `const query = new URLSearchParams("x=1"); query.append("x","2");`
		if failInitialization {
			source += `try {query.toString();} catch (_) {}`
		}
		source += `async function execute() {try {query.getAll("x");} catch (_) {} return true;}`
		program, err := jsruntime.CompileAsync(source, nil, nil, limits)
		require.NoError(t, err)
		if failInitialization {
			require.ErrorIs(t, program.Validate(context.Background()), jsruntime.ErrLimitExceeded)
		} else {
			require.NoError(t, program.Validate(context.Background()))
		}
		started := false
		result, err := program.Evaluate(context.Background(), func(ctx context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
			invocation.Start = func() { started = true }
			invocation.Params, invocation.Context = goja.Undefined(), goja.Undefined()
			return nil
		})
		// A failed initializer cannot open the host I/O gate.
		require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
		require.Nil(t, result)
		require.Equal(t, !failInitialization, started)
	}
}

// TestSearchParamsConcurrentIsolation shares compiled source, never mutable
// query tuples or execution counters, across concurrent evaluations.
func TestSearchParamsConcurrentIsolation(t *testing.T) {
	limits := searchParamsLimits()
	limits.MaxWorkUnits, limits.MaxAllocationUnits = 256, 256
	program, err := jsruntime.CompileAsync(`const query = new URLSearchParams("x=first");
		async function execute() {query.append("x", "second"); return query.getAll("x");}`, nil, nil, limits)
	require.NoError(t, err)
	limits.MaxWorkUnits, limits.MaxAllocationUnits = 1, 1
	require.NoError(t, program.Validate(context.Background()))
	errors := make(chan error, 8)
	var workers sync.WaitGroup
	for range cap(errors) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, err := program.Evaluate(context.Background(), asyncInput(limits))
			if err == nil && string(result) != `["first","second"]` {
				err = fmt.Errorf("unexpected query result: %s", result)
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

// TestSearchParamsDeadline distinguishes caller cancellation from the runtime
// timeout while larger execution allowances permit repeated native query work.
func TestSearchParamsDeadline(t *testing.T) {
	limits := searchParamsLimits()
	limits.MaxWorkUnits, limits.MaxAllocationUnits = 1<<30, 1<<30
	source := `const query = new URLSearchParams("x=" + "a".repeat(65536)); for (;;) {query.toString();}`
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	result, err := evaluateSource(ctx, limits, source)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, result)
	limits.Timeout = 25 * time.Millisecond
	result, err = evaluateSource(context.Background(), limits, source)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.Nil(t, result)
}
