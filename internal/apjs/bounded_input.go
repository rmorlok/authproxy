package apjs

import (
	"context"
	"encoding/json"
	"math"
	"unicode/utf8"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// encodeVariables serializes the caller's JSON-like variables before native VM
// decoding. It bounds traversal and each append rather than first marshaling an
// arbitrary Go graph. Nil variables mean no globals and encode as an empty object.
// Callers must keep all maps and slices stable until the call returns; successful
// bytes are detached, and errors return no partial document or private values.
//
// Supported inputs are ordinary JSON containers (map[string]any,
// map[string]string, []any, and []string) and their primitive values. Named
// wrappers, structs, pointers, custom marshalers, and live Go objects are rejected
// without calling their methods. Cyclic containers stop at the depth/node limit.
// These limits bound conversion work and output, not the runtime's total heap.
func encodeVariables(
	ctx context.Context,
	vars map[string]any,
	limits jsruntime.JSONLimits,
) (json.RawMessage, error) {
	if ctx == nil ||
		limits.MaxBytes <= 0 ||
		limits.MaxDepth <= 0 ||
		limits.MaxNodes <= 0 {
		return nil, jsruntime.ErrInvalidInput
	}

	if vars == nil {
		vars = map[string]any{}
	}

	encoder := variableEncoder{
		ctx:    ctx,
		limits: limits,
	}

	if err := encoder.value(vars, 0); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return json.RawMessage(encoder.bytes), nil
}

// variableEncoder owns one bounded serialization of all input globals. It never
// retains a second Go object graph or invokes reflection/custom marshal hooks.
type variableEncoder struct {
	ctx    context.Context      // Caller cancellation and the execution deadline.
	limits jsruntime.JSONLimits // Aggregate serialized bytes, root-zero depth, and nodes.
	bytes  []byte               // Detached JSON, grown only after checking its byte cap.
	nodes  int                  // Values plus object keys, matching JSONDecoder accounting.
}

// value charges one value before inspecting it or descending into a container.
// Primitive marshaling is restricted to exact built-in types, so custom methods
// cannot run. Large strings and numeric spellings are bounded before marshaling.
func (e *variableEncoder) value(value any, depth int) error {
	if err := e.node(); err != nil {
		return err
	}

	if depth > e.limits.MaxDepth {
		return jsruntime.ErrLimitExceeded
	}

	switch value := value.(type) {
	case nil:
		return e.append([]byte("null"))
	case string:
		return e.string(value)
	case json.Number:
		if len(value) > e.limits.MaxBytes-len(e.bytes) {
			return jsruntime.ErrLimitExceeded
		}
		number, err := value.Float64()
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return jsruntime.ErrInvalidInput
		}
		// Marshal validates the JSON numeric grammar after the bounded float check.
		encoded, err := json.Marshal(value)
		if err != nil {
			return jsruntime.ErrInvalidInput
		}
		return e.append(encoded)
	case bool,
		int,
		int8,
		int16,
		int32,
		int64,
		uint,
		uint8,
		uint16,
		uint32,
		uint64,
		float32,
		float64:
		encoded, err := json.Marshal(value)
		if err != nil {
			return jsruntime.ErrInvalidInput
		}
		return e.append(encoded)
	case map[string]any:
		return encodeVariableMap(e, value, depth)
	case map[string]string:
		return encodeVariableMap(e, value, depth)
	case []any:
		return encodeVariableSlice(e, value, depth)
	case []string:
		return encodeVariableSlice(e, value, depth)
	default:
		return jsruntime.ErrInvalidInput
	}
}

// encodeVariableMap writes an ordinary JSON object without allocating/sorting a
// list of its keys. Ordering is unspecified, and keys consume nodes without
// increasing depth. Nil nested maps retain JSON null semantics.
func encodeVariableMap[V any](e *variableEncoder, values map[string]V, depth int) error {
	if values == nil {
		return e.append([]byte("null"))
	}
	if err := e.append([]byte("{")); err != nil {
		return err
	}
	first := true

	for key, value := range values {
		if err := e.node(); err != nil {
			return err
		}
		if !first {
			if err := e.append([]byte(",")); err != nil {
				return err
			}
		}

		first = false
		if err := e.string(key); err != nil {
			return err
		}
		if err := e.append([]byte(":")); err != nil {
			return err
		}
		if err := e.value(value, depth+1); err != nil {
			return err
		}
	}

	return e.append([]byte("}"))
}

// encodeVariableSlice writes native array elements with no additional charge
// for indices; each element is a value one level deeper than the array itself.
func encodeVariableSlice[V any](e *variableEncoder, values []V, depth int) error {
	if values == nil {
		return e.append([]byte("null"))
	}
	if err := e.append([]byte("[")); err != nil {
		return err
	}

	for i, value := range values {
		if i != 0 {
			if err := e.append([]byte(",")); err != nil {
				return err
			}
		}
		if err := e.value(value, depth+1); err != nil {
			return err
		}
	}

	return e.append([]byte("]"))
}

// node charges traversal work before visiting a value or object key. This also
// stops recursive/cyclic input before conversion can consume unbounded work.
func (e *variableEncoder) node() error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if e.nodes >= e.limits.MaxNodes {
		return jsruntime.ErrLimitExceeded
	}
	e.nodes++
	return nil
}

// string encodes a bounded UTF-8 string, preserving JSON escaping. Its raw
// length is checked first, so temporary marshaling can grow by at most JSON's
// fixed escaping factor; the exact encoded length is checked before append.
func (e *variableEncoder) string(value string) error {
	remaining := e.limits.MaxBytes - len(e.bytes)
	if remaining < 2 || len(value) > remaining-2 {
		return jsruntime.ErrLimitExceeded
	}

	if !utf8.ValidString(value) {
		return jsruntime.ErrInvalidInput
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return jsruntime.ErrInvalidInput
	}

	return e.append(encoded)
}

// append checks cancellation and the exact serialized-byte cap before growing
// the output buffer. Fixed punctuation and scalar encodings use the same budget.
func (e *variableEncoder) append(encoded []byte) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}

	if len(encoded) > e.limits.MaxBytes-len(e.bytes) {
		return jsruntime.ErrLimitExceeded
	}

	e.bytes = append(e.bytes, encoded...)

	return nil
}
