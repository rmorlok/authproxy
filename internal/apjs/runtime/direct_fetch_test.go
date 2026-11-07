package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// directFetchClient supplies one fake transport with explicit target evidence.
// The adapter owns uploads even in scenarios that never inspect their bytes.
type directFetchClient func(*http.Request) (*http.Response, aphttp.SendState, error)

// Do closes the upload after the scenario finishes without accessing the VM.
func (f directFetchClient) Do(request *http.Request) (*http.Response, aphttp.SendState, error) {
	if request.Body != nil {
		defer request.Body.Close()
	}
	return f(request)
}

// directFetchLimits gives mixed requests finite shared allowances.
func directFetchLimits() aphttp.Limits {
	return aphttp.Limits{Timeout: 10 * time.Second, MaxRequests: 8, MaxResponseBytes: 8192,
		MaxStreamBytes: 8192, MaxHeaderBytes: 4096}
}

// directFetchResponse returns detached metadata and a lazily consumed body.
func directFetchResponse(text string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(text))}
}

// directFetchInput binds both clients to the same owner, never a default client.
func directFetchInput(t *testing.T, owner *aphttp.Session, direct, connection aphttp.Client) jsruntime.AsyncInput {
	t.Helper()
	bind := func(client aphttp.Client) func(*http.Request) (*aphttp.Response, error) {
		handle, err := owner.Bind(client)
		require.NoError(t, err)
		return func(request *http.Request) (*aphttp.Response, error) {
			response, _, err := handle.Do(request)
			return response, err
		}
	}
	input := jsruntime.AsyncInput{}
	if direct != nil {
		input.DirectDispatch = bind(direct)
	}
	if connection != nil {
		dispatch := bind(connection)
		input.Connections = []jsruntime.ConnectionInput{{
			Snapshot: json.RawMessage(`{"id":"captured"}`),
			Dispatcher: connectionTestDispatcher{dispatch: func(request *http.Request, _ map[string]string) (*aphttp.Response, error) {
				return dispatch(request)
			}},
		}}
	}
	return input
}

// compileDirectFetch exercises the built-in fetch policy without caller descriptors.
func compileDirectFetch(t *testing.T, source string) *jsruntime.AsyncProgram {
	t.Helper()
	program, err := jsruntime.CompileAsync(source, nil, nil, runtimeLimits())
	require.NoError(t, err)
	return program
}

