package javascript

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/rmorlok/authproxy/internal/tools/render"
)

// bindInput validates and copies the captured context and optional response
// into native JavaScript values before any authored expression executes. The
// globals cannot be replaced, but nested mutations remain local to this VM.
// It returns fixed classifications rather than parser errors containing input.
func bindInput(
	ctx context.Context,
	vm *goja.Runtime,
	input Input,
	limits Limits,
) error {
	if ctx == nil || vm == nil || input.Context == nil {
		return ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	response := input.Response
	if response == nil {
		response = []byte("{}")
	}
	if len(response) >= limits.MaxInputBytes {
		return ErrLimitExceeded
	}

	// Share the serialized byte allowance with snapshot encoding; the response
	// consumes the same budget even when it contains only an empty object.
	snapshot, err := input.Context.SnapshotJSON(ctx, render.Limits{
		MaxBytes: limits.MaxInputBytes - len(response),
		MaxDepth: limits.MaxDepth,
		MaxSteps: limits.MaxNodes,
	})
	if err != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return ErrLimitExceeded
	}

	// Count values across both documents before JSON.parse constructs its
	// object graph. The snapshot encoder already bounds its own traversal.
	budget := inputBudget{ctx: ctx, limits: limits}
	if err := budget.validateObject(snapshot); err != nil {
		return err
	}
	if err := budget.validateObject(response); err != nil {
		return err
	}

	parse, ok := goja.AssertFunction(vm.Get("JSON").ToObject(vm).Get("parse"))
	if !ok {
		return ErrInvalidInput
	}

	root, err := parse(goja.Undefined(), vm.ToValue(string(snapshot)))
	if err != nil {
		return inputParseError(ctx)
	}

	responseValue, err := parse(goja.Undefined(), vm.ToValue(string(response)))
	if err != nil {
		return inputParseError(ctx)
	}

	rootObject := root.ToObject(vm)

	// A zero render.Context has no captured scopes. Only snapshots created by
	// render.NewContext supply every required object, including empty scopes.
	for _, name := range []string{
		"params",
		"cfg",
		"labels",
		"annotations",
		"connection",
		"tool",
	} {
		if _, ok := rootObject.Get(name).(*goja.Object); !ok {
			return ErrInvalidInput
		}
	}

	contextObject := vm.NewObject()
	for _, name := range []string{
		"cfg",
		"labels",
		"annotations",
		"connection",
		"tool",
	} {
		if err := contextObject.DefineDataProperty(
			name,
			rootObject.Get(name),
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
		); err != nil {
			return ErrInvalidInput
		}
	}

	// Each alias is the identical native object stored in context, not a new
	// wrapper or copy. No Go map, slice, struct, or function enters the VM.
	globals := []struct {
		name  string
		value goja.Value
	}{
		{"params", rootObject.Get("params")},
		{"context", contextObject},
		{"response", responseValue},
		{"cfg", contextObject.Get("cfg")},
		{"labels", contextObject.Get("labels")},
		{"annotations", contextObject.Get("annotations")},
	}

	for _, global := range globals {
		if err := vm.GlobalObject().DefineDataProperty(
			global.name,
			global.value,
			goja.FLAG_FALSE,
			goja.FLAG_FALSE,
			goja.FLAG_TRUE,
		); err != nil {
			return ErrInvalidInput
		}
	}

	return ctx.Err()
}

// inputParseError preserves cancellation while withholding native JSON parser
// diagnostics, which can contain private configuration keys or response text.
func inputParseError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrInvalidInput
}

// inputBudget validates the two encoded input documents with shared accounting.
// Each JSON value, including a container, and each object property name consumes
// one node. Document roots have depth zero; property names do not add depth.
type inputBudget struct {
	ctx    context.Context
	limits Limits
	nodes  int
}

// validateObject requires exactly one UTF-8 object document and validates its
// values without retaining a second decoded Go object graph.
func (b *inputBudget) validateObject(encoded []byte) error {
	if !utf8.Valid(encoded) {
		return ErrInvalidInput
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()

	if err := b.value(decoder, 0, true); err != nil {
		return err
	}

	if _, err := decoder.Token(); err != io.EOF {
		return ErrInvalidInput
	}

	if err := validateInputSurrogates(b.ctx, encoded); err != nil {
		return err
	}

	return b.ctx.Err()
}

// validateInputSurrogates rejects unpaired escaped UTF-16 surrogate units in
// already validated JSON strings and keys. Goja's native JSON.parse uses Go's
// JSON decoder, which would silently replace those units with U+FFFD. Rejecting
// them avoids changing input values; valid pairs and literal backslash-u text
// retain ordinary JSON behavior. This scan does not parse JSON structure.
func validateInputSurrogates(ctx context.Context, encoded []byte) error {
	inString := false
	for i := 0; i < len(encoded); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if encoded[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || encoded[i] != '\\' {
			continue
		}
		i++
		if encoded[i] != 'u' {
			continue
		}
		unit := inputHexUnit(encoded[i+1 : i+5])
		i += 4
		if unit >= 0xdc00 && unit <= 0xdfff {
			return ErrInvalidInput
		}
		if unit < 0xd800 || unit > 0xdbff {
			continue
		}
		if i+6 >= len(encoded) || encoded[i+1] != '\\' || encoded[i+2] != 'u' {
			return ErrInvalidInput
		}
		low := inputHexUnit(encoded[i+3 : i+7])
		if low < 0xdc00 || low > 0xdfff {
			return ErrInvalidInput
		}
		i += 6
	}
	return nil
}

// inputHexUnit reads four known hexadecimal digits from validated JSON syntax.
func inputHexUnit(digits []byte) uint16 {
	var value uint16
	for _, digit := range digits {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value += uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value += uint16(digit-'a') + 10
		default:
			value += uint16(digit-'A') + 10
		}
	}
	return value
}

// value consumes one JSON value, enforcing work/depth limits before descending.
// Finite JSON numbers use ordinary JavaScript Number precision when parsed.
func (b *inputBudget) value(
	decoder *json.Decoder,
	depth int,
	objectRoot bool,
) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if depth > b.limits.MaxDepth || b.nodes >= b.limits.MaxNodes {
		return ErrLimitExceeded
	}
	b.nodes++
	token, err := decoder.Token()
	if err != nil {
		return ErrInvalidInput
	}
	if objectRoot && token != json.Delim('{') {
		return ErrInvalidInput
	}

	switch token := token.(type) {
	case json.Number:
		number, err := token.Float64()
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return ErrInvalidInput
		}
	case json.Delim:
		if token != '{' && token != '[' {
			return ErrInvalidInput
		}
		for decoder.More() {
			if token == '{' {
				if err := b.ctx.Err(); err != nil {
					return err
				}
				if b.nodes >= b.limits.MaxNodes {
					return ErrLimitExceeded
				}
				b.nodes++
				key, err := decoder.Token()
				if _, ok := key.(string); err != nil || !ok {
					return ErrInvalidInput
				}
			}
			if err := b.value(decoder, depth+1, false); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || (token == '{' && closing != json.Delim('}')) || (token == '[' && closing != json.Delim(']')) {
			return ErrInvalidInput
		}
	}
	return nil
}
