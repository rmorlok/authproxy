package runtime_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// TestTextCodecPolicy keeps the new hosts opt-in with explicit positive bounds.
func TestTextCodecPolicy(t *testing.T) {
	result, err := evaluateSource(context.Background(), runtimeLimits(), `[typeof TextEncoder, typeof TextDecoder]`)
	require.NoError(t, err)
	require.JSONEq(t, `["undefined","undefined"]`, string(result))

	for _, change := range []func(*jsruntime.UtilityPolicy){
		func(p *jsruntime.UtilityPolicy) { p.MaxCalls = 0 },
		func(p *jsruntime.UtilityPolicy) { p.MaxInputBytes = 0 },
		func(p *jsruntime.UtilityPolicy) { p.MaxOutputBytes = 0 },
	} {
		// Text-only policies must not bypass the same validation used for base64.
		limits := textCodecTestLimits()
		change(&limits.Utilities)
		_, err := jsruntime.CompileExpression(`0`, limits)
		require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	}
	for _, name := range []string{"TextEncoder", "TextDecoder"} {
		// Enabled names are reserved in source and libraries; disabled generic code may own them.
		source := "const " + name + " = 1; async function execute() {}"
		_, err := jsruntime.CompileAsync(source, nil, nil, textCodecTestLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
		_, err = jsruntime.CompileAsync(`async function execute() {}`, asyncLibrary(t, "var "+name+";"), nil, textCodecTestLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
		_, err = jsruntime.CompileAsync(source, nil, nil, runtimeLimits())
		require.NoError(t, err)
	}
}

// TestTextCodecSharedBudgets checks exact accounting across constructor entries,
// string code units, buffer bytes, and previously available base64 helpers.
func TestTextCodecSharedBudgets(t *testing.T) {
	for _, test := range []struct {
		name, source, want   string
		calls, input, output int
		base64               bool
	}{
		{"UTF-8", `const e = new TextEncoder(), d = new TextDecoder(); d.decode(e.encode("é"));`, `"é"`, 4, 3, 3, false},
		{"supplementary units", `const e = new TextEncoder(), d = new TextDecoder(); d.decode(e.encode("😀"));`, `"😀"`, 4, 6, 6, false},
		{"base64 combined", `const e = new TextEncoder(), d = new TextDecoder(); d.decode(e.encode("x")) + atob(btoa("x"));`, `"xx"`, 6, 7, 7, true},
		{"decoder label", `new TextDecoder(" UTF8 ").decode(new Uint8Array([65]));`, `"A"`, 2, 7, 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := textCodecTestLimits()
			limits.Utilities = jsruntime.UtilityPolicy{
				TextCodecs: true, Base64: test.base64, MaxCalls: test.calls,
				MaxInputBytes: test.input, MaxOutputBytes: test.output,
			}
			// One evaluation consumes the exact shared allowances without resetting between helpers.
			result, err := evaluateSource(context.Background(), limits, test.source)
			require.NoError(t, err)
			require.JSONEq(t, test.want, string(result))
			for _, reduce := range []func(*jsruntime.UtilityPolicy){
				func(p *jsruntime.UtilityPolicy) { p.MaxCalls-- },
				func(p *jsruntime.UtilityPolicy) { p.MaxInputBytes-- },
				func(p *jsruntime.UtilityPolicy) { p.MaxOutputBytes-- },
			} {
				tight := limits
				reduce(&tight.Utilities)
				// All enabled caps must remain positive, including the one-unit output example.
				if tight.Utilities.MaxOutputBytes == 0 {
					continue
				}
				result, err = evaluateSource(context.Background(), tight,
					`try {`+test.source+`} catch (_) {} "must not escape";`)
				require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
				require.Nil(t, result)
			}
		})
	}
}

// TestTextCodecMalformedAccounting charges failed calls and admitted inputs,
// while leaving output available after a recoverable decoder or argument error.
func TestTextCodecMalformedAccounting(t *testing.T) {
	for _, test := range []struct {
		name, source string
		calls, input int
	}{
		{"fatal input", `const d = new TextDecoder(undefined, {fatal:true});
			try { d.decode(new Uint8Array([0xe2,0x82])); } catch (_) {}
			d.decode(new Uint8Array([65]));`, 3, 3},
		{"unsupported label", `try { new TextDecoder("bad"); } catch (_) {}
			new TextDecoder().decode(new Uint8Array([65]));`, 3, 4},
		{"malformed option", `try { new TextDecoder("utf-8", {fatal:1}); } catch (_) {}
			new TextDecoder().decode(new Uint8Array([65]));`, 3, 6},
		{"invalid receiver", `try { TextDecoder.prototype.decode.call({}); } catch (_) {}
			new TextDecoder().decode(new Uint8Array([65]));`, 3, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := textCodecTestLimits()
			limits.Utilities.MaxCalls = test.calls
			limits.Utilities.MaxInputBytes = test.input
			limits.Utilities.MaxOutputBytes = 1
			// Recoverable failure reserves no output, so the final valid byte still fits.
			result, err := evaluateSource(context.Background(), limits, test.source)
			require.NoError(t, err)
			require.JSONEq(t, `"A"`, string(result))
			limits.Utilities.MaxCalls--
			result, err = evaluateSource(context.Background(), limits, test.source)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, result)
		})
	}
}

