package runtime

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// TestResultEncoderSharedBytes checks that JSON and typed text consume one
// allowance without retaining or reusing earlier result buffers.
func TestResultEncoderSharedBytes(t *testing.T) {
	limits := outputTestLimits()
	limits.MaxOutputBytes = len(`{"x":1}`) + len(`"a\n"`)
	vm := goja.New()
	encoder := newResultEncoder(context.Background(), vm, limits)
	value, err := vm.RunString(`({x: 1})`)
	require.NoError(t, err)

	// Independent fields fit exactly when text is charged with JSON escaping.
	first, err := encoder.Encode(value)
	require.NoError(t, err)
	text, err := encoder.String(vm.ToValue("a\n"))
	require.NoError(t, err)
	require.Equal(t, "a\n", text)
	require.Equal(t, `{"x":1}`, string(first))
	require.Nil(t, encoder.state.bytes)

	// A later conversion cannot reset the allowance or replace the first result.
	result, err := encoder.Encode(goja.Null())
	require.ErrorIs(t, err, ErrLimitExceeded)
	require.Nil(t, result)
	require.Equal(t, `{"x":1}`, string(first))
	require.ErrorIs(t, encoder.err, ErrLimitExceeded)
	require.Nil(t, encoder.state.bytes)
}

// TestResultEncoderSharedNodes counts object keys and values across fields,
// including typed strings that do not return their intermediate JSON.
func TestResultEncoderSharedNodes(t *testing.T) {
	limits := outputTestLimits()
	limits.MaxNodes = 6
	vm := goja.New()
	encoder := newResultEncoder(context.Background(), vm, limits)
	object, err := vm.RunString(`({x: 1})`)
	require.NoError(t, err)
	array, err := vm.RunString(`[true]`)
	require.NoError(t, err)

	// The object costs three nodes, the array two, and the text one.
	_, err = encoder.Encode(object)
	require.NoError(t, err)
	_, err = encoder.Encode(array)
	require.NoError(t, err)
	_, err = encoder.String(vm.ToValue("last"))
	require.NoError(t, err)

	// Even a tiny later value fails once the shared traversal work is exhausted.
	result, err := encoder.Encode(goja.Null())
	require.ErrorIs(t, err, ErrLimitExceeded)
	require.Nil(t, result)
}

// TestResultEncoderIndependentDepth checks that each selected field is a root,
// while the shared encoder still rejects excessive nesting inside one field.
func TestResultEncoderIndependentDepth(t *testing.T) {
	limits := outputTestLimits()
	limits.MaxDepth = 1
	vm := goja.New()
	encoder := newResultEncoder(context.Background(), vm, limits)
	value, err := vm.RunString(`[1]`)
	require.NoError(t, err)

	// Reusing a value in separate fields does not deepen it or form a cycle.
	for range 2 {
		result, err := encoder.Encode(value)
		require.NoError(t, err)
		require.Equal(t, `[1]`, string(result))
	}

	// Nested contents still use the normal root-zero depth limit.
	value, err = vm.RunString(`[[1]]`)
	require.NoError(t, err)
	result, err := encoder.Encode(value)
	require.ErrorIs(t, err, ErrLimitExceeded)
	require.Nil(t, result)
}

// TestResultEncoderStickyFailure checks that ignored failures cannot be erased
// by switching conversion methods or presenting a later valid value.
func TestResultEncoderStickyFailure(t *testing.T) {
	for _, name := range []string{"invalid JSON", "invalid string", "limit", "cancellation"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			limits := outputTestLimits()
			limits.MaxOutputBytes = 4
			vm := goja.New()
			encoder := newResultEncoder(ctx, vm, limits)

			// Trigger one failure before attempting either valid conversion.
			var first error
			switch name {
			case "invalid JSON":
				_, first = encoder.Encode(goja.Undefined())
			case "invalid string":
				_, first = encoder.String(goja.Null())
			case "limit":
				_, first = encoder.Encode(vm.ToValue("long"))
			case "cancellation":
				cancel()
				_, first = encoder.Encode(goja.Null())
			}
			require.Error(t, first)
			require.Equal(t, first, encoder.err)

			// All subsequent methods retain the first failure and emit no data.
			result, err := encoder.Encode(goja.Null())
			require.Equal(t, first, err)
			require.Nil(t, result)
			text, err := encoder.String(vm.ToValue(""))
			require.Equal(t, first, err)
			require.Empty(t, text)
		})
	}
}

