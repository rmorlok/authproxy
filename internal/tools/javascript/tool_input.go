package javascript

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/dop251/goja"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
)

// prepareInvocation adapts the captured Tool snapshot into the shared async
// input contract. A supplied host/session pair produces exactly one connection;
// the no-I/O path keeps ordinary data scopes and an empty connections array.
// Configuration appears in only one decoded document, and aliases reference its
// native copy rather than consuming the input budget again.
func (p *ToolProgram) prepareInvocation(
	ctx context.Context,
	vm *goja.Runtime,
	invocation *apjsruntime.AsyncInvocation,
	input Input,
	host *apjsruntime.HTTPHost,
	session *httphost.Session,
) error {
	if (host == nil) != (session == nil) {
		return apjsruntime.ErrInvalidInput
	}

	snapshot, response, err := inputJSON(ctx, input, p.limits)
	if err != nil {
		return err
	}

	// SnapshotJSON has already bounded these bytes. Split its fixed scopes into
	// raw documents without converting numeric values or constructing Goja data.
	scopes, err := decodeAsyncInput(ctx, snapshot)
	if err != nil {
		return err
	}

	for _, name := range []string{
		"params",
		"cfg",
		"labels",
		"annotations",
		"connection",
		"tool",
	} {
		if len(scopes[name]) == 0 {
			return apjsruntime.ErrInvalidInput
		}
	}
	prepared := apjsruntime.AsyncInput{Params: scopes["params"]}
	delete(scopes, "params")

	if session != nil {
		connection, err := decodeAsyncInput(ctx, scopes["connection"])
		if err != nil {
			return err
		}
		if connection == nil {
			return apjsruntime.ErrInvalidInput
		}

		for _, name := range []string{"cfg", "labels", "annotations"} {
			connection[name] = scopes[name]
			delete(scopes, name)
		}
		delete(scopes, "connection")

		metadata, err := encodeAsyncInput(ctx, connection)
		if err != nil {
			return err
		}

		prepared.Connections = []apjsruntime.ConnectionInput{
			{
				Snapshot:   metadata,
				Dispatcher: session,
			},
		}
	}

	// Include response in the same batch even though it is exposed only as a
	// legacy global. Remove the temporary context field after native copying.
	scopes["response"] = response

	prepared.Context, err = encodeAsyncInput(ctx, scopes)
	if err != nil {
		return err
	}

	if err := invocation.PrepareInput(ctx, vm, prepared, p.limits, host); err != nil {
		return err
	}

	contextObject := invocation.Context.(*goja.Object)
	responseObject, ok := contextObject.Get("response").(*goja.Object)
	if !ok || responseObject.ClassName() != "Object" {
		return apjsruntime.ErrInvalidInput
	}

	if err := contextObject.Delete("response"); err != nil {
		return apjsruntime.ErrInvalidInput
	}

	if session != nil {
		connection := contextObject.Get("connections").(*goja.Object).Get("0").(*goja.Object)
		if err := bindConnectionAliases(vm, contextObject, connection); err != nil {
			return err
		}
	}

	if err := bindToolGlobals(vm, invocation.Params, contextObject, responseObject); err != nil {
		return err
	}
	return apjsruntime.ChargeWork(ctx, 0)
}

// decodeAsyncInput splits already-bounded JSON into raw scopes. Admit its byte
// scan and copied raw fields before unmarshalling; serialized size also bounds
// the map's native slots by fixed multiples. Parser details remain private.
func decodeAsyncInput(ctx context.Context, encoded json.RawMessage) (map[string]json.RawMessage, error) {
	if err := chargeInputCopy(ctx, len(encoded)); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, apjsruntime.ErrInvalidInput
	}
	if err := apjsruntime.ChargeWork(ctx, 0); err != nil {
		return nil, err
	}
	return fields, nil
}

// encodeAsyncInput reshapes already-bounded raw scopes without HTML escaping.
// These documents enter a JSON parser, never HTML. Escaping would unnecessarily
// expand configuration or response strings and could exhaust the input allowance.
func encodeAsyncInput(ctx context.Context, fields map[string]json.RawMessage) (json.RawMessage, error) {
	if err := apjsruntime.ChargeWork(ctx, 1); err != nil {
		return nil, err
	}
	if err := apjsruntime.ReserveAllocation(ctx, len(fields)); err != nil {
		return nil, err
	}
	// Fixed Tool scope names and their bounded raw values admit encoder scratch
	// before allocation. JSON escaping can grow these source units only by a
	// fixed factor. Charge the detached buffer's exact size once Encode exposes
	// it, including its temporary trailing newline before that byte is dropped.
	for name, value := range fields {
		if err := chargeInputCopy(ctx, len(name)); err != nil {
			return nil, err
		}
		if err := chargeInputCopy(ctx, len(value)); err != nil {
			return nil, err
		}
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(fields); err != nil {
		return nil, apjsruntime.ErrInvalidInput
	}
	if err := chargeInputCopy(ctx, buffer.Len()); err != nil {
		return nil, err
	}

	// Encoder appends one newline; it is not part of the copied input document.
	return json.RawMessage(buffer.Bytes()[:buffer.Len()-1]), nil
}

// bindConnectionAliases retains the Tool context conventions for existing
// connector helpers. Metadata aliases point at the same copied native objects.
// The connection identity view contains only data, so returning that view never
// attempts to encode the branded connection capability or grants another client.
func bindConnectionAliases(vm *goja.Runtime, contextObject, connection *goja.Object) error {
	identity := vm.NewObject()
	for _, name := range []string{"id", "name", "namespace"} {
		if err := identity.DefineDataProperty(
			name,
			connection.Get(name),
			goja.FLAG_TRUE, // writable
			goja.FLAG_TRUE, // configurable
			goja.FLAG_TRUE, // enumerable
		); err != nil {
			return apjsruntime.ErrInvalidInput
		}
	}

	if err := contextObject.DefineDataProperty(
		"connection",
		identity,
		goja.FLAG_TRUE, // writable
		goja.FLAG_TRUE, // configurable
		goja.FLAG_TRUE, // enumerable
	); err != nil {
		return apjsruntime.ErrInvalidInput
	}

	for _, name := range []string{"cfg", "labels", "annotations"} {
		if err := contextObject.DefineDataProperty(
			name,
			connection.Get(name),
			goja.FLAG_TRUE, // writable
			goja.FLAG_TRUE, // configurable
			goja.FLAG_TRUE, // enumerable
		); err != nil {
			return apjsruntime.ErrInvalidInput
		}
	}

	return nil
}
