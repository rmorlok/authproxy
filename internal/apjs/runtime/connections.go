package runtime

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dop251/goja"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
)

// AsyncInput supplies detached JSON and already-authorized HTTP capabilities for
// execute(params, context). Keep its bytes and slice stable during preparation.
// Authorization, safe metadata selection, and client attribution belong to the
// caller; this package never looks up a connection from its metadata.
type AsyncInput struct {
	// Params accepts any JSON value. Omitted bytes become an empty object.
	Params json.RawMessage

	// Context contains extra JSON object fields, such as domain metadata. Omitted
	// bytes become an empty object; the connections property is reserved.
	Context json.RawMessage

	// Connections preserves caller order. All snapshots share the initial input
	// allowance with Params and Context; each ID must be nonempty and unique.
	Connections []ConnectionInput
}

// ConnectionInput pairs a captured JSON snapshot with its private HTTP dispatch
// capability. Snapshot edits in JavaScript never alter or select the dispatcher.
type ConnectionInput struct {
	// Snapshot is an object with id, name, namespace, cfg, labels, annotations.
	// id is a required nonempty string; missing name/namespace default to "".
	// cfg is a JSON object; labels/annotations are string maps. Missing objects
	// default to {}; explicit null and unknown top-level fields are rejected.
	Snapshot json.RawMessage

	// Dispatch has HTTPHost.BindFetch's ownership and bounded-response contract.
	// Supply handles from one HTTP session for shared request/response budgets,
	// deadline, and evidence. It must capture no VM values and cannot be nil.
	Dispatch func(*http.Request) (*aphttp.Response, error)
}

// connectionBrand marks a capability independently of its enumerable metadata.
// It is nonconfigurable on a frozen object, so authored code cannot remove it.
// The bounded output encoder rejects symbol keys even when non-enumerable;
// copying selected string-keyed metadata creates ordinary data, not authority.
var connectionBrand = goja.NewSymbol("AuthProxy connection")

