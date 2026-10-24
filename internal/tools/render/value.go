package render

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/common"
)

// RenderValue renders a structured JSON value, preserving numbers and the types
// inserted by exact single-key $value directives. $literal copies its entire
// payload without interpreting templates or directives. Object keys are literal.
// Returned JSON owns its bytes and never aliases the context or input source.
func RenderValue(ctx context.Context, source common.RawJSON, data *Context, limits Limits) (common.RawJSON, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	if data == nil || ctx == nil {
		return nil, errors.New("render requires a context and invocation snapshot")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(source) > limits.MaxBytes {
		return nil, errors.New("render source byte limit exceeded")
	}
	if !utf8.Valid(source) {
		return nil, errors.New("render source must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("render source must be valid JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("render source must contain exactly one JSON value")
	}
	w := valueWriter{data: data, budget: newBudget(ctx, limits)}
	if err := w.emit(value, true, 0); err != nil {
		return nil, err
	}
	return common.RawJSON(w.output.Bytes()), nil
}

// valueWriter streams the JSON tree into a bounded buffer. Inserting the same
// large context value repeatedly cannot allocate an unbounded intermediate tree.
type valueWriter struct {
	data   *Context
	budget *budget
	output bytes.Buffer
}

// write charges serialized bytes before extending the output buffer.
func (w *valueWriter) write(text string) error {
	if err := w.budget.addBytes(len(text)); err != nil {
		return err
	}
	w.output.WriteString(text)
	return nil
}

// emit visits values with a shared work budget. render is false for injected
// values and literal escapes: data that resembles source stays inert data.
func (w *valueWriter) emit(value any, render bool, depth int) error {
	if err := w.budget.step(); err != nil {
		return err
	}
	if depth > w.budget.limit.MaxDepth {
		return errors.New("render depth limit exceeded")
	}
	switch v := value.(type) {
	case map[string]any:
		if render && len(v) == 1 {
			if reference, found := v["$value"]; found {
				name, ok := reference.(string)
				if !ok {
					return errors.New("$value requires a dotted reference string")
				}
				resolved, err := w.data.lookupRaw(strings.TrimSpace(name), nil)
				if err != nil {
					return err
				}
				return w.emit(resolved, false, depth)
			}
			if literal, found := v["$literal"]; found {
				return w.emit(literal, false, depth)
			}
		}
		if err := w.write("{"); err != nil {
			return err
		}
		// Stable ordering makes generated request bytes reproducible.
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i > 0 {
				if err := w.write(","); err != nil {
					return err
				}
			}
			if err := w.writeQuoted(key); err != nil {
				return err
			}
			if err := w.write(":"); err != nil {
				return err
			}
			if err := w.emit(v[key], render, depth+1); err != nil {
				return err
			}
		}
		return w.write("}")
	case []any:
		if err := w.write("["); err != nil {
			return err
		}
		for i, item := range v {
			if i > 0 {
				if err := w.write(","); err != nil {
					return err
				}
			}
			if err := w.emit(item, render, depth+1); err != nil {
				return err
			}
		}
		return w.write("]")
	case string:
		if render {
			template, err := Compile(v, w.budget.limit)
			if err != nil {
				return err
			}
			// Count template work globally, but charge its output only when it
			// is JSON-quoted below. Both intermediate and final bytes are capped.
			textBudget := *w.budget
			textBudget.bytes = 0
			v, err = template.renderWithBudget(w.data, &textBudget)
			w.budget.steps = textBudget.steps
			if err != nil {
				return err
			}
		}
		return w.writeQuoted(v)
	default:
		text, err := scalarText(v)
		if err != nil {
			return err
		}
		return w.write(text)
	}
}

// writeQuoted escapes JSON syntax while leaving ordinary HTML characters alone.
// The input is bounded by source/context/template limits before encoding.
func (w *valueWriter) writeQuoted(text string) error {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(text); err != nil {
		return errors.New("cannot encode rendered JSON string")
	}
	return w.write(strings.TrimSuffix(encoded.String(), "\n"))
}
