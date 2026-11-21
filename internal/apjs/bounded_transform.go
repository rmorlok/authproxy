package apjs

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// TransformOptionsWithContext evaluates a connector statement list and converts
// its final array into detached dropdown options. Source, copied native inputs,
// execution, and output obey explicit finite limits. The timeout also covers Go
// decoding, string conversion, and the final serialized option-array size.
// Callers must bound library source before compiling it and keep inputs stable.
//
// Each plain object must have own enumerable value and label data properties.
// Supported JSON values retain the legacy fmt.Sprint representation, including
// sorted map keys and <nil> for null or present undefined values. Missing fields
// are invalid. Getters, proxies, custom prototypes, cycles, and non-JSON values
// are rejected without authored coercion. Native input copying normalizes Go
// numeric types to JavaScript Numbers; see TransformJSONWithContext.
// Failures return no partial options and only safe runtime error categories.
func (c Context) TransformOptionsWithContext(
	ctx context.Context, expression string, limits jsruntime.Limits,
) ([]DataSourceOption, error) {
	return c.transformOptionsWithContext(ctx, expression, nil, false, limits)
}

// TransformJSONWithContext binds a private native copy of data under the global
// "data", then applies TransformOptionsWithContext's bounded conversion. Like
// other bounded connector evaluators, it copies JSON inputs instead of exposing
// reflected Go values. Consequently an integral Go float such as float64(1e6)
// becomes a JavaScript integer and formats as "1000000", even for identity
// transforms; legacy reflected identity transforms could format it as "1e+06".
func (c Context) TransformJSONWithContext(
	ctx context.Context, expression string, data any, limits jsruntime.Limits,
) ([]DataSourceOption, error) {
	return c.transformOptionsWithContext(ctx, expression, data, true, limits)
}

// transformOptionsWithContext owns the deadline through both VM evaluation and
// detached Go conversion. The VM's child timeout cannot extend this deadline.
// Adding data is also bounded, rather than copying an unchecked variable map.
func (c Context) transformOptionsWithContext(
	ctx context.Context, expression string, data any, bindData bool, limits jsruntime.Limits,
) (options []DataSourceOption, err error) {
	if ctx == nil {
		return nil, jsruntime.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, limits.Timeout)
	phase := "input"
	defer func() {
		defer cancel()
		diagnostic := FailureDiagnostic{Phase: phase}
		if existing, ok := Diagnostic(err); ok {
			diagnostic = existing
		}
		// A cancellation during detached conversion still invalidates all output;
		// caller cancellation wins over the adapter's own policy deadline.
		// Observe the child first: if its cancellation was inherited, the parent
		// must already be canceled by the time we select the final category.
		deadlineErr := runCtx.Err()
		if contextErr := ctx.Err(); contextErr != nil {
			err = contextErr
			diagnostic.Source, diagnostic.Line, diagnostic.Column = "", 0, 0
		} else if deadlineErr != nil {
			err = jsruntime.ErrLimitExceeded
			diagnostic.Source, diagnostic.Line, diagnostic.Column = "", 0, 0
		}
		if err != nil {
			options = nil
			err = withFailureDiagnostic(err, diagnostic)
		}
	}()

	if bindData {
		if len(c.vars) > limits.MaxNodes {
			return nil, jsruntime.ErrLimitExceeded
		}
		vars := make(map[string]any, len(c.vars))
		for name, value := range c.vars {
			if err := runCtx.Err(); err != nil {
				return nil, err
			}
			vars[name] = value
		}
		vars["data"] = data
		c.vars = vars
	}

	encoded, err := c.evaluateJSONWithContext(
		runCtx, expression, limits, jsruntime.JSONOutputOptions{UndefinedAsNull: true},
		func(_ context.Context, _ *goja.Runtime, value goja.Value) (goja.Value, error) {
			return value, nil
		},
	)
	if err != nil {
		return nil, err
	}
	phase = "output"
	return dataSourceOptionsFromJSON(runCtx, encoded, limits)
}

