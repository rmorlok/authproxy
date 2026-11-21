package runtime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// textCodecTestLimits selects UTF-8 helpers with room for ordinary text fixtures.
func textCodecTestLimits() jsruntime.Limits {
	limits := runtimeLimits()
	limits.Utilities = jsruntime.UtilityPolicy{
		TextCodecs: true, MaxCalls: 64, MaxInputBytes: 4096, MaxOutputBytes: 4096,
	}
	return limits
}

// TestTextEncoderUTF8 replaces unpaired UTF-16 surrogates while preserving valid
// scalar values and emits a real native Uint8Array rather than a reflected slice.
func TestTextEncoderUTF8(t *testing.T) {
	for _, test := range []struct {
		argument string
		bytes    []int
	}{
		{"", []int{}},
		{"undefined", []int{}},
		{`""`, []int{}},
		{`"A\u0000z"`, []int{65, 0, 122}},
		{`"\u007f\u0080\u07ff\u0800"`, []int{127, 194, 128, 223, 191, 224, 160, 128}},
		{`"\ud7ff\ue000\ufffe\uffff"`, []int{237, 159, 191, 238, 128, 128, 239, 191, 190, 239, 191, 191}},
		{`"é雪"`, []int{195, 169, 233, 155, 170}},
		{`"\ud83d\udca9"`, []int{240, 159, 146, 169}},
		{`"\ud800\udc00\udbff\udfff"`, []int{240, 144, 128, 128, 244, 143, 191, 191}},
		{`"\ud800"`, []int{239, 191, 189}},
		{`"\udfff"`, []int{239, 191, 189}},
		{`"\ud800A\udfff"`, []int{239, 191, 189, 65, 239, 191, 189}},
		{`"\ud800\ud800\udc00"`, []int{239, 191, 189, 240, 144, 128, 128}},
	} {
		t.Run(test.argument, func(t *testing.T) {
			// The expected bytes are independent fixtures, including Unicode boundary cases.
			source := fmt.Sprintf(`(() => {
				const encoder = new TextEncoder();
				const bytes = encoder.encode(%s);
				return [encoder.encoding, bytes instanceof Uint8Array,
					ArrayBuffer.isView(bytes), Array.from(bytes), typeof encoder.encodeInto];
			})()`, test.argument)
			want, err := json.Marshal([]any{"utf-8", true, true, test.bytes, "undefined"})
			require.NoError(t, err)
			got, err := evaluateSource(context.Background(), textCodecTestLimits(), source)
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
	}
}

// TestTextDecoderUTF8 checks maximal-subpart replacement rather than replacing
// every invalid byte independently, and applies fatal mode to the same fixtures.
func TestTextDecoderUTF8(t *testing.T) {
	for _, test := range []struct {
		name    string
		bytes   []int
		units   []int
		invalid bool
	}{
		{"empty", []int{}, []int{}, false},
		{"ASCII and NUL", []int{65, 0, 122}, []int{65, 0, 122}, false},
		{"multibyte", []int{195, 169, 233, 155, 170}, []int{0xe9, 0x96ea}, false},
		{"three byte boundaries", []int{0xe0, 0xa0, 0x80, 0xed, 0x9f, 0xbf, 0xee, 0x80, 0x80, 0xef, 0xbf, 0xbe, 0xef, 0xbf, 0xbf}, []int{0x800, 0xd7ff, 0xe000, 0xfffe, 0xffff}, false},
		{"supplementary", []int{0xf0, 0x9f, 0x92, 0xa9}, []int{0xd83d, 0xdca9}, false},
		{"smallest supplementary", []int{0xf0, 0x90, 0x80, 0x80}, []int{0xd800, 0xdc00}, false},
		{"largest scalar", []int{0xf4, 0x8f, 0xbf, 0xbf}, []int{0xdbff, 0xdfff}, false},
		{"truncated three byte prefix", []int{0xe2, 0x82}, []int{0xfffd}, true},
		{"truncated four byte prefix", []int{0xf0, 0x9f, 0x92}, []int{0xfffd}, true},
		{"valid prefix then ASCII", []int{0xe2, 0x82, 0x41}, []int{0xfffd, 0x41}, true},
		{"invalid continuation", []int{0xe2, 0x28, 0xa1}, []int{0xfffd, 0x28, 0xfffd}, true},
		{"overlong", []int{0xc0, 0x80}, []int{0xfffd, 0xfffd}, true},
		{"overlong three byte", []int{0xe0, 0x80, 0x80}, []int{0xfffd, 0xfffd, 0xfffd}, true},
		{"overlong four byte", []int{0xf0, 0x80, 0x80, 0x80}, []int{0xfffd, 0xfffd, 0xfffd, 0xfffd}, true},
		{"invalid leader", []int{0xf5, 0x80}, []int{0xfffd, 0xfffd}, true},
		{"surrogate", []int{0xed, 0xa0, 0x80}, []int{0xfffd, 0xfffd, 0xfffd}, true},
		{"out of range", []int{0xf4, 0x90, 0x80, 0x80}, []int{0xfffd, 0xfffd, 0xfffd, 0xfffd}, true},
		{"reprocessed lead", []int{0xe2, 0x80, 0xc2, 0x41}, []int{0xfffd, 0xfffd, 0x41}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			bytes, err := json.Marshal(test.bytes)
			require.NoError(t, err)
			units, err := json.Marshal(test.units)
			require.NoError(t, err)

			// Inspect UTF-16 units so a supplementary scalar must produce its surrogate pair.
			source := fmt.Sprintf(`(() => {
				const text = new TextDecoder().decode(new Uint8Array(%s));
				return Array.from({length: text.length}, (_, i) => text.charCodeAt(i));
			})()`, bytes)
			got, err := evaluateSource(context.Background(), textCodecTestLimits(), source)
			require.NoError(t, err)
			require.JSONEq(t, string(units), string(got))

			// Fatal decoding rejects malformed data with a catchable fixed TypeError.
			source = fmt.Sprintf(`(() => {try {
				new TextDecoder("utf-8", {fatal: true}).decode(new Uint8Array(%s));
				return false;
			} catch (error) {return error instanceof TypeError;}})()`, bytes)
			got, err = evaluateSource(context.Background(), textCodecTestLimits(), source)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprint(test.invalid), string(got))
		})
	}
}

