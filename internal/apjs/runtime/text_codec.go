package runtime

import (
	"reflect"

	"github.com/dop251/goja"
)

// textCodecConstructorFactory keeps JavaScript's constructor/new.target behavior
// while delegating bounded initialization to a private native callback. Goja's
// native ConstructorCall alone does not distinguish ordinary new from a call
// without new. The fixed class source adds no authored parsing or capabilities.
var textCodecConstructorFactory = goja.MustCompile("", `(function (initialize) {
	return class {
		constructor(label, options) { initialize(this, label, options); }
	};
})`, true)

// textCodecs owns one VM's UTF-8 codec bindings. Private instance maps implement
// receiver checks without exposing Go state. Constructor calls consume execution work and allocation units,
// so the retained metadata is bounded and released with the evaluation's VM.
type textCodecs struct {
	host               *utilityHost                        // Shared accounting, cancellation, and owning VM.
	buffers            *textCodecBuffers                   // Pristine native buffer and view operations.
	propertyDescriptor goja.Callable                       // Reads own data options without getters.
	objectPrototype    *goja.Object                        // Permitted ordinary options-object prototype.
	objectExportType   reflect.Type                        // Rejects proxies before reflective operations.
	rangeError         goja.Constructor                    // Creates fixed label errors after global tampering.
	encoders           map[*goja.Object]struct{}           // Genuine encoder receivers in this VM.
	decoders           map[*goja.Object]textDecoderOptions // Immutable options by receiver.
}

// textDecoderOptions captures the one-shot decoder's configuration. No byte
// prefix or BOM state is retained between decode calls.
type textDecoderOptions struct {
	fatal     bool // Reject malformed UTF-8 instead of emitting U+FFFD.
	ignoreBOM bool // Preserve an initial U+FEFF instead of stripping it.
}

// newTextCodecs captures intrinsics before any authored code can replace them.
func newTextCodecs(host *utilityHost) *textCodecs {
	vm := host.vm
	object := vm.NewObject()
	descriptor, _ := goja.AssertFunction(vm.Get("Object").ToObject(vm).Get("getOwnPropertyDescriptor"))
	rangeError, _ := goja.AssertConstructor(vm.Get("RangeError"))
	return &textCodecs{
		host:               host,
		buffers:            newTextCodecBuffers(vm),
		propertyDescriptor: descriptor,
		objectPrototype:    object.Prototype(),
		objectExportType:   object.ExportType(),
		rangeError:         rangeError,
		encoders:           make(map[*goja.Object]struct{}),
		decoders:           make(map[*goja.Object]textDecoderOptions),
	}
}

// install exposes read-only constructors and prototype methods. The selected
// globals are also reserved by async compilation; no constructor performs I/O.
func (t *textCodecs) install() error {
	vm := t.host.vm
	factoryValue, err := vm.RunProgram(textCodecConstructorFactory)
	if err != nil {
		return EvaluationError(err)
	}
	factory, _ := goja.AssertFunction(factoryValue)
	for _, definition := range []struct {
		name       string
		initialize func(goja.FunctionCall) goja.Value
		methodName string
		method     func(goja.FunctionCall) goja.Value
	}{
		{"TextEncoder", t.newEncoder, "encode", t.encode},
		{"TextDecoder", t.newDecoder, "decode", t.decode},
	} {
		constructorValue, err := factory(goja.Undefined(), vm.ToValue(definition.initialize))
		if err != nil {
			return EvaluationError(err)
		}

		constructor := constructorValue.ToObject(vm)
		if err := constructor.DefineDataProperty(
			"name",
			vm.ToValue(definition.name),
			goja.FLAG_FALSE, // writable
			goja.FLAG_FALSE, // configurable
			goja.FLAG_FALSE, // enumerable
		); err != nil {
			return ErrInvalidInput
		}

		prototype := constructor.Get("prototype").ToObject(vm)
		if err := prototype.DefineDataProperty(
			definition.methodName,
			vm.ToValue(definition.method),
			goja.FLAG_FALSE, // writable
			goja.FLAG_FALSE, // configurable
			goja.FLAG_FALSE, // enumerable
		); err != nil {
			return ErrInvalidInput
		}
		if err := constructor.DefineDataProperty(
			"prototype",
			prototype,
			goja.FLAG_FALSE, // writable
			goja.FLAG_FALSE, // configurable
			goja.FLAG_FALSE, // enumerable
		); err != nil {
			return ErrInvalidInput
		}
		if err := vm.GlobalObject().DefineDataProperty(
			definition.name,
			constructor,
			goja.FLAG_FALSE, // writable
			goja.FLAG_FALSE, // configurable
			goja.FLAG_FALSE, // enumerable
		); err != nil {
			return ErrInvalidInput
		}
	}
	return nil
}

