package javascript

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// inputTestLimits keeps input-boundary fixtures independent of execution policy.
func inputTestLimits() Limits {
	return Limits{
		MaxSourceBytes: 1 << 16,
		MaxInputBytes:  1 << 16,
		MaxOutputBytes: 1 << 16,
		MaxDepth:       32,
		MaxNodes:       1000,
		MaxStackDepth:  64,
		Timeout:        time.Second,
	}
}

// inputTestContext captures only JSON-compatible data under generous limits.
func inputTestContext(t *testing.T, input render.Input) *render.Context {
	t.Helper()
	snapshot, err := render.NewContext(input, render.Limits{MaxBytes: 1 << 20, MaxDepth: 64, MaxSteps: 10000})
	require.NoError(t, err)
	return snapshot
}

// inputTestRun evaluates an assertion in the VM after the input boundary runs.
func inputTestRun(t *testing.T, vm *goja.Runtime, expression string) goja.Value {
	t.Helper()
	value, err := vm.RunString(expression)
	require.NoError(t, err)
	return value
}

func TestBindInputNativeContext(t *testing.T) {
	// Capture metadata, nested arrays, literal template syntax, and typed values.
	input := render.Input{
		Params:              map[string]any{"items": []any{"first", false, nil}, "directive": map[string]any{"$value": "cfg.tenant"}},
		Cfg:                 map[string]any{"tenant": "original", "literal": "{{params.missing}}"},
		Labels:              map[string]string{"region": "us"},
		Annotations:         map[string]string{"note": "captured"},
		ConnectionID:        "cxn_example",
		ConnectionNamespace: "root.product.owner",
		ToolID:              "tol_example",
		Revision:            7,
	}
	snapshot := inputTestContext(t, input)
	response := common.RawJSON(`{"status":200,"body":{"name":"Alice"},"headers":{"content-type":["application/json"]}}`)
	vm := goja.New()
	require.NoError(t, bindInput(context.Background(), vm, Input{Context: snapshot, Response: response}, inputTestLimits()))

	// Native JavaScript prototypes and aliases work without reflected Go values.
	assertion := `
		Object.getPrototypeOf(params) === Object.prototype &&
		Object.getPrototypeOf(params.items) === Array.prototype &&
		Array.isArray(params.items) && params.items[1] === false && params.items[2] === null &&
		params.directive.$value === "cfg.tenant" && cfg.literal === "{{params.missing}}" &&
		context.cfg === cfg && context.labels === labels && context.annotations === annotations &&
		context.params === undefined && context.connection.id === "cxn_example" &&
		context.connection.namespace === "root.product.owner" && context.tool.revision === 7 &&
		response.body.name === "Alice" && Array.isArray(response.headers["content-type"])
	`
	require.True(t, inputTestRun(t, vm, assertion).ToBoolean())

	// Local mutations do not change captured inputs, response bytes, or later VMs.
	inputTestRun(t, vm, `params.items.push("local"); cfg.tenant = "local"; response.body.name = "local"`)
	require.Equal(t, "original", input.Cfg["tenant"])
	require.Len(t, input.Params["items"], 3)
	require.Contains(t, string(response), `"Alice"`)
	next := goja.New()
	require.NoError(t, bindInput(context.Background(), next, Input{Context: snapshot, Response: response}, inputTestLimits()))
	require.True(t, inputTestRun(t, next, `params.items.length === 3 && cfg.tenant === "original" && response.body.name === "Alice"`).ToBoolean())
}

func TestBindInputReservedGlobals(t *testing.T) {
	// Host binding names cannot be reassigned, redefined, deleted, or shadowed by
	// global lexical declarations after input injection.
	for _, name := range []string{"params", "context", "response", "cfg", "labels", "annotations"} {
		t.Run(name, func(t *testing.T) {
			vm := goja.New()
			require.NoError(t, bindInput(context.Background(), vm, Input{Context: inputTestContext(t, render.Input{})}, inputTestLimits()))
			_, err := vm.RunString(`"use strict"; ` + name + ` = {}`)
			require.Error(t, err)
			_, err = vm.RunString(`Object.defineProperty(globalThis, "` + name + `", {value: {}})`)
			require.Error(t, err)
			require.False(t, inputTestRun(t, vm, `delete globalThis["`+name+`"]`).ToBoolean())
			_, err = vm.RunString(`let ` + name + ` = {}`)
			require.Error(t, err)
		})
	}
}

