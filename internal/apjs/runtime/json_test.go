package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// jsonTestLimits leaves room for ordinary input while keeping every cap explicit.
func jsonTestLimits() JSONLimits {
	return JSONLimits{MaxBytes: 64 << 10, MaxDepth: 32, MaxNodes: 1000}
}

// jsonTestDecoder captures pristine parsing in a VM owned by the calling test.
func jsonTestDecoder(t *testing.T) (*goja.Runtime, *JSONDecoder) {
	t.Helper()
	vm := goja.New()
	decoder, err := NewJSONDecoder(vm)
	require.NoError(t, err)
	return vm, decoder
}

// jsonTestRun evaluates assertions without exporting a second Go object graph.
func jsonTestRun(t *testing.T, vm *goja.Runtime, source string) goja.Value {
	t.Helper()
	value, err := vm.RunString(source)
	require.NoError(t, err)
	return value
}

// TestJSONDecoderRootValues preserves every JSON value kind without domain defaults.
func TestJSONDecoderRootValues(t *testing.T) {
	vm, decoder := jsonTestDecoder(t)
	for _, encoded := range []string{`{}`, `[null,false,1.25,"text"]`, `null`, `true`, `false`, `12`, `"text"`} {
		t.Run(encoded, func(t *testing.T) {
			// The generic boundary accepts scalar and array roots as well as objects.
			values, err := decoder.Decode(context.Background(), jsonTestLimits(), json.RawMessage(encoded))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.NoError(t, vm.Set("value", values[0]))
			require.JSONEq(t, encoded, jsonTestRun(t, vm, `JSON.stringify(value)`).String())
		})
	}

	// No documents means no values; an omitted document is invalid JSON, not {}.
	values, err := decoder.Decode(context.Background(), jsonTestLimits())
	require.NoError(t, err)
	require.Empty(t, values)
	values, err = decoder.Decode(context.Background(), jsonTestLimits(), nil)
	require.ErrorIs(t, err, ErrInvalidInput)
	require.Nil(t, values)
}

// TestJSONDecoderNativeOwnership copies input into ordinary JavaScript objects and arrays.
func TestJSONDecoderNativeOwnership(t *testing.T) {
	vm, decoder := jsonTestDecoder(t)
	encoded := json.RawMessage(`{"items":["first",false,null],"__proto__":{"injected":true},"constructor":"literal"}`)
	values, err := decoder.Decode(context.Background(), jsonTestLimits(), encoded)
	require.NoError(t, err)
	require.NoError(t, vm.Set("value", values[0]))

	// Prototype-shaped keys remain own data and do not replace native prototypes.
	require.True(t, jsonTestRun(t, vm, `
		Object.getPrototypeOf(value) === Object.prototype &&
		Object.getPrototypeOf(value.items) === Array.prototype &&
		Array.isArray(value.items) && value.items[1] === false && value.items[2] === null &&
		value.injected === undefined && value.__proto__.injected === true &&
		value.constructor === "literal"
	`).ToBoolean())

	// Mutations stay local; decoding the same bytes again starts a distinct graph.
	jsonTestRun(t, vm, `value.items.push("local"); value.__proto__.injected = false`)
	require.NotContains(t, string(encoded), "local")
	next, err := decoder.Decode(context.Background(), jsonTestLimits(), encoded)
	require.NoError(t, err)
	require.NoError(t, vm.Set("next", next[0]))
	require.True(t, jsonTestRun(t, vm, `next !== value && next.items.length === 3 && next.__proto__.injected === true`).ToBoolean())

	// The caller can reuse its byte buffer once copying completes without changing VM data.
	copy(encoded, strings.Replace(string(encoded), "first", "other", 1))
	require.Equal(t, "first", jsonTestRun(t, vm, `next.items[0]`).String())
}

// TestJSONDecoderRejectsInvalidJSON keeps malformed or unsupported input out of native values.
func TestJSONDecoderRejectsInvalidJSON(t *testing.T) {
	for _, encoded := range []string{
		``, ` `, `undefined`, `NaN`, `1e400`, `-1e400`,
		`{"private-field":"private-value",}`, `{} {}`, `[] true`,
		`{"private-field":undefined}`, `{"private-field":NaN}`,
		`{"private-field":1e400}`, `{"private-field":-1e400}`,
		"{\"private-field\":\"" + string([]byte{0xff}) + "\"}",
		"\"" + string([]byte{0xff}) + "\"",
	} {
		t.Run(encoded, func(t *testing.T) {
			_, decoder := jsonTestDecoder(t)
			// A later invalid document discards the entire batch, without private diagnostics.
			values, err := decoder.Decode(context.Background(), jsonTestLimits(), json.RawMessage(`{"valid":true}`), json.RawMessage(encoded))
			require.ErrorIs(t, err, ErrInvalidInput)
			require.Nil(t, values)
			require.NotContains(t, err.Error(), "private-field")
			require.NotContains(t, err.Error(), "private-value")
		})
	}
}