// TestTextDecoderBOMAndIndependentCalls strips only the initial BOM unless the
// caller opts to preserve it, and starts fresh decoder state on every call.
func TestTextDecoderBOMAndIndependentCalls(t *testing.T) {
	source := `(() => {
		const decoder = new TextDecoder();
		const preserving = new TextDecoder("utf-8", {fatal: true, ignoreBOM: true});
		const bom = [239,187,191];
		return [decoder.encoding, decoder.fatal, decoder.ignoreBOM,
			preserving.fatal, preserving.ignoreBOM,
			decoder.decode(), decoder.decode(undefined),
			decoder.decode(new Uint8Array([...bom, 65, ...bom])),
			preserving.decode(new Uint8Array([...bom, 65])),
			decoder.decode(new Uint8Array([...bom, ...bom])),
			decoder.decode(new Uint8Array([0, ...bom])),
			decoder.decode(new Uint8Array([226,130])),
			decoder.decode(new Uint8Array([172])),
			decoder.decode(new Uint8Array([...bom, 65]), {stream: false})];
	})()`

	// Separate calls neither retain an incomplete sequence nor remember a previous BOM.
	got, err := evaluateSource(context.Background(), textCodecTestLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `["utf-8",false,false,true,true,"","","A\ufeff","\ufeffA","\ufeff","\u0000\ufeff","\ufffd","\ufffd","A"]`, string(got))
}

// TestTextDecoderLabels accepts UTF-8 aliases with ASCII trimming and case
// folding, while rejecting unsupported encodings without exposing their names.
func TestTextDecoderLabels(t *testing.T) {
	for _, label := range []string{"utf-8", "utf8", "unicode-1-1-utf-8", "unicode11utf8", "unicode20utf8", "x-unicode20utf8", " \t\n\r\fUtF-8 \t"} {
		argument, err := json.Marshal(label)
		require.NoError(t, err)
		got, err := evaluateSource(context.Background(), textCodecTestLimits(), "new TextDecoder("+string(argument)+").encoding")
		require.NoError(t, err, label)
		require.JSONEq(t, `"utf-8"`, string(got))
	}
	for _, label := range []string{"", "ascii", "utf-16", "latin1", "privateDetail", "\u00a0utf-8", "utf-8\v"} {
		argument, err := json.Marshal(label)
		require.NoError(t, err)
		// Unsupported primitive labels raise a fixed RangeError, not an echoed label.
		source := fmt.Sprintf(`(() => {try {new TextDecoder(%s); return "accepted";}
			catch (error) {return [error instanceof RangeError, error.message.includes("privateDetail")];}})()`, argument)
		got, err := evaluateSource(context.Background(), textCodecTestLimits(), source)
		require.NoError(t, err)
		require.JSONEq(t, `[true,false]`, string(got))
	}
}