// TestResultEncoderStringUTF16 distinguishes raw JSON's lossless surrogate
// escapes from typed UTF-8 text, which must reject unrepresentable code units.
func TestResultEncoderStringUTF16(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		json   string
		text   string
		valid  bool
	}{
		{"empty", `""`, `""`, "", true},
		{"unicode", `"café 世界 \ud83d\ude00"`, `"café 世界 😀"`, "café 世界 😀", true},
		{"escaping", `"\u0000\n\"\\"`, `"\u0000\n\"\\"`, "\x00\n\"\\", true},
		{"high surrogate", `"\ud800"`, `"\ud800"`, "", false},
		{"low surrogate", `"\udfff"`, `"\udfff"`, "", false},
		{"high before text", `"\ud800x"`, `"\ud800x"`, "", false},
		{"high before pair", `"\ud800\ud83d\ude00"`, `"\ud800😀"`, "", false},
		{"pair before low", `"\ud83d\ude00\udfff"`, `"😀\udfff"`, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			vm := goja.New()
			encoder := newResultEncoder(context.Background(), vm, outputTestLimits())
			value, err := vm.RunString(test.source)
			require.NoError(t, err)

			// Raw JSON preserves every UTF-16 unit without replacement characters.
			result, err := encoder.Encode(value)
			require.NoError(t, err)
			require.Equal(t, test.json, string(result))

			// Typed text is permitted only when the complete string is valid UTF-16.
			text, err := encoder.String(value)
			if test.valid {
				require.NoError(t, err)
				require.Equal(t, test.text, text)
			} else {
				require.ErrorIs(t, err, ErrInvalidOutput)
				require.Empty(t, text)
				require.ErrorIs(t, encoder.err, ErrInvalidOutput)
			}
		})
	}
}

// TestResultEncoderStringDoesNotCoerce rejects objects and non-string
// primitives instead of invoking authored conversion methods.
func TestResultEncoderStringDoesNotCoerce(t *testing.T) {
	for _, source := range []string{
		`({toString() { throw "private"; }, valueOf() { throw "private"; }})`,
		`new String("text")`, `true`, `123`, `null`, `undefined`, `Symbol("private")`,
	} {
		t.Run(source, func(t *testing.T) {
			// Primitive-only conversion must reject these values without coercion.
			vm := goja.New()
			encoder := newResultEncoder(context.Background(), vm, outputTestLimits())
			value, err := vm.RunString(source)
			require.NoError(t, err)
			text, err := encoder.String(value)
			require.ErrorIs(t, err, ErrInvalidOutput)
			require.Empty(t, text)
			require.NotContains(t, err.Error(), "private")
		})
	}
}

// TestResultEncoderCapturesIntrinsics checks that extension results use the
// reflection functions captured before authored code could replace them.
func TestResultEncoderCapturesIntrinsics(t *testing.T) {
	vm := goja.New()
	encoder := newResultEncoder(context.Background(), vm, outputTestLimits())
	value, err := vm.RunString(`
		Reflect.ownKeys = () => { throw "private"; };
		Object.getOwnPropertyDescriptor = () => { throw "private"; };
		Object.prototype.toJSON = () => { throw "private"; };
		({message: "preserved"});
	`)
	require.NoError(t, err)

	// Captured reflection ignores replacements and never calls inherited toJSON.
	result, err := encoder.Encode(value)
	require.NoError(t, err)
	require.Equal(t, `{"message":"preserved"}`, string(result))
}