// TestJSONDecoderNumericSemantics uses JavaScript Numbers while preserving string identifiers.
func TestJSONDecoderNumericSemantics(t *testing.T) {
	vm, decoder := jsonTestDecoder(t)
	values, err := decoder.Decode(context.Background(), jsonTestLimits(), json.RawMessage(`{
		"integer":9007199254740993,"fraction":1.25,"tiny":1e-400,"id":"18446744073709551615"
	}`))
	require.NoError(t, err)
	require.NoError(t, vm.Set("value", values[0]))

	// Finite underflow and unsafe-integer rounding follow native Number semantics.
	require.True(t, jsonTestRun(t, vm, `
		typeof value.integer === "number" && value.integer === 9007199254740992 &&
		value.fraction === 1.25 && value.tiny === 0 && value.id === "18446744073709551615"
	`).ToBoolean())
}

// TestJSONDecoderSurrogateEscapes rejects silent replacement while preserving valid Unicode.
func TestJSONDecoderSurrogateEscapes(t *testing.T) {
	for _, encoded := range []string{
		`"\ud800"`, `"\udfff"`, `{"body":"\ud800"}`, `{"body":"\udfff"}`,
		`{"\ud800":"value"}`, `{"\udc00":"value"}`,
		`{"body":"\ud800\u0041"}`, `{"body":"\ud800\\udc00"}`,
	} {
		t.Run(encoded, func(t *testing.T) {
			// These have valid JSON syntax, but native parsing would replace lone surrogates.
			_, decoder := jsonTestDecoder(t)
			values, err := decoder.Decode(context.Background(), jsonTestLimits(), json.RawMessage(encoded))
			require.ErrorIs(t, err, ErrInvalidInput)
			require.Nil(t, values)
		})
	}

	// Paired units, literal backslash-u sequences, quotes, and ordinary Unicode survive.
	vm, decoder := jsonTestDecoder(t)
	values, err := decoder.Decode(context.Background(), jsonTestLimits(), json.RawMessage(`{"body":"\uD83D\uDE00","\ud83d\ude00":"paired key","literal":"\\ud800","escaped":"quoted\" text","unicode":"é"}`))
	require.NoError(t, err)
	require.NoError(t, vm.Set("value", values[0]))
	require.True(t, jsonTestRun(t, vm, `value.body === "😀" && value["😀"] === "paired key" && value.literal === "\\ud800" && value.escaped === 'quoted" text' && value.unicode === "é"`).ToBoolean())
}

// TestJSONDecoderCombinedLimits charges all documents together, including punctuation and keys.
func TestJSONDecoderCombinedLimits(t *testing.T) {
	_, decoder := jsonTestDecoder(t)
	documents := []json.RawMessage{json.RawMessage(`{"a":"\n"}`), json.RawMessage(` [true,null] `)}
	limits := JSONLimits{MaxBytes: len(documents[0]) + len(documents[1]), MaxDepth: 1, MaxNodes: 6}

	// Exact byte and node caps pass; a new call receives a fresh traversal allowance.
	for range 2 {
		values, err := decoder.Decode(context.Background(), limits, documents...)
		require.NoError(t, err)
		require.Len(t, values, 2)
	}
	for _, change := range []struct {
		name string
		set  func(*JSONLimits)
	}{
		{"bytes", func(l *JSONLimits) { l.MaxBytes-- }},
		{"nodes", func(l *JSONLimits) { l.MaxNodes-- }},
	} {
		t.Run(change.name, func(t *testing.T) {
			// Each document fits alone, but the combined batch exceeds the reduced cap.
			reduced := limits
			change.set(&reduced)
			values, err := decoder.Decode(context.Background(), reduced, documents...)
			require.ErrorIs(t, err, ErrLimitExceeded)
			require.Nil(t, values)
		})
	}

	// Roots start at depth zero independently; an array child adds another level.
	limits = jsonTestLimits()
	limits.MaxDepth = 2
	_, err := decoder.Decode(context.Background(), limits, json.RawMessage(`{"items":[1]}`))
	require.NoError(t, err)
	limits.MaxDepth = 1
	values, err := decoder.Decode(context.Background(), limits, json.RawMessage(`{"items":[1]}`))
	require.ErrorIs(t, err, ErrLimitExceeded)
	require.Nil(t, values)
}

