package runtime

import (
	"context"
	"encoding/json"

	"github.com/dop251/goja"
)

// ResultEncoder converts values selected by a trusted result extension into
// detached JSON or UTF-8 text. All conversions share one output byte and node
// allowance, including JSON punctuation and escaping for String results. Each
// selected value starts at depth zero. Any failure permanently poisons this
// encoder so an extension cannot discard a limit error and publish partial data.
//
// The runtime supplies one encoder per invocation after capturing pristine
// intrinsics, before authored initialization. It belongs to the VM's owner
// goroutine and must not escape the result callback or outlive the runtime.
// Returned bytes and strings are independently owned and may outlive the VM.
type ResultEncoder struct {
	// state retains conversion intrinsics and cumulative work charges, but drops
	// each result buffer once its conversion returns.
	state *outputState

	// err is the first conversion failure. Evaluation checks it even when the
	// trusted extension ignores a returned error.
	err error
}

// newResultEncoder captures intrinsics in a fresh runtime and creates the single
// output allowance shared by an invocation's result fields. The caller keeps
// the VM's interruption policy active until all conversions and cleanup finish.
func newResultEncoder(ctx context.Context, vm *goja.Runtime, limits Limits) *ResultEncoder {
	return &ResultEncoder{
		state: &outputState{
			encoder: newOutputEncoder(vm, limits),
			ctx:     ctx,
			active:  make(map[*goja.Object]bool),
		},
	}
}

// Encode converts one native value into independently owned JSON without
// invoking getters, proxy traps, coercion methods, or toJSON. Unsupported values,
// exhausted shared limits, and cancellation return no partial output and remain
// errors for every subsequent conversion on this encoder.
func (e *ResultEncoder) Encode(value goja.Value) (json.RawMessage, error) {
	if e.err != nil {
		return nil, e.err
	}

	// Drop the buffer reference on every exit. The cumulative byte counter and
	// node counter survive, but returned buffers are never reused or retained.
	defer func() { e.state.bytes = nil }()

	result, err := e.state.encode(value)
	if err != nil {
		e.err = err
		return nil, err
	}

	return result, nil
}

// String converts only a primitive JavaScript string into detached UTF-8 text.
// It charges the same bytes and nodes as Encode, including quotes and escapes,
// then rejects lone UTF-16 surrogate units that a Go string cannot preserve.
// Encode instead preserves those units as JSON escapes. No authored coercion is
// performed; objects and non-string primitives are invalid output.
func (e *ResultEncoder) String(value goja.Value) (string, error) {
	if e.err != nil {
		return "", e.err
	}

	text, ok := value.(goja.String)
	if !ok {
		e.err = ErrInvalidOutput
		return "", e.err
	}

	if _, err := e.Encode(text); err != nil {
		return "", err
	}
	for i := 0; i < text.Length(); i++ {
		if err := e.state.ctx.Err(); err != nil {
			e.err = err
			return "", err
		}

		unit := text.CharAt(i)
		if unit < 0xD800 || unit > 0xDFFF {
			continue
		}

		if unit <= 0xDBFF && i+1 < text.Length() {
			next := text.CharAt(i + 1)
			if next >= 0xDC00 && next <= 0xDFFF {
				i++
				continue
			}
		}

		e.err = ErrInvalidOutput

		return "", e.err
	}

	// Primitive conversion is bounded by the JSON already charged above. Check
	// cancellation again after conversion so late cancellation discards the text.
	result := text.String()

	if err := e.state.ctx.Err(); err != nil {
		e.err = err
		return "", err
	}

	return result, nil
}
