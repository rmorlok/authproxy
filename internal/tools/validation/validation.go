// Package validation checks Tool arguments and successful structured output
// against the native schemas captured with a definition revision.
package validation

import (
	"context"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/common"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

var (
	// ErrInvalidInput means arguments are not one JSON object matching the input schema.
	ErrInvalidInput = errors.New("tool arguments do not match the input schema")
	// ErrInvalidOutput means required structured content is absent or does not match its schema.
	ErrInvalidOutput = errors.New("tool structured output does not match the output schema")
	// ErrLimit means JSON data exceeds a configured byte, nesting, value, or number bound.
	ErrLimit = errors.New("tool schema validation data limit exceeded")
)

// maxNumberDigits caps rational expansion independently of caller policy. It
// stays below the exponents rejected by math/big, which the schema library
// assumes are representable when checking integers and numeric constraints.
const maxNumberDigits = 4096

// Limits bounds the JSON data prepared for one validation. All fields must be
// positive. These are data limits, not a count of JSON Schema evaluation steps
// or a hard memory/time quota for the underlying schema library.
type Limits struct {
	MaxInputBytes  int // Encoded argument bytes, including whitespace.
	MaxOutputBytes int // Encoded structured-content bytes, including whitespace.
	MaxDepth       int // Nested containers; a root object or array has depth one.
	MaxValues      int // JSON values, including containers; object keys are not values.
	// MaxNumberDigits bounds mantissa digits plus the absolute decimal exponent
	// and must not exceed 4096, the maximum rational expansion allowance.
	// This limits the rational-number expansion used by schema comparisons while
	// preserving the original json.Number spelling and precision.
	MaxNumberDigits int
}

// Program owns a compiled input schema and an optional compiled output schema.
// Compile once per definition revision and reuse concurrently. No invocation
// values or caller-owned schema bytes are retained between calls.
type Program struct {
	input  *jsonschema.Schema // Required object-rooted argument contract.
	output *jsonschema.Schema // Nil only when the definition omits outputSchema.
	limits Limits             // Effective caller-supplied data limits, copied at compile time.
}

// Compile applies the same schema policy as authored Tool definitions: JSON
// Schema 2020-12 with local/bundled references and no external resource loads.
// Empty outputSchema disables output checking; true and false remain schemas.
// Every limit must be positive, and MaxNumberDigits must not exceed 4096.
// Errors describe trusted definition problems and are not invocation failures.
func Compile(inputSchema, outputSchema common.RawJSON, limits Limits) (*Program, error) {
	if limits.MaxInputBytes <= 0 || limits.MaxOutputBytes <= 0 || limits.MaxDepth <= 0 ||
		limits.MaxValues <= 0 || limits.MaxNumberDigits <= 0 {
		return nil, errors.New("tool schema validation limits must all be positive")
	}
	if limits.MaxNumberDigits > maxNumberDigits {
		return nil, fmt.Errorf("tool schema validation number limit must not exceed %d", maxNumberDigits)
	}

	input, err := toolschema.CompileInputSchema(inputSchema)
	if err != nil {
		return nil, fmt.Errorf("input schema: %w", err)
	}
	var output *jsonschema.Schema
	if len(outputSchema) != 0 {
		output, err = toolschema.CompileOutputSchema(outputSchema)
		if err != nil {
			return nil, fmt.Errorf("output schema: %w", err)
		}
	}
	return &Program{input: input, output: output, limits: limits}, nil
}

// ValidateInput decodes and validates one argument object, returning a private
// JSON tree suitable for render.Input.Params. Numbers remain json.Number; no
// defaults, coercion, or removal of unknown fields occurs. Failures return no
// partial tree and never expose argument values or library validation details.
// The caller must not modify raw during this call.
func (p *Program) ValidateInput(ctx context.Context, raw common.RawJSON) (map[string]any, error) {
	value, err := decode(ctx, raw, p.limits.MaxInputBytes, p.limits)
	if err != nil {
		return nil, validationError(err, ErrInvalidInput)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, ErrInvalidInput
	}
	if err := validate(ctx, p.input, object, ErrInvalidInput); err != nil {
		return nil, err
	}
	return object, nil
}

// ValidateOutput checks successful structuredContent without changing its raw
// bytes. Callers must skip this method for Tool error results. Omitted schemas
// skip validation; any configured schema requires present JSON, so absent
// content differs from an explicit null even for the boolean schema true.
// Rich content blocks and the result envelope are checked by their producers.
func (p *Program) ValidateOutput(ctx context.Context, raw common.RawJSON) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.output == nil {
		return nil
	}
	value, err := decode(ctx, raw, p.limits.MaxOutputBytes, p.limits)
	if err != nil {
		return validationError(err, ErrInvalidOutput)
	}
	return validate(ctx, p.output, value, ErrInvalidOutput)
}

// validate checks cancellation around the synchronous library call and hides
// diagnostics that can quote private values. The library has no interrupt or
// work-budget hook; it is not run in a detached goroutine after cancellation.
func validate(ctx context.Context, schema *jsonschema.Schema, value any, invalid error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := schema.Validate(value)
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if err != nil {
		return invalid
	}
	return nil
}

// validationError preserves cancellation and data limits while replacing JSON
// decoder diagnostics, which may include private source text, with a fixed error.
func validationError(err, invalid error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrLimit) {
		return err
	}
	return invalid
}
