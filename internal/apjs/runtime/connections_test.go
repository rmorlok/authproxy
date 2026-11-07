package runtime_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/dop251/goja"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// connectionInputs binds each supplied snapshot to a distinct client under one
// execution owner. Observations identify the captured client, not editable data.
func connectionInputs(t *testing.T, snapshots ...string) ([]jsruntime.ConnectionInput, chan string) {
	t.Helper()
	owner, err := aphttp.New(context.Background(), aphttp.Limits{
		Timeout: 10 * time.Second, MaxRequests: 16, MaxResponseBytes: 8192,
		MaxStreamBytes: 8192, MaxHeaderBytes: 4096,
	})
	require.NoError(t, err)
	t.Cleanup(owner.Close)
	requests := make(chan string, 32)
	inputs := make([]jsruntime.ConnectionInput, len(snapshots))
	for index, snapshot := range snapshots {
		handle, err := owner.Bind(httpHostClient{name: strconv.Itoa(index), requests: requests})
		require.NoError(t, err)
		inputs[index] = jsruntime.ConnectionInput{
			Snapshot: json.RawMessage(snapshot),
			Dispatcher: connectionTestDispatcher{dispatch: func(request *http.Request, _ map[string]string) (*aphttp.Response, error) {
				response, _, err := handle.Do(request)
				return response, err
			}},
		}
	}
	return inputs, requests
}

// evaluateConnections exercises shared input preparation before initialization;
// capabilities share one worker host and returned data crosses the real encoder.
func evaluateConnections(t *testing.T, source string, input jsruntime.AsyncInput, limits jsruntime.Limits) (json.RawMessage, error) {
	t.Helper()
	program, err := jsruntime.CompileAsync(source, nil, nil, limits)
	require.NoError(t, err)
	return program.Evaluate(context.Background(), func(ctx context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		var host *jsruntime.HTTPHost
		if len(input.Connections) != 0 {
			var err error
			host, err = jsruntime.NewHTTPHost(ctx, vm, jsruntime.HTTPOptions{
				MaxRequestBytes: 8192, MaxPendingOperations: 4,
			}, limits)
			if err != nil {
				return err
			}
			// Register cleanup before input copying can be interrupted.
			invocation.Close, invocation.Start, invocation.Await = host.Close, host.Start, host.Await
		}
		return invocation.PrepareInput(ctx, vm, input, limits, host)
	})
}

// TestAsyncInputDefaults preserves arbitrary JSON params while providing a
// native context and an immutable empty connection array without an HTTP host.
func TestAsyncInputDefaults(t *testing.T) {
	for _, params := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`[1,"two"]`), json.RawMessage(`42`)} {
		result, err := evaluateConnections(t, `async function execute(params, context) {
			return {params, empty: context.connections.length === 0,
				array: Array.isArray(context.connections), frozen: Object.isFrozen(context.connections),
				extra: context.extra, fetch: typeof fetch};
		}`, jsruntime.AsyncInput{Params: params, Context: json.RawMessage(`{"extra":"copied"}`)}, runtimeLimits())

		// Omitted params become {}, while explicit JSON null retains its meaning.
		require.NoError(t, err)
		if params == nil {
			params = json.RawMessage(`{}`)
		}
		require.JSONEq(t, `{"params":`+string(params)+`,"empty":true,"array":true,"frozen":true,"extra":"copied","fetch":"undefined"}`, string(result))
	}

	// Omitting extra context also produces the standard empty connections field.
	result, err := evaluateConnections(t, `async function execute(params, context) { return context; }`, jsruntime.AsyncInput{}, runtimeLimits())
	require.NoError(t, err)
	require.JSONEq(t, `{"connections":[]}`, string(result))
}

// TestAsyncConnectionsPreserveOrderAndBinding lets authored code choose by
// metadata while borrowed methods remain bound to the original admitted client.
func TestAsyncConnectionsPreserveOrderAndBinding(t *testing.T) {
	connections, requests := connectionInputs(t,
		`{"id":"first","name":"West","namespace":"root.west","cfg":{"host":"west.example.test"},"labels":{"region":"west"},"annotations":{"note":"captured"}}`,
		`{"id":"second","labels":{"region":"east"}}`,
	)
	result, err := evaluateConnections(t, `async function execute(params, context) {
		const selected = context.connections.find(c => c.labels.region === params.region);
		const borrowed = selected.fetch;
		const response = await borrowed.call(context.connections[0], "https://example.test/chosen");
		const forged = {...context.connections[0]};
		return {ids: context.connections.map(c => c.id), selected: await response.text(),
			name: selected.name, namespace: selected.namespace, cfg: selected.cfg,
			annotations: selected.annotations, forgedFetch: typeof forged.fetch,
			first: {name: context.connections[0].name, namespace: context.connections[0].namespace}};
	}`, jsruntime.AsyncInput{Params: json.RawMessage(`{"region":"east"}`), Connections: connections}, runtimeLimits())

	// Caller order and omitted metadata survive; neither the receiver nor copied
	// identity fields can select a different HTTP client or acquire a method.
	require.NoError(t, err)
	require.JSONEq(t, `{"ids":["first","second"],"selected":"1","name":"","namespace":"","cfg":{},"annotations":{},"forgedFetch":"undefined","first":{"name":"West","namespace":"root.west"}}`, string(result))
	require.Len(t, requests, 1)
	require.Equal(t, "1:/chosen", <-requests)
}

