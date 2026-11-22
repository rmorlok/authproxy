package runtime

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// TestOutputRejectionReasons checks the bounded encoder's fixed classifications
// without letting rejected values, keys, or authored conversion hooks escape.
func TestOutputRejectionReasons(t *testing.T) {
	for _, test := range []struct {
		name, source string
		reason       OutputRejectionReason
	}{
		{"undefined", `undefined`, OutputUnsupportedValue},
		{"symbol value", `Symbol("privateDetail")`, OutputUnsupportedValue},
		{"big integer", `1n`, OutputUnsupportedValue},
		{"nonfinite", `NaN`, OutputUnsupportedValue},
		{"function", `(function privateDetail() {touched++;})`, OutputUnsupportedObject},
		{"date", `new Date(0)`, OutputUnsupportedObject},
		{"proxy", `new Proxy({}, {ownKeys() {touched++; throw "privateDetail";}})`, OutputUnsupportedObject},
		{"revoked proxy", `(() => {const p = Proxy.revocable({}, {}); p.revoke(); return p.proxy;})()`, OutputUnsupportedObject},
		{"custom prototype", `Object.create({privateDetail: true})`, OutputCustomPrototype},
		{"custom array prototype", `Object.setPrototypeOf([], {privateDetail: true})`, OutputCustomPrototype},
		{"getter", `({get privateDetail() {touched++; throw "privateDetail";}})`, OutputAccessor},
		{"hidden setter", `Object.defineProperty({}, "privateDetail", {set(_) {touched++;}})`, OutputAccessor},
		{"undefined accessor", `Object.defineProperty({}, "privateDetail", {get: undefined})`, OutputAccessor},
		{"object cycle", `(() => {const a = {}; a.privateDetail = a; return a;})()`, OutputCycle},
		{"array cycle", `(() => {const a = []; a.push(a); return a;})()`, OutputCycle},
		{"symbol key", `({[Symbol("privateDetail")]: true})`, OutputSymbolKey},
		{"sparse array", `[, 1]`, OutputArrayShape},
		{"extra array property", `Object.assign([], {privateDetail: true})`, OutputArrayShape},
		{"nested private key", `({privateDetail: {get privateNested() {touched++;}}})`, OutputAccessor},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Capture pristine intrinsics before evaluating the authored result.
			vm := goja.New()
			encoder := newOutputEncoder(vm, outputTestLimits())
			value, err := vm.RunString(`var touched = 0; ` + test.source)
			require.NoError(t, err)
			result, err := encoder.encode(context.Background(), value)

			// The public category remains opaque; only a fixed reason reaches trusted code.
			require.Nil(t, result)
			require.ErrorIs(t, err, ErrInvalidOutput)
			require.Equal(t, ErrInvalidOutput.Error(), err.Error())
			reason, ok := OutputReason(fmt.Errorf("trusted wrapper: %w", err))
			require.True(t, ok)
			require.Equal(t, test.reason, reason)
			require.Zero(t, vm.Get("touched").ToInteger())
			require.NotContains(t, fmt.Sprintf("%#v %v", err, reason), "private")
			var exception *goja.Exception
			require.False(t, errors.As(err, &exception))
			require.Same(t, ErrInvalidOutput, errors.Unwrap(err))
		})
	}
}

// TestOutputRejectionClosedVocabulary prevents an adapter from attaching free
// text as metadata, and leaves ordinary categories without an invented reason.
func TestOutputRejectionClosedVocabulary(t *testing.T) {
	for _, reason := range []OutputRejectionReason{
		OutputUnsupportedValue, OutputUnsupportedObject, OutputCustomPrototype,
		OutputAccessor, OutputCycle, OutputSymbolKey, OutputArrayShape,
		OutputInvalidString, OutputReflectionFailure, OutputUnexpectedType, OutputInvalidShape,
	} {
		// Trusted root validators and generic conversion use the same vocabulary.
		err := RejectOutput(reason)
		got, ok := OutputReason(err)
		require.True(t, ok)
		require.Equal(t, reason, got)
		require.ErrorIs(t, err, ErrInvalidOutput)
		require.Equal(t, ErrInvalidOutput.Error(), err.Error())
	}
	for _, reason := range []OutputRejectionReason{"", "privateDetail", "accessor/privateDetail"} {
		// Unsupported labels are discarded rather than retained in any error field.
		err := RejectOutput(reason)
		require.Same(t, ErrInvalidOutput, err)
		got, ok := OutputReason(err)
		require.False(t, ok)
		require.Empty(t, got)
	}
	for _, err := range []error{nil, ErrInvalidOutput, ErrEvaluation, ErrLimitExceeded, context.Canceled, context.DeadlineExceeded} {
		got, ok := OutputReason(err)
		require.False(t, ok)
		require.Empty(t, got)
	}
}

// TestOutputReflectionDiagnostic drops private native errors without changing
// the special handling of uncatchable VM interruption or stack exhaustion.
func TestOutputReflectionDiagnostic(t *testing.T) {
	err := outputReflectionError(errors.New("privateDetail"))
	reason, ok := OutputReason(err)
	require.True(t, ok)
	require.Equal(t, OutputReflectionFailure, reason)
	require.Same(t, ErrInvalidOutput, errors.Unwrap(err))

	// Even an unexpected reflection exception is classified without retaining it.
	vm := goja.New()
	encoder := newOutputEncoder(vm, outputTestLimits())
	encoder.ownKeys = func(goja.Value, ...goja.Value) (goja.Value, error) {
		return nil, vm.Try(func() { panic(vm.NewTypeError("privateDetail")) })
	}
	result, err := encoder.encode(context.Background(), vm.NewObject())
	require.Nil(t, result)
	reason, ok = OutputReason(err)
	require.True(t, ok)
	require.Equal(t, OutputReflectionFailure, reason)
	var exception *goja.Exception
	require.False(t, errors.As(err, &exception))
	require.NotContains(t, fmt.Sprintf("%#v", err), "privateDetail")
}

// TestTypedStringOutputReason keeps the first converter failure when a result
// extension switches methods, and distinguishes UTF-8 text from raw JSON.
func TestTypedStringOutputReason(t *testing.T) {
	for _, test := range []struct {
		source string
		reason OutputRejectionReason
	}{
		{`true`, OutputUnexpectedType},
		{`"\ud800"`, OutputInvalidString},
	} {
		vm := goja.New()
		encoder := newResultEncoder(context.Background(), vm, outputTestLimits())
		value, err := vm.RunString(test.source)
		require.NoError(t, err)
		text, first := encoder.String(value)
		require.Empty(t, text)
		reason, ok := OutputReason(first)
		require.True(t, ok)
		require.Equal(t, test.reason, reason)

		// A valid later result cannot replace the first reason or publish partial data.
		result, err := encoder.Encode(goja.Null())
		require.Nil(t, result)
		require.Same(t, first, err)
	}

	// JSON's existing surrogate escape contract is unchanged by typed text diagnostics.
	vm := goja.New()
	encoder := newOutputEncoder(vm, outputTestLimits())
	value, err := vm.RunString(`"\ud800"`)
	require.NoError(t, err)
	result, err := encoder.encode(context.Background(), value)
	require.NoError(t, err)
	require.Equal(t, `"\ud800"`, string(result))
}
