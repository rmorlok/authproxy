package javascript

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// outputTestLimits provides generous caps so individual cases can lower one
// dimension without accidentally testing a different limit first.
func outputTestLimits() Limits {
	return Limits{
		MaxSourceBytes: 64 << 10, MaxInputBytes: 64 << 10, MaxOutputBytes: 64 << 10,
		MaxDepth: 32, MaxNodes: 4096, MaxStackDepth: 128, Timeout: time.Second,
	}
}

// outputTestValue captures intrinsics before running an expression, matching
// the production lifecycle while leaving binding and execution to other tests.
func outputTestValue(t *testing.T, expression string, limits Limits) (*outputEncoder, goja.Value) {
	t.Helper()
	vm := goja.New()
	encoder := newOutputEncoder(vm, limits)
	value, err := vm.RunString("(" + expression + ")")
	require.NoError(t, err)
	return encoder, value
}

func TestOutputJSONValues(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       string
	}{
		{"null", `null`, `null`},
		{"boolean", `false`, `false`},
		{"integer", `123`, `123`},
		{"fraction", `-1.25`, `-1.25`},
		{"empty object", `{}`, `{}`},
		{"empty array", `[]`, `[]`},
		{"nested", `({value: [null, false, 0, ""], count: 2})`, `{"value":[null,false,0,""],"count":2}`},
		{"null prototype object", `Object.assign(Object.create(null), {value: 1})`, `{"value":1}`},
		{"null prototype array", `Object.setPrototypeOf([1, 2], null)`, `[1,2]`},
		{"key order", `({b: 1, 3: 2, 1: 3, a: 4})`, `{"1":3,"3":2,"b":1,"a":4}`},
		{"shared child", `(() => {const child = {x: 1}; return [child, child]})()`, `[{"x":1},{"x":1}]`},
		{"hidden data", `Object.defineProperty({visible: 1}, "hidden", {value: undefined})`, `{"visible":1}`},
		{"hidden toJSON", `Object.defineProperty({x: 1}, "toJSON", {value: () => "secret"})`, `{"x":1}`},
		{"array hidden index", `Object.defineProperty([1], "0", {value: 2, enumerable: false})`, `[2]`},
		{"prototype tag", `(() => {Object.prototype[Symbol.toStringTag] = "Date"; return {x: 1}})()`, `{"x":1}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Run author code only after conversion intrinsics are captured.
			encoder, value := outputTestValue(t, test.expression, outputTestLimits())
			actual, err := encoder.encode(context.Background(), value)

			// Primitive values, ordering, and shared acyclic graphs survive intact.
			require.NoError(t, err)
			require.Equal(t, test.want, string(actual))
			require.True(t, json.Valid(actual))
		})
	}
}

func TestOutputUTF16Strings(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       string
	}{
		{"control and quoting", `"\u0000\b\f\n\r\t\u001f\"\\"`, `"\u0000\b\f\n\r\t\u001f\"\\"`},
		{"unicode", `"café 世界 😀"`, `"café 世界 😀"`},
		{"lone high surrogate", `"\ud800"`, `"\ud800"`},
		{"lone low surrogate", `"\udfff"`, `"\udfff"`},
		{"mixed surrogate sequence", `"\ud800x\udfff\ud83d\ude00"`, `"\ud800x\udfff😀"`},
		{"distinct malformed keys", `({"\ud800": 1, "\ud801": 2, "�": 3})`, `{"\ud800":1,"\ud801":2,"�":3}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Keep JavaScript UTF-16 until JSON escaping rather than converting keys
			// through Go strings, which would replace distinct lone surrogates.
			encoder, value := outputTestValue(t, test.expression, outputTestLimits())
			actual, err := encoder.encode(context.Background(), value)

			// The JSON carries all code units, including malformed UTF-16 input.
			require.NoError(t, err)
			require.Equal(t, test.want, string(actual))
			require.True(t, json.Valid(actual))
		})
	}
}