// runDirectFetch models the trusted adapter's shared owner/error lifecycle.
// It closes the owner on every exit while retaining cumulative evidence.
func runDirectFetch(ctx context.Context, program *jsruntime.AsyncProgram, owner *aphttp.Session, input jsruntime.AsyncInput, maxPending int) (json.RawMessage, error) {
	parent, cancel := context.WithCancel(ctx)
	defer cancel()
	if owner != nil {
		defer owner.Close()
		stop := context.AfterFunc(owner.Context(), cancel)
		defer stop()
	}
	result, err := program.Evaluate(parent, func(runCtx context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		var host *jsruntime.HTTPHost
		if input.DirectDispatch != nil || len(input.Connections) != 0 {
			var err error
			host, err = jsruntime.NewHTTPHost(runCtx, vm, jsruntime.HTTPOptions{
				MaxRequestBytes: 8192, MaxPendingOperations: maxPending,
			}, runtimeLimits())
			if err != nil {
				return err
			}
			invocation.Close, invocation.Start, invocation.Await = host.Close, host.Start, host.Await
		}
		return invocation.PrepareInput(runCtx, vm, input, runtimeLimits(), host)
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if owner != nil && owner.Err() != nil {
		return nil, owner.Err()
	}
	return result, err
}

// TestDirectFetchRequiresExplicitCapability leaves the reserved global disabled
// without a supplied dispatcher and refuses enablement without a worker host.
func TestDirectFetchRequiresExplicitCapability(t *testing.T) {
	program := compileDirectFetch(t, `async function execute(params, context) {
		return {fetch: typeof fetch, connections: context.connections, visible: Object.keys(globalThis).includes("fetch")};
	}`)
	result, err := runDirectFetch(context.Background(), program, nil, jsruntime.AsyncInput{}, 2)
	require.NoError(t, err)
	require.JSONEq(t, `{"fetch":"undefined","connections":[],"visible":false}`, string(result))

	// Providing metadata or a callback alone cannot bypass host lifecycle setup.
	invocation := &jsruntime.AsyncInvocation{}
	err = invocation.PrepareInput(context.Background(), goja.New(), jsruntime.AsyncInput{
		DirectDispatch: func(*http.Request) (*aphttp.Response, error) { panic("must not dispatch") },
	}, runtimeLimits(), nil)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
}

// TestDirectFetchUsesOnlyItsSuppliedClient preserves authored headers and keeps
// failed connection requests from silently falling back to direct networking.
func TestDirectFetchUsesOnlyItsSuppliedClient(t *testing.T) {
	owner, err := aphttp.New(context.Background(), directFetchLimits())
	require.NoError(t, err)
	t.Cleanup(owner.Close)
	directRequests := make(chan *http.Request, 2)
	connectionRequests := make(chan string, 1)
	input := directFetchInput(t, owner,
		directFetchClient(func(request *http.Request) (*http.Response, aphttp.SendState, error) {
			directRequests <- request
			return directFetchResponse("direct"), aphttp.SendStateSent, nil
		}),
		directFetchClient(func(request *http.Request) (*http.Response, aphttp.SendState, error) {
			connectionRequests <- request.URL.Path
			return nil, aphttp.SendStateNotSent, aphttp.ErrCredentialsUnavailable
		}),
	)
	program := compileDirectFetch(t, `async function execute(params, context) {
		const failed = await context.connections[0].fetch("https://example.test/connection").then(() => false, () => true);
		let protectedGlobal = false;
		try { fetch = () => {}; } catch (e) { protectedGlobal = e instanceof TypeError; }
		const unsupported = (await Promise.all([{}, null, undefined, {purpose:"authored"}].map(
			labels => fetch("https://example.test/labels", {labels}).then(() => false, () => true)
		))).every(Boolean);
		const response = await fetch("https://example.test/direct", {headers:{Authorization:"authored", Host:"tenant.example.test"}});
		return {failed, protectedGlobal, unsupported, text:await response.text(), visible:Object.keys(globalThis).includes("fetch")};
	}`)

	// Exactly one explicit direct call reaches its client; the failed connection
	// and rejected extension cannot produce a hidden network attempt.
	result, err := runDirectFetch(context.Background(), program, owner, input, 4)
	require.NoError(t, err)
	require.JSONEq(t, `{"failed":true,"protectedGlobal":true,"unsupported":true,"text":"direct","visible":false}`, string(result))
	require.Len(t, directRequests, 1)
	request := <-directRequests
	require.Equal(t, "/direct", request.URL.Path)
	require.Equal(t, "authored", request.Header.Get("Authorization"))
	require.Equal(t, "tenant.example.test", request.Host)
	require.Len(t, connectionRequests, 1)
	require.Equal(t, aphttp.SendStateSent, owner.SendState())
}

// TestDirectAndConnectionFetchShareLimits proves that direct dispatch does not
// create another request, decoded-response, or pending-worker allowance.
func TestDirectAndConnectionFetchShareLimits(t *testing.T) {
	for _, test := range []struct {
		name      string
		source    string
		requests  int
		response  int64
		pending   int
		want      string
		wantError error
		wantCalls int
	}{
		{"requests", `await context.connections[0].fetch("https://example.test/connection");
			return await fetch("https://example.test/direct").then(() => false, () => true);`, 1, 8192, 4, `true`, nil, 1},
		{"response bytes", `await (await context.connections[0].fetch("https://example.test/connection")).text();
			return await (await fetch("https://example.test/direct")).text();`, 8, 5, 4, ``, aphttp.ErrResponseLimit, 2},
		{"workers", `const request = context.connections[0].fetch("https://example.test/connection");
			const requestRefused = await fetch("https://example.test/blocked-request").then(() => false, () => true);
			const response = await request;
			const reading = response.text();
			const bodyRefused = await fetch("https://example.test/blocked-read").then(() => false, () => true);
			await reading;
			return [requestRefused, bodyRefused, await (await fetch("https://example.test/direct")).text()];`, 8, 8192, 1, `[true,true,"abc"]`, nil, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := directFetchLimits()
			limits.MaxRequests, limits.MaxResponseBytes = test.requests, test.response
			owner, err := aphttp.New(context.Background(), limits)
			require.NoError(t, err)
			t.Cleanup(owner.Close)
			calls := make(chan string, 8)
			client := directFetchClient(func(request *http.Request) (*http.Response, aphttp.SendState, error) {
				calls <- request.URL.Path
				return directFetchResponse("abc"), aphttp.SendStateSent, nil
			})
			input := directFetchInput(t, owner, client, client)
			program := compileDirectFetch(t, `async function execute(params, context) {`+test.source+`}`)

			// A caught admission refusal can succeed; cumulative body exhaustion
			// ends the shared owner and discards even an otherwise valid result.
			result, err := runDirectFetch(context.Background(), program, owner, input, test.pending)
			require.ErrorIs(t, err, test.wantError)
			if test.wantError == nil {
				require.JSONEq(t, test.want, string(result))
			} else {
				require.Nil(t, result)
			}
			require.Len(t, calls, test.wantCalls)
			require.Equal(t, aphttp.SendStateSent, owner.SendState())
		})
	}
}

// TestDirectFetchInitializationNeverDispatches includes Promise jobs scheduled
// by entry-point inspection before the evaluator opens its host I/O gate.
func TestDirectFetchInitializationNeverDispatches(t *testing.T) {
	for _, source := range []string{
		`fetch("https://example.test/top-level"); async function execute() {return true;}`,
		`Promise.resolve().then(() => fetch("https://example.test/job")); async function execute() {return true;}`,
		`async function execute() {return true;}
		 execute = new Proxy(execute, {getPrototypeOf(target) {
			Promise.resolve().then(() => fetch("https://example.test/trap"));
			return Object.getPrototypeOf(target);
		 }});`,
	} {
		owner, err := aphttp.New(context.Background(), directFetchLimits())
		require.NoError(t, err)
		t.Cleanup(owner.Close)
		calls := make(chan string, 1)
		input := directFetchInput(t, owner, directFetchClient(func(request *http.Request) (*http.Response, aphttp.SendState, error) {
			calls <- request.URL.Path
			return directFetchResponse("unexpected"), aphttp.SendStateSent, nil
		}), nil)
		program := compileDirectFetch(t, source)

		// Publication has no capability, and evaluation must still reject every
		// initialization attempt even though execute would have direct access.
		require.ErrorIs(t, program.Validate(context.Background()), jsruntime.ErrEvaluation)
		result, err := runDirectFetch(context.Background(), program, owner, input, 4)
		require.ErrorIs(t, err, jsruntime.ErrEvaluation)
		require.Nil(t, result)
		require.Empty(t, calls)
		require.Equal(t, aphttp.SendStateNotSent, owner.SendState())
	}
}

// directFetchWait bounds lifecycle synchronization without guessing scheduling.
func directFetchWait(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("direct fetch did not reach the expected lifecycle boundary")
	}
}

