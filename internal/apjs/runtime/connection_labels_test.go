package runtime_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dop251/goja"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

// connectionTestDispatcher supplies schema validation at the adapter boundary;
// the shared runtime sees only the narrow validation and HTTP capability methods.
type connectionTestDispatcher struct {
	validate func(map[string]string) error // Nil selects normal AuthProxy user-label rules.
	dispatch func(*http.Request, map[string]string) (*aphttp.Response, error)
}

// ValidateLabels uses the supplied test policy or the same validator as real adapters.
func (d connectionTestDispatcher) ValidateLabels(labels map[string]string) error {
	if d.validate != nil {
		return d.validate(labels)
	}
	return meta.ValidateUserLabels(labels)
}

// DoWithLabels transfers the detached request and labels to the scenario.
func (d connectionTestDispatcher) DoWithLabels(request *http.Request, labels map[string]string) (*aphttp.Response, error) {
	return d.dispatch(request, labels)
}

// connectionLabelRequest records per-call metadata without copying it again, so
// tests can detect aliases to authored objects or another request's map.
type connectionLabelRequest struct {
	path   string
	labels map[string]string
}

// connectionLabelInput binds a fake provider to one shared HTTP owner and records
// the actual maps delivered to worker dispatch, after synchronous validation.
func connectionLabelInput(t *testing.T, validate func(map[string]string) error) (jsruntime.AsyncInput, chan connectionLabelRequest) {
	t.Helper()
	owner, err := aphttp.New(context.Background(), directFetchLimits())
	require.NoError(t, err)
	t.Cleanup(owner.Close)
	handle, err := owner.Bind(directFetchClient(func(*http.Request) (*http.Response, aphttp.SendState, error) {
		return directFetchResponse("ok"), aphttp.SendStateSent, nil
	}))
	require.NoError(t, err)
	requests := make(chan connectionLabelRequest, 32)
	return jsruntime.AsyncInput{Connections: []jsruntime.ConnectionInput{{
		Snapshot: json.RawMessage(`{"id":"captured","labels":{"operation":"baseline"}}`),
		Dispatcher: connectionTestDispatcher{validate: validate, dispatch: func(request *http.Request, labels map[string]string) (*aphttp.Response, error) {
			requests <- connectionLabelRequest{path: request.URL.Path, labels: labels}
			response, _, err := handle.Do(request)
			return response, err
		}},
	}}}, requests
}

// evaluateConnectionLabels runs the public connection binding with explicit
// request/worker limits while preserving the normal input and result conversion.
func evaluateConnectionLabels(t *testing.T, source string, input jsruntime.AsyncInput, maxBytes, maxPending int) (json.RawMessage, error) {
	t.Helper()
	limits := runtimeLimits()
	program, err := jsruntime.CompileAsync(source, nil, nil, limits)
	require.NoError(t, err)
	return program.Evaluate(context.Background(), func(ctx context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		host, err := jsruntime.NewHTTPHost(ctx, vm, jsruntime.HTTPOptions{
			MaxRequestBytes: maxBytes, MaxPendingOperations: maxPending,
		}, limits)
		if err != nil {
			return err
		}
		invocation.Close, invocation.Start, invocation.Await = host.Close, host.Start, host.Await
		return invocation.PrepareInput(ctx, vm, input, limits, host)
	})
}

// TestConnectionFetchLabelsAreDetached copies per-call maps before workers can
// observe them and keeps connection metadata independent of request overrides.
func TestConnectionFetchLabelsAreDetached(t *testing.T) {
	input, requests := connectionLabelInput(t, nil)
	result, err := evaluateConnectionLabels(t, `async function execute(params, context) {
		const c = context.connections[0];
		const labels = {operation:"first"};
		const first = c.fetch("https://example.test/first", {labels});
		labels.operation = "second";
		const second = c.fetch("https://example.test/second", {labels});
		labels.operation = "after-dispatch";
		await Promise.all([first, second]);
		return {operation:c.labels.operation, edited:labels.operation};
	}`, input, 4096, 2)

	// Both workers retain the values at their own call boundary; neither override
	// changes the VM's connection metadata or the original Go snapshot document.
	require.NoError(t, err)
	require.JSONEq(t, `{"operation":"baseline","edited":"after-dispatch"}`, string(result))
	require.Len(t, requests, 2)
	observed := map[string]map[string]string{}
	for range 2 {
		request := <-requests
		observed[request.path] = request.labels
	}
	require.Equal(t, map[string]string{"operation": "first"}, observed["/first"])
	require.Equal(t, map[string]string{"operation": "second"}, observed["/second"])
	require.JSONEq(t, `{"id":"captured","labels":{"operation":"baseline"}}`, string(input.Connections[0].Snapshot))
}

// TestConnectionFetchLabelsRejectInvalidInput prevents invalid shapes, authored
// exceptions, and protected/invalid label data from reaching any worker.
func TestConnectionFetchLabelsRejectInvalidInput(t *testing.T) {
	for _, expression := range []string{
		`true`, `1`, `"text"`, `[]`, `() => ({})`, `new Date()`,
		`{operation:1}`, `{operation:null}`, `{operation:undefined}`,
		`{[Symbol("private-symbol")]:"value"}`,
		`{"apxy/act/-/id":"private-actor"}`, `{"apxy/cxn/-/id":"private-connection"}`,
		`{"invalid key":"private-value"}`, `{operation:"private invalid value"}`,
		`{get operation(){throw new Error("private-getter");}}`,
		`new Proxy({}, {ownKeys(){throw new Error("private-trap");}})`,
	} {
		input, requests := connectionLabelInput(t, nil)
		result, err := evaluateConnectionLabels(t, `async function execute(params, context) {
			return await context.connections[0].fetch("https://example.test/rejected", {labels:`+expression+`})
				.then(() => ({rejected:false}), error => ({rejected:true, message:error.message}));
		}`, input, 4096, 1)

		// Promise rejection contains only the fixed public input category.
		require.NoError(t, err, expression)
		require.JSONEq(t, `{"rejected":true,"message":"invalid JavaScript input"}`, string(result), expression)
		require.Empty(t, requests, expression)
	}
}