// TestTextCodecInitializationBudget keeps constructor and method allowances
// active across initialization, invocation, and the host-start boundary.
func TestTextCodecInitializationBudget(t *testing.T) {
	for _, initializationFailure := range []bool{false, true} {
		limits := textCodecTestLimits()
		limits.Utilities.MaxCalls = 2
		source := `const e = new TextEncoder(); const bytes = e.encode("a");`
		if initializationFailure {
			source += `try { new TextDecoder(); } catch (_) {}`
		}
		source += `async function execute() { try { e.encode("b"); } catch (_) {} return true; }`
		program, err := jsruntime.CompileAsync(source, nil, nil, limits)
		require.NoError(t, err)
		// Validation gets its own allowance and only runs initialization.
		if initializationFailure {
			require.ErrorIs(t, program.Validate(context.Background()), jsruntime.ErrLimitExceeded)
		} else {
			require.NoError(t, program.Validate(context.Background()))
		}
		started := false
		result, err := program.Evaluate(context.Background(), func(ctx context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
			invocation.Start = func() { started = true }
			return asyncInput(limits)(ctx, vm, invocation)
		})
		// A caught limit cannot produce output or enable hosts after failed initialization.
		require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
		require.Nil(t, result)
		require.Equal(t, !initializationFailure, started)
	}
}

// TestTextCodecConcurrentIsolation gives each evaluation independent branded
// receivers and budgets, using the immutable policy captured by compilation.
func TestTextCodecConcurrentIsolation(t *testing.T) {
	limits := textCodecTestLimits()
	limits.Utilities.MaxCalls = 4
	limits.Utilities.MaxInputBytes, limits.Utilities.MaxOutputBytes = 4, 4
	program, err := jsruntime.CompileAsync(`const e = new TextEncoder(), d = new TextDecoder();
		async function execute() { return d.decode(e.encode("雪")); }`, nil, nil, limits)
	require.NoError(t, err)
	limits.Utilities = jsruntime.UtilityPolicy{}
	require.NoError(t, program.Validate(context.Background()))

	// Concurrent evaluations reuse compiled code, never codec objects or counters.
	errors := make(chan error, 8)
	var workers sync.WaitGroup
	for range cap(errors) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, err := program.Evaluate(context.Background(), asyncInput(limits))
			if err == nil && string(result) != `"雪"` {
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

// TestTextEncoderNativeCancellation stops each conversion pass before the whole
// input is read, using deterministic cancellation rather than timing assertions.
func TestTextEncoderNativeCancellation(t *testing.T) {
	text := strings.Repeat("A", 64<<10)
	for _, cancelAfter := range []int{128, len(text) + 128} {
		limits := textCodecTestLimits()
		limits.Utilities.MaxInputBytes, limits.Utilities.MaxOutputBytes = len(text), len(text)
		ctx, cancel := context.WithCancel(context.Background())
		reads := 0
		result, err := jsruntime.EvaluateJSON(ctx, limits, func(_ context.Context, vm *goja.Runtime) (goja.Value, error) {
			constructor, _ := goja.AssertConstructor(vm.Get("TextEncoder"))
			encoder, err := constructor(nil)
			if err != nil {
				return nil, jsruntime.EvaluationError(err)
			}
			input := &base64CancelString{base64PrimitiveString: vm.ToValue(text).(goja.String), cancel: cancel, cancelAfter: cancelAfter}
			defer func() { reads = input.reads }()
			encode, _ := goja.AssertFunction(encoder.Get("encode"))
			value, err := encode(encoder, input)
			return value, jsruntime.EvaluationError(err)
		})
		cancel()
		// Cancellation during a native pass discards its partially constructed result.
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
		require.GreaterOrEqual(t, reads, cancelAfter)
		fullPass := len(text)
		if cancelAfter > len(text) {
			fullPass *= 2
		}
		require.Less(t, reads, fullPass)
	}
}

// TestTextDecoderDeadline keeps repeated native decoding under the caller and
// execution deadlines, independent of the much larger utility allowances.
func TestTextDecoderDeadline(t *testing.T) {
	limits := textCodecTestLimits()
	limits.Utilities.MaxCalls = 1 << 30
	limits.Utilities.MaxInputBytes, limits.Utilities.MaxOutputBytes = 1<<30, 1<<30
	source := `const decoder = new TextDecoder(), bytes = new Uint8Array(65536);
		for (;;) { decoder.decode(bytes); }`
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