// TestDirectFetchCancellationStopsMixedRequests propagates caller cancellation
// through the shared owner to both kinds of pending transport call.
func TestDirectFetchCancellationStopsMixedRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	owner, err := aphttp.New(ctx, directFetchLimits())
	require.NoError(t, err)
	t.Cleanup(owner.Close)
	entered, stopped := make(chan time.Time, 2), make(chan struct{}, 2)
	client := directFetchClient(func(request *http.Request) (*http.Response, aphttp.SendState, error) {
		deadline, _ := request.Context().Deadline()
		entered <- deadline
		<-request.Context().Done()
		stopped <- struct{}{}
		return nil, aphttp.SendStateMayHaveSent, errors.New("private transport cancellation")
	})
	input := directFetchInput(t, owner, client, client)
	program := compileDirectFetch(t, `async function execute(params, context) {
		return await Promise.all([fetch("https://example.test/direct"), context.connections[0].fetch("https://example.test/connection")]);
	}`)
	done := make(chan error, 1)
	go func() {
		_, err := runDirectFetch(ctx, program, owner, input, 4)
		done <- err
	}()

	// Both clients have entered before cancellation, so the result cannot be a
	// pre-admission failure and both outstanding requests must observe shutdown.
	var sharedDeadline time.Time
	for range 2 {
		select {
		case deadline := <-entered:
			require.False(t, deadline.IsZero())
			if sharedDeadline.IsZero() {
				sharedDeadline = deadline
			} else {
				require.Equal(t, sharedDeadline, deadline)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("mixed clients did not enter before cancellation")
		}
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("mixed evaluation did not cancel")
	}
	directFetchWait(t, stopped)
	directFetchWait(t, stopped)
	require.Equal(t, aphttp.SendStateMayHaveSent, owner.SendState())
}

// directFetchClosedBody signals eventual cleanup of a transport's late response.
type directFetchClosedBody struct {
	closed chan struct{} // Closed after ownership is released without VM access.
	once   sync.Once     // Cancellation and late-result cleanup may both close it.
}

// Read supplies an empty body without blocking a cleanup-only scenario.
func (b *directFetchClosedBody) Read([]byte) (int, error) { return 0, io.EOF }

// Close records release safely even when independent cleanup paths race.
func (b *directFetchClosedBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

// TestDirectFetchRaceClosesLateBody abandons a direct request after a connection
// wins the root race; its eventual body is closed without returning to the VM.
func TestDirectFetchRaceClosesLateBody(t *testing.T) {
	owner, err := aphttp.New(context.Background(), directFetchLimits())
	require.NoError(t, err)
	t.Cleanup(owner.Close)
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	body := &directFetchClosedBody{closed: make(chan struct{})}
	input := directFetchInput(t, owner,
		directFetchClient(func(*http.Request) (*http.Response, aphttp.SendState, error) {
			close(entered)
			<-release // This transport deliberately ignores cancellation.
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}, aphttp.SendStateSent, nil
		}),
		directFetchClient(func(request *http.Request) (*http.Response, aphttp.SendState, error) {
			select {
			case <-entered:
				return directFetchResponse("winner"), aphttp.SendStateSent, nil
			case <-request.Context().Done():
				return nil, aphttp.SendStateNotSent, request.Context().Err()
			}
		}),
	)
	program := compileDirectFetch(t, `async function execute(params, context) {
		const winner = await Promise.race([fetch("https://example.test/late"), context.connections[0].fetch("https://example.test/winner")]);
		return winner.status;
	}`)

	// Evaluation completes before the uncooperative direct client is released;
	// a confirmed connection send already dominates aggregate uncertainty.
	result, err := runDirectFetch(context.Background(), program, owner, input, 4)
	require.NoError(t, err)
	require.JSONEq(t, `200`, string(result))
	require.Equal(t, aphttp.SendStateSent, owner.SendState())
	unblock()
	directFetchWait(t, body.closed)
}
