package javascript

import (
	"bytes"
	"context"
	"strconv"

	"github.com/dop251/goja"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
)

// Result is the detached success or explicitly authored failure from one JavaScript Tool.
// It is not the public invocation envelope: the executor supplies trusted Tool,
// connection, revision, and invocation identities, performs output-schema checks,
// and bounds the complete ToolResult envelope. Runtime failures use Go errors.
// The serialized fields define the success payload accepted by toolResult.
type Result struct {
	// Error is set only when execute rejects with a genuine ToolError. Evaluation
	// then returns this Result with a nil Go error: conversion succeeded, but
	// the authored operation reported a failure. Code, Message, and Details are
	// explicitly chosen public data. ExecutionState is unset and Retryable is
	// false; invocation assembly supplies trusted evidence and retry policy.
	// The success helper cannot set this field through JSON or YAML decoding.
	Error *api.ToolResultError `json:"-" yaml:"-"`

	// StructuredContent is absent when nil and explicitly null when its bytes
	// are "null". Ordinary returns populate this field, including scalar values.
	StructuredContent common.RawJSON `json:"structuredContent,omitempty" yaml:"structuredContent,omitempty"`

	// Content preserves authored block order and is always a nonnil array.
	// Ordinary strings also produce one text block; other ordinary values do not.
	Content []api.ToolContentBlock `json:"content" yaml:"content"`

	// Provider carries optional metadata about the upstream service or its
	// response, selected by the Tool author through toolResult({provider: ...}).
	// For example, {"requestId":"req-7","apiVersion":"2026-01"} lets a caller
	// correlate a result with the provider's support logs or diagnose API-version
	// differences. For a customer lookup, put the customer record in
	// StructuredContent and its display text or media in Content.
	//
	// Keys are application-defined. This adapter copies the JSON object into
	// the result without interpreting its fields or using them for permissions,
	// retries, or invocation identity. It does not populate or redact metadata
	// automatically; the Tool author chooses which fields are safe to return.
	// Nil omits the field. When supplied, it must be an object, not null, an
	// array, or a scalar, and it shares the result's size and execution budgets.
	Provider common.RawJSON `json:"provider,omitempty" yaml:"provider,omitempty"`
}

// toolResultFactory captures private branding operations before authored code
// can alter globals or prototypes. A WeakMap avoids retaining every discarded
// handle in a Go registry. Private prototypes distinguish nested handles from
// ordinary JSON objects; separate maps identify success and failure handles.
var toolResultFactory = goja.MustCompile("tool-result-helper.js", `
(function (admitResult, admitError) {
    const results = new WeakMap();
    const has = results.has.bind(results);
    const get = results.get.bind(results);
    const set = results.set.bind(results);
    const errors = new WeakMap();
    const errorHas = errors.has.bind(errors);
    const errorGet = errors.get.bind(errors);
    const errorSet = errors.set.bind(errors);
    const create = Object.create;
    const freeze = Object.freeze;
    const Invalid = TypeError;
    const prototype = freeze(create(null));
    const errorPrototype = freeze(create(null));
    return {
        has, 
        get, 
        errorHas, 
        errorGet,
        helper: function toolResult(payload) {
            admitResult();
            if (arguments.length !== 1) throw new Invalid("toolResult requires one payload");
            const handle = create(prototype);
            set(handle, payload);
            return freeze(handle);
        },
        ToolError: function ToolError(code, message, details) {
            admitError();
            if (arguments.length < 2 || arguments.length > 3 ||
                typeof code !== "string" || typeof message !== "string") {
                throw new Invalid("ToolError requires a string code, string message, and optional details");
            }
            const payload = create(null);
            payload.code = code;
            payload.message = message;
            if (details !== undefined) payload.details = details;
            const handle = create(errorPrototype);
            errorSet(handle, payload);
            return freeze(handle);
        }
    };
})
`, true)

