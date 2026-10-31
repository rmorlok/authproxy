package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// jsonReader constructs a private JSON tree with limits checked before each
// value/container allocation. A reader belongs to one validation call.
type jsonReader struct {
	decoder *json.Decoder   // Token decoder preserves number spelling with UseNumber.
	ctx     context.Context // Cancellation is checked between tokens.
	limits  Limits          // Depth, value count, and numeric expansion bounds.
	values  int             // Values already charged, including open containers.
}

// decode reads exactly one JSON value. Its byte bound applies before decoding;
// token traversal then bounds container depth and the number of allocated values.
func decode(ctx context.Context, raw []byte, maxBytes int, limits Limits) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(raw) > maxBytes {
		return nil, ErrLimit
	}
	if !utf8.Valid(raw) {
		return nil, errors.New("invalid JSON encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	r := jsonReader{decoder: decoder, ctx: ctx, limits: limits}
	value, err := r.read(0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("expected one JSON value")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return value, nil
}

// read charges a value before decoding it. depth counts its enclosing
// containers, so scalar children do not add a level of container nesting.
func (r *jsonReader) read(depth int) (any, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if r.values >= r.limits.MaxValues {
		return nil, ErrLimit
	}
	r.values++
	token, err := r.decoder.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case json.Delim:
		if depth >= r.limits.MaxDepth {
			return nil, ErrLimit
		}
		switch value {
		case '{':
			object := make(map[string]any)
			for r.decoder.More() {
				key, err := r.decoder.Token()
				if err != nil {
					return nil, err
				}
				name, ok := key.(string)
				if !ok {
					return nil, errors.New("expected object key")
				}
				// Reject duplicates instead of silently overwriting an argument.
				if _, exists := object[name]; exists {
					return nil, errors.New("duplicate object key")
				}
				child, err := r.read(depth + 1)
				if err != nil {
					return nil, err
				}
				object[name] = child
			}
			if _, err := r.decoder.Token(); err != nil {
				return nil, err
			}
			return object, nil
		case '[':
			array := make([]any, 0)
			for r.decoder.More() {
				child, err := r.read(depth + 1)
				if err != nil {
					return nil, err
				}
				array = append(array, child)
			}
			if _, err := r.decoder.Token(); err != nil {
				return nil, err
			}
			return array, nil
		default:
			return nil, errors.New("unexpected JSON delimiter")
		}
	case json.Number:
		if !numberWithinLimit(string(value), r.limits.MaxNumberDigits) {
			return nil, ErrLimit
		}
	}
	return token, nil
}

// numberWithinLimit bounds rational expansion without converting the number.
// A short token such as 1e999999999 must not reach the library's big.Rat parser.
// The decoder has already checked JSON number syntax, including exponent signs.
func numberWithinLimit(number string, limit int) bool {
	mantissa, exponent := number, ""
	if index := strings.IndexAny(number, "eE"); index >= 0 {
		mantissa, exponent = number[:index], number[index+1:]
	}
	for _, c := range mantissa {
		if c >= '0' && c <= '9' {
			if limit == 0 {
				return false
			}
			limit--
		}
	}
	exponent = strings.TrimLeft(exponent, "+-")
	// Check before multiplying so arbitrary-length exponents cannot overflow.
	magnitude := 0
	for _, c := range exponent {
		digit := int(c - '0')
		if magnitude > limit/10 || (magnitude == limit/10 && digit > limit%10) {
			return false
		}
		magnitude = magnitude*10 + digit
	}
	return true
}
