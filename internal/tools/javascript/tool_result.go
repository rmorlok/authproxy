package javascript

import (
	"bytes"
	"context"
	"strconv"

	"github.com/dop251/goja"
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
(function () {
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
            if (arguments.length !== 1) throw new Invalid("toolResult requires one payload");
            const handle = create(prototype);
            set(handle, payload);
            return freeze(handle);
        }
    };
})()
`, true)

// resultEncoder recognizes only this invocation's helper handles and converts
// their payload with the same bounded JSON traversal used by ordinary returns.
// All operations belong to the runtime's owner goroutine; nothing escapes it.
type resultEncoder struct {
	json *outputEncoder // Pristine reflection and byte/depth/node policy.
	has  goja.Callable  // Bound WeakMap.has; plain objects cannot forge a handle.
	get  goja.Callable  // Bound WeakMap.get; retrieves the original authored payload.
}

// newResultEncoder installs the pure toolResult helper in a fresh runtime before
// initialization. Validation also installs it so top-level constants can use the
// same helper without network access. Payload validation occurs on conversion.
func newResultEncoder(vm *goja.Runtime, limits Limits) (*resultEncoder, error) {
	encoder := newOutputEncoder(vm, limits)
	value, err := vm.RunProgram(toolResultFactory)
	if err != nil {
		return nil, runtimeEvaluationError(err)
	}
	registry := value.ToObject(vm)
	has, _ := goja.AssertFunction(registry.Get("has"))
	get, _ := goja.AssertFunction(registry.Get("get"))
	if err := vm.Set("toolResult", registry.Get("helper")); err != nil {
		return nil, ErrInvalidInput
	}
	return &resultEncoder{json: encoder, has: has, get: get}, nil
}

// encode maps ordinary JSON or an explicitly branded payload into detached
// content. MaxOutputBytes counts the ordinary JSON or the entire authored helper
// payload, including base64 and metadata. Derived text duplication and trusted
// envelope fields need the executor's separate final serialized-result limit.
func (e *resultEncoder) encode(ctx context.Context, value goja.Value) (*Result, error) {
	branded, err := e.has(goja.Undefined(), value)
	if err != nil {
		return nil, runtimeEvaluationError(err)
	}
	if branded.ToBoolean() {
		value, err = e.get(goja.Undefined(), value)
		if err != nil {
			return nil, runtimeEvaluationError(err)
		}
	}
	encoded, err := e.json.encode(ctx, value)
	if err != nil {
		return nil, err
	}
	result := &Result{Content: []api.ToolContentBlock{}}
	if !branded.ToBoolean() {
		result.StructuredContent = encoded
		if encoded[0] == '"' {
			// A text block cannot represent a lone UTF-16 surrogate. Unquote
			// rejects it instead of silently replacing authored output.
			text, err := strconv.Unquote(string(encoded))
			if err != nil {
				return nil, ErrInvalidOutput
			}
			result.Content = append(result.Content, api.ToolContentBlock{Type: api.ToolContentTypeText, Text: &text})
		}
	} else {
		if encoded[0] != '{' {
			return nil, ErrInvalidOutput
		}
		// Go's typed JSON decoder would replace lone surrogate escapes in
		// strings. Check before decoding to keep rich text and URIs lossless.
		if err := validateInputSurrogates(ctx, encoded); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrInvalidOutput
		}
		if err := util.DecodeJSONStrict(encoded, result); err != nil || result.Content == nil {
			return nil, ErrInvalidOutput
		}
		if len(result.Provider) != 0 && bytes.TrimSpace(result.Provider)[0] != '{' {
			return nil, ErrInvalidOutput
		}
		for i := range result.Content {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			// Reuse the canonical content union, including media/base64, URI,
			// annotations, and provider rules; never copy diagnostics outward.
			if err := result.Content[i].Validate(nil); err != nil {
				return nil, ErrInvalidOutput
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