// TestAsyncConnectionProtection separates immutable capability identity from
// mutable VM-local metadata, and verifies fresh copies on subsequent invocations.
func TestAsyncConnectionProtection(t *testing.T) {
	const snapshot = `{"id":"one","name":"Original","namespace":"root.owner","cfg":{"nested":{"value":"original"}},"labels":{"team":"blue"},"annotations":{"note":"captured"}}`
	for range 2 {
		connections, requests := connectionInputs(t, snapshot)
		result, err := evaluateConnections(t, `async function execute(params, context) {
			const c = context.connections[0];
			const before = [c.cfg.nested.value, c.labels.team, c.annotations.note];
			let denied = 0;
			for (const change of [
				() => context.connections = [], () => context.connections[0] = {},
				() => context.connections.push(c), () => context.connections.length = 0,
				() => c.id = "other", () => c.name = "changed", () => c.namespace = "elsewhere",
				() => c.cfg = {}, () => c.labels = {}, () => c.annotations = {},
				() => c.fetch = () => {}, () => delete c.fetch,
				() => Object.setPrototypeOf(c, null)
			]) { try { change(); } catch (e) { denied++; } }
			c.cfg.nested.value = "local"; c.labels.team = "red"; c.annotations.note = "local";
			const response = await c.fetch("https://example.test/unchanged");
			return {before, denied, id: c.id, after: [c.cfg.nested.value, c.labels.team, c.annotations.note],
				client: await response.text(), frozen: Object.isFrozen(c)};
		}`, jsruntime.AsyncInput{Connections: connections}, runtimeLimits())

		// Shallow freezing protects every identity/capability slot, while copied
		// metadata edits remain useful and never persist to the caller's snapshot.
		require.NoError(t, err)
		require.JSONEq(t, `{"before":["original","blue","captured"],"denied":13,"id":"one","after":["local","red","local"],"client":"0","frozen":true}`, string(result))
		require.Len(t, requests, 1)
		require.Equal(t, snapshot, string(connections[0].Snapshot))
	}
}

// TestAsyncConnectionCapabilitiesCannotBeOutput rejects capability objects even
// though their fetch method is non-enumerable; selected metadata remains JSON.
func TestAsyncConnectionCapabilitiesCannotBeOutput(t *testing.T) {
	for _, expression := range []string{"context.connections[0]", "{nested:context.connections[0]}", "context.connections"} {
		connections, _ := connectionInputs(t, `{"id":"one"}`)
		result, err := evaluateConnections(t, `async function execute(params, context) { return `+expression+`; }`, jsruntime.AsyncInput{Connections: connections}, runtimeLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidOutput)
		require.Nil(t, result)
	}

	// Returning explicitly selected metadata never carries live dispatch authority.
	connections, _ := connectionInputs(t, `{"id":"one","cfg":{"tenant":"acme"}}`)
	result, err := evaluateConnections(t, `async function execute(params, context) {
		const c = context.connections[0]; return {id:c.id, cfg:c.cfg};
	}`, jsruntime.AsyncInput{Connections: connections}, runtimeLimits())
	require.NoError(t, err)
	require.JSONEq(t, `{"id":"one","cfg":{"tenant":"acme"}}`, string(result))
}

// TestAsyncConnectionsIsolateConcurrentCopies permits shared immutable input
// bytes while each invocation receives separate metadata and HTTP capabilities.
func TestAsyncConnectionsIsolateConcurrentCopies(t *testing.T) {
	snapshot := json.RawMessage(`{"id":"shared","cfg":{"counter":1}}`)
	t.Run("evaluations", func(t *testing.T) {
		for index := range 8 {
			t.Run(strconv.Itoa(index), func(t *testing.T) {
				t.Parallel()
				connections, requests := connectionInputs(t, string(snapshot))
				connections[0].Snapshot = snapshot
				result, err := evaluateConnections(t, `async function execute(params, context) {
					const c = context.connections[0];
					const original = c.cfg.counter;
					c.cfg.counter++; c.labels.local = "changed";
					return original;
				}`, jsruntime.AsyncInput{Connections: connections}, runtimeLimits())

				// Every runtime observes the original value despite concurrent edits.
				require.NoError(t, err)
				require.JSONEq(t, `1`, string(result))
				require.Empty(t, requests)
			})
		}
	})
	require.JSONEq(t, `{"id":"shared","cfg":{"counter":1}}`, string(snapshot))
}