// toolResultExtension supplies Tool success and explicit failure semantics. The
// shared runtime owns initialization, promise settlement, cancellation, and
// output accounting. Unrecognized rejections remain private execution failures.
func toolResultExtension() apjsruntime.ResultExtension[*Result] {
	return apjsruntime.ResultExtension[*Result]{
		Bindings: []apjsruntime.HostBinding{
			{Name: "toolResult"},
			{Name: "ToolError"},
		},
		Install: installToolResult,
	}
}

// toolResultInterpreter recognizes only this invocation's helper handles. Its
// captured callables belong to the VM owner and never escape result conversion.
type toolResultInterpreter struct {
	has goja.Callable // Bound WeakMap.has; plain objects cannot forge a handle.
	get goja.Callable // Bound WeakMap.get; retrieves the original success payload.

	errorHas goja.Callable // Separate brand check; only explicit failures are public.
	errorGet goja.Callable // Captured error payload, unaffected by handle properties.
}

// installToolResult installs pure success/error helpers before initialization in
// both validation and evaluation. Discarded handles do not stay alive through a
// Go registry, but every creation still consumes the execution's shared budget.
func installToolResult(
	ctx context.Context,
	vm *goja.Runtime,
) (apjsruntime.ResultInterpreter[*Result], error) {
	var empty apjsruntime.ResultInterpreter[*Result]

	value, err := vm.RunProgram(toolResultFactory)
	if err != nil {
		return empty, apjsruntime.EvaluationError(err)
	}

	factory, _ := goja.AssertFunction(value)
	value, err = factory(
		goja.Undefined(),
		toolHelperAdmission(ctx, vm, 2), // Success wrapper and branding tuple.
		toolHelperAdmission(ctx, vm, 6), // Error wrapper, tuple, payload, and three fields.
	)
	if err != nil {
		return empty, apjsruntime.EvaluationError(err)
	}

	registry := value.ToObject(vm)
	has, _ := goja.AssertFunction(registry.Get("has"))
	get, _ := goja.AssertFunction(registry.Get("get"))

	if err := vm.Set("toolResult", registry.Get("helper")); err != nil {
		return empty, apjsruntime.ErrInvalidInput
	}

	if err := vm.Set("ToolError", registry.Get("ToolError")); err != nil {
		return empty, apjsruntime.ErrInvalidInput
	}
	errorHas, _ := goja.AssertFunction(registry.Get("errorHas"))
	errorGet, _ := goja.AssertFunction(registry.Get("errorGet"))
	interpreter := &toolResultInterpreter{has: has, get: get, errorHas: errorHas, errorGet: errorGet}

	return apjsruntime.ResultInterpreter[*Result]{
		Fulfilled: interpreter.fulfilled,
		Rejected:  interpreter.rejected,
	}, nil
}

// toolHelperAdmission creates a private native gate used only by the captured
// helper factories. Each call charges entry work and its fixed native slots
// before creating handles; discarded or caught values do not refund the ledger.
func toolHelperAdmission(ctx context.Context, vm *goja.Runtime, slots int) goja.Value {
	return vm.ToValue(func(goja.FunctionCall) goja.Value {
		err := apjsruntime.ChargeWork(ctx, 1)
		if err == nil {
			err = apjsruntime.ReserveAllocation(ctx, slots)
		}
		if err != nil {
			// Interrupt makes exhaustion terminal even if authored code catches
			// the fixed native exception and tries to issue another request.
			vm.Interrupt(err)
			panic(vm.NewTypeError("tool helper budget exhausted"))
		}
		return goja.Undefined()
	})
}