// newEncoder initializes the class-created receiver as a branded UTF-8 encoder.
// Constructor arguments are unused, as in the web API.
func (t *textCodecs) newEncoder(call goja.FunctionCall) goja.Value {
	t.host.beginCall()

	receiver, ok := call.Argument(0).(*goja.Object)
	if !ok {
		t.host.invalidTextInput()
	}

	t.readOnly(receiver, "encoding", t.host.vm.ToValue("utf-8"))

	t.host.reserveOutput(1) // Retained native receiver metadata.
	t.encoders[receiver] = struct{}{}

	return goja.Undefined()
}

// newDecoder accepts UTF-8 labels and fixed Boolean options. Only supplied
// label text consumes input units; the constant-size options are bounded by the
// constructor-call allowance and never invoke authored accessors or coercion.
func (t *textCodecs) newDecoder(call goja.FunctionCall) goja.Value {
	t.host.beginCall()

	receiver, ok := call.Argument(0).(*goja.Object)
	if !ok {
		t.host.invalidTextInput()
	}

	if label := call.Argument(1); !goja.IsUndefined(label) {
		text, ok := label.(goja.String)
		if !ok {
			t.host.invalidTextInput()
		}

		t.host.consumeInput(text.Length())
		if !t.utf8Label(text) {
			errValue, err := t.rangeError(nil, t.host.vm.ToValue("Unsupported text encoding"))
			if err != nil {
				t.host.textCodecError(err)
			}
			panic(errValue)
		}
	}

	options := t.options(call.Argument(2))
	settings := textDecoderOptions{
		fatal:     t.booleanOption(options, "fatal"),
		ignoreBOM: t.booleanOption(options, "ignoreBOM"),
	}
	t.readOnly(receiver, "encoding", t.host.vm.ToValue("utf-8"))
	t.readOnly(receiver, "fatal", t.host.vm.ToValue(settings.fatal))
	t.readOnly(receiver, "ignoreBOM", t.host.vm.ToValue(settings.ignoreBOM))
	t.host.reserveOutput(1) // Retained native receiver metadata.
	t.decoders[receiver] = settings

	return goja.Undefined()
}

// encode returns a native Uint8Array containing UTF-8. Undefined input means
// empty text; all other input must be a primitive string. Conversion replaces
// lone UTF-16 surrogates, reserves exact bytes, and never invokes coercion.
func (t *textCodecs) encode(call goja.FunctionCall) goja.Value {
	t.host.beginCall()

	receiver, ok := call.This.(*goja.Object)
	if !ok {
		t.host.invalidTextInput()
	}

	if _, ok := t.encoders[receiver]; !ok {
		t.host.invalidTextInput()
	}

	value := call.Argument(0)
	if goja.IsUndefined(value) {
		value = t.host.vm.ToValue("")
	}

	text, ok := value.(goja.String)
	if !ok {
		t.host.invalidTextInput()
	}

	t.host.consumeInput(text.Length())
	bytes := t.host.encodeUTF8(text)

	result, err := t.buffers.uint8Array(t.host.vm, bytes)
	if err != nil {
		t.host.textCodecError(err)
	}

	t.host.checkContext()

	return result
}

// decode reads only a genuine native buffer or view, using its captured byte
// range on the VM owner. Each call starts fresh UTF-8/BOM state. Streaming is
// rejected explicitly instead of silently losing an incomplete byte prefix.
func (t *textCodecs) decode(call goja.FunctionCall) goja.Value {
	t.host.beginCall()

	receiver, ok := call.This.(*goja.Object)
	if !ok {
		t.host.invalidTextInput()
	}

	settings, ok := t.decoders[receiver]
	if !ok {
		t.host.invalidTextInput()
	}

	var bytes []byte
	if value := call.Argument(0); !goja.IsUndefined(value) {
		bytes, ok = t.buffers.bytes(value)
		if !ok {
			t.host.invalidTextInput()
		}
	}

	t.host.consumeInput(len(bytes))
	if t.booleanOption(t.options(call.Argument(1)), "stream") {
		t.host.invalidTextInput()
	}

	return t.host.decodeUTF8(bytes, settings.fatal, settings.ignoreBOM)
}

