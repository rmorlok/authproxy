package runtime

import (
	"reflect"

	"github.com/dop251/goja"
)

// textCodecViewAccessors reads native view metadata through captured intrinsic
// getters. Calling these directly bypasses authored properties and prototypes.
type textCodecViewAccessors struct {
	buffer     goja.Callable // Returns the view's native backing ArrayBuffer.
	byteOffset goja.Callable // Returns the first viewed byte within that buffer.
	byteLength goja.Callable // Returns the number of viewed bytes, regardless of element type.
}

// textCodecBuffers converts between native BufferSource values and codec-owned
// bytes. Capture it before authored code runs and use it only on the VM owner.
// It never exports a view, traverses its properties, or copies an input buffer.
type textCodecBuffers struct {
	uint8Constructor goja.Constructor       // Original Uint8Array constructor for owned output bytes.
	isView           goja.Callable          // Original ArrayBuffer.isView, which checks native identity.
	typedArray       textCodecViewAccessors // Intrinsics shared by every native typed-array element type.
	dataView         textCodecViewAccessors // DataView has separate intrinsic getters.
}

// newTextCodecBuffers captures native constructors and accessors from a fresh
// VM with pristine standard globals. All metadata lookups happen here, before
// scripts can replace globals or add getters. Unexpected capture failures are
// host integration errors; Goja interrupts propagate to the enclosing Run.
func newTextCodecBuffers(vm *goja.Runtime) *textCodecBuffers {
	uint8 := vm.Get("Uint8Array").ToObject(vm)
	uint8Constructor, _ := goja.AssertConstructor(uint8)
	isView, _ := goja.AssertFunction(vm.Get("ArrayBuffer").ToObject(vm).Get("isView"))
	descriptor, _ := goja.AssertFunction(vm.Get("Object").ToObject(vm).Get("getOwnPropertyDescriptor"))

	// These descriptors belong to untouched built-ins, so their get fields are
	// ordinary own data properties rather than authored accessors.
	getter := func(prototype *goja.Object, name string) goja.Callable {
		value, err := descriptor(goja.Undefined(), prototype, vm.ToValue(name))
		if err != nil {
			panic(err)
		}
		call, _ := goja.AssertFunction(value.ToObject(vm).Get("get"))
		return call
	}
	accessors := func(prototype *goja.Object) textCodecViewAccessors {
		return textCodecViewAccessors{
			buffer:     getter(prototype, "buffer"),
			byteOffset: getter(prototype, "byteOffset"),
			byteLength: getter(prototype, "byteLength"),
		}
	}

	return &textCodecBuffers{
		uint8Constructor: uint8Constructor,
		isView:           isView,
		typedArray:       accessors(uint8.Get("prototype").ToObject(vm).Prototype()),
		dataView:         accessors(vm.Get("DataView").ToObject(vm).Get("prototype").ToObject(vm)),
	}
}

// bytes returns a view of a native ArrayBuffer or ArrayBufferView without
// allocating in proportion to its size. The caller must admit the returned
// length before scanning or copying and keep these aliased bytes on the VM owner
// without running authored code. Detached buffers, reflected Go values, proxies,
// and lookalike objects are rejected without invoking authored getters or traps.
func (b *textCodecBuffers) bytes(value goja.Value) ([]byte, bool) {
	object, ok := value.(*goja.Object)
	if !ok {
		return nil, false
	}
	if object.ExportType() == reflect.TypeFor[goja.ArrayBuffer]() {
		buffer := object.Export().(goja.ArrayBuffer)
		if buffer.Detached() {
			return nil, false
		}
		return buffer.Bytes(), true
	}

	// isView checks Goja's internal type, including on revoked proxies, without
	// consulting Symbol.toStringTag or prototype properties. Only after that
	// check is it safe to call view getters, whose invalid-receiver error paths
	// could otherwise inspect an authored object's string tag.
	view, err := b.isView(goja.Undefined(), object)
	if err != nil {
		outputReflectionError(err)
		return nil, false
	}
	if !view.ToBoolean() {
		return nil, false
	}
	accessors := b.dataView
	if object.ExportType().Kind() == reflect.Slice {
		// Native typed arrays export numeric slices; the only other native
		// ArrayBufferView accepted by isView is DataView.
		accessors = b.typedArray
	}
	bufferValue, err := accessors.buffer(object)
	if err != nil {
		outputReflectionError(err)
		return nil, false
	}
	buffer := bufferValue.Export().(goja.ArrayBuffer)
	if buffer.Detached() {
		return nil, false
	}

	// Check detachment before reading offsets: detached DataView getters throw,
	// while detached typed-array getters return zero. Direct view Export would
	// instead slice detached storage and can panic inside Goja.
	offsetValue, err := accessors.byteOffset(object)
	if err != nil {
		outputReflectionError(err)
		return nil, false
	}
	lengthValue, err := accessors.byteLength(object)
	if err != nil {
		outputReflectionError(err)
		return nil, false
	}
	data := buffer.Bytes()
	offset, length := offsetValue.ToInteger(), lengthValue.ToInteger()
	if offset < 0 || offset > int64(len(data)) || length < 0 || length > int64(len(data))-offset {
		return nil, false
	}
	return data[int(offset):int(offset+length)], true
}

// uint8Array transfers already-reserved output bytes into a native Uint8Array.
// NewArrayBuffer retains data, so the caller must stop using that slice after
// this call. The captured constructor avoids replaced globals, iterators, and
// reflected Go wrappers; ordinary construction errors are returned safely.
func (b *textCodecBuffers) uint8Array(vm *goja.Runtime, data []byte) (goja.Value, error) {
	value, err := b.uint8Constructor(nil, vm.ToValue(vm.NewArrayBuffer(data)))
	if err != nil {
		return nil, outputReflectionError(err)
	}
	return value, nil
}
