package javascript

import (
	"context"
	"encoding/json"

	"github.com/dop251/goja"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/tools/render"
)

// bindInput validates and copies the captured context and optional response
// into native JavaScript values before any authored expression executes. The
// globals cannot be replaced, but nested mutations remain local to this VM.
// It returns fixed classifications rather than parser errors containing input.
func bindInput(
	ctx context.Context,
	vm *goja.Runtime,
	input Input,
	limits Limits,
) error {
	if ctx == nil || vm == nil || input.Context == nil {
		return apjsruntime.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	response := input.Response
	if response == nil {
		response = []byte("{}")
	}
	if len(response) >= limits.MaxInputBytes {
		return apjsruntime.ErrLimitExceeded
	}

	// Share the serialized byte allowance with snapshot encoding; the response
	// consumes the same budget even when it contains only an empty object.
	snapshot, err := input.Context.SnapshotJSON(ctx, render.Limits{
		MaxBytes: limits.MaxInputBytes - len(response),
		MaxDepth: limits.MaxDepth,
		MaxSteps: limits.MaxNodes,
	})
	if err != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return apjsruntime.ErrLimitExceeded
	}

	// The shared decoder charges both documents before constructing native
	// values. Tool-specific object shapes and aliases remain this adapter's job.
	decoder, err := apjsruntime.NewJSONDecoder(vm)
	if err != nil {
		return err
	}
	values, err := decoder.Decode(ctx, apjsruntime.JSONLimits{
		MaxBytes: limits.MaxInputBytes,
		MaxDepth: limits.MaxDepth,
		MaxNodes: limits.MaxNodes,
	}, json.RawMessage(snapshot), json.RawMessage(response))
	if err != nil {
		return err
	}
	rootObject, ok := values[0].(*goja.Object)
	if !ok || rootObject.ClassName() != "Object" {
		return apjsruntime.ErrInvalidInput
	}
	responseValue, ok := values[1].(*goja.Object)
	if !ok || responseValue.ClassName() != "Object" {
		return apjsruntime.ErrInvalidInput
	}

	// A zero render.Context has no captured scopes. Only snapshots created by
	// render.NewContext supply every required object, including empty scopes.
	for _, name := range []string{
		"params",
		"cfg",
		"labels",
		"annotations",
		"connection",
		"tool",
	} {
		if _, ok := rootObject.Get(name).(*goja.Object); !ok {
			return apjsruntime.ErrInvalidInput
		}
	}

	contextObject := vm.NewObject()
	for _, name := range []string{
		"cfg",
		"labels",
		"annotations",
		"connection",
		"tool",
	} {
		if err := contextObject.DefineDataProperty(
			name,
			rootObject.Get(name),
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
			goja.FLAG_TRUE,
		); err != nil {
			return apjsruntime.ErrInvalidInput
		}
	}

	// Each alias is the identical native object stored in context, not a new
	// wrapper or copy. No Go map, slice, struct, or function enters the VM.
	globals := []struct {
		name  string
		value goja.Value
	}{
		{"params", rootObject.Get("params")},
		{"context", contextObject},
		{"response", responseValue},
		{"cfg", contextObject.Get("cfg")},
		{"labels", contextObject.Get("labels")},
		{"annotations", contextObject.Get("annotations")},
	}

	for _, global := range globals {
		if err := vm.GlobalObject().DefineDataProperty(
			global.name,
			global.value,
			goja.FLAG_FALSE,
			goja.FLAG_FALSE,
			goja.FLAG_TRUE,
		); err != nil {
			return apjsruntime.ErrInvalidInput
		}
	}

	return ctx.Err()
}