func TestBindInputJSONKeysStayData(t *testing.T) {
	// JSON keys resembling JavaScript prototype hooks remain own data values.
	vm := goja.New()
	response := common.RawJSON(`{"__proto__":{"injected":true},"constructor":"literal","body":null}`)
	require.NoError(t, bindInput(context.Background(), vm, Input{Context: inputTestContext(t, render.Input{}), Response: response}, inputTestLimits()))
	require.True(t, inputTestRun(t, vm, `Object.getPrototypeOf(response) === Object.prototype && response.injected === undefined && response.__proto__.injected === true && response.constructor === "literal" && response.body === null`).ToBoolean())

	// Omission creates an empty response object; null and undefined stay distinct.
	vm = goja.New()
	require.NoError(t, bindInput(context.Background(), vm, Input{Context: inputTestContext(t, render.Input{})}, inputTestLimits()))
	require.Equal(t, "{}", inputTestRun(t, vm, `JSON.stringify(response)`).String())
}

func TestBindInputRejectsInvalidJSON(t *testing.T) {
	for _, response := range []string{
		``, `null`, `[]`, `"private-value"`, `true`, `12`,
		`{"private-field":"private-value",}`, `{} {}`, `{"private-field":undefined}`,
		`{"private-field":NaN}`, `{"private-field":1e400}`, `{"private-field":-1e400}`,
		"{\"private-field\":\"" + string([]byte{0xff}) + "\"}",
	} {
		t.Run(response, func(t *testing.T) {
			// Invalid response documents fail before any input globals are exposed.
			vm := goja.New()
			err := bindInput(context.Background(), vm, Input{Context: inputTestContext(t, render.Input{}), Response: common.RawJSON(response)}, inputTestLimits())
			require.ErrorIs(t, err, ErrInvalidInput)
			require.NotContains(t, err.Error(), "private-field")
			require.NotContains(t, err.Error(), "private-value")
			require.Nil(t, vm.Get("params"))
		})
	}

	// Valid JSON numbers that overflow JavaScript are rejected in context too.
	snapshot := inputTestContext(t, render.Input{Cfg: map[string]any{"private-field": json.Number("1e400")}})
	err := bindInput(context.Background(), goja.New(), Input{Context: snapshot}, inputTestLimits())
	require.ErrorIs(t, err, ErrInvalidInput)
	require.NotContains(t, err.Error(), "private-field")
}

func TestBindInputNumericSemantics(t *testing.T) {
	// JavaScript consumes ordinary Number values, including rounding beyond its
	// safe integer range and underflow to finite zero.
	snapshot := inputTestContext(t, render.Input{Params: map[string]any{
		"integer":  json.Number("9007199254740993"),
		"fraction": json.Number("1.25"),
		"tiny":     json.Number("1e-400"),
	}})
	vm := goja.New()
	require.NoError(t, bindInput(context.Background(), vm, Input{Context: snapshot}, inputTestLimits()))
	require.True(t, inputTestRun(t, vm, `typeof params.integer === "number" && params.integer === 9007199254740992 && params.fraction === 1.25 && params.tiny === 0`).ToBoolean())
}

func TestExpressionPreservesStringIdentifiers(t *testing.T) {
	// Keep precision-sensitive identifiers as strings before either input path
	// reaches JavaScript; converting an already rounded Number would be too late.
	snapshot := inputTestContext(t, render.Input{Params: map[string]any{
		"id": "18446744073709551615",
	}})
	program, err := CompileExpression(`({ inputId: params.id, responseId: response.body.id })`, inputTestLimits())
	require.NoError(t, err)
	result, err := program.Evaluate(context.Background(), Input{
		Context:  snapshot,
		Response: common.RawJSON(`{"body":{"id":"9007199254740993"}}`),
	})

	// Both strings survive parsing, projection, and final JSON serialization.
	require.NoError(t, err)
	require.JSONEq(t, `{"inputId":"18446744073709551615","responseId":"9007199254740993"}`, string(result))
}

func TestBindInputSurrogateEscapes(t *testing.T) {
	for _, response := range []string{
		`{"body":"\ud800"}`, `{"body":"\udfff"}`,
		`{"\ud800":"value"}`, `{"\udc00":"value"}`,
		`{"body":"\ud800\u0041"}`, `{"body":"\ud800\\udc00"}`,
	} {
		t.Run(response, func(t *testing.T) {
			// Unpaired units are valid JSON syntax, but native JSON.parse would
			// replace them. Reject rather than silently changing keys or values.
			err := bindInput(context.Background(), goja.New(), Input{Context: inputTestContext(t, render.Input{}), Response: common.RawJSON(response)}, inputTestLimits())
			require.ErrorIs(t, err, ErrInvalidInput)
		})
	}

	// Paired units, ordinary Unicode, and literal backslash-u sequences survive.
	response := common.RawJSON(`{"body":"\uD83D\uDE00","\ud83d\ude00":"paired key","literal":"\\ud800","escaped":"quoted\" text","unicode":"é"}`)
	vm := goja.New()
	require.NoError(t, bindInput(context.Background(), vm, Input{Context: inputTestContext(t, render.Input{}), Response: response}, inputTestLimits()))
	require.True(t, inputTestRun(t, vm, `response.body === "😀" && response["😀"] === "paired key" && response.literal === "\\ud800" && response.escaped === 'quoted" text' && response.unicode === "é"`).ToBoolean())
}

