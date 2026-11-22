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

// Result is the detached, successful content produced by one JavaScript Tool.
// It is not the public invocation envelope: the executor supplies trusted Tool,
// connection, revision, and invocation identities, performs output-schema checks,
// and bounds the complete ToolResult envelope. Authored failures remain separate.
// The JSON tags also define the payload accepted by the toolResult helper.
type Result struct {
	// StructuredContent is absent when nil and explicitly null when its bytes
	// are "null". Ordinary returns populate this field, including scalar values.
	StructuredContent common.RawJSON `json:"structuredContent,omitempty" yaml:"structuredContent,omitempty"`

	// Content preserves authored block order and is always a nonnil array.
	// Ordinary strings also produce one text block; other ordinary values do not.
	Content []api.ToolContentBlock `json:"content" yaml:"content"`

	// Provider is optional administrator-selected extension metadata. When
	// present it must be a JSON object; it conveys no authority or instructions.
	Provider common.RawJSON `json:"provider,omitempty" yaml:"provider,omitempty"`
}

// toolResultFactory captures private branding operations before authored code
// can alter globals or prototypes. A WeakMap avoids retaining every discarded
// handle in a Go registry. The private prototype distinguishes nested handles
// from ordinary JSON objects, while the map identifies actual helper results.
var toolResultFactory = goja.MustCompile("tool-result-helper.js", `
(function (admit) {
    const results = new WeakMap();
    const has = results.has.bind(results);
    const get = results.get.bind(results);
    const set = results.set.bind(results);
    const create = Object.create;
    const freeze = Object.freeze;
    const Invalid = TypeError;
    const prototype = freeze(create(null));
    return {
        has, get,
        helper: function toolResult(payload) {
            admit();
            if (arguments.length !== 1) throw new Invalid("toolResult requires one payload");
            const handle = create(prototype);
            set(handle, payload);
            return freeze(handle);
        }
    };
})
`, true)

// toolResultExtension supplies only Tool success semantics. The shared runtime
// owns initialization, promise settlement, cancellation, and output accounting.
// Ordinary rejections remain private execution failures; ToolError is separate.
func toolResultExtension() apjsruntime.ResultExtension[*Result] {
	return apjsruntime.ResultExtension[*Result]{
		Bindings: []apjsruntime.HostBinding{
			{Name: "toolResult"},
		},
		Install: installToolResult,
	}
}

// toolResultInterpreter recognizes only this invocation's helper handles. Its
// captured callables belong to the VM owner and never escape result conversion.
type toolResultInterpreter struct {
	has goja.Callable // Bound WeakMap.has; plain objects cannot forge a handle.
	get goja.Callable // Bound WeakMap.get; retrieves the original authored payload.
}

// installToolResult installs the pure helper before authored initialization in
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
	admit := vm.ToValue(func(goja.FunctionCall) goja.Value {
		err := apjsruntime.ChargeWork(ctx, 1)
		if err == nil {
			// Reserve one wrapper and one private branding tuple before creating
			// either. Dropping a handle never refunds cumulative allocation.
			err = apjsruntime.ReserveAllocation(ctx, 2)
		}
		if err != nil {
			// Interrupt makes exhaustion terminal even if authored code catches
			// the fixed native exception and tries to issue another request.
			vm.Interrupt(err)
			panic(vm.NewTypeError("tool result budget exhausted"))
		}

		return goja.Undefined()
	})

	value, err = factory(goja.Undefined(), admit)
	if err != nil {
		return empty, apjsruntime.EvaluationError(err)
	}

	registry := value.ToObject(vm)
	has, _ := goja.AssertFunction(registry.Get("has"))
	get, _ := goja.AssertFunction(registry.Get("get"))

	if err := vm.Set("toolResult", registry.Get("helper")); err != nil {
		return empty, apjsruntime.ErrInvalidInput
	}

	interpreter := &toolResultInterpreter{has: has, get: get}

	return apjsruntime.ResultInterpreter[*Result]{Fulfilled: interpreter.fulfilled}, nil
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
		// Typed Go JSON decoding replaces lone surrogate escapes. Require
		// lossless strings throughout the helper payload before that boundary.
		if err := apjsruntime.ValidateJSONStrings(ctx, encoded); err != nil {
			if budgetErr := apjsruntime.ChargeWork(ctx, 0); budgetErr != nil {
				return nil, budgetErr
			}
			return nil, apjsruntime.RejectOutput(apjsruntime.OutputInvalidString)
		}
		// The encoded payload bounds decoded strings and native slots by fixed
		// implementation multiples. Charge decoding and canonical validation
		// separately, before their Go routines can scan or allocate.
		for range 2 {
			if err := apjsruntime.ChargeWork(ctx, len(encoded)); err != nil {
				return nil, err
			}
			if err := apjsruntime.ReserveAllocation(ctx, len(encoded)); err != nil {
				return nil, err
			}
		}
		if err := util.DecodeJSONStrict(encoded, result); err != nil || result.Content == nil {
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
