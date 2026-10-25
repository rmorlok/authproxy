package javascript

import (
	"context"
	"math"
	"reflect"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// outputEncoder captures pristine reflection intrinsics before the author's
// program runs. It accepts JSON data without invoking getters, proxy traps, or
// toJSON methods, even when the program replaces the corresponding globals.
type outputEncoder struct {
	// vm is the evaluation's runtime. Reflection calls and temporary JavaScript
	// values must use this same runtime; it is accessed only by its owning goroutine.
	vm *goja.Runtime
	// limits holds the resolved policy used to bound output bytes, nesting, and
	// traversal work. Each encode call starts fresh counters against these caps.
	limits Limits
	// ownKeys is the original Reflect.ownKeys function, captured before authored
	// code runs. It includes symbol and non-enumerable keys so validation can
	// inspect properties that ordinary JSON serialization would silently omit.
	ownKeys goja.Callable
	// propertyDescriptor is the original Object.getOwnPropertyDescriptor function.
	// After an object passes validation, it reads property metadata so accessors
	// can be rejected and data values read without invoking getters.
	propertyDescriptor goja.Callable
	// objectPrototype is the original Object.prototype identity. Ordinary objects
	// must have this direct prototype or a null prototype; custom ones are rejected.
	objectPrototype *goja.Object
	// arrayPrototype is the original Array.prototype identity. Arrays must have
	// this direct prototype or a null prototype to use the dense-array encoder.
	arrayPrototype *goja.Object
	// objectExportType is Goja's Go export type for a fresh ordinary object.
	// Together with ClassName, it helps reject unsupported object kinds, including
	// proxies, before reflective access without actually exporting their contents.
	objectExportType reflect.Type
	// arrayExportType is Goja's Go export type for a fresh native array. Together
	// with ClassName, it selects array validation and encoding without exporting
	// the array into an intermediate Go slice.
	arrayExportType reflect.Type
}

// newOutputEncoder prepares an encoder for a fresh runtime whose standard
// globals have not yet been changed. The encoder and runtime belong to one
// evaluation and must only be used by its goroutine.
func newOutputEncoder(vm *goja.Runtime, limits Limits) *outputEncoder {
	ownKeys, _ := goja.
		AssertFunction(vm.
			Get("Reflect").
			ToObject(vm).
			Get("ownKeys"),
		)
	propertyDescriptor, _ := goja.AssertFunction(vm.
		Get("Object").
		ToObject(vm).
		Get("getOwnPropertyDescriptor"),
	)
	object, array := vm.NewObject(), vm.NewArray()

	return &outputEncoder{
		vm:                 vm,
		limits:             limits,
		ownKeys:            ownKeys,
		propertyDescriptor: propertyDescriptor,
		objectPrototype:    object.Prototype(),
		arrayPrototype:     array.Prototype(),
		objectExportType:   object.ExportType(),
		arrayExportType:    array.ExportType(),
	}
}

// outputState owns one traversal's bytes, work counter, and active object path.
// Shared acyclic objects may appear repeatedly; only a repeated object on the
// current path is a cycle. No source object or exported Go graph is retained.
type outputState struct {
	// encoder supplies the runtime, captured reflection functions, object-type
	// checks, and limits shared throughout this traversal.
	encoder *outputEncoder
	// ctx carries caller cancellation and the evaluation timeout. Traversal and
	// string encoding check it cooperatively to stop work after cancellation.
	ctx context.Context
	// bytes accumulates JSON owned by this traversal, bounded by MaxOutputBytes.
	// It becomes the returned result on success; partial output is discarded on error.
	bytes []byte
	// nodes counts visited values and inspected ordinary-object keys against
	// MaxNodes, including non-enumerable keys. Array indices and length metadata
	// do not add separate charges beyond the array and its element values.
	nodes int
	// active contains objects on the current recursion path. Revisiting one is
	// a cycle; removing it on return allows shared acyclic objects to appear again.
	active map[*goja.Object]bool
}

// encode converts a JavaScript value to independently owned JSON. Object keys
// and values count toward the node budget; array indices and length metadata
// do not. Root depth is zero. JSON escaping is charged before bytes are added.
// The caller must keep the runtime's cancellation interrupt active until this
// method returns and classify Goja interrupt/stack-overflow panics.
func (e *outputEncoder) encode(
	ctx context.Context,
	value goja.Value,
) (common.RawJSON, error) {
	state := &outputState{
		encoder: e,
		ctx:     ctx,
		active:  make(map[*goja.Object]bool),
	}
	var err error
	if exception := e.vm.Try(
		func() { err = state.value(value, 0) },
	); exception != nil {
		return nil, ErrInvalidOutput
	}
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return common.RawJSON(state.bytes), nil
}

// value appends one JSON value after checking cancellation and structural
// limits. Only primitive type-specific conversions are used: object Export
// and JavaScript coercions could allocate unchecked graphs or execute code.
func (s *outputState) value(value goja.Value, depth int) error {
	if err := s.node(); err != nil {
		return err
	}
	if depth > s.encoder.limits.MaxDepth {
		return ErrLimitExceeded
	}
	if value == nil || goja.IsUndefined(value) {
		return ErrInvalidOutput
	}
	if goja.IsNull(value) {
		return s.append("null")
	}
	if text, ok := value.(goja.String); ok {
		return s.quoted(text)
	}
	if object, ok := value.(*goja.Object); ok {
		return s.object(object, depth)
	}
	if _, ok := value.(*goja.Symbol); ok {
		return ErrInvalidOutput
	}

	switch value.ExportType().Kind() {
	case reflect.Bool:
		return s.append(strconv.FormatBool(value.ToBoolean()))
	case reflect.Int64:
		return s.append(strconv.FormatInt(value.ToInteger(), 10))
	case reflect.Float64:
		number := value.ToFloat()
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return ErrInvalidOutput
		}
		return s.append(strconv.FormatFloat(number, 'g', -1, 64))
	default:
		// BigInt and any future non-JSON primitive must not silently change type.
		return ErrInvalidOutput
	}
}