// PrepareInput copies a complete initial input batch and sets the invocation's
// native Params and Context only on success. Call it during Evaluate preparation,
// before authored code runs, with the evaluation's VM, context, and limits.
// It installs no globals; adapters may bind aliases to these same copied objects.
//
// The resulting context always has a read-only ordered connections array. Its
// members and their top-level properties are frozen; nested metadata stays
// mutable and private to this VM. Fetch methods retain their captured dispatcher
// when borrowed, and connection objects cannot be returned as generic JSON.
//
// A host is required only when connections are supplied. It must belong to this
// VM, and its Close/Start/Await must already be registered on the invocation.
// This helper owns no session or worker lifecycle and never enables I/O.
func (i *AsyncInvocation) PrepareInput(
	ctx context.Context,
	vm *goja.Runtime,
	input AsyncInput,
	limits Limits,
	host *HTTPHost,
) error {
	if i == nil ||
		ctx == nil ||
		vm == nil ||
		limits.Validate() != nil ||
		(len(input.Connections) != 0 && host == nil) ||
		(host != nil && host.vm != vm) {
		return ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Every document consumes at least one node. Bound the document slice before
	// allocating it, including the two default argument/context roots.
	if limits.MaxNodes < 2 || len(input.Connections) > limits.MaxNodes-2 {
		return ErrLimitExceeded
	}

	documents := make([]json.RawMessage, 2, 2+len(input.Connections))
	documents[0], documents[1] = input.Params, input.Context
	for index := range documents {
		if documents[index] == nil {
			documents[index] = json.RawMessage(`{}`)
		}
	}

	for _, connection := range input.Connections {
		if err := ctx.Err(); err != nil {
			return err
		}
		if connection.Dispatch == nil {
			return ErrInvalidInput
		}
		documents = append(documents, connection.Snapshot)
	}

	decoder, err := NewJSONDecoder(vm)
	if err != nil {
		return err
	}

	values, err := decoder.Decode(
		ctx,
		JSONLimits{
			MaxBytes: limits.MaxInputBytes,
			MaxDepth: limits.MaxDepth,
			MaxNodes: limits.MaxNodes,
		},
		documents...,
	)
	if err != nil {
		return err
	}

	contextObject, ok := inputObject(values[1])
	if !ok {
		return ErrInvalidInput
	}

	for _, key := range contextObject.GetOwnPropertyNames() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if key == "connections" {
			return ErrInvalidInput
		}
	}

	// Validate every snapshot before binding any capability or exposing input to
	// the adapter. The decoded graphs are native JSON, with no getters or proxies.
	connections := make([]*goja.Object, len(input.Connections))
	seen := make(map[string]bool, len(connections))
	for index := range connections {
		if err := ctx.Err(); err != nil {
			return err
		}

		connection, err := connectionSnapshot(ctx, vm, values[index+2])
		if err != nil {
			return err
		}

		id := connection.Get("id").String()
		if seen[id] {
			return ErrInvalidInput
		}

		seen[id] = true
		connections[index] = connection
	}

	freeze, ok := goja.AssertFunction(
		vm.Get("Object").
			ToObject(vm).
			Get("freeze"),
	)
	if !ok {
		return ErrInvalidInput
	}

	array := vm.NewArray()
	for index, connection := range connections {
		if err := ctx.Err(); err != nil {
			return err
		}

		fetch, err := host.BindFetch(input.Connections[index].Dispatch)
		if err != nil {
			return err
		}

		if err := connection.DefineDataProperty(
			"fetch",
			vm.ToValue(fetch),
			goja.FLAG_FALSE, // writable
			goja.FLAG_FALSE, // configurable
			goja.FLAG_FALSE, // enumerable
		); err != nil {
			return ErrInvalidInput
		}
		if err := connection.DefineDataPropertySymbol(
			connectionBrand,
			vm.ToValue(true),
			goja.FLAG_FALSE, // writable
			goja.FLAG_FALSE, // configurable
			goja.FLAG_FALSE, // enumerable
		); err != nil {
			return ErrInvalidInput
		}

		if _, err := freeze(goja.Undefined(), connection); err != nil {
			return ErrInvalidInput
		}

		if err := array.Set(vm.ToValue(index).String(), connection); err != nil {
			return ErrInvalidInput
		}
	}

	if _, err := freeze(goja.Undefined(), array); err != nil {
		return ErrInvalidInput
	}

	if err := contextObject.DefineDataProperty("connections", array, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return ErrInvalidInput
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	i.Params, i.Context = values[0], contextObject

	return nil
}

// inputObject recognizes object-shaped values from the bounded JSON decoder.
// It is not a validator for arbitrary authored or reflected Go objects.
func inputObject(value goja.Value) (*goja.Object, bool) {
	object, ok := value.(*goja.Object)
	return object, ok && object.ClassName() == "Object"
}

// connectionSnapshot validates one already-bounded native JSON object and fills
// omitted metadata with fixed empty defaults. Unknown fields cannot override a
// host method, and labels/annotations retain their string-map contract.
func connectionSnapshot(ctx context.Context, vm *goja.Runtime, value goja.Value) (*goja.Object, error) {
	object, ok := inputObject(value)
	if !ok {
		return nil, ErrInvalidInput
	}

	for _, key := range object.GetOwnPropertyNames() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		switch key {
		case "id",
			"name",
			"namespace",
			"cfg",
			"labels",
			"annotations":
		default:
			return nil, ErrInvalidInput
		}
	}

	id, ok := object.Get("id").(goja.String)
	if !ok || id.Length() == 0 {
		return nil, ErrInvalidInput
	}

	for _, key := range []string{
		"name",
		"namespace",
		"cfg",
		"labels",
		"annotations",
	} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		value := object.Get(key)
		if value == nil {
			if key == "name" || key == "namespace" {
				value = vm.ToValue("")
			} else {
				value = vm.NewObject()
			}
			if err := object.Set(key, value); err != nil {
				return nil, ErrInvalidInput
			}
		}

		if key == "name" || key == "namespace" {
			if _, ok := value.(goja.String); !ok {
				return nil, ErrInvalidInput
			}
			continue
		}

		metadata, ok := inputObject(value)
		if !ok {
			return nil, ErrInvalidInput
		}

		if key != "cfg" {
			for _, label := range metadata.GetOwnPropertyNames() {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if _, ok := metadata.Get(label).(goja.String); !ok {
					return nil, ErrInvalidInput
				}
			}
		}
	}

	return object, nil
}
