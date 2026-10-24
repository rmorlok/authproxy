package httpcodec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rmorlok/authproxy/internal/schema/common"
)

// EncodeParameters encodes already-resolved JSON fields for a URL query or an
// application/x-www-form-urlencoded body. Unlike CompileParameters, it never
// interprets templates or directives. Values must be scalars or arrays of
// scalars; objects and nested arrays are rejected. Numbers retain their JSON
// spelling, null becomes "null", and an empty array emits no pairs. Key order is
// deterministic and repeated values retain their order. Names must be nonempty.
//
// maxBytes must be positive and bounds both aggregate input (names plus JSON
// values framed as an array) and encoded output. Parsing and temporary escapes
// may allocate proportional to this limit; this is not a hard memory quota.
// Cancellation or any error returns an empty string without echoing field data.
func EncodeParameters(ctx context.Context, fields map[string]common.RawJSON, maxBytes int) (string, error) {
	names, source, err := parameterSource(ctx, fields, maxBytes)
	if err != nil {
		return "", err
	}
	return encodeParameterValues(ctx, names, source, maxBytes)
}

// parameterSource orders literal names and copies their independently valid
// JSON values into an array. Counting names and array framing before allocating
// keeps many small fields within the same source budget as one large field.
func parameterSource(ctx context.Context, fields map[string]common.RawJSON, maxBytes int) ([]string, common.RawJSON, error) {
	if ctx == nil {
		return nil, nil, errors.New("parameter encoding context is required")
	}
	if maxBytes <= 0 {
		return nil, nil, errors.New("parameter byte limit must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	// Every valid field needs at least one JSON byte and an array separator.
	if maxBytes < 2 || len(fields) > maxBytes {
		return nil, nil, errors.New("parameter source byte limit exceeded")
	}
	remaining := maxBytes - 2 // The array's opening and closing brackets.
	for name, value := range fields {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if len(name) > remaining {
			return nil, nil, errors.New("parameter source byte limit exceeded")
		}
		remaining -= len(name)
		if len(value) > remaining {
			return nil, nil, errors.New("parameter source byte limit exceeded")
		}
		remaining -= len(value)
	}
	if len(fields) > 0 && len(fields)-1 > remaining {
		return nil, nil, errors.New("parameter source byte limit exceeded")
	}

	names := make([]string, 0, len(fields))
	for name, value := range fields {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if name == "" {
			return nil, nil, errors.New("parameter names must be nonempty")
		}
		if !utf8.ValidString(name) || !utf8.Valid(value) || !json.Valid(value) {
			return nil, nil, errors.New("parameters require UTF-8 names and valid UTF-8 JSON values")
		}
		names = append(names, name)
	}
	sort.Strings(names)

	source := common.RawJSON{'['}
	for i, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if i > 0 {
			source = append(source, ',')
		}
		source = append(source, fields[name]...)
	}
	source = append(source, ']')
	return names, source, nil
}

// encodeParameterValues decodes a bounded array produced by parameterSource
// or the compiled renderer. Its entries correspond exactly to names. UseNumber
// keeps large integers and authored decimal/exponent spellings intact.
func encodeParameterValues(ctx context.Context, names []string, source common.RawJSON, maxBytes int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	var values []any
	if err := decoder.Decode(&values); err != nil {
		return "", errors.New("parameter values must be valid JSON")
	}
	// Both callers preserve one value per name; reject a broken internal plan
	// before indexing or silently discarding any rendered values.
	if len(values) != len(names) {
		return "", errors.New("parameter names and values must have matching lengths")
	}

	var result strings.Builder
	for i, name := range names {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		key := url.QueryEscape(name)
		if repeated, ok := values[i].([]any); ok {
			for _, value := range repeated {
				if err := appendParameter(ctx, &result, key, value, maxBytes); err != nil {
					return "", err
				}
			}
		} else if err := appendParameter(ctx, &result, key, values[i], maxBytes); err != nil {
			return "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return result.String(), nil
}

// appendParameter writes one scalar pair after checking its complete escaped
// size, including the equals sign and any separator. Keys are escaped once per
// field; values are escaped once per occurrence. It never flattens nested arrays.
func appendParameter(ctx context.Context, result *strings.Builder, key string, value any, maxBytes int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var text string
	switch value := value.(type) {
	case nil:
		text = "null"
	case string:
		text = value
	case bool:
		text = strconv.FormatBool(value)
	case json.Number:
		text = string(value)
	default:
		return errors.New("parameter values must be scalars or arrays of scalars")
	}
	encoded := url.QueryEscape(text)
	remaining := maxBytes - result.Len()
	// Subtract each component separately to avoid overflowing a length sum.
	for _, n := range []int{len(key), len(encoded), 1} {
		if n > remaining {
			return errors.New("encoded parameter byte limit exceeded")
		}
		remaining -= n
	}
	if result.Len() > 0 {
		if remaining < 1 {
			return errors.New("encoded parameter byte limit exceeded")
		}
		result.WriteByte('&')
	}
	result.WriteString(key)
	result.WriteByte('=')
	result.WriteString(encoded)
	return nil
}
