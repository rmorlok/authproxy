package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"unicode/utf8"

	"github.com/dop251/goja"
)

// JSONLimits bounds one batch of native JSON conversion. Initial inputs share
// one batch; independently bounded HTTP bodies each receive a fresh allowance.
// All fields must be positive. These caps do not limit the VM's heap usage.
type JSONLimits struct {
	// MaxBytes bounds the combined serialized documents, including whitespace.
	MaxBytes int

	// MaxDepth bounds value nesting, with each document root at depth zero.
	MaxDepth int

	// MaxNodes counts JSON values (including containers) and object property
	// names across every document. Property names do not add nesting depth.
	MaxNodes int
}

// JSONDecoder copies validated JSON into native values in one owned runtime.
// It retains the original JSON.parse so later authored changes to globals cannot
// replace conversion. It and its returned values belong to the VM goroutine and
// must never escape the owning Run or EvaluateJSON callback.
type JSONDecoder struct {
	// vm creates the native strings passed to this runtime's captured parser.
	vm *goja.Runtime

	// parse is captured before any authored code or library initialization.
	parse goja.Callable
}

// NewJSONDecoder captures JSON.parse from a fresh runtime. Trusted adapters must
// call it before authored code can change the standard globals. It installs no
// globals or capabilities and returns ErrInvalidInput for missing arguments or
// parser bindings; it does not verify the identity of a supplied callable.
func NewJSONDecoder(vm *goja.Runtime) (*JSONDecoder, error) {
	if vm == nil {
		return nil, ErrInvalidInput
	}
	object, ok := vm.Get("JSON").(*goja.Object)
	if !ok {
		return nil, ErrInvalidInput
	}
	parse, ok := goja.AssertFunction(object.Get("parse"))
	if !ok {
		return nil, ErrInvalidInput
	}
	return &JSONDecoder{vm: vm, parse: parse}, nil
}

// Decode validates a complete batch before parsing any document. Values and
// object keys share the node allowance; serialized bytes share the byte cap.
// Every JSON root kind is accepted. Nil/empty documents are invalid rather than
// defaulting to an object; domain adapters choose their own defaults and shapes.
// Ordinary JavaScript Number precision applies, including integer rounding.
//
// Callers must keep document bytes stable until Decode returns and pass all
// initial inputs in one call so their allowances are not reset. Each subsequent
// call starts fresh counters, suitable for a separately bounded response body.
// Call within Run/EvaluateJSON using its runCtx to share the execution deadline;
// this helper does not start a timeout or interrupt watcher. Failures return
// safe classifications or ctx.Err() and no partial values.
func (d *JSONDecoder) Decode(
	ctx context.Context,
	limits JSONLimits,
	documents ...json.RawMessage,
) ([]goja.Value, error) {
	if d == nil || d.vm == nil || d.parse == nil || ctx == nil ||
		limits.MaxBytes <= 0 || limits.MaxDepth <= 0 || limits.MaxNodes <= 0 {
		return nil, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(documents) > limits.MaxNodes {
		return nil, ErrLimitExceeded
	}

	// Charge bytes before parsing or allocating values. Subtraction avoids
	// integer overflow when callers supply many individually valid documents.
	remaining := limits.MaxBytes
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(document) > remaining {
			return nil, ErrLimitExceeded
		}
		remaining -= len(document)
	}
	budget := inputBudget{ctx: ctx, limits: limits}
	for _, document := range documents {
		if err := budget.validate(document); err != nil {
			return nil, err
		}
	}

	// Validation bounds the slice length too: every document consumes a node.
	values := make([]goja.Value, len(documents))
	for i, document := range documents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value, err := d.parse(goja.Undefined(), d.vm.ToValue(string(document)))
		if err != nil {
			return nil, inputParseError(ctx)
		}
		values[i] = value
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

// inputParseError preserves cancellation while withholding native JSON parser
// diagnostics, which can contain private configuration keys or response text.
func inputParseError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrInvalidInput
}

// inputBudget validates a batch of encoded JSON documents with shared accounting.
// Each JSON value, including a container, and each object property name consumes
// one node. Document roots have depth zero; property names do not add depth.
type inputBudget struct {
	ctx    context.Context // Evaluation deadline and caller cancellation.
	limits JSONLimits      // Bounds the whole batch, with each root at depth zero.
	nodes  int             // Values and property names visited across all documents.
}

// validate requires exactly one UTF-8 JSON document and validates its
// values without retaining a second decoded Go object graph.
func (b *inputBudget) validate(encoded []byte) error {
	if !utf8.Valid(encoded) {
		return ErrInvalidInput
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()

	if err := b.value(decoder, 0); err != nil {
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
			if err := b.value(decoder, depth+1); err != nil {
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