// dataSourceOptionsFromJSON consumes only the shared encoder's bounded JSON.
// Decode cannot be interrupted midway, but bytes, depth, and nodes were already
// admitted and cancellation is checked around it. UseNumber retains the encoder's
// integer/float spelling instead of rounding integers through Go float64.
func dataSourceOptionsFromJSON(
	ctx context.Context, encoded json.RawMessage, limits jsruntime.Limits,
) ([]DataSourceOption, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, jsruntime.ErrInvalidOutput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok {
		return nil, jsruntime.ErrInvalidOutput
	}

	// Reuse the bounded Go JSON writer for exact final response accounting,
	// including field names, punctuation, quotes, HTML escaping, and Unicode.
	final := variableEncoder{ctx: ctx, limits: jsruntime.JSONLimits{
		MaxBytes: limits.MaxOutputBytes, MaxDepth: limits.MaxDepth, MaxNodes: limits.MaxNodes,
	}}
	if err := final.node(); err != nil {
		return nil, err
	}
	if err := final.append([]byte("[")); err != nil {
		return nil, err
	}
	options := make([]DataSourceOption, 0, len(items))
	formatter := optionFormatter{ctx: ctx, remaining: limits.MaxOutputBytes}
	for i, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, jsruntime.ErrInvalidOutput
		}
		value, hasValue := object["value"]
		label, hasLabel := object["label"]
		if !hasValue || !hasLabel {
			return nil, jsruntime.ErrInvalidOutput
		}
		var option DataSourceOption
		var err error
		if option.Value, err = formatter.format(value); err != nil {
			return nil, err
		}
		if option.Label, err = formatter.format(label); err != nil {
			return nil, err
		}
		if i != 0 {
			if err := final.append([]byte(",")); err != nil {
				return nil, err
			}
		}
		if err := final.value(map[string]string{"value": option.Value, "label": option.Label}, 1); err != nil {
			return nil, err
		}
		options = append(options, option)
	}
	if err := final.append([]byte("]")); err != nil {
		return nil, err
	}
	return options, nil
}

// optionFormatter reproduces fmt.Sprint for the encoder's plain JSON data,
// without calling authored methods or building an unchecked intermediate string.
// The input graph is already bounded and acyclic. All option fields share the
// raw text allowance; final JSON escaping is separately checked before return.
type optionFormatter struct {
	ctx       context.Context // Overall evaluation deadline and caller cancellation.
	remaining int             // Raw UTF-8 bytes still available across value/label strings.
}

// format returns one owned field string, charging each append before allocation.
func (f *optionFormatter) format(value any) (string, error) {
	var text strings.Builder
	if err := f.value(&text, value); err != nil {
		return "", err
	}
	return text.String(), nil
}

// value formats only native JSON values, checking cancellation while walking
// containers. Map sorting matches fmt's lexical Go string-key order; its work
// is bounded by the encoder's key/node caps and checked again before emission.
func (f *optionFormatter) value(text *strings.Builder, value any) error {
	if err := f.ctx.Err(); err != nil {
		return err
	}
	switch value := value.(type) {
	case nil:
		return f.append(text, "<nil>")
	case string:
		return f.append(text, value)
	case json.Number:
		return f.append(text, string(value))
	case bool:
		return f.append(text, strconv.FormatBool(value))
	case []any:
		if err := f.append(text, "["); err != nil {
			return err
		}
		for i, item := range value {
			if i != 0 {
				if err := f.append(text, " "); err != nil {
					return err
				}
			}
			if err := f.value(text, item); err != nil {
				return err
			}
		}
		return f.append(text, "]")
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			if err := f.ctx.Err(); err != nil {
				return err
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if err := f.append(text, "map["); err != nil {
			return err
		}
		for i, key := range keys {
			if i != 0 {
				if err := f.append(text, " "); err != nil {
					return err
				}
			}
			if err := f.append(text, key); err != nil {
				return err
			}
			if err := f.append(text, ":"); err != nil {
				return err
			}
			if err := f.value(text, value[key]); err != nil {
				return err
			}
		}
		return f.append(text, "]")
	default:
		return jsruntime.ErrInvalidOutput
	}
}

// append enforces the shared field-text cap before growing a result string.
func (f *optionFormatter) append(text *strings.Builder, value string) error {
	if err := f.ctx.Err(); err != nil {
		return err
	}
	if len(value) > f.remaining {
		return jsruntime.ErrLimitExceeded
	}
	f.remaining -= len(value)
	text.WriteString(value)
	return nil
}
