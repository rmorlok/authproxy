package config

import (
	"math"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// Tools contains operator policy for Tool execution. It does not define Tool
// resources or grant permission to invoke them.
type Tools struct {
	// Invocation configures the ceilings shared by Tool invocations. Omitted
	// fields use runtime defaults; authored Tool limits can only lower them.
	Invocation *ToolInvocationLimits `json:"invocation,omitempty" yaml:"invocation,omitempty"`
}

// ToolInvocationLimits configures independent execution and processing ceilings.
// Nil fields select runtime defaults; explicitly configured limits must be
// positive. Data boundaries remain separate; the runtime derives shared native
// processing budgets from these settings. These caps bound data processing and
// interruptible work, not total process memory or the underlying JSON Schema
// evaluator's work.
type ToolInvocationLimits struct {
	// Timeout spans preparation and execution after authorization.
	Timeout *HumanDuration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	// MaxRequests counts logical outbound requests. Existing proxy credential
	// recovery and redirects may make additional transport attempts.
	MaxRequests *int `json:"maxRequests,omitempty" yaml:"maxRequests,omitempty"`
	// MaxRequestBytes bounds one JavaScript request's URL, method, headers, body,
	// and label overrides before admitting an HTTP worker.
	MaxRequestBytes *HumanByteSize `json:"maxRequestBytes,omitempty" yaml:"maxRequestBytes,omitempty"`
	// MaxPendingOperations bounds outstanding JavaScript fetch and response-read
	// workers, including completions the VM has not consumed yet.
	MaxPendingOperations *int `json:"maxPendingOperations,omitempty" yaml:"maxPendingOperations,omitempty"`
	// MaxInputBytes bounds the caller's encoded argument JSON.
	MaxInputBytes *HumanByteSize `json:"maxInputBytes,omitempty" yaml:"maxInputBytes,omitempty"`
	// MaxResponseBytes bounds aggregate decoded upstream response bytes.
	MaxResponseBytes *HumanByteSize `json:"maxResponseBytes,omitempty" yaml:"maxResponseBytes,omitempty"`
	// MaxOutputBytes bounds successful content before result envelope encoding.
	// JavaScript also applies it to rich results, complete ToolError payloads,
	// and each authored console record when logging is enabled.
	MaxOutputBytes *HumanByteSize `json:"maxOutputBytes,omitempty" yaml:"maxOutputBytes,omitempty"`
	// MaxEncodedResultBytes includes result identities, metadata, JSON escaping,
	// and base64 expansion; content within MaxOutputBytes can still exceed it.
	MaxEncodedResultBytes *HumanByteSize `json:"maxEncodedResultBytes,omitempty" yaml:"maxEncodedResultBytes,omitempty"`
	// MaxStackDepth bounds function call depth in JavaScript Tools and response
	// transforms, including recursion.
	MaxStackDepth *int `json:"maxStackDepth,omitempty" yaml:"maxStackDepth,omitempty"`
	// MaxJavascriptSourceBytes bounds a JavaScript Tool's source together with
	// its selected connector helper library, before either source is parsed.
	MaxJavascriptSourceBytes *HumanByteSize `json:"maxJavascriptSourceBytes,omitempty" yaml:"maxJavascriptSourceBytes,omitempty"`
	// MaxJavascriptInputBytes bounds the combined serialized arguments, captured
	// Tool context, and sole connection supplied to a JavaScript Tool's VM.
	// It is independent of MaxInputBytes, which applies only to caller arguments.
	MaxJavascriptInputBytes *HumanByteSize `json:"maxJavascriptInputBytes,omitempty" yaml:"maxJavascriptInputBytes,omitempty"`
	// MaxRenderBytes bounds each snapshot copy, template compilation, and complete
	// request render, including connection configuration as well as arguments.
	MaxRenderBytes *HumanByteSize `json:"maxRenderBytes,omitempty" yaml:"maxRenderBytes,omitempty"`
	// MaxTransformInputBytes bounds the combined serialized snapshot and response
	// supplied to a response transform, including serialization expansion.
	MaxTransformInputBytes *HumanByteSize `json:"maxTransformInputBytes,omitempty" yaml:"maxTransformInputBytes,omitempty"`
	// MaxResponseStreamBytes bounds each response before Tool-host decoding.
	// The HTTP transport may already have decompressed it, so this is not a wire limit.
	MaxResponseStreamBytes *HumanByteSize `json:"maxResponseStreamBytes,omitempty" yaml:"maxResponseStreamBytes,omitempty"`
	// MaxResponseHeaderBytes bounds each copied response header map, including
	// names, values, and line overhead.
	MaxResponseHeaderBytes *HumanByteSize `json:"maxResponseHeaderBytes,omitempty" yaml:"maxResponseHeaderBytes,omitempty"`
	// MaxResponseSourceBytes bounds response rule metadata and transform source.
	// Request template source is instead covered by MaxRenderBytes.
	MaxResponseSourceBytes *HumanByteSize `json:"maxResponseSourceBytes,omitempty" yaml:"maxResponseSourceBytes,omitempty"`
	// MaxDepth bounds value nesting during validation, rendering, and JavaScript.
	// Each processor retains its own root-depth convention.
	MaxDepth *int `json:"maxDepth,omitempty" yaml:"maxDepth,omitempty"`
	// MaxValues bounds JSON values during validation and value/property inspections
	// in JavaScript Tools and transforms. It does not count JSON Schema operations.
	MaxValues *int `json:"maxValues,omitempty" yaml:"maxValues,omitempty"`
	// MaxRenderSteps bounds visited values and template operations, including
	// repeated section bodies, in each render operation.
	MaxRenderSteps *int `json:"maxRenderSteps,omitempty" yaml:"maxRenderSteps,omitempty"`
	// MaxNumberDigits bounds mantissa digits plus absolute decimal exponent for
	// schema validation's number processing. It cannot exceed 4096.
	MaxNumberDigits *int `json:"maxNumberDigits,omitempty" yaml:"maxNumberDigits,omitempty"`
}

// Validate checks the optional Tool policy without filling in runtime defaults.
// Errors identify invalid fields beneath vc; a nil receiver is valid.
func (t *Tools) Validate(vc *common.ValidationContext) error {
	if t == nil {
		return nil
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}
	return t.Invocation.Validate(vc.PushField("invocation"))
}

// Validate rejects explicit zero, negative, or unrepresentable limits while
// preserving nil values for default resolution by the runtime. Byte allowances
// must fit an int because the execution processors use int-sized lengths.
func (l *ToolInvocationLimits) Validate(vc *common.ValidationContext) error {
	if l == nil {
		return nil
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	var result *multierror.Error

	if l.Timeout != nil && l.Timeout.Duration <= 0 {
		result = multierror.Append(result, vc.NewErrorForField("timeout", "must be positive"))
	}

	for _, field := range []struct {
		name  string
		value *HumanByteSize
	}{
		{"maxRequestBytes", l.MaxRequestBytes},
		{"maxInputBytes", l.MaxInputBytes},
		{"maxResponseBytes", l.MaxResponseBytes},
		{"maxOutputBytes", l.MaxOutputBytes},
		{"maxEncodedResultBytes", l.MaxEncodedResultBytes},
		{"maxJavascriptSourceBytes", l.MaxJavascriptSourceBytes},
		{"maxJavascriptInputBytes", l.MaxJavascriptInputBytes},
		{"maxRenderBytes", l.MaxRenderBytes},
		{"maxTransformInputBytes", l.MaxTransformInputBytes},
		{"maxResponseStreamBytes", l.MaxResponseStreamBytes},
		{"maxResponseHeaderBytes", l.MaxResponseHeaderBytes},
		{"maxResponseSourceBytes", l.MaxResponseSourceBytes},
	} {
		if field.value != nil &&
			(field.value.Value() == 0 || field.value.Value() > uint64(math.MaxInt)) {
			result = multierror.Append(result, vc.NewErrorfForField(field.name, "must be between 1 and %d bytes", math.MaxInt))
		}
	}

	for _, field := range []struct {
		name  string
		value *int
	}{
		{"maxRequests", l.MaxRequests},
		{"maxPendingOperations", l.MaxPendingOperations},
		{"maxStackDepth", l.MaxStackDepth},
		{"maxDepth", l.MaxDepth},
		{"maxValues", l.MaxValues},
		{"maxRenderSteps", l.MaxRenderSteps},
		{"maxNumberDigits", l.MaxNumberDigits},
	} {
		if field.value != nil && *field.value <= 0 {
			result = multierror.Append(result, vc.NewErrorForField(field.name, "must be positive"))
		}
	}

	if l.MaxNumberDigits != nil && *l.MaxNumberDigits > 4096 {
		result = multierror.Append(result, vc.NewErrorForField("maxNumberDigits", "must not exceed 4096"))
	}

	return result.ErrorOrNil()
}