// fulfilled maps ordinary JSON or a branded payload into detached content.
// MaxOutputBytes counts the ordinary JSON or the entire authored helper payload,
// including media and metadata. Derived text and the trusted envelope still need
// the executor's separate limit on the complete serialized invocation result.
func (i *toolResultInterpreter) fulfilled(
	ctx context.Context,
	value goja.Value,
	encoder *apjsruntime.ResultEncoder,
) (*Result, error) {
	branded, err := i.has(goja.Undefined(), value)
	if err != nil {
		return nil, apjsruntime.EvaluationError(err)
	}

	if branded.ToBoolean() {
		value, err = i.get(goja.Undefined(), value)
		if err != nil {
			return nil, apjsruntime.EvaluationError(err)
		}
	}

	encoded, err := encoder.Encode(value)
	if err != nil {
		return nil, err
	}

	if err := apjsruntime.ReserveAllocation(ctx, 1); err != nil {
		return nil, err
	}

	result := &Result{Content: []api.ToolContentBlock{}}
	if !branded.ToBoolean() {
		result.StructuredContent = common.RawJSON(encoded)
		if encoded[0] == '"' {
			// Conversion works from the already bounded JSON so the output byte
			// and node allowances are charged once. Reserve the source string,
			// unquoted copy, and text block before native conversion.
			if err := apjsruntime.ChargeWork(ctx, len(encoded)); err != nil {
				return nil, err
			}

			for range 2 {
				if err := apjsruntime.ReserveAllocation(ctx, len(encoded)); err != nil {
					return nil, err
				}
			}

			if err := apjsruntime.ReserveAllocation(ctx, 1); err != nil {
				return nil, err
			}

			// Unlike Go's JSON decoder, Unquote rejects lone UTF-16 units
			// instead of silently changing them to replacement characters.
			text, err := strconv.Unquote(string(encoded))
			if err != nil {
				return nil, apjsruntime.RejectOutput(apjsruntime.OutputInvalidString)
			}

			result.Content = append(result.Content, api.ToolContentBlock{
				Type: api.ToolContentTypeText,
				Text: &text,
			})
		}
	} else {
		if encoded[0] != '{' {
			return nil, apjsruntime.RejectOutput(apjsruntime.OutputUnexpectedType)
		}
		if err := decodeToolPayload(ctx, encoded, result); err != nil {
			return nil, err
		}
		if result.Content == nil {
			return nil, apjsruntime.RejectOutput(apjsruntime.OutputInvalidShape)
		}
		if len(result.Provider) != 0 && bytes.TrimSpace(result.Provider)[0] != '{' {
			return nil, apjsruntime.RejectOutput(apjsruntime.OutputInvalidShape)
		}
		for j := range result.Content {
			if err := apjsruntime.ChargeWork(ctx, 0); err != nil {
				return nil, err
			}
			// Reuse the API's content union, including media/base64, URI,
			// annotations, and provider rules; never expose its diagnostics.
			if err := result.Content[j].Validate(nil); err != nil {
				return nil, apjsruntime.RejectOutput(apjsruntime.OutputInvalidShape)
			}
		}
	}

	if err := apjsruntime.ChargeWork(ctx, 0); err != nil {
		return nil, err
	}

	return result, nil
}

// decodeToolPayload admits typed decoding and subsequent field validation of
// already bounded JSON. It rejects lossy strings and hides decoder diagnostics;
// success/error interpreters validate their own domain shape after this call.
func decodeToolPayload(ctx context.Context, encoded []byte, target any) error {
	// Go's typed decoder replaces lone surrogate escapes, including in keys.
	if err := apjsruntime.ValidateJSONStrings(ctx, encoded); err != nil {
		if budgetErr := apjsruntime.ChargeWork(ctx, 0); budgetErr != nil {
			return budgetErr
		}
		return apjsruntime.RejectOutput(apjsruntime.OutputInvalidString)
	}

	// Encoded bytes bound decoded strings and native slots by fixed multiples.
	// Charge decoding and canonical validation separately before native work.
	for range 2 {
		if err := apjsruntime.ChargeWork(ctx, len(encoded)); err != nil {
			return err
		}
		if err := apjsruntime.ReserveAllocation(ctx, len(encoded)); err != nil {
			return err
		}
	}

	if err := util.DecodeJSONStrict(encoded, target); err != nil {
		return apjsruntime.RejectOutput(apjsruntime.OutputInvalidShape)
	}

	return apjsruntime.ChargeWork(ctx, 0)
}
