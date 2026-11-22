package runtime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// utilityLimits gives all pure helpers a generous shared execution allowance.
func utilityLimits() jsruntime.Limits { return runtimeLimits() }

// runUtilitySource isolates native utility accounting from JSON result conversion.
// Its authored fixtures return ordinary data; test-only export happens on the VM
// owner and returns detached JSON, never a live runtime value.
func runUtilitySource(ctx context.Context, limits jsruntime.Limits, source string) ([]byte, error) {
	var result []byte
	err := jsruntime.Run(ctx, limits, func(_ context.Context, vm *goja.Runtime) error {
		value, err := vm.RunString(source)
		if err != nil {
			return jsruntime.EvaluationError(err)
		}
		result, err = json.Marshal(value.Export())
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// TestUtilityInvalidLimits rejects missing cumulative execution allowances.
func TestUtilityInvalidLimits(t *testing.T) {
	for name, change := range map[string]func(*jsruntime.Limits){
		"missing work":        func(p *jsruntime.Limits) { p.MaxWorkUnits = 0 },
		"negative work":       func(p *jsruntime.Limits) { p.MaxWorkUnits = -1 },
		"missing allocation":  func(p *jsruntime.Limits) { p.MaxAllocationUnits = 0 },
		"negative allocation": func(p *jsruntime.Limits) { p.MaxAllocationUnits = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			limits := utilityLimits()
			change(&limits)
			// Invalid execution policy fails before installing or evaluating helpers.
			_, err := jsruntime.CompileExpression(`btoa("a")`, limits)
			require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
			called := false
			err = jsruntime.Run(context.Background(), limits, func(context.Context, *goja.Runtime) error { called = true; return nil })
			require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
			require.False(t, called)
		})
	}
}

// TestUtilityAvailability installs pure helpers in every managed expression and async profile.
func TestUtilityAvailability(t *testing.T) {
	// Ordinary execution profiles install the complete pure-helper surface.
	expression, err := jsruntime.CompileExpression(`[typeof atob, typeof btoa, typeof TextEncoder, typeof TextDecoder, typeof URLSearchParams, typeof URL]`, runtimeLimits())
	require.NoError(t, err)
	result, err := expression.Evaluate(context.Background(), nil)
	require.NoError(t, err)
	require.JSONEq(t, `["function", "function", "function", "function", "function", "function"]`, string(result))

	// Helpers are available during validation, initialization, and execution.
	limits := utilityLimits()
	program, err := jsruntime.CompileAsync(`
		const initialized = btoa("hi");
		async function execute() { return atob(initialized); }
	`, nil, nil, limits)
	require.NoError(t, err)
	require.NoError(t, program.Validate(context.Background()))
	result, err = program.Evaluate(context.Background(), asyncInput(limits))
	require.NoError(t, err)
	require.JSONEq(t, `"hi"`, string(result))

	// Pure expressions receive the same helpers without an async or Tool adapter.
	expression, err = jsruntime.CompileExpression(`atob(btoa("hello"))`, limits)
	require.NoError(t, err)
	result, err = expression.Evaluate(context.Background(), nil)
	require.NoError(t, err)
	require.JSONEq(t, `"hello"`, string(result))
}

// TestUtilityExtendedAsync installs base64 before domain helpers and authored initialization.
func TestUtilityExtendedAsync(t *testing.T) {
	limits := utilityLimits()
	extension := brandedResultExtension()
	install := extension.Install
	extension.Install = func(ctx context.Context, vm *goja.Runtime) (jsruntime.ResultInterpreter[extensionOutcome], error) {
		// Trusted extension setup sees the same pure utility hosts as execute.
		value, err := vm.RunString(`atob(btoa("ready"))`)
		if err != nil {
			return jsruntime.ResultInterpreter[extensionOutcome]{}, jsruntime.EvaluationError(err)
		}
		require.Equal(t, "ready", value.String())
		return install(ctx, vm)
	}
	program, err := jsruntime.CompileExtendedAsync(`
		const initialized = domainResult(atob(btoa("ok")));
		async function execute() { return initialized; }
	`, nil, nil, limits, extension)
	require.NoError(t, err)
	require.NoError(t, program.Validate(context.Background()))
	result, err := program.Evaluate(context.Background(), asyncInput(limits))
	require.NoError(t, err)
	require.Equal(t, "success", result.Kind)
	require.Len(t, result.Fields, 1)
	require.JSONEq(t, `"ok"`, string(result.Fields[0]))
}

// TestUtilityReservedBindings protects all installed names in every execution profile.
func TestUtilityReservedBindings(t *testing.T) {
	for name, test := range map[string]struct{ source, library string }{
		"source lexical": {`const atob = 1; async function execute() {}`, ""},
		"source var":     {`var btoa; async function execute() {}`, ""},
		"library":        {`async function execute() {}`, `function atob() {}`},
	} {
		t.Run(name, func(t *testing.T) {
			// Authored global declarations cannot replace the standard utility hosts.
			library := asyncLibrary(t, test.library)
			_, err := jsruntime.CompileAsync(test.source, library, nil, utilityLimits())
			require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
		})
	}

	// Existing adapters may repeat the shared reserved names with matching policy.
	bindings := []jsruntime.HostBinding{{Name: "atob"}, {Name: "btoa"}}
	program, err := jsruntime.CompileAsync(`async function execute() {}`, nil, bindings, utilityLimits())
	require.NoError(t, err)
	require.NoError(t, program.Validate(context.Background()))
	for _, name := range []string{"atob", "btoa"} {
		_, err := jsruntime.CompileAsync(`async function execute() {}`, nil,
			[]jsruntime.HostBinding{{Name: name, PreserveEnumerable: true}}, utilityLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	}

	// Expressions cannot reassign or reconfigure installed globals either.
	expression, err := jsruntime.CompileExpression(`(() => {
		let rejected = 0;
		try { atob = () => "changed"; } catch (_) { rejected++; }
		try { Object.defineProperty(globalThis, "btoa", {value: null}); } catch (_) { rejected++; }
		return [rejected, ...["atob", "btoa"].map(name => {
			const d = Object.getOwnPropertyDescriptor(globalThis, name);
			return [d.writable, d.configurable, d.enumerable];
		}), atob(btoa("ok"))];
	})()`, utilityLimits())
	require.NoError(t, err)
	result, err := expression.Evaluate(context.Background(), nil)
	require.NoError(t, err)
	require.JSONEq(t, `[2,[false,false,false],[false,false,false],"ok"]`, string(result))
}

// TestUtilityAggregateLimits makes caught failures unable to replenish shared
// execution work or allocation allowances, including malformed native attempts.
func TestUtilityAggregateLimits(t *testing.T) {
	for name, test := range map[string]struct {
		work, allocation int
		body             string
	}{
		"entry work includes malformed attempts": {1, 256, `try {atob(null);} catch (_) {} try {btoa("");} catch (_) {}`},
		"aggregate scanning":                     {4, 256, `btoa("a"); try {btoa("bb");} catch (_) {}`},
		"malformed input keeps its work":         {4, 256, `try {atob("%%");} catch (_) {} try {btoa("a");} catch (_) {}`},
		"aggregate allocation":                   {256, 7, `btoa("a"); try {btoa("b");} catch (_) {}`},
	} {
		t.Run(name, func(t *testing.T) {
			limits := utilityLimits()
			limits.MaxWorkUnits, limits.MaxAllocationUnits = test.work, test.allocation
			// Catching a helper failure cannot make the enclosing execution successful.
			result, err := runUtilitySource(context.Background(), limits, `(() => {`+test.body+`; return "hidden failure";})()`)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, result)
		})
	}
}

// TestUtilityBudgetSpansInitialization shares one allowance through initialization and execute.
func TestUtilityBudgetSpansInitialization(t *testing.T) {
	limits := utilityLimits()
	limits.MaxWorkUnits = 3
	for name, source := range map[string]string{
		"initialization": `const initialized = btoa("hi");
			try { atob(initialized); } catch (_) {} async function execute() { return true; }`,
		"execute": `const initialized = btoa("hi");
			async function execute() { try { atob(initialized); } catch (_) {} return true; }`,
	} {
		t.Run(name, func(t *testing.T) {
			program, err := jsruntime.CompileAsync(source, nil, nil, limits)
			require.NoError(t, err)
			// Validation checks initialization only, preserving its sticky failures.
			if name == "initialization" {
				require.ErrorIs(t, program.Validate(context.Background()), jsruntime.ErrLimitExceeded)
			} else {
				require.NoError(t, program.Validate(context.Background()))
			}
			// Invocation never resets the counter when initialization hands off to execute.
			started := false
			result, err := program.Evaluate(context.Background(), func(ctx context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
				invocation.Start = func() { started = true }
				invocation.Params, invocation.Context = goja.Undefined(), goja.Undefined()
				return nil
			})
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, result)
			// Failed initialization cannot enable outbound hosts despite a caught error.
			require.Equal(t, name == "execute", started)
		})
	}
}

// TestUtilityLimitsCaptureAndIsolation gives validation and every concurrent VM their own counters.
func TestUtilityLimitsCaptureAndIsolation(t *testing.T) {
	limits := utilityLimits()
	limits.MaxWorkUnits, limits.MaxAllocationUnits = 256, 256
	program, err := jsruntime.CompileAsync(`
		const initialized = btoa("hi");
		async function execute() { return atob(initialized); }
	`, nil, nil, limits)
	require.NoError(t, err)

	// Mutating caller-owned limits cannot replace the compiled execution allowance.
	limits.MaxWorkUnits, limits.MaxAllocationUnits = 1, 1
	require.NoError(t, program.Validate(context.Background()))
	errors := make(chan error, 8)
	var workers sync.WaitGroup
	for range cap(errors) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, err := program.Evaluate(context.Background(), asyncInput(limits))
			if err == nil && string(result) != `"hi"` {
				err = fmt.Errorf("unexpected detached result: %s", result)
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

// TestUtilityLowLevelInitialize requires the caller to supply the managed utility lifecycle.
func TestUtilityLowLevelInitialize(t *testing.T) {
	limits := utilityLimits()
	program, err := jsruntime.CompileAsync(`const initialized = btoa("hi"); async function execute() {}`, nil, nil, limits)
	require.NoError(t, err)

	// Bare VMs cannot silently initialize without the managed utility hosts.
	_, err = program.Initialize(goja.New())
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)

	// Run owns installation, budgets, and cancellation for this lower-level API.
	err = jsruntime.Run(context.Background(), limits, func(_ context.Context, vm *goja.Runtime) error {
		entry, err := program.Initialize(vm)
		if err == nil {
			require.NotNil(t, entry)
		}
		return err
	})
	require.NoError(t, err)
}

// TestUtilityCancellationPreservesCallerError applies cancellation after caught budget exhaustion.
func TestUtilityCancellationPreservesCallerError(t *testing.T) {
	limits := utilityLimits()
	limits.MaxWorkUnits = 2
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Caller cancellation still wins over a sticky utility limit during cleanup.
	err := jsruntime.Run(ctx, limits, func(_ context.Context, vm *goja.Runtime) error {
		_, err := vm.RunString(`btoa("a"); try { btoa("b"); } catch (_) {}`)
		cancel()
		return jsruntime.EvaluationError(err)
	})
	require.ErrorIs(t, err, context.Canceled)
}