// TestTextCodecRejectsCoercion validates primitive arguments and options without
// invoking authored string conversions, getters, or proxy traps.
func TestTextCodecRejectsCoercion(t *testing.T) {
	for _, source := range []string{
		`new TextEncoder().encode(null)`,
		`new TextEncoder().encode(42)`,
		`new TextEncoder().encode(new String("privateDetail"))`,
		`new TextEncoder().encode({toString() { touched++; return "privateDetail"; }})`,
		`new TextDecoder(null)`,
		`new TextDecoder({toString() { touched++; return "utf-8"; }})`,
		`new TextDecoder("utf-8", {fatal: 1})`,
		`new TextDecoder("utf-8", {ignoreBOM: "true"})`,
		`new TextDecoder("utf-8", {fatal: new Boolean(false)})`,
		`new TextDecoder("utf-8", {get fatal() { touched++; return false; }})`,
		`new TextDecoder("utf-8", new Proxy({}, {get() { touched++; }, getOwnPropertyDescriptor() { touched++; }}))`,
		`new TextDecoder().decode(null)`,
		`new TextDecoder().decode("privateDetail")`,
		`new TextDecoder().decode([65])`,
		`new TextDecoder().decode({buffer: new ArrayBuffer(1)})`,
		`new TextDecoder().decode(new Uint8Array([65]), {stream: true})`,
		`new TextDecoder().decode(new Uint8Array([65]), {stream: 0})`,
		`new TextDecoder().decode(new Uint8Array([65]), {get stream() { touched++; return false; }})`,
		`new TextDecoder().decode(new Uint8Array([65]), new Proxy({}, {get() { touched++; }, getOwnPropertyDescriptor() { touched++; }}))`,
	} {
		t.Run(source, func(t *testing.T) {
			// The counter detects even a harmless getter invocation before rejection.
			wrapped := `(() => {let touched = 0; try {` + source + `; return "accepted";}
				catch (error) {return [error instanceof TypeError, touched, error.message.includes("privateDetail")];}})()`
			got, err := evaluateSource(context.Background(), textCodecTestLimits(), wrapped)
			require.NoError(t, err)
			require.JSONEq(t, `[true,0,false]`, string(got))
		})
	}
}

// TestTextCodecOptionsAndReceivers reads only own option data and uses the
// method receiver's private codec state when a real instance borrows a method.
func TestTextCodecOptionsAndReceivers(t *testing.T) {
	source := `(() => {
		let touched = 0;
		Object.defineProperty(Object.prototype, "fatal", {get() {touched++; return true;}});
		Object.defineProperty(Object.prototype, "stream", {get() {touched++; return true;}});
		const options = Object.create(null);
		options.ignoreBOM = true;
		options.fatal = undefined;
		Object.defineProperty(options, "unused", {get() {touched++;}});
		const decoder = new TextDecoder(undefined, options);
		const strict = new TextDecoder("utf-8", {fatal: true});
		const encoder = new TextEncoder();
		let rejected = 0;
		for (const [method, value] of [[encoder.encode, "a"], [decoder.decode, new Uint8Array([65])]]) {
			try {method.call({}, value);} catch (error) {if (error instanceof TypeError) rejected++;}
		}
		return [decoder.fatal, decoder.ignoreBOM,
			decoder.decode(new Uint8Array([239,187,191,65]), {}),
			strict.decode.call(decoder, new Uint8Array([255])),
			Array.from(encoder.encode.call(new TextEncoder(), "ok")),
			new TextDecoder("utf-8", null).decode(new Uint8Array([65]), null),
			rejected, touched];
	})()`

	// Inherited/unused options never execute, and malformed bytes use the receiving decoder's policy.
	got, err := evaluateSource(context.Background(), textCodecTestLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[false,true,"\ufeffA","\ufffd",[111,107],"A",2,0]`, string(got))
}

// TestTextCodecConstructorSemantics requires new without branding its receiver
// on a failed call, and retains genuine brands through subclass and new-target construction.
func TestTextCodecConstructorSemantics(t *testing.T) {
	source := `(() => {
		const fake = {};
		let rejected = 0;
		for (const constructor of [TextEncoder, TextDecoder]) {
			try {constructor();} catch (error) {if (error instanceof TypeError) rejected++;}
			try {constructor.call(fake);} catch (error) {if (error instanceof TypeError) rejected++;}
		}
		for (const [method, value] of [[TextEncoder.prototype.encode, "a"],
			[TextDecoder.prototype.decode, new Uint8Array([65])]]) {
			try {method.call(fake, value);} catch (error) {if (error instanceof TypeError) rejected++;}
		}
		class Encoder extends TextEncoder {}
		class Decoder extends TextDecoder {}
		function Alternate() {}
		const encoder = new Encoder();
		const decoder = new Decoder("utf8", {fatal: true, ignoreBOM: true});
		const alternateEncoder = Reflect.construct(TextEncoder, [], Alternate);
		const alternateDecoder = Reflect.construct(TextDecoder, ["utf8", {ignoreBOM: true}], Alternate);
		return [rejected, Object.hasOwn(globalThis, "encoding"), Object.hasOwn(fake, "encoding"),
			encoder instanceof Encoder, encoder instanceof TextEncoder,
			decoder instanceof Decoder, decoder instanceof TextDecoder,
			encoder.encoding, decoder.encoding, decoder.fatal, decoder.ignoreBOM,
			decoder.decode(encoder.encode("é")),
			Object.getPrototypeOf(alternateEncoder) === Alternate.prototype,
			Object.getPrototypeOf(alternateDecoder) === Alternate.prototype,
			alternateEncoder.encoding, alternateDecoder.fatal, alternateDecoder.ignoreBOM,
			Array.from(TextEncoder.prototype.encode.call(alternateEncoder, "ok")),
			TextDecoder.prototype.decode.call(alternateDecoder, new Uint8Array([239,187,191,65]))];
	})()`

	// Class construction may choose a different prototype without changing the private codec brand.
	got, err := evaluateSource(context.Background(), textCodecTestLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[6,false,false,true,true,true,true,"utf-8","utf-8",true,true,"é",true,true,"utf-8",false,true,[111,107],"\ufeffA"]`, string(got))
}

