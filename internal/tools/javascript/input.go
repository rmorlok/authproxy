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
	if vm == nil {
		return apjsruntime.ErrInvalidInput
	}
	snapshot, response, err := inputJSON(ctx, input, limits)
	if err != nil {
		return err
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

	if err := bindToolGlobals(
		vm,
		rootObject.Get("params"),
		contextObject,
		responseValue,
	); err != nil {
		return err
	}
	return apjsruntime.ChargeWork(ctx, 0)
}

// inputJSON bounds snapshot serialization before allocating raw JSON fields for
// either the expression or async input adapter. The response and snapshot share
// this byte allowance; the subsequent decoder bounds their combined traversal.
// Adapter serialization also consumes the enclosing execution's shared budget.
func inputJSON(
	ctx context.Context,
	input Input,
	limits Limits,
) (json.RawMessage, json.RawMessage, error) {
	if ctx == nil || input.Context == nil {
		return nil, nil, apjsruntime.ErrInvalidInput
	}
	if err := apjsruntime.ChargeWork(ctx, 0); err != nil {
		return nil, nil, err
	}

	response := input.Response
	if response == nil {
		response = []byte("{}")
	}

	if len(response) >= limits.MaxInputBytes {
		return nil, nil, apjsruntime.ErrLimitExceeded
	}

	snapshot, err := input.Context.SnapshotJSON(ctx, render.Limits{
		MaxBytes: limits.MaxInputBytes - len(response),
		MaxDepth: limits.MaxDepth,
		MaxSteps: limits.MaxNodes,
	})
	if err != nil {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		return nil, nil, apjsruntime.ErrLimitExceeded
	}
	// SnapshotJSON exposes no size preflight. Its own byte/node/depth caps
	// bound serialization first, then its measured output charges this execution
	// immediately. These source units also bound serializer scratch by fixed
	// implementation multiples; this is not a hard heap allocation quota.
	if err := chargeInputCopy(ctx, len(snapshot)); err != nil {
		return nil, nil, err
	}

	return json.RawMessage(snapshot), json.RawMessage(response), nil
}

// chargeInputCopy admits one bounded native pass over known-size input and its
// copied storage. It preserves cancellation and terminal shared-budget errors.
func chargeInputCopy(ctx context.Context, size int) error {
	if err := apjsruntime.ChargeWork(ctx, size); err != nil {
		return err
	}
	return apjsruntime.ReserveAllocation(ctx, size)
}

// bindToolGlobals installs protected aliases to already copied native values.
// Aliases share their context objects and incur no second input copy or charge.
func bindToolGlobals(
	vm *goja.Runtime,
	params goja.Value,
	contextObject *goja.Object,
	response *goja.Object,
) error {
	globals := []struct {
		name  string
		value goja.Value
	}{
		{"params", params},
		{"context", contextObject},
		{"response", response},
		{"cfg", contextObject.Get("cfg")},
		{"labels", contextObject.Get("labels")},
		{"annotations", contextObject.Get("annotations")},
	}

	for _, global := range globals {
		if err := vm.GlobalObject().DefineDataProperty(
			global.name,
			global.value,
			goja.FLAG_FALSE, // writable
			goja.FLAG_FALSE, // configurable
			goja.FLAG_TRUE,  // enumerable
		); err != nil {
			return apjsruntime.ErrInvalidInput
		}
	}

	return nil
}
