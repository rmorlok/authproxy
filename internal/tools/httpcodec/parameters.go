// Package httpcodec encodes rendered Tool request data without performing I/O.
// Its codecs are shared by authored HTTP Tools and generated request plans.
package httpcodec

import (
	"context"
	"errors"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/render"
)

// Parameters is an immutable query/form template program. Field names stay
// literal, while each value supports the Tool renderer's string templates and
// typed directives. A whole-field $optional reference omits missing/null values.
// A program can be reused concurrently across invocations.
type Parameters struct {
	names    []string
	template *render.ValueTemplate
	limits   render.Limits
}

// CompileParameters captures field names and compiles their values under one
// shared rendering budget. MaxBytes separately bounds the combined source
// (including names), intermediate JSON, and final encoded output. Escaping and
// repeated names can make the encoded output larger than the JSON input, so
// callers must size MaxBytes for both representations. MaxSteps and MaxDepth
// apply to the complete value program. All limits must be positive. Final
// scalar/repeated-scalar shape validation happens after rendering, because a
// $value reference can resolve to any JSON type.
func CompileParameters(fields map[string]common.RawJSON, limits render.Limits) (*Parameters, error) {
	if limits.MaxBytes <= 0 || limits.MaxDepth <= 0 || limits.MaxSteps <= 0 {
		return nil, errors.New("parameter rendering limits must all be positive")
	}
	if len(fields) > limits.MaxSteps {
		return nil, errors.New("parameter field count exceeds render step limit")
	}
	names, source, err := parameterSource(context.Background(), fields, limits.MaxBytes)
	if err != nil {
		return nil, err
	}

	// An array lets literal field names such as "$value" remain ordinary names.
	// One program also shares work/output accounting across every field value.
	program, err := render.CompileParameterValues(source, limits)
	if err != nil {
		return nil, err
	}
	return &Parameters{names: names, template: program, limits: limits}, nil
}

// Render resolves every value, checks its shape, and returns query/form text.
// Keys are sorted; arrays repeat the key in input order, and empty arrays emit
// nothing. An exact whole-field $optional lookup omits missing or null values;
// ordinary $value references remain strict, and ordinary null becomes "null".
// Strings are encoded once and must not be pre-escaped. Errors return no partial
// result and do not include resolved values. No request is sent by this method.
func (p *Parameters) Render(ctx context.Context, data *render.Context) (string, error) {
	if p == nil {
		return "", errors.New("parameter program is required")
	}

	values, err := p.template.Render(ctx, data)
	if err != nil {
		return "", err
	}

	return encodeParameterValues(ctx, p.names, values, p.limits.MaxBytes)
}