func TestBindInputCombinedLimits(t *testing.T) {
	// Count encoded snapshot and response bytes together, including JSON escapes.
	snapshot := inputTestContext(t, render.Input{Params: map[string]any{"value": "quoted\nvalue"}})
	encoded, err := snapshot.SnapshotJSON(context.Background(), render.Limits{MaxBytes: 1 << 20, MaxDepth: 64, MaxSteps: 10000})
	require.NoError(t, err)
	response := common.RawJSON(`{"body":"value"}`)
	limits := inputTestLimits()
	limits.MaxInputBytes = len(encoded) + len(response)
	require.NoError(t, bindInput(context.Background(), goja.New(), Input{Context: snapshot, Response: response}, limits))
	limits.MaxInputBytes--
	require.ErrorIs(t, bindInput(context.Background(), goja.New(), Input{Context: snapshot, Response: response}, limits), ErrLimitExceeded)

	// Empty input has seven snapshot values, six keys, and one response object.
	empty := inputTestContext(t, render.Input{})
	limits = inputTestLimits()
	limits.MaxNodes = 14
	limits.MaxDepth = 1
	require.NoError(t, bindInput(context.Background(), goja.New(), Input{Context: empty}, limits))
	limits.MaxNodes = 13
	require.ErrorIs(t, bindInput(context.Background(), goja.New(), Input{Context: empty}, limits), ErrLimitExceeded)

	// Depth applies to snapshot and response roots with the same zero origin.
	limits = inputTestLimits()
	limits.MaxDepth = 1
	require.ErrorIs(t, bindInput(context.Background(), goja.New(), Input{Context: empty, Response: common.RawJSON(`{"body":[1]}`)}, limits), ErrLimitExceeded)
	nested := inputTestContext(t, render.Input{Params: map[string]any{"value": true}})
	require.ErrorIs(t, bindInput(context.Background(), goja.New(), Input{Context: nested}, limits), ErrLimitExceeded)

	// A response that alone exhausts the byte cap is rejected before snapshotting.
	limits = inputTestLimits()
	limits.MaxInputBytes = 32
	err = bindInput(context.Background(), goja.New(), Input{Context: empty, Response: common.RawJSON(strings.Repeat(" ", 32))}, limits)
	require.ErrorIs(t, err, ErrLimitExceeded)
}

func TestBindInputRequiredArgumentsAndCancellation(t *testing.T) {
	// Missing and zero snapshots cannot provide a valid invocation object.
	limits := inputTestLimits()
	valid := Input{Context: inputTestContext(t, render.Input{})}
	require.ErrorIs(t, bindInput(context.Background(), goja.New(), Input{}, limits), ErrInvalidInput)
	require.ErrorIs(t, bindInput(context.Background(), goja.New(), Input{Context: &render.Context{}}, limits), ErrInvalidInput)
	require.ErrorIs(t, bindInput(nil, goja.New(), valid, limits), ErrInvalidInput)
	require.ErrorIs(t, bindInput(context.Background(), nil, valid, limits), ErrInvalidInput)

	// An already canceled invocation neither encodes nor exposes inputs.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	vm := goja.New()
	require.ErrorIs(t, bindInput(canceled, vm, valid, limits), context.Canceled)
	require.Nil(t, vm.Get("params"))
}

// FuzzInputJSON exercises malformed strings, delimiters, and surrogate escapes
// without allocating a VM. Accepted documents must remain valid object JSON.
func FuzzInputJSON(f *testing.F) {
	for _, seed := range []string{
		`{}`, `{"body":"\ud800"}`, `{"body":"\ud83d\ude00"}`,
		`{"body":"\\ud800"}`, `{"body":[null,false,1.25,{"key":"text"}]}`,
		`{"body":"escaped\"quote"}`, `{"body":1e400}`, `{"body":`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, encoded []byte) {
		if len(encoded) > 4096 {
			return
		}
		budget := inputBudget{ctx: context.Background(), limits: inputTestLimits()}
		if err := budget.validateObject(encoded); err != nil {
			require.True(t, err == ErrInvalidInput || err == ErrLimitExceeded)
			return
		}

		// A successful boundary never admits a scalar, trailing value, or syntax
		// error that would fail the subsequent native object parse.
		var object map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(encoded, &object))
		require.NotNil(t, object)
	})
}