// TestTextCodecReadOnlySurface prevents authored replacement of globals,
// prototype methods, or the immutable settings captured by each instance.
func TestTextCodecReadOnlySurface(t *testing.T) {
	source := `(() => {"use strict";
		const encoder = new TextEncoder(), decoder = new TextDecoder();
		const attempts = [
			() => {globalThis.TextEncoder = null;},
			() => {globalThis.TextDecoder = null;},
			() => {TextEncoder.prototype = {};},
			() => {TextDecoder.prototype.decode = () => "changed";},
			() => {encoder.encoding = "changed";},
			() => {decoder.encoding = "changed";},
			() => {decoder.fatal = true;},
			() => {decoder.ignoreBOM = true;},
			() => {Object.defineProperty(TextEncoder.prototype, "encode", {value: null});}
		];
		const rejected = attempts.map(attempt => {
			try {attempt(); return false;} catch (error) {return error instanceof TypeError;}
		});
		const properties = [[globalThis, "TextEncoder"], [globalThis, "TextDecoder"],
			[TextEncoder, "prototype"], [TextDecoder, "prototype"],
			[TextEncoder.prototype, "encode"], [TextDecoder.prototype, "decode"],
			[encoder, "encoding"], [decoder, "encoding"], [decoder, "fatal"], [decoder, "ignoreBOM"]];
		const protectedProperties = properties.every(([object, name]) => {
			const descriptor = Object.getOwnPropertyDescriptor(object, name);
			return !descriptor.writable && !descriptor.configurable && !descriptor.enumerable;
		});
		return [rejected.every(Boolean), protectedProperties, encoder.encoding, decoder.encoding,
			decoder.fatal, decoder.ignoreBOM, decoder.decode(encoder.encode("ok"))];
	})()`

	// The protected surface retains its original behavior after every attempted mutation.
	got, err := evaluateSource(context.Background(), textCodecTestLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[true,true,"utf-8","utf-8",false,false,"ok"]`, string(got))
}

// TestTextDecoderBufferSources respects byte offsets and lengths for every
// native view family instead of exporting or interpreting its element values.
func TestTextDecoderBufferSources(t *testing.T) {
	for _, test := range []struct{ view, want string }{
		{`bytes.buffer`, "abcdefghijklmnop"},
		{`new Uint8Array(bytes.buffer, 2, 3)`, "cde"},
		{`new Uint8ClampedArray(bytes.buffer, 2, 3)`, "cde"},
		{`new Int8Array(bytes.buffer, 2, 3)`, "cde"},
		{`new Uint16Array(bytes.buffer, 2, 2)`, "cdef"},
		{`new Int16Array(bytes.buffer, 2, 2)`, "cdef"},
		{`new Uint32Array(bytes.buffer, 4, 1)`, "efgh"},
		{`new Int32Array(bytes.buffer, 4, 1)`, "efgh"},
		{`new Float32Array(bytes.buffer, 4, 1)`, "efgh"},
		{`new Float64Array(bytes.buffer, 8, 1)`, "ijklmnop"},
		{`new BigInt64Array(bytes.buffer, 8, 1)`, "ijklmnop"},
		{`new BigUint64Array(bytes.buffer, 8, 1)`, "ijklmnop"},
		{`new DataView(bytes.buffer, 2, 3)`, "cde"},
		{`new Uint8Array(bytes.buffer, 16, 0)`, ""},
		{`new DataView(bytes.buffer, 16, 0)`, ""},
	} {
		t.Run(test.view, func(t *testing.T) {
			// Element endianness does not matter: decoding sees only the selected raw bytes.
			source := `(() => {const bytes = new TextEncoder().encode("abcdefghijklmnop");
				return new TextDecoder().decode(` + test.view + `);})()`
			got, err := evaluateSource(context.Background(), textCodecTestLimits(), source)
			require.NoError(t, err)
			want, err := json.Marshal(test.want)
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
	}
}

// TestTextCodecNativeIntrinsics cannot redirect buffer reads or output
// construction through replaced globals, poisoned getters, or changed prototypes.
func TestTextCodecNativeIntrinsics(t *testing.T) {
	source := `(() => {
		let touched = 0;
		const encoder = new TextEncoder(), decoder = new TextDecoder();
		const bytes = new Uint8Array([88,65,66,89]);
		const view = new DataView(bytes.buffer, 1, 2);
		const typed = new Uint8Array(bytes.buffer, 1, 2);
		const fail = function() {touched++; throw "privateDetail";};
		for (const object of [view, typed, DataView.prototype, Object.getPrototypeOf(Uint8Array.prototype)]) {
			for (const name of ["buffer", "byteOffset", "byteLength"]) {
				Object.defineProperty(object, name, {get: fail});
			}
		}
		Object.setPrototypeOf(view, null);
		Object.setPrototypeOf(typed, null);
		Uint8Array = fail;
		ArrayBuffer.isView = fail;
		Object.getOwnPropertyDescriptor = fail;
		const first = encoder.encode("ok");
		first[0] = 88;
		const second = encoder.encode("ok");
		return [decoder.decode(view), decoder.decode(typed), decoder.decode(second), first[0], second[0], touched];
	})()`

	// Intrinsics were captured before this code, and each encode owns a distinct byte buffer.
	got, err := evaluateSource(context.Background(), textCodecTestLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `["AB","AB","ok",88,111,0]`, string(got))
}

// TestTextDecoderRejectsFakeBuffers rejects proxies and prototype lookalikes
// before running any authored traps, even when a proxy has been revoked.
func TestTextDecoderRejectsFakeBuffers(t *testing.T) {
	source := `(() => {
		let touched = 0;
		const decoder = new TextDecoder();
		const bytes = new Uint8Array([65]);
		const proxy = new Proxy(bytes, {get() {touched++; throw "privateDetail";}});
		const revoked = Proxy.revocable(bytes, {});
		revoked.revoke();
		return [[proxy, revoked.proxy, Object.create(Uint8Array.prototype), Object.create(ArrayBuffer.prototype)].map(value => {
			try {decoder.decode(value); return false;} catch (error) {return error instanceof TypeError;}
		}), touched];
	})()`
	got, err := evaluateSource(context.Background(), textCodecTestLimits(), source)
	require.NoError(t, err)
	require.JSONEq(t, `[[true,true,true,true],0]`, string(got))
}

// TestTextDecoderDetachedAndReflectedInputs rejects unavailable backing stores
// and reflected Go slices without exporting or dereferencing their contents.
func TestTextDecoderDetachedAndReflectedInputs(t *testing.T) {
	got, err := jsruntime.EvaluateJSON(context.Background(), textCodecTestLimits(), func(_ context.Context, vm *goja.Runtime) (goja.Value, error) {
		// Construct views while the buffer is live, then detach before authored decoding.
		buffer := vm.NewArrayBuffer([]byte{65, 66, 67, 68})
		require.NoError(t, vm.Set("buffer", buffer))
		require.NoError(t, vm.Set("reflected", []byte{65}))
		_, err := vm.RunString(`const inputs = [buffer, new Uint8Array(buffer), new DataView(buffer), new Uint16Array(buffer), reflected];`)
		require.NoError(t, err)
		require.True(t, buffer.Detach())
		value, err := vm.RunString(`(() => {const decoder = new TextDecoder();
			return inputs.map(input => {try {decoder.decode(input); return false;}
				catch (error) {return error instanceof TypeError;}});})()`)
		return value, jsruntime.EvaluationError(err)
	})

	// Detachment is invalid rather than an empty string, and Go slices are not BufferSources.
	require.NoError(t, err)
	require.JSONEq(t, `[true,true,true,true,true]`, string(got))
}
