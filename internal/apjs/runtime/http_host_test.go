package runtime_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// httpHostClient records which fixed client received a request and returns its
// identity as the body. The buffered record channel never blocks a worker.
type httpHostClient struct {
	name     string      // Distinguishes capabilities independently of the requested URL.
	requests chan string // Detached request observations consumed after evaluation.
}

// Do owns the upload and returns a finite body without performing network I/O.
func (c httpHostClient) Do(request *http.Request) (*http.Response, aphttp.SendState, error) {
	if request.Body != nil {
		_ = request.Body.Close()
	}
	c.requests <- c.name + ":" + request.URL.Path
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(c.name)),
	}, aphttp.SendStateSent, nil
}

// evaluateHTTPHost binds two authorized clients to one HTTP owner and one VM
// worker allowance. Its host policy deliberately has no Tool-specific inputs.
func evaluateHTTPHost(t *testing.T, source string, maxPending int, requests chan string) json.RawMessage {
	t.Helper()
	limits := runtimeLimits()
	bindings := []jsruntime.HostBinding{{Name: "first"}, {Name: "second"}}
	program, err := jsruntime.CompileAsync(source, nil, bindings, limits)
	require.NoError(t, err)

	session, err := aphttp.New(context.Background(), aphttp.Limits{
		Timeout: 10 * time.Second, MaxRequests: 8, MaxResponseBytes: 8192,
		MaxStreamBytes: 8192, MaxHeaderBytes: 4096,
	})
	require.NoError(t, err)
	defer session.Close()

	// Both bindings retain this owner; constructing the second adds no budget.
	handles := make([]*aphttp.ClientHandle, len(bindings))
	for i, binding := range bindings {
		handles[i], err = session.Bind(httpHostClient{name: binding.Name, requests: requests})
		require.NoError(t, err)
	}
	result, err := program.Evaluate(context.Background(), func(ctx context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		host, err := jsruntime.NewHTTPHost(ctx, vm, jsruntime.HTTPOptions{
			MaxRequestBytes: 8192, MaxPendingOperations: maxPending,
		}, limits)
		if err != nil {
			return err
		}
		// Register cleanup before another VM operation can be interrupted.
		invocation.Close, invocation.Start, invocation.Await = host.Close, host.Start, host.Await
		invocation.Params, invocation.Context = goja.Undefined(), goja.Undefined()
		for i, binding := range bindings {
			handle := handles[i]
			fetch, err := host.BindFetch(func(request *http.Request) (*aphttp.Response, error) {
				response, _, err := handle.Do(request)
				return response, err
			})
			if err != nil {
				return err
			}
			if err := vm.Set(binding.Name, fetch); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)
	return result
}

// TestHTTPHostBindingsShareRequestWorkers rejects another binding while one
// fetch is pending, then permits it after the owner consumes that completion.
func TestHTTPHostBindingsShareRequestWorkers(t *testing.T) {
	requests := make(chan string, 8)
	result := evaluateHTTPHost(t, `async function execute() {
		const pending = first("https://example.test/first");
		const blocked = await second("https://example.test/blocked").then(() => false, () => true);
		const a = await pending;
		const b = await second.call({fetch: first}, "https://example.test/second");
		return {blocked, bodies: [await a.text(), await b.text()]};
	}`, 1, requests)

	// Even an already-finished worker occupies its slot until the VM settles it.
	// A borrowed fetch function remains bound to its original client.
	require.JSONEq(t, `{"blocked":true,"bodies":["first","second"]}`, string(result))
	require.Len(t, requests, 2)
	require.ElementsMatch(t, []string{"first:/first", "second:/second"}, []string{<-requests, <-requests})
}

// TestHTTPHostBindingsShareBodyWorkers counts body reads against the same cap
// as requests, without consuming a response when its read cannot be admitted.
func TestHTTPHostBindingsShareBodyWorkers(t *testing.T) {
	requests := make(chan string, 8)
	result := evaluateHTTPHost(t, `async function execute() {
		const a = await first("https://example.test/first");
		const b = await second("https://example.test/second");
		const reading = a.text();
		const blocked = await Promise.all([
			second("https://example.test/blocked").then(() => false, () => true),
			b.text().then(() => false, () => true)
		]);
		const unused = !b.bodyUsed;
		const firstBody = await reading;
		const secondBody = await b.text();
		return {blocked, unused, bodies: [firstBody, secondBody], consumed: b.bodyUsed};
	}`, 1, requests)

	// Both denied operations leave the other client's response available to retry.
	require.JSONEq(t, `{"blocked":[true,true],"unused":true,"bodies":["first","second"],"consumed":true}`, string(result))
	require.Len(t, requests, 2)
	require.ElementsMatch(t, []string{"first:/first", "second:/second"}, []string{<-requests, <-requests})
}

// TestHTTPHostBindingsWaitForInitialization gates both direct initialization
// calls and initialization microtasks until AsyncInvocation.Start is reached.
func TestHTTPHostBindingsWaitForInitialization(t *testing.T) {
	requests := make(chan string, 8)
	result := evaluateHTTPHost(t, `let blocked = [];
		first("https://example.test/initialization").catch(() => blocked.push("first"));
		Promise.resolve().then(() => second("https://example.test/initialization").catch(() => blocked.push("second")));
		async function execute() {
			const a = await first("https://example.test/execute");
			const b = await second("https://example.test/execute");
			return {blocked: blocked.sort(), bodies: [await a.text(), await b.text()]};
		}`, 2, requests)

	// Handled initialization failures permit execution, but never reach a client.
	require.JSONEq(t, `{"blocked":["first","second"],"bodies":["first","second"]}`, string(result))
	require.Len(t, requests, 2)
	require.ElementsMatch(t, []string{"first:/execute", "second:/execute"}, []string{<-requests, <-requests})
}

// TestHTTPHostRejectsIncompletePolicy prevents a reusable host from accidentally
// bypassing the positive limits that its former Tool-only caller checked.
func TestHTTPHostRejectsIncompletePolicy(t *testing.T) {
	for _, name := range []string{"context", "runtime", "request bytes", "pending operations", "runtime limits"} {
		t.Run(name, func(t *testing.T) {
			ctx, vm := context.Background(), goja.New()
			options := jsruntime.HTTPOptions{MaxRequestBytes: 8192, MaxPendingOperations: 2}
			limits := runtimeLimits()
			switch name {
			case "context":
				ctx = nil
			case "runtime":
				vm = nil
			case "request bytes":
				options.MaxRequestBytes = 0
			case "pending operations":
				options.MaxPendingOperations = 0
			case "runtime limits":
				limits.MaxDepth = 0
			}

			// Invalid inputs return no partially initialized host to clean up.
			host, err := jsruntime.NewHTTPHost(ctx, vm, options, limits)
			require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
			require.Nil(t, host)
		})
	}

	// A valid owner still refuses a missing dispatch capability before VM binding.
	host, err := jsruntime.NewHTTPHost(context.Background(), goja.New(), jsruntime.HTTPOptions{
		MaxRequestBytes: 8192, MaxPendingOperations: 2,
	}, runtimeLimits())
	require.NoError(t, err)
	defer host.Close()
	fetch, err := host.BindFetch(nil)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, fetch)
}
