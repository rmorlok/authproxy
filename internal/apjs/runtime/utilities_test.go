package runtime_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// utilityLimits enables both base64 helpers with one invocation-wide allowance.
func utilityLimits() jsruntime.Limits {
	limits := runtimeLimits()
	limits.Utilities = jsruntime.UtilityPolicy{
		Base64: true, MaxCalls: 16, MaxInputBytes: 256, MaxOutputBytes: 256,
	}
	return limits
}

// TestUtilityInvalidPolicy rejects enabled helpers without positive bounded allowances.
func TestUtilityInvalidPolicy(t *testing.T) {
	for name, change := range map[string]func(*jsruntime.UtilityPolicy){
		"missing calls":  func(p *jsruntime.UtilityPolicy) { p.MaxCalls = 0 },
		"negative calls": func(p *jsruntime.UtilityPolicy) { p.MaxCalls = -1 },
		"missing input":  func(p *jsruntime.UtilityPolicy) { p.MaxInputBytes = 0 },
		"negative input": func(p *jsruntime.UtilityPolicy) { p.MaxInputBytes = -1 },
		"missing output": func(p *jsruntime.UtilityPolicy) { p.MaxOutputBytes = 0 },
		"negative output": func(p *jsruntime.UtilityPolicy) {
			p.MaxOutputBytes = -1
		},
	} {
		t.Run(name, func(t *testing.T) {
			limits := utilityLimits()
			change(&limits.Utilities)
			// Compilation and low-level execution both reject invalid policy before work.
			_, err := jsruntime.CompileExpression(`btoa("a")`, limits)
			require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
			called := false
			err = jsruntime.Run(context.Background(), limits, func(context.Context, *goja.Runtime) error {
				called = true
				return nil
			})
			require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
			require.False(t, called)
		})
	}
}

// TestUtilityAvailability keeps pure helpers optional across expression and async profiles.
func TestUtilityAvailability(t *testing.T) {
	// Existing callers retain fresh VMs without the optional base64 globals.
	expression, err := jsruntime.CompileExpression(`[typeof atob, typeof btoa]`, runtimeLimits())
	require.NoError(t, err)
	result, err := expression.Evaluate(context.Background(), nil)
	require.NoError(t, err)
	require.JSONEq(t, `["undefined", "undefined"]`, string(result))

	// Enabled helpers are available during validation initialization and execution.
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

	// Pure expressions select the same helpers without an async or Tool adapter.
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
		// Trusted extension setup sees the same selected utility hosts as execute.
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

// TestUtilityReservedBindings protects enabled names without restricting disabled generic programs.
func TestUtilityReservedBindings(t *testing.T) {
	for name, test := range map[string]struct{ source, library string }{
		"source lexical": {`const atob = 1; async function execute() {}`, ""},
		"source var":     {`var btoa; async function execute() {}`, ""},
		"library":        {`async function execute() {}`, `function atob() {}`},
	} {
		t.Run(name, func(t *testing.T) {
			// Authored global declarations cannot replace enabled utility hosts.
			library := asyncLibrary(t, test.library)
			_, err := jsruntime.CompileAsync(test.source, library, nil, utilityLimits())
			require.ErrorIs(t, err, jsruntime.ErrInvalidSource)

			// Disabled utilities leave generic programs free to define their own helpers.
			program, err := jsruntime.CompileAsync(test.source, library, nil, runtimeLimits())
			require.NoError(t, err)
			require.NoError(t, program.Validate(context.Background()))
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

// TestUtilityAggregateLimits makes caught failures unable to restore a utility allowance.
func TestUtilityAggregateLimits(t *testing.T) {
	for name, test := range map[string]struct {
		policy jsruntime.UtilityPolicy
		body   string
	}{
		"calls include malformed attempts": {
			jsruntime.UtilityPolicy{Base64: true, MaxCalls: 1, MaxInputBytes: 256, MaxOutputBytes: 256},
			`try { atob("%"); } catch (_) {} try { btoa("a"); } catch (_) {}`,
		},
		"aggregate input": {
			jsruntime.UtilityPolicy{Base64: true, MaxCalls: 16, MaxInputBytes: 2, MaxOutputBytes: 256},
			`btoa("a"); try { btoa("bb"); } catch (_) {}`,
		},
		"malformed input still consumes allowance": {
			jsruntime.UtilityPolicy{Base64: true, MaxCalls: 16, MaxInputBytes: 2, MaxOutputBytes: 256},
			`try { atob("%%"); } catch (_) {} try { btoa("a"); } catch (_) {}`,
		},
		"aggregate output": {
			jsruntime.UtilityPolicy{Base64: true, MaxCalls: 16, MaxInputBytes: 256, MaxOutputBytes: 7},
			`btoa("a"); try { btoa("b"); } catch (_) {}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			limits := utilityLimits()
			limits.Utilities = test.policy
			expression, err := jsruntime.CompileExpression(`(() => {`+test.body+`; return "hidden failure"; })()`, limits)
			require.NoError(t, err)

			// Catching the helper's exception cannot publish a successful result.
			result, err := expression.Evaluate(context.Background(), nil)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, result)
		})
	}
}

// TestUtilityBudgetSpansInitialization shares one allowance through initialization and execute.
func TestUtilityBudgetSpansInitialization(t *testing.T) {
	limits := utilityLimits()
	limits.Utilities.MaxCalls = 1
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
				return asyncInput(limits)(ctx, vm, invocation)
			})
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, result)
			// Failed initialization cannot enable outbound hosts despite a caught error.
			require.Equal(t, name == "execute", started)
		})
	}
}

// TestUtilityPolicyCaptureAndIsolation gives validation and every concurrent VM their own counters.
func TestUtilityPolicyCaptureAndIsolation(t *testing.T) {
	limits := utilityLimits()
	limits.Utilities = jsruntime.UtilityPolicy{Base64: true, MaxCalls: 2, MaxInputBytes: 6, MaxOutputBytes: 6}
	program, err := jsruntime.CompileAsync(`
		const initialized = btoa("hi");
		async function execute() { return atob(initialized); }
	`, nil, nil, limits)
	require.NoError(t, err)

	// Mutating caller-owned policy cannot remove helpers from the compiled program.
	limits.Utilities = jsruntime.UtilityPolicy{}
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

	// Bare VMs cannot silently initialize with a missing utility policy or hosts.
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
	limits.Utilities.MaxCalls = 1
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