// object accepts ordinary object/array metadata and original or null
// prototypes. ExportType is inspected before any property operation so even a
// revoked Proxy is rejected without executing its traps. Goja gives some
// built-ins, such as weak collections and iterators, ordinary object metadata:
// an author who deliberately resets their prototype gets only their own data
// properties projected, never their hidden collection or iterator state.
func (s *outputState) object(object *goja.Object, depth int) error {
	e := s.encoder
	exportType := object.ExportType()
	isArray := exportType == e.arrayExportType && object.ClassName() == "Array"
	if !isArray && (exportType != e.objectExportType || object.ClassName() != "Object") {
		return ErrInvalidOutput
	}
	prototype := object.Prototype()
	if prototype != nil && ((!isArray && prototype != e.objectPrototype) ||
		(isArray && prototype != e.arrayPrototype)) {
		return ErrInvalidOutput
	}
	if s.active[object] {
		return ErrInvalidOutput
	}
	s.active[object] = true
	defer delete(s.active, object)

	// Native arrays have an intrinsic own data length. Reject an array whose
	// elements cannot fit before ownKeys allocates a second array of its keys.
	// The container has already consumed one node, so only remaining work counts.
	length := int64(0)
	if isArray {
		lengthValue, _, err := s.property(object, e.vm.ToValue("length").(goja.String))
		if err != nil {
			return err
		}
		length = lengthValue.ToInteger()
		if length > int64(e.limits.MaxNodes-s.nodes) {
			return ErrLimitExceeded
		}
	}

	// Goja's public reflection API materializes a key array. Bound its traversal
	// before inspecting properties; this is not a hard heap allocation quota.
	keysValue, err := e.ownKeys(goja.Undefined(), object)
	if err != nil {
		return outputReflectionError(err)
	}
	keys := keysValue.(*goja.Object)
	keyCount := keys.Get("length").ToInteger()
	metadataCount := int64(0)
	if isArray {
		metadataCount = 1 // The intrinsic length property is not a JSON value.
	}
	if keyCount-metadataCount > int64(e.limits.MaxNodes-s.nodes) {
		return ErrLimitExceeded
	}
	if isArray {
		return s.array(object, keys, keyCount, length, depth)
	}
	return s.properties(object, keys, keyCount, depth)
}

// properties emits own enumerable string data properties in JavaScript's key
// order. Hidden data properties are omitted but consume inspection work;
// accessors and symbol keys are rejected instead of invoking or losing them.
func (s *outputState) properties(object, keys *goja.Object, keyCount int64, depth int) error {
	if err := s.append("{"); err != nil {
		return err
	}
	separator := ""
	for i := int64(0); i < keyCount; i++ {
		if err := s.node(); err != nil {
			return err
		}
		key, ok := keys.Get(strconv.FormatInt(i, 10)).(goja.String)
		if !ok {
			return ErrInvalidOutput
		}
		value, enumerable, err := s.property(object, key)
		if err != nil {
			return err
		}
		if !enumerable {
			continue
		}
		if err := s.append(separator); err != nil {
			return err
		}
		if err := s.quoted(key); err != nil {
			return err
		}
		if err := s.append(":"); err != nil {
			return err
		}
		if err := s.value(value, depth+1); err != nil {
			return err
		}
		separator = ","
	}
	return s.append("}")
}

