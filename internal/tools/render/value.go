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

// ValueTemplate is an immutable structured JSON program. Its string templates
// are parsed once, and its directive forms are validated before invocation.
// Independent renders may reuse a ValueTemplate concurrently.
type ValueTemplate struct {
	root   any
	limits Limits
}

// valueReference marks a compiled $value directive. This private type cannot
// occur in decoded JSON or invocation data, so inserted values remain inert.
type valueReference string

// CompileValue parses structured JSON and all embedded string templates under
// one compilation budget. $value preserves referenced types; $literal leaves
// its payload uninterpreted. Other $-prefixed source keys are rejected to catch
// malformed directives. Source bytes are not retained by the returned program.
func CompileValue(source common.RawJSON, limits Limits) (*ValueTemplate, error) {
	return compileValue(source, newBudget(context.Background(), limits))
}

// compileValue shares accounting across JSON nodes and embedded Mustache syntax.
// The caller chooses whether compilation uses an invocation cancellation context.
func compileValue(source common.RawJSON, b *budget) (*ValueTemplate, error) {
	if err := b.limit.validate(); err != nil {
		return nil, err
	}
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	if len(source) > b.limit.MaxBytes {
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

	root, err := compileValueNode(value, true, b, 0)
	if err != nil {
		return nil, err
	}
	return &ValueTemplate{root: root, limits: b.limit}, nil
}

// compileValueNode replaces authored strings and directives with private
// executable nodes. Literal payloads are still traversed to enforce limits,
// but their keys and strings have no template meaning.
func compileValueNode(value any, interpret bool, b *budget, depth int) (any, error) {
	if err := b.step(); err != nil {
		return nil, err
	}
	if depth > b.limit.MaxDepth {
		return nil, errors.New("render depth limit exceeded")
	}
	switch v := value.(type) {
	case map[string]any:
		if interpret {
			if len(v) == 1 {
				if reference, found := v["$value"]; found {
					name, ok := reference.(string)
					if !ok {
						return nil, errors.New("$value requires a dotted reference string")
					}
					name = strings.TrimSpace(name)
					if name != "." {
						for _, part := range strings.Split(name, ".") {
							if part == "" {
								return nil, errors.New("$value requires a nonempty dotted reference")
							}
						}
					}
					if encoding, _ := encodingReference(name); encoding != "" {
						return nil, errors.New("$value requires a plain reference without an encoding view")
					}
					return valueReference(name), nil
				}
				if literal, found := v["$literal"]; found {
					return compileValueNode(literal, false, b, depth)
				}
			}
			for key := range v {
				if strings.HasPrefix(key, "$") {
					return nil, errors.New("reserved $-prefixed keys require an exact $value or $literal directive; wrap literal data in $literal")
				}
			}
		}
		for key, child := range v {
			compiled, err := compileValueNode(child, interpret, b, depth+1)
			if err != nil {
				return nil, err
			}
			v[key] = compiled
		}
		return v, nil
	case []any:
		for i, child := range v {
			compiled, err := compileValueNode(child, interpret, b, depth+1)
			if err != nil {
				return nil, err
			}
			v[i] = compiled
		}
		return v, nil
	case string:
		if interpret {
			return compileTemplate(v, b)
		}
	}
	return value, nil
}

// Render evaluates a compiled JSON program against an invocation snapshot.
// Returned JSON owns its bytes; failures return no partial body.
func (v *ValueTemplate) Render(ctx context.Context, data *Context) (common.RawJSON, error) {
	if v == nil || data == nil || ctx == nil {
		return nil, errors.New("value template, context, and invocation data are required")
	}
	return v.renderWithBudget(data, newBudget(ctx, v.limits))
}

// renderWithBudget emits the already compiled program using caller-owned limits.
func (v *ValueTemplate) renderWithBudget(data *Context, b *budget) (common.RawJSON, error) {
	w := valueWriter{data: data, budget: b}
	if err := w.emit(v.root, 0); err != nil {
		return nil, err
	}
	return common.RawJSON(w.output.Bytes()), nil
}

// RenderValue compiles and renders a one-off structured value. Compilation and
// rendering share its work budget. Repeated invocations should use CompileValue
// once and ValueTemplate.Render for each invocation instead.
func RenderValue(
	ctx context.Context,
	source common.RawJSON,
	data *Context,
	limits Limits,
) (common.RawJSON, error) {
	if data == nil || ctx == nil {
		return nil, errors.New("render requires a context and invocation snapshot")
	}
	b := newBudget(ctx, limits)
	v, err := compileValue(source, b)
	if err != nil {
		return nil, err
	}
	return v.renderWithBudget(data, b)
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

// emit visits compiled nodes and inert JSON data with a shared work budget.
// Only private compiled node types execute; strings and object keys stay literal.
func (w *valueWriter) emit(value any, depth int) error {
	if err := w.budget.step(); err != nil {
		return err
	}

	if depth > w.budget.limit.MaxDepth {
		return errors.New("render depth limit exceeded")
	}

	switch v := value.(type) {
	case valueReference:
		resolved, err := w.data.lookupRaw(string(v), nil)
		if err != nil {
			return err
		}
		return w.emit(resolved, depth)
	case *Template:
		// Count template work globally, but charge its output only when it
		// is JSON-quoted below. Both intermediate and final bytes are capped.
		textBudget := *w.budget
		textBudget.bytes = 0
		text, err := v.renderWithBudget(w.data, &textBudget)
		w.budget.steps = textBudget.steps
		if err != nil {
			return err
		}
		return w.writeQuoted(text)
	case map[string]any:
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

			if err := w.emit(v[key], depth+1); err != nil {
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
			if err := w.emit(item, depth+1); err != nil {
				return err
			}
		}
		return w.write("]")
	case string:
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