func TestOutputRejectsNonJSONValues(t *testing.T) {
	tests := []struct {
		name       string
		expression string
	}{
		{"undefined", `undefined`},
		{"nested undefined", `({value: undefined})`},
		{"array undefined", `[undefined]`},
		{"function", `function() {}`},
		{"class", `class Value {}`},
		{"symbol", `Symbol("private")`},
		{"bigint", `1n`},
		{"nan", `NaN`},
		{"infinity", `Infinity`},
		{"negative infinity", `-Infinity`},
		{"date", `new Date(0)`},
		{"map", `new Map([["key", "secret"]])`},
		{"set", `new Set([1])`},
		{"promise", `Promise.resolve(1)`},
		{"regexp", `/private/`},
		{"typed array", `new Uint8Array([1, 2])`},
		{"array buffer", `new ArrayBuffer(4)`},
		{"data view", `new DataView(new ArrayBuffer(4))`},
		{"boxed string", `new String("private")`},
		{"boxed number", `new Number(1)`},
		{"boxed boolean", `new Boolean(false)`},
		{"error", `new Error("private")`},
		{"weak map", `new WeakMap()`},
		{"weak set", `new WeakSet()`},
		{"custom object prototype", `Object.create({inherited: 1})`},
		{"custom array prototype", `Object.setPrototypeOf([1], {})`},
		{"disguised map", `Object.setPrototypeOf(new Map(), Object.prototype)`},
		{"object cycle", `(() => {const x = {}; x.self = x; return x})()`},
		{"array cycle", `(() => {const x = []; x.push(x); return x})()`},
		{"symbol key", `({[Symbol("private")]: 1})`},
		{"hidden symbol key", `Object.defineProperty({}, Symbol("private"), {value: 1})`},
		{"sparse array", `[1, , 3]`},
		{"inherited array hole", `(() => {Array.prototype[0] = 1; return new Array(1)})()`},
		{"array extra property", `Object.assign([1], {extra: 2})`},
		{"array hidden extra", `Object.defineProperty([], "private", {value: 1})`},
		{"enumerable toJSON", `({toJSON: () => 1})`},
		{"getter", `({get value() {throw new Error("private")}})`},
		{"setter", `({set value(v) {}})`},
		{"empty accessor", `Object.defineProperty({}, "value", {get: undefined, set: undefined})`},
		{"hidden accessor", `Object.defineProperty({}, "value", {get() {throw "private"}})`},
		{"array accessor", `Object.defineProperty([1], "0", {get() {throw "private"}})`},
		{"proxy", `new Proxy({}, {ownKeys() {throw "private"}})`},
		{"array proxy", `new Proxy([], {})`},
		{"revoked proxy", `(() => {const x = Proxy.revocable({}, {}); x.revoke(); return x.proxy})()`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// These values have semantics that JSON cannot represent faithfully.
			encoder, value := outputTestValue(t, test.expression, outputTestLimits())
			actual, err := encoder.encode(context.Background(), value)

			// Reject rather than drop fields, coerce values, or expose author text.
			require.ErrorIs(t, err, ErrInvalidOutput)
			require.Equal(t, ErrInvalidOutput.Error(), err.Error())
			require.Nil(t, actual)
		})
	}
}

func TestOutputCapturedIntrinsics(t *testing.T) {
	// Poison globals and prototypes after capture. Reflection and descriptor
	// inspection must not consult any of these author-controlled functions.
	expression := `(() => {
		Object.getOwnPropertyDescriptor = () => {throw "private"};
		Reflect.ownKeys = () => {throw "private"};
		JSON.stringify = () => {throw "private"};
		Object.defineProperty(Object.prototype, "get", {get() {throw "private"}});
		Object.defineProperty(Object.prototype, "toJSON", {get() {throw "private"}});
		const result = {values: [1, false, null]};
		Object = null; Reflect = null; Array = null; JSON = null;
		return result;
	})()`
	encoder, value := outputTestValue(t, expression, outputTestLimits())
	actual, err := encoder.encode(context.Background(), value)

	// Own data remains serializable, without invoking inherited getters.
	require.NoError(t, err)
	require.Equal(t, `{"values":[1,false,null]}`, string(actual))
}