// array requires a dense array with only indexed data properties and length.
// Its intrinsic length has already been checked against the remaining nodes.
// Checking every own key rejects extra properties that JSON would otherwise
// drop, while checking each descriptor rejects holes even when a prototype
// supplies an inherited value at the missing index.
func (s *outputState) array(object, keys *goja.Object, keyCount, length int64, depth int) error {
	if keyCount != length+1 {
		return ErrInvalidOutput
	}
	for i := int64(0); i < keyCount; i++ {
		if err := s.ctx.Err(); err != nil {
			return err
		}
		key, ok := keys.Get(strconv.FormatInt(i, 10)).(goja.String)
		if !ok {
			return ErrInvalidOutput
		}
		// Array indices are returned first in numeric order, followed by length.
		// Comparing primitive keys does not invoke author-defined coercions.
		expected := "length"
		if i < length {
			expected = strconv.FormatInt(i, 10)
		}
		if !key.SameAs(s.encoder.vm.ToValue(expected)) {
			return ErrInvalidOutput
		}
	}
	if err := s.append("["); err != nil {
		return err
	}
	for i := int64(0); i < length; i++ {
		if i > 0 {
			if err := s.append(","); err != nil {
				return err
			}
		}
		key := s.encoder.vm.ToValue(strconv.FormatInt(i, 10)).(goja.String)
		value, _, err := s.property(object, key)
		if err != nil {
			return err
		}
		if err := s.value(value, depth+1); err != nil {
			return err
		}
	}
	return s.append("]")
}

// property reads an own property descriptor without invoking accessors. A
// null prototype on the fresh descriptor prevents poisoned Object.prototype
// getters from being consulted for absent descriptor fields.
func (s *outputState) property(object *goja.Object, key goja.String) (goja.Value, bool, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, false, err
	}
	descriptorValue, err := s.encoder.propertyDescriptor(goja.Undefined(), object, key)
	if err != nil {
		return nil, false, outputReflectionError(err)
	}
	descriptor, ok := descriptorValue.(*goja.Object)
	if !ok {
		return nil, false, ErrInvalidOutput
	}
	if err := descriptor.SetPrototype(nil); err != nil {
		return nil, false, outputReflectionError(err)
	}
	// Accessor descriptors always have get/set and no writable field, including
	// accessors whose getter and setter are both undefined.
	if descriptor.Get("writable") == nil {
		return nil, false, ErrInvalidOutput
	}
	return descriptor.Get("value"), descriptor.Get("enumerable").ToBoolean(), nil
}

// outputReflectionError preserves uncatchable VM failures for the evaluation
// wrapper and replaces ordinary exceptions with a value-free public error.
func outputReflectionError(err error) error {
	switch err.(type) {
	case *goja.InterruptedError, *goja.StackOverflowError:
		panic(err)
	default:
		return ErrInvalidOutput
	}
}

// node charges one value or inspected object key without overflowing the
// counter. Cancellation is checked even when the next value is very small.
func (s *outputState) node() error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if s.nodes >= s.encoder.limits.MaxNodes {
		return ErrLimitExceeded
	}
	s.nodes++
	return nil
}

// append checks the encoded byte budget before growing the result buffer.
func (s *outputState) append(text string) error {
	if len(text) > s.encoder.limits.MaxOutputBytes-len(s.bytes) {
		return ErrLimitExceeded
	}
	s.bytes = append(s.bytes, text...)
	return nil
}

// quoted emits a JavaScript UTF-16 string as JSON without first constructing
// an unbounded Go string. Paired surrogates become UTF-8; lone surrogates remain
// JSON escapes instead of being silently replaced by Unicode replacement text.
func (s *outputState) quoted(text goja.String) error {
	if text.Length() > s.encoder.limits.MaxOutputBytes-len(s.bytes)-2 {
		return ErrLimitExceeded
	}
	if err := s.append("\""); err != nil {
		return err
	}
	for i := 0; i < text.Length(); i++ {
		if err := s.ctx.Err(); err != nil {
			return err
		}
		unit := text.CharAt(i)
		var encoded string
		switch unit {
		case '\\', '"':
			encoded = string([]byte{'\\', byte(unit)})
		case '\b':
			encoded = `\b`
		case '\f':
			encoded = `\f`
		case '\n':
			encoded = `\n`
		case '\r':
			encoded = `\r`
		case '\t':
			encoded = `\t`
		default:
			if unit < 0x20 || (unit >= 0xD800 && unit <= 0xDFFF) {
				if unit >= 0xD800 && unit <= 0xDBFF && i+1 < text.Length() {
					next := text.CharAt(i + 1)
					if next >= 0xDC00 && next <= 0xDFFF {
						encoded = string(utf16.DecodeRune(rune(unit), rune(next)))
						i++
					}
				}
				if encoded == "" {
					const hex = "0123456789abcdef"
					encoded = string([]byte{'\\', 'u', hex[unit>>12], hex[(unit>>8)&15], hex[(unit>>4)&15], hex[unit&15]})
				}
			} else {
				var bytes [utf8.UTFMax]byte
				n := utf8.EncodeRune(bytes[:], rune(unit))
				encoded = string(bytes[:n])
			}
		}
		if err := s.append(encoded); err != nil {
			return err
		}
	}
	return s.append("\"")
}