// readOnly installs constant instance metadata without exposing private state.
func (t *textCodecs) readOnly(object *goja.Object, name string, value goja.Value) {
	if err := object.DefineDataProperty(
		name,
		value,
		goja.FLAG_FALSE, // writable
		goja.FLAG_FALSE, // configurable
		goja.FLAG_FALSE, // enumerable
	); err != nil {
		t.host.textCodecError(err)
	}
}

// options accepts ordinary or null-prototype records. Unknown and inherited
// fields are ignored, so option validation never enumerates an authored graph.
func (t *textCodecs) options(value goja.Value) *goja.Object {
	if goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}

	object, ok := value.(*goja.Object)
	if !ok || object.ExportType() != t.objectExportType ||
		object.ClassName() != "Object" ||
		(object.Prototype() != nil && object.Prototype() != t.objectPrototype) {
		t.host.invalidTextInput()
	}

	return object
}

// booleanOption reads one own data field through a pristine descriptor helper.
// Accessors are rejected even when their getter is undefined. Only primitive
// Booleans are accepted; omitted/undefined options retain the false default.
func (t *textCodecs) booleanOption(options *goja.Object, name string) bool {
	if options == nil {
		return false
	}

	descriptorValue, err := t.propertyDescriptor(
		goja.Undefined(),
		options,
		t.host.vm.ToValue(name),
	)
	if err != nil {
		t.host.textCodecError(err)
	}

	if goja.IsUndefined(descriptorValue) {
		return false
	}

	descriptor := descriptorValue.ToObject(t.host.vm)
	if err := descriptor.SetPrototype(nil); err != nil {
		t.host.textCodecError(err)
	}
	if descriptor.Get("writable") == nil {
		t.host.invalidTextInput()
	}

	value := descriptor.Get("value")
	if goja.IsUndefined(value) {
		return false
	}

	if _, object := value.(*goja.Object); object ||
		value.ExportType() != reflect.TypeFor[bool]() {
		t.host.invalidTextInput()
	}

	return value.ToBoolean()
}

// utf8Label matches the Encoding Standard's six UTF-8 labels after ASCII-only
// trimming and case folding. The supplied text is already charged; only the
// at-most-17-unit normalized label is copied, even with large surrounding space.
func (t *textCodecs) utf8Label(text goja.String) bool {
	start, end := 0, text.Length()
	for start < end && base64Whitespace(text.CharAt(start)) {
		if start&base64PollMask == 0 {
			t.host.checkContext()
		}
		start++
	}

	for end > start && base64Whitespace(text.CharAt(end-1)) {
		if end&base64PollMask == 0 {
			t.host.checkContext()
		}
		end--
	}

	t.host.checkContext()

	if end-start > len("unicode-1-1-utf-8") {
		return false
	}

	normalized := make([]byte, end-start)
	for i := range normalized {
		unit := text.CharAt(start + i)

		if unit > 0x7f {
			return false
		}

		if unit >= 'A' && unit <= 'Z' {
			unit += 'a' - 'A'
		}

		normalized[i] = byte(unit)
	}

	switch string(normalized) {
	case "unicode-1-1-utf-8",
		"unicode11utf8",
		"unicode20utf8",
		"utf-8",
		"utf8",
		"x-unicode20utf8":
		return true
	default:
		return false
	}
}

// invalidTextInput throws fixed text for malformed data, unsupported streaming,
// or invalid arguments/receivers. No input, label, or decoder detail is exposed.
func (h *utilityHost) invalidTextInput() {
	panic(h.vm.NewTypeError("Invalid text codec input"))
}

// textCodecError preserves uncatchable VM failures and otherwise throws a fixed
// text-codec error. Authored exception strings never cross the host boundary.
func (h *utilityHost) textCodecError(err error) {
	switch err.(type) {
	case *goja.InterruptedError, *goja.StackOverflowError:
		panic(err)
	default:
		h.invalidTextInput()
	}
}