func TestOutputDeliberatePrototypeNormalization(t *testing.T) {
	tests := []string{
		`(() => {
			const value = new WeakMap([[{}, {private: "secret"}]]);
			value.visible = 1;
			return Object.setPrototypeOf(value, null);
		})()`,
		`(() => {
			const value = ["private"].values();
			value.visible = 1;
			return Object.setPrototypeOf(value, Object.prototype);
		})()`,
	}
	for _, expression := range tests {
		// Goja exposes these built-in brands as ordinary object metadata. An
		// author can deliberately give them an accepted data-object prototype.
		encoder, value := outputTestValue(t, expression, outputTestLimits())
		actual, err := encoder.encode(context.Background(), value)

		// Only own JSON data is projected; hidden state is never exported.
		require.NoError(t, err)
		require.Equal(t, `{"visible":1}`, string(actual))
	}
}

func TestOutputLimits(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		change     func(*Limits)
		want       string
		wantErr    error
	}{
		{"empty array exact nodes", `[]`, func(l *Limits) { l.MaxNodes = 1 }, `[]`, nil},
		{"array exact nodes", `[1,2]`, func(l *Limits) { l.MaxNodes = 3 }, `[1,2]`, nil},
		{"array nodes exceeded", `[1,2]`, func(l *Limits) { l.MaxNodes = 2 }, ``, ErrLimitExceeded},
		{"object exact nodes", `({a: 1})`, func(l *Limits) { l.MaxNodes = 3 }, `{"a":1}`, nil},
		{"object nodes exceeded", `({a: 1})`, func(l *Limits) { l.MaxNodes = 2 }, ``, ErrLimitExceeded},
		{"hidden property exact nodes", `Object.defineProperty({}, "a", {value: 1})`, func(l *Limits) { l.MaxNodes = 2 }, `{}`, nil},
		{"hidden property nodes exceeded", `Object.defineProperty({}, "a", {value: 1})`, func(l *Limits) { l.MaxNodes = 1 }, ``, ErrLimitExceeded},
		{"depth exact", `({a: [1]})`, func(l *Limits) { l.MaxDepth = 2 }, `{"a":[1]}`, nil},
		{"depth exceeded", `({a: [1]})`, func(l *Limits) { l.MaxDepth = 1 }, ``, ErrLimitExceeded},
		{"shared value counts twice", `(() => {const x = {}; return [x, x]})()`, func(l *Limits) { l.MaxNodes = 2 }, ``, ErrLimitExceeded},
		{"bytes exact", `({a: 1})`, func(l *Limits) { l.MaxOutputBytes = 7 }, `{"a":1}`, nil},
		{"closing brace over budget", `({a: 1})`, func(l *Limits) { l.MaxOutputBytes = 6 }, ``, ErrLimitExceeded},
		{"escaped value exact", `"\u0000"`, func(l *Limits) { l.MaxOutputBytes = 8 }, `"\u0000"`, nil},
		{"escaped value over budget", `"\u0000"`, func(l *Limits) { l.MaxOutputBytes = 7 }, ``, ErrLimitExceeded},
		{"escaped key exact", `({"\u0000": 0})`, func(l *Limits) { l.MaxOutputBytes = 12 }, `{"\u0000":0}`, nil},
		{"escaped key over budget", `({"\u0000": 0})`, func(l *Limits) { l.MaxOutputBytes = 11 }, ``, ErrLimitExceeded},
		{"utf8 exact", `"😀"`, func(l *Limits) { l.MaxOutputBytes = 6 }, `"😀"`, nil},
		{"utf8 over budget", `"😀"`, func(l *Limits) { l.MaxOutputBytes = 5 }, ``, ErrLimitExceeded},
		{"large string precheck", `"x".repeat(100000)`, func(l *Limits) { l.MaxOutputBytes = 32 }, ``, ErrLimitExceeded},
		{"wide object precheck", `Object.fromEntries(Array.from({length: 100}, (_, i) => [i, i]))`, func(l *Limits) { l.MaxNodes = 16 }, ``, ErrLimitExceeded},
		{"sparse huge length", `new Array(4294967295)`, func(l *Limits) { l.MaxNodes = 16 }, ``, ErrLimitExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Change only the boundary under test; compilation is deliberately not
			// involved because the limits belong to output traversal here.
			limits := outputTestLimits()
			test.change(&limits)
			encoder, value := outputTestValue(t, test.expression, limits)
			actual, err := encoder.encode(context.Background(), value)

			// Exact limits succeed; exceeding any cap returns no partial bytes.
			require.ErrorIs(t, err, test.wantErr)
			require.Equal(t, test.want, string(actual))
			if test.wantErr != nil {
				require.Nil(t, actual)
			}
		})
	}
}