// TestAsyncInputRejectsInvalidShapes validates the whole input batch before
// initialization or HTTP, including malformed capabilities after a valid one.
func TestAsyncInputRejectsInvalidShapes(t *testing.T) {
	for _, snapshot := range []string{
		`null`, `[]`, `{}`, `{"id":""}`, `{"id":42}`, `{"id":"valid"}`,
		`{"id":"two","name":null}`, `{"id":"two","namespace":1}`,
		`{"id":"two","cfg":null}`, `{"id":"two","cfg":[]}`,
		`{"id":"two","labels":{"team":1}}`, `{"id":"two","labels":null}`,
		`{"id":"two","annotations":[]}`, `{"id":"two","fetch":"forged"}`,
	} {
		connections, requests := connectionInputs(t, `{"id":"valid"}`, snapshot)
		result, err := evaluateConnections(t, `async function execute(params, context) {
			await context.connections[0].fetch("https://example.test/should-not-send"); return true;
		}`, jsruntime.AsyncInput{Connections: connections}, runtimeLimits())

		// Invalid or duplicate identity and malformed metadata are safe input errors.
		require.ErrorIs(t, err, jsruntime.ErrInvalidInput, snapshot)
		require.Nil(t, result)
		require.Empty(t, requests)
	}
	for _, extra := range []string{`null`, `[]`, `42`, `{"connections":[]}`, `{"connections":null}`} {
		result, err := evaluateConnections(t, `async function execute() { return true; }`, jsruntime.AsyncInput{Context: json.RawMessage(extra)}, runtimeLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidInput, extra)
		require.Nil(t, result)
	}
	for _, input := range []jsruntime.AsyncInput{
		{Params: json.RawMessage{}}, {Context: json.RawMessage{}},
	} {
		// Empty supplied bytes are malformed JSON; only omitted fields default.
		result, err := evaluateConnections(t, `async function execute() { return true; }`, input, runtimeLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
		require.Nil(t, result)
	}

	// A snapshot alone grants no authority: the dispatcher and worker host are required.
	vm, invocation := goja.New(), &jsruntime.AsyncInvocation{}
	connections, requests := connectionInputs(t, `{"id":"one"}`)
	err := invocation.PrepareInput(context.Background(), vm, jsruntime.AsyncInput{Connections: connections}, runtimeLimits(), nil)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	connections[0].Dispatcher = nil
	result, err := evaluateConnections(t, `async function execute() { return true; }`, jsruntime.AsyncInput{Connections: connections}, runtimeLimits())
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, result)
	require.Empty(t, requests)
}

// TestAsyncInputSharesBudgets charges params, extra context, and every connection
// in one batch rather than granting a new allowance to each snapshot.
func TestAsyncInputSharesBudgets(t *testing.T) {
	for _, budget := range []string{"bytes", "nodes", "depth"} {
		t.Run(budget, func(t *testing.T) {
			for _, allowed := range []bool{true, false} {
				connections, requests := connectionInputs(t, `{"id":"one"}`, `{"id":"two"}`)
				input := jsruntime.AsyncInput{Params: json.RawMessage(`{"n":1}`), Context: json.RawMessage(`{"extra":2}`), Connections: connections}
				limits := runtimeLimits()
				switch budget {
				case "bytes":
					limits.MaxInputBytes = len(input.Params) + len(input.Context) + len(connections[0].Snapshot) + len(connections[1].Snapshot)
					if !allowed {
						limits.MaxInputBytes--
					}
				case "nodes":
					// Each of these four objects contributes a container, key, and value.
					limits.MaxNodes = 12
					if !allowed {
						limits.MaxNodes--
					}
				case "depth":
					connections[1].Snapshot = json.RawMessage(`{"id":"two","cfg":{"nested":{"value":1}}}`)
					limits.MaxDepth = 3
					if !allowed {
						limits.MaxDepth--
					}
				}

				// Constant output leaves only initial-input accounting under test.
				result, err := evaluateConnections(t, `async function execute() { return true; }`, input, limits)
				if allowed {
					require.NoError(t, err)
					require.JSONEq(t, `true`, string(result))
				} else {
					require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
					require.Nil(t, result)
				}
				require.Empty(t, requests)
			}
		})
	}
}