// TestConnectionFetchLabelsRecordSemantics copies only enumerable own strings;
// omitted, null, and undefined labels add no per-request metadata.
func TestConnectionFetchLabelsRecordSemantics(t *testing.T) {
	for _, init := range []string{
		`{}`, `{labels:null}`, `{labels:undefined}`, `{labels:{}}`,
		`{labels:Object.create({"apxy/act/-/id":"ignored"})}`,
		`{labels:Object.defineProperty({}, "ignored", {value:42, enumerable:false})}`,
	} {
		input, requests := connectionLabelInput(t, nil)
		result, err := evaluateConnectionLabels(t, `async function execute(params, context) {
			return (await context.connections[0].fetch("https://example.test/empty", `+init+`)).ok;
		}`, input, 4096, 1)
		require.NoError(t, err, init)
		require.JSONEq(t, `true`, string(result))
		require.Len(t, requests, 1)
		require.Empty(t, (<-requests).labels)
	}

	// Validation happens before worker admission, so a rejected label map cannot
	// occupy the only slot and block the valid call immediately following it.
	input, requests := connectionLabelInput(t, nil)
	result, err := evaluateConnectionLabels(t, `async function execute(params, context) {
		const c = context.connections[0];
		const rejected = c.fetch("https://example.test/rejected", {labels:{"apxy/act/-/id":"forged"}}).then(() => false, () => true);
		const accepted = c.fetch("https://example.test/accepted", {labels:{operation:"read"}});
		return {rejected:await rejected, accepted:(await accepted).ok};
	}`, input, 4096, 1)
	require.NoError(t, err)
	require.JSONEq(t, `{"rejected":true,"accepted":true}`, string(result))
	require.Len(t, requests, 1)
	require.Equal(t, "/accepted", (<-requests).path)
}

// TestConnectionFetchLabelByteAccounting includes exact UTF-8 key/value bytes
// in the same allowance as method, URL, headers, and body before dispatch.
func TestConnectionFetchLabelByteAccounting(t *testing.T) {
	const source = `async function execute(params, context) {
		return await context.connections[0].fetch("https://example.test/items", {
			method:"POST", headers:{"X-Test":"header"}, body:"body💩",
			labels:{"é":"😀", purpose:"copy"}
		}).then(() => true, () => false);
	}`
	const size = len("https://example.test/items") + len("POST") + len("X-Test") + len("header") + len("body💩") + len("é") + len("😀") + len("purpose") + len("copy")
	for _, maxBytes := range []int{size, size - 1} {
		// This adapter permits Unicode to isolate generic byte conversion from
		// AuthProxy's separate ASCII label policy, exercised above.
		input, requests := connectionLabelInput(t, func(map[string]string) error { return nil })
		result, err := evaluateConnectionLabels(t, source, input, maxBytes, 1)
		require.NoError(t, err)
		if maxBytes == size {
			require.JSONEq(t, `true`, string(result))
			require.Len(t, requests, 1)
			require.Equal(t, map[string]string{"é": "😀", "purpose": "copy"}, (<-requests).labels)
		} else {
			require.JSONEq(t, `false`, string(result))
			require.Empty(t, requests)
		}
	}
}

// TestConnectionFetchLabelsStayDisabledDuringInitialization covers both immediate
// calls and Promise jobs before the execution host enables outbound work.
func TestConnectionFetchLabelsStayDisabledDuringInitialization(t *testing.T) {
	for _, prefix := range []string{
		`connectionFetch("https://example.test/init", {labels:{operation:"read"}});`,
		`Promise.resolve().then(() => connectionFetch("https://example.test/init", {labels:{operation:"read"}}));`,
	} {
		input, requests := connectionLabelInput(t, nil)
		limits := runtimeLimits()
		program, err := jsruntime.CompileAsync(prefix+`async function execute() { return true; }`, nil,
			[]jsruntime.HostBinding{{Name: "connectionFetch"}}, limits)
		require.NoError(t, err)
		result, err := program.Evaluate(context.Background(), func(ctx context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
			host, err := jsruntime.NewHTTPHost(ctx, vm, jsruntime.HTTPOptions{MaxRequestBytes: 4096, MaxPendingOperations: 1}, limits)
			if err != nil {
				return err
			}
			invocation.Close, invocation.Start, invocation.Await = host.Close, host.Start, host.Await
			invocation.Params, invocation.Context = goja.Undefined(), goja.Undefined()
			fetch, err := host.BindConnectionFetch(input.Connections[0].Dispatcher)
			if err != nil {
				return err
			}
			return vm.Set("connectionFetch", fetch)
		})

		// Neither a valid map nor a Promise continuation bypasses the start gate.
		require.ErrorIs(t, err, jsruntime.ErrEvaluation)
		require.Nil(t, result)
		require.Empty(t, requests)
	}
}