// TestJSONDecoderPristineParser keeps authored replacement of globals out of later decoding.
func TestJSONDecoderPristineParser(t *testing.T) {
	vm, decoder := jsonTestDecoder(t)
	jsonTestRun(t, vm, `JSON.parse = () => ({forged: true}); JSON = null;`)

	// Response decoding can reuse the captured intrinsic after arbitrary authored code.
	values, err := decoder.Decode(context.Background(), jsonTestLimits(), json.RawMessage(`{"actual":7}`))
	require.NoError(t, err)
	require.NoError(t, vm.Set("value", values[0]))
	require.True(t, jsonTestRun(t, vm, `value.actual === 7 && value.forged === undefined`).ToBoolean())
}

// TestJSONDecoderInvalidOptions rejects missing parser state and nonpositive policy.
func TestJSONDecoderInvalidOptions(t *testing.T) {
	decoder, err := NewJSONDecoder(nil)
	require.ErrorIs(t, err, ErrInvalidInput)
	require.Nil(t, decoder)
	for _, source := range []string{`JSON = null`, `delete globalThis.JSON`, `JSON.parse = 7`} {
		vm := goja.New()
		jsonTestRun(t, vm, source)
		decoder, err := NewJSONDecoder(vm)
		require.ErrorIs(t, err, ErrInvalidInput)
		require.Nil(t, decoder)
	}

	// Empty state and nil contexts cannot start conversion or return partial values.
	_, decoder = jsonTestDecoder(t)
	for _, invalid := range []*JSONDecoder{nil, {}} {
		values, err := invalid.Decode(context.Background(), jsonTestLimits(), json.RawMessage(`{}`))
		require.ErrorIs(t, err, ErrInvalidInput)
		require.Nil(t, values)
	}
	values, err := decoder.Decode(nil, jsonTestLimits(), json.RawMessage(`{}`))
	require.ErrorIs(t, err, ErrInvalidInput)
	require.Nil(t, values)

	for _, change := range []func(*JSONLimits){
		func(l *JSONLimits) { l.MaxBytes = 0 },
		func(l *JSONLimits) { l.MaxBytes = -1 },
		func(l *JSONLimits) { l.MaxDepth = 0 },
		func(l *JSONLimits) { l.MaxNodes = 0 },
	} {
		limits := jsonTestLimits()
		change(&limits)
		values, err := decoder.Decode(context.Background(), limits, json.RawMessage(`{}`))
		require.ErrorIs(t, err, ErrInvalidInput)
		require.Nil(t, values)
	}
}

// jsonCancelContext cancels its real parent after several cooperative checks.
// It lets validation begin before cancellation without a timing-sensitive goroutine.
type jsonCancelContext struct {
	context.Context
	cancel    context.CancelFunc // Cancels Context and closes its Done channel.
	remaining int                // Successful Err checks before cancellation.
}

// Err advances the deterministic cancellation point and preserves context semantics.
func (c *jsonCancelContext) Err() error {
	if c.remaining == 0 {
		c.cancel()
	} else {
		c.remaining--
	}
	return c.Context.Err()
}

// TestJSONDecoderCancellation stops active conversion without returning a partial batch.
func TestJSONDecoderCancellation(t *testing.T) {
	_, decoder := jsonTestDecoder(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	values, err := decoder.Decode(ctx, jsonTestLimits(), json.RawMessage(`{}`))
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, values)

	// Cancel while scanning a later long string, after validation has started.
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	controlled := &jsonCancelContext{Context: ctx, cancel: cancel, remaining: 40}
	values, err = decoder.Decode(controlled, jsonTestLimits(), json.RawMessage(`{"first":true}`), json.RawMessage(`"`+strings.Repeat("x", 1000)+`"`))
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Nil(t, values)
}

// FuzzInputJSON exercises malformed syntax, nesting, and surrogate escapes
// without allocating a VM. Every accepted document must remain valid JSON.
func FuzzInputJSON(f *testing.F) {
	for _, seed := range []string{
		`{}`, `null`, `[true,1.25]`, `"text"`, `"\ud800"`,
		`{"body":"\ud800"}`, `{"body":"\ud83d\ude00"}`,
		`{"body":"\\ud800"}`, `{"body":[null,false,1.25,{"key":"text"}]}`,
		`{"body":"escaped\"quote"}`, `{"body":1e400}`, `{"body":`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, encoded []byte) {
		if len(encoded) > 4096 {
			return
		}
		budget := inputBudget{ctx: context.Background(), limits: jsonTestLimits()}
		if err := budget.validate(encoded); err != nil {
			require.True(t, err == ErrInvalidInput || err == ErrLimitExceeded)
			return
		}

		// A successful boundary never admits trailing values or malformed JSON.
		require.True(t, json.Valid(encoded))
	})
}