func TestOutputArrayLengthBeforeKeys(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		maxNodes   int
		want       string
		wantErr    error
		wantKeys   int
	}{
		{"empty exact", `[]`, 1, `[]`, nil, 1},
		{"dense exact", `[1, 2]`, 3, `[1,2]`, nil, 1},
		{"dense over", `[1, 2]`, 2, ``, ErrLimitExceeded, 0},
		{"wide dense over", `new Array(64).fill(0)`, 4, ``, ErrLimitExceeded, 0},
		{"wide sparse over", `new Array(4294967295)`, 4, ``, ErrLimitExceeded, 0},
		{"nested exact", `[[1, 2]]`, 4, `[[1,2]]`, nil, 2},
		{"nested remaining budget", `[[1, 2]]`, 3, ``, ErrLimitExceeded, 1},
		{"sparse within budget", `[1, , 3]`, 4, ``, ErrInvalidOutput, 1},
		{"extra property within budget", `Object.assign([1], {extra: 2})`, 4, ``, ErrInvalidOutput, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Build small arrays but intercept key enumeration. This proves the
			// ordering without allocating millions of values to provoke the bug.
			limits := outputTestLimits()
			limits.MaxNodes = test.maxNodes
			encoder, value := outputTestValue(t, test.expression, limits)
			ownKeys := encoder.ownKeys
			keyCalls := 0
			encoder.ownKeys = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
				keyCalls++
				if keyCalls > test.wantKeys {
					return nil, errors.New("unexpected key enumeration")
				}
				return ownKeys(this, args...)
			}
			actual, err := encoder.encode(context.Background(), value)

			// Oversized arrays stop before enumeration; exact lengths still pass
			// and affordable arrays still reject holes or additional properties.
			require.Equal(t, test.wantKeys, keyCalls)
			require.ErrorIs(t, err, test.wantErr)
			require.Equal(t, test.want, string(actual))
			if test.wantErr != nil {
				require.Nil(t, actual)
			}
		})
	}
}

func TestOutputCancellationAndOwnership(t *testing.T) {
	// A previously canceled context must stop before inspecting the first value.
	encoder, value := outputTestValue(t, `({value: [1, 2]})`, outputTestLimits())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	actual, err := encoder.encode(ctx, value)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, actual)

	// Each conversion owns its bytes and traversal counters, even if a private
	// encoder is reused sequentially by a caller.
	first, err := encoder.encode(context.Background(), value)
	require.NoError(t, err)
	first[0] = '!'
	second, err := encoder.encode(context.Background(), value)
	require.NoError(t, err)
	require.Equal(t, `{"value":[1,2]}`, string(second))
}

func TestOutputReflectionErrorClassification(t *testing.T) {
	// Ordinary errors lose their private message; uncatchable VM errors retain
	// their identity for the outer evaluator to classify as cancellation/limits.
	require.ErrorIs(t, outputReflectionError(errors.New("private")), ErrInvalidOutput)
	interrupted := &goja.InterruptedError{}
	require.PanicsWithValue(t, interrupted, func() { outputReflectionError(interrupted) })
	stackOverflow := &goja.StackOverflowError{}
	require.PanicsWithValue(t, stackOverflow, func() { outputReflectionError(stackOverflow) })
}

func FuzzOutputJSONStrings(f *testing.F) {
	// Starting from JSON literals exercises escaping without admitting arbitrary
	// programs or allowing fuzz input to create an unbounded JavaScript loop.
	for _, seed := range []string{"", "ordinary", "\x00\n\"\\", "café 世界 😀", strings.Repeat("x", 40)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 4096 {
			t.Skip()
		}
		encoded, err := json.Marshal(text)
		require.NoError(t, err)
		encoder, value := outputTestValue(t, string(encoded), outputTestLimits())
		actual, err := encoder.encode(context.Background(), value)
		require.NoError(t, err)

		// The result must be valid JSON with the same Unicode string semantics.
		var original, roundTrip string
		require.NoError(t, json.Unmarshal(encoded, &original))
		require.NoError(t, json.Unmarshal(actual, &roundTrip))
		require.Equal(t, original, roundTrip)
	})
}
