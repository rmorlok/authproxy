package javascript_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// fetchClient supplies the captured connection's narrow HTTP capability.
type fetchClient func(*http.Request) (*http.Response, error)

// Do delegates a request to the scenario's transport behavior.
func (f fetchClient) Do(request *http.Request) (*http.Response, error) { return f(request) }

// fetchBody detects eager reads and records resource release across goroutines.
type fetchBody struct {
	reader io.Reader
	reads  atomic.Int32
	closes atomic.Int32
}

// Read forwards to one finite reader and counts each attempt to consume it.
func (b *fetchBody) Read(p []byte) (int, error) {
	b.reads.Add(1)
	return b.reader.Read(p)
}

// Close records release without accessing the potentially active reader.
func (b *fetchBody) Close() error { b.closes.Add(1); return nil }

// fetchBlockedBody models a transport read that only Close can unblock.
type fetchBlockedBody struct {
	started   chan struct{} // Signals that the worker is blocked inside Read.
	closed    chan struct{} // Signals resource release and wakes the blocked reader.
	returned  chan struct{} // Signals that the reader has left the blocked call.
	closeOnce sync.Once     // Close can race with cancellation and invocation cleanup.
}

// Read waits for transport closure before reporting a private read diagnostic.
func (b *fetchBlockedBody) Read([]byte) (int, error) {
	close(b.started)
	defer close(b.returned)
	<-b.closed
	return 0, errors.New("private-body-read-failure")
}

// Close interrupts the transport read independently of the host's read mutex.
func (b *fetchBlockedBody) Close() error {
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

// fetchBlockedCloseBody models a transport whose Close ignores cancellation.
type fetchBlockedCloseBody struct {
	started  chan struct{} // Signals that cleanup has entered raw transport Close.
	release  chan struct{} // Test-owned gate that allows Close to return.
	returned chan struct{} // Signals completion so the test leaves no blocked cleanup.
}

// Read supplies an empty body; the metadata-only scenario must not need it.
func (b *fetchBlockedCloseBody) Read([]byte) (int, error) { return 0, io.EOF }

// Close remains blocked until the test proves evaluation has already returned.
func (b *fetchBlockedCloseBody) Close() error {
	close(b.started)
	defer close(b.returned)
	<-b.release
	return nil
}

// fetchLimits gives normal examples enough room without disabling any host policy.
func fetchLimits() aphttp.Limits {
	return aphttp.Limits{
		Timeout: 10 * time.Second, MaxRequests: 8, MaxResponseBytes: 8192,
		MaxStreamBytes: 8192, MaxHeaderBytes: 4096,
	}
}

// fetchOptions creates a dedicated invocation session with idempotent fallback cleanup.
func fetchOptions(t *testing.T, ctx context.Context, client fetchClient, limits aphttp.Limits) javascript.FetchOptions {
	t.Helper()
	session, err := httphost.New(ctx, client, limits)
	require.NoError(t, err)
	t.Cleanup(session.Close)
	return javascript.FetchOptions{Session: session, MaxRequestBytes: 8192, MaxPendingOperations: 8}
}

// fetchInput captures one connection identity with otherwise empty invocation data.
func fetchInput(t *testing.T) javascript.Input {
	t.Helper()
	return javascript.Input{Context: expressionContext(t, render.Input{
		ConnectionID: "cxn_example", ConnectionName: "Example", ConnectionNamespace: "root.product.owner",
	})}
}

// fetchResponse builds a valid streaming response whose owner must close its body.
func fetchResponse(status int, body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: body}
}

// fetchWait bounds a synchronization point without sleeping to guess progress.
func fetchWait(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("fetch operation did not reach its synchronization point")
	}
}

// TestToolFetchOptionsRequirePositiveLimits rejects incomplete host policy before evaluation.
func TestToolFetchOptionsRequirePositiveLimits(t *testing.T) {
	program := compileTool(t, `async function execute() { return true; }`, nil)
	input := fetchInput(t)
	for _, test := range []struct {
		name   string
		change func(*javascript.FetchOptions)
	}{
		{"session", func(o *javascript.FetchOptions) { o.Session = nil }},
		{"uninitialized session", func(o *javascript.FetchOptions) { o.Session = &httphost.Session{} }},
		{"request bytes", func(o *javascript.FetchOptions) { o.MaxRequestBytes = 0 }},
		{"pending operations", func(o *javascript.FetchOptions) { o.MaxPendingOperations = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
				return fetchResponse(http.StatusOK, http.NoBody), nil
			}, fetchLimits())
			test.change(&options)

			// Missing policy is an input error even when the source never calls fetch.
			result, err := program.EvaluateWithHTTP(context.Background(), input, options)
			require.ErrorIs(t, err, apjsruntime.ErrInvalidInput)
			require.Nil(t, result)

			// A valid transferred session is consumed even when another option is invalid.
			if options.Session != nil && options.Session.Context() != nil {
				require.ErrorIs(t, options.Session.Err(), aphttp.ErrClosed)
			}
		})
	}
}

// TestToolFetchInvalidEvaluationClosesSession consumes a valid session on every early failure path.
func TestToolFetchInvalidEvaluationClosesSession(t *testing.T) {
	program := compileTool(t, `async function execute() { return true; }`, nil)
	input := fetchInput(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		name    string
		program *javascript.ToolProgram
		ctx     context.Context
		input   javascript.Input
		want    error
	}{
		{"nil program", nil, context.Background(), input, apjsruntime.ErrInvalidInput},
		{"uninitialized program", &javascript.ToolProgram{}, context.Background(), input, apjsruntime.ErrInvalidInput},
		{"nil context", program, nil, input, apjsruntime.ErrInvalidInput},
		{"invalid input", program, context.Background(), javascript.Input{}, apjsruntime.ErrInvalidInput},
		{"canceled context", program, canceled, input, context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return fetchResponse(http.StatusOK, http.NoBody), nil
			}, fetchLimits())

			// Ownership transfers before validating the program, context, and captured input.
			result, err := test.program.EvaluateWithHTTP(test.ctx, test.input, options)
			require.ErrorIs(t, err, test.want)
			require.Nil(t, result)
			require.ErrorIs(t, options.Session.Err(), aphttp.ErrClosed)
			require.Zero(t, calls.Load())
			require.Equal(t, api.ToolExecutionStateNotStarted, options.Session.ExecutionState())
		})
	}
}

// TestToolFetchSequentialRequests passes one endpoint's result to the next request.
func TestToolFetchSequentialRequests(t *testing.T) {
	var calls atomic.Int32
	firstBody := &fetchBody{reader: strings.NewReader(`{"id":"contact-7"}`)}
	secondBody := &fetchBody{reader: strings.NewReader("saved")}
	options := fetchOptions(t, context.Background(), func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		switch request.URL.Path {
		case "/contacts":
			response := fetchResponse(http.StatusOK, firstBody)
			response.Header.Set("Content-Type", "application/json")
			response.Header.Set("X-Request-ID", "trace-7")
			response.Header.Set("Authorization", "private-credential")
			return response, nil
		case "/contacts/contact-7":
			defer request.Body.Close()
			body, err := io.ReadAll(request.Body)
			if err != nil || request.Method != http.MethodPost ||
				request.Header.Get("Content-Type") != "application/json" || string(body) != `{"enabled":true}` {
				return nil, errors.New("unexpected request configuration")
			}
			return fetchResponse(http.StatusCreated, secondBody), nil
		default:
			return nil, errors.New("unexpected endpoint")
		}
	}, fetchLimits())
	program := compileTool(t, `async function execute() {
		const first = await context.connections[0].fetch("https://provider.example/contacts");
		const contact = await first.json();
		const second = await context.connections[0].fetch("https://provider.example/contacts/" + contact.id, {
			method: "POST", headers: {"Content-Type": "application/json"},
			body: JSON.stringify({enabled: true})
		});
		return {status: second.status, ok: second.ok, text: await second.text(),
			trace: first.headers.get("x-request-id"), secret: first.headers.get("authorization")};
	}`, nil)

	// A resolved fetch exposes safe metadata; body methods drive the next async step.
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.NoError(t, err)
	require.JSONEq(t, `{"status":201,"ok":true,"text":"saved","trace":"trace-7","secret":null}`, string(result))
	require.Equal(t, int32(2), calls.Load())
	require.Eventually(t, func() bool { return firstBody.closes.Load() == 1 }, 5*time.Second, time.Millisecond)
	require.Eventually(t, func() bool { return secondBody.closes.Load() == 1 }, 5*time.Second, time.Millisecond)
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, options.Session.ExecutionState())

	// Evaluation closes the dedicated session before returning, preventing late admissions.
	require.ErrorIs(t, options.Session.Err(), aphttp.ErrClosed)
	request, err := http.NewRequest(http.MethodGet, "https://provider.example/late", nil)
	require.NoError(t, err)
	response, err := options.Session.Do(request)
	require.ErrorIs(t, err, aphttp.ErrClosed)
	require.Nil(t, response)
	require.Equal(t, int32(2), calls.Load())
}

// TestToolFetchParallelRequests requires both transports to start before either completes.
func TestToolFetchParallelRequests(t *testing.T) {
	var entered atomic.Int32
	bothStarted := make(chan struct{})
	options := fetchOptions(t, context.Background(), func(request *http.Request) (*http.Response, error) {
		if entered.Add(1) == 2 {
			close(bothStarted)
		}
		select {
		case <-bothStarted:
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
		return fetchResponse(http.StatusOK, io.NopCloser(strings.NewReader(request.URL.Path))), nil
	}, fetchLimits())
	program := compileTool(t, `async function execute() {
		return Promise.all(["/one", "/two"].map(async path => {
			const response = await context.connections[0].fetch("https://provider.example" + path);
			return await response.text();
		}));
	}`, nil)

	// Concurrent HTTP completion still resumes every continuation on its owning VM.
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.NoError(t, err)
	require.JSONEq(t, `["/one","/two"]`, string(result))
	require.Equal(t, int32(2), entered.Load())
}

// TestToolFetchStatusDoesNotReadBody permits status checks before consuming error content.
func TestToolFetchStatusDoesNotReadBody(t *testing.T) {
	body := &fetchBody{reader: strings.NewReader("private-provider-error")}
	options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
		response := fetchResponse(http.StatusServiceUnavailable, body)
		response.Header.Set("Content-Encoding", "unsupported")
		return response, nil
	}, fetchLimits())
	program := compileTool(t, `async function execute() {
		const response = await context.connections[0].fetch("https://provider.example/items");
		return {status: response.status, ok: response.ok, used: response.bodyUsed};
	}`, nil)

	// Non-2xx statuses resolve normally and unused bodies close without decoding.
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.NoError(t, err)
	require.JSONEq(t, `{"status":503,"ok":false,"used":false}`, string(result))
	require.Zero(t, body.reads.Load())
	require.Eventually(t, func() bool { return body.closes.Load() == 1 }, 5*time.Second, time.Millisecond)
}

// TestToolFetchBodyIsSingleUse shares consumption state across all three body methods.
func TestToolFetchBodyIsSingleUse(t *testing.T) {
	for _, method := range []string{"text", "json", "arrayBuffer"} {
		t.Run(method, func(t *testing.T) {
			body := &fetchBody{reader: strings.NewReader(`{"n":7}`)}
			options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
				return fetchResponse(http.StatusOK, body), nil
			}, fetchLimits())
			program := compileTool(t, fmt.Sprintf(`async function execute() {
				const response = await context.connections[0].fetch("https://provider.example/items");
				const before = response.bodyUsed;
				const pending = response.%s();
				const during = response.bodyUsed;
				const value = await pending;
				let refused = false;
				try { await response.text(); } catch (err) { refused = err instanceof Error; }
				return {before, during, after: response.bodyUsed, refused,
					value: value instanceof ArrayBuffer ? Array.from(new Uint8Array(value)) : value};
			}`, method), nil)

			// Claiming the first read marks the body used before its Promise settles.
			result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
			require.NoError(t, err)
			value := `"{\"n\":7}"`
			if method == "json" {
				value = `{"n":7}`
			} else if method == "arrayBuffer" {
				value = `[123,34,110,34,58,55,125]`
			}
			require.JSONEq(t, `{"before":false,"during":true,"after":true,"refused":true,"value":`+value+`}`, string(result))
			require.Eventually(t, func() bool { return body.closes.Load() == 1 }, 5*time.Second, time.Millisecond)
		})
	}
}

// TestToolFetchBodyReadCanRetryAfterPendingLimit leaves an unadmitted read unconsumed.
func TestToolFetchBodyReadCanRetryAfterPendingLimit(t *testing.T) {
	body := &fetchBody{reader: strings.NewReader("retained body")}
	options := fetchOptions(t, context.Background(), func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/body" {
			return fetchResponse(http.StatusOK, body), nil
		}
		return fetchResponse(http.StatusOK, http.NoBody), nil
	}, fetchLimits())
	options.MaxPendingOperations = 1
	program := compileTool(t, `async function execute() {
		const response = await context.connections[0].fetch("https://provider.example/body");
		const other = context.connections[0].fetch("https://provider.example/other");
		let refused = false;
		try { await response.text(); }
		catch (err) { refused = err instanceof Error; }
		const usedAfterRefusal = response.bodyUsed;
		await other;
		const text = await response.text();
		return {refused, usedAfterRefusal, text, usedAfterRead: response.bodyUsed};
	}`, nil)

	// The other fetch occupies the only slot; refusing a reader must not claim its body.
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.NoError(t, err)
	require.JSONEq(t, `{"refused":true,"usedAfterRefusal":false,"text":"retained body","usedAfterRead":true}`, string(result))
	require.Eventually(t, func() bool { return body.closes.Load() == 1 }, 5*time.Second, time.Millisecond)
}

// TestToolFetchArrayBufferRequest transfers explicit bytes without serializing a Go object.
func TestToolFetchArrayBufferRequest(t *testing.T) {
	var observed []byte
	options := fetchOptions(t, context.Background(), func(request *http.Request) (*http.Response, error) {
		defer request.Body.Close()
		var err error
		observed, err = io.ReadAll(request.Body)
		return fetchResponse(http.StatusNoContent, http.NoBody), err
	}, fetchLimits())
	program := compileTool(t, `async function execute() {
		const response = await context.connections[0].fetch("https://provider.example/upload", {
			method: "POST", body: new Uint8Array([0, 128, 255]).buffer
		});
		return response.status;
	}`, nil)

	// Binary request bytes survive the VM-to-worker copy without text conversion.
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.NoError(t, err)
	require.JSONEq(t, `204`, string(result))
	require.Equal(t, []byte{0, 128, 255}, observed)
}

// TestToolFetchSanitizesTransportFailure protects both caught and uncaught error boundaries.
func TestToolFetchSanitizesTransportFailure(t *testing.T) {
	for _, caught := range []bool{false, true} {
		t.Run(fmt.Sprint(caught), func(t *testing.T) {
			options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
				return nil, errors.New("private-token https://secret.example/path")
			}, fetchLimits())
			body := `return await context.connections[0].fetch("https://provider.example/items");`
			if caught {
				body = `try { await context.connections[0].fetch("https://provider.example/items"); }
					catch (err) { return {error: err instanceof Error, message: err.message}; }`
			}
			program := compileTool(t, "async function execute() {"+body+"}", nil)

			// Authored handlers get a useful Error but never the underlying transport diagnostic.
			result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
			if caught {
				require.NoError(t, err)
				require.Contains(t, string(result), `"error":true`)
			} else {
				require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
				require.Nil(t, result)
				require.NotContains(t, err.Error(), "private-token")
			}
			require.NotContains(t, string(result), "private-token")
			require.NotContains(t, string(result), "secret.example")
			require.Equal(t, api.ToolExecutionStateMayHaveExecuted, options.Session.ExecutionState())
		})
	}
}

// TestToolFetchJSONUsesPristineParser prevents source initialization from replacing host decoding.
func TestToolFetchJSONUsesPristineParser(t *testing.T) {
	options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
		return fetchResponse(http.StatusOK, io.NopCloser(strings.NewReader(`{"n":7}`))), nil
	}, fetchLimits())
	program := compileTool(t, `
		JSON.parse = () => ({tampered: true});
		async function execute() {
			return await (await context.connections[0].fetch("https://provider.example/items")).json();
		}
	`, nil)

	// Response parsing uses the intrinsic captured before either library or Tool initialization.
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.NoError(t, err)
	require.JSONEq(t, `{"n":7}`, string(result))
}

// TestToolFetchJSONBudgetsAreSeparate keeps response conversion independent of initial inputs and other bodies.
func TestToolFetchJSONBudgetsAreSeparate(t *testing.T) {
	// Each body exceeds the initial-input byte cap, but fits the HTTP allowance.
	// Two bodies together exceed one traversal allowance; each must get its own.
	body := `["` + strings.Repeat("x", 512) + `"` + strings.Repeat(`,null`, 20) + `]`
	options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
		return fetchResponse(http.StatusOK, io.NopCloser(strings.NewReader(body))), nil
	}, fetchLimits())
	limits := expressionLimits()
	limits.MaxInputBytes = 256
	limits.MaxNodes = 32
	limits.MaxDepth = 2
	program, err := javascript.CompileTool(`async function execute() {
		const first = await (await context.connections[0].fetch("https://provider.example/first")).json();
		const second = await (await context.connections[0].fetch("https://provider.example/second")).json();
		return [first.length, second.length];
	}`, nil, limits)
	require.NoError(t, err)

	// Shared JSON conversion preserves the HTTP host's per-response traversal policy.
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.NoError(t, err)
	require.JSONEq(t, `[21,21]`, string(result))
}

// TestToolFetchInvalidJSONSanitizesParserErrors keeps response content out of JavaScript errors.
func TestToolFetchInvalidJSONSanitizesParserErrors(t *testing.T) {
	body := &fetchBody{reader: strings.NewReader(`private-malformed-response`)}
	options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
		return fetchResponse(http.StatusOK, body), nil
	}, fetchLimits())
	program := compileTool(t, `async function execute() {
		const response = await context.connections[0].fetch("https://provider.example/items");
		try { await response.json(); }
		catch (err) { return {error: err instanceof Error, message: err.message, used: response.bodyUsed}; }
	}`, nil)

	// Failed decoding consumes and closes the body but exposes only a safe error.
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.NoError(t, err)
	require.Contains(t, string(result), `"error":true`)
	require.Contains(t, string(result), `"used":true`)
	require.NotContains(t, string(result), "private-malformed-response")
	require.Eventually(t, func() bool { return body.closes.Load() == 1 }, 5*time.Second, time.Millisecond)
}

// TestToolFetchInitializationHasNoIO preserves publication's no-network initialization boundary.
func TestToolFetchInitializationHasNoIO(t *testing.T) {
	var calls atomic.Int32
	options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return fetchResponse(http.StatusOK, http.NoBody), nil
	}, fetchLimits())
	program := compileTool(t, `context.connections[0].fetch("https://provider.example/items");
		async function execute() { return true; }`, nil)

	// Enabling HTTP for execute must not enable it for top-level source or helpers.
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
	require.Nil(t, result)
	require.Zero(t, calls.Load())
	require.Equal(t, api.ToolExecutionStateNotStarted, options.Session.ExecutionState())
}

// TestToolFetchEntrypointTrapCannotScheduleIO drains initialization jobs before enabling fetch.
func TestToolFetchEntrypointTrapCannotScheduleIO(t *testing.T) {
	var calls atomic.Int32
	options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return fetchResponse(http.StatusOK, http.NoBody), nil
	}, fetchLimits())
	program := compileTool(t, `
		async function execute() { return true; }
		execute = new Proxy(execute, {
			getPrototypeOf(target) {
				Promise.resolve().then(() => context.connections[0].fetch("https://example.test"));
				return Object.getPrototypeOf(target);
			}
		});
	`, nil)

	// Inspecting the entry point is still initialization, including its Proxy's jobs.
	require.ErrorIs(t, program.Validate(context.Background()), apjsruntime.ErrEvaluation)

	// A job scheduled by that inspection cannot cross into the enabled HTTP phase.
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
	require.Nil(t, result)
	require.Zero(t, calls.Load())
	require.Equal(t, api.ToolExecutionStateNotStarted, options.Session.ExecutionState())
}

// TestToolFetchHandledInitializationHasNoIO keeps caught initialization failures from authorizing a send.
func TestToolFetchHandledInitializationHasNoIO(t *testing.T) {
	var calls atomic.Int32
	options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return fetchResponse(http.StatusOK, http.NoBody), nil
	}, fetchLimits())
	program := compileTool(t, `
		Promise.resolve().then(() => context.connections[0].fetch("https://example.test")).catch(() => {});
		async function execute() { return true; }
	`, nil)

	// An intentionally caught initialization error permits validation without I/O.
	require.NoError(t, program.Validate(context.Background()))
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.NoError(t, err)
	require.JSONEq(t, `true`, string(result))
	require.Zero(t, calls.Load())
	require.Equal(t, api.ToolExecutionStateNotStarted, options.Session.ExecutionState())
}

// TestToolFetchLimits applies request admission and shared response accounting across calls.
func TestToolFetchLimits(t *testing.T) {
	t.Run("request count", func(t *testing.T) {
		var calls atomic.Int32
		limits := fetchLimits()
		limits.MaxRequests = 1
		options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return fetchResponse(http.StatusOK, http.NoBody), nil
		}, limits)
		program := compileTool(t, `async function execute() {
			await context.connections[0].fetch("https://provider.example/one");
			try { await context.connections[0].fetch("https://provider.example/two"); }
			catch (err) { return err instanceof Error; }
			return false;
		}`, nil)

		// A rejected second handoff does not reach the connection's client.
		result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
		require.NoError(t, err)
		require.JSONEq(t, `true`, string(result))
		require.Equal(t, int32(1), calls.Load())
	})

	t.Run("request bytes", func(t *testing.T) {
		var calls atomic.Int32
		options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return fetchResponse(http.StatusOK, http.NoBody), nil
		}, fetchLimits())
		options.MaxRequestBytes = 128
		program := compileTool(t, `async function execute() {
			try { await context.connections[0].fetch("https://provider.example/items", {method: "POST", body: "x".repeat(129)}); }
			catch (err) { return err instanceof Error; }
			return false;
		}`, nil)

		// Oversized input is rejected before worker allocation or any target handoff.
		result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
		require.NoError(t, err)
		require.JSONEq(t, `true`, string(result))
		require.Zero(t, calls.Load())
		require.Equal(t, api.ToolExecutionStateNotStarted, options.Session.ExecutionState())
	})

	t.Run("pending operations", func(t *testing.T) {
		var calls atomic.Int32
		options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return fetchResponse(http.StatusOK, http.NoBody), nil
		}, fetchLimits())
		options.MaxPendingOperations = 1
		program := compileTool(t, `async function execute() {
			const first = context.connections[0].fetch("https://provider.example/one");
			let refused = false;
			try { await context.connections[0].fetch("https://provider.example/two"); }
			catch (err) { refused = err instanceof Error; }
			await first;
			return refused;
		}`, nil)

		// Pending completions still consume capacity until the owner resumes them.
		result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
		require.NoError(t, err)
		require.JSONEq(t, `true`, string(result))
		require.Equal(t, int32(1), calls.Load())
	})

	t.Run("aggregate response bytes", func(t *testing.T) {
		limits := fetchLimits()
		limits.MaxResponseBytes = 5
		options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
			return fetchResponse(http.StatusOK, io.NopCloser(strings.NewReader("abc"))), nil
		}, limits)
		program := compileTool(t, `async function execute() {
			await (await context.connections[0].fetch("https://provider.example/one")).text();
			try { await (await context.connections[0].fetch("https://provider.example/two")).text(); }
			catch (err) { return "caught"; }
		}`, nil)

		// Catching a body rejection cannot recover an exhausted invocation-wide budget.
		result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
		require.ErrorIs(t, err, aphttp.ErrResponseLimit)
		require.Nil(t, result)
		require.Equal(t, api.ToolExecutionStateMayHaveExecuted, options.Session.ExecutionState())
	})
}

// TestToolFetchCancellation interrupts both a suspended Promise and a busy JavaScript loop.
func TestToolFetchCancellation(t *testing.T) {
	for _, policy := range []bool{false, true} {
		for _, busy := range []bool{false, true} {
			t.Run(fmt.Sprintf("policy=%v/busy=%v", policy, busy), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				started, stopped := make(chan struct{}), make(chan struct{})
				limits := fetchLimits()
				if policy {
					limits.Timeout = 500 * time.Millisecond
				}
				options := fetchOptions(t, ctx, func(request *http.Request) (*http.Response, error) {
					close(started)
					<-request.Context().Done()
					close(stopped)
					return nil, request.Context().Err()
				}, limits)
				body := `return await context.connections[0].fetch("https://provider.example/items");`
				if busy {
					body = `context.connections[0].fetch("https://provider.example/items"); while (true) {}`
				}
				jsLimits := expressionLimits()
				jsLimits.Timeout = 5 * time.Second
				program, err := javascript.CompileTool("async function execute() {"+body+"}", nil, jsLimits)
				require.NoError(t, err)
				input := fetchInput(t)
				var result common.RawJSON
				var evaluationErr error
				done := make(chan struct{})
				go func() {
					defer close(done)
					result, evaluationErr = program.EvaluateWithHTTP(ctx, input, options)
				}()

				// Wait for real in-flight HTTP before triggering caller cancellation.
				fetchWait(t, started)
				if !policy {
					cancel()
				}
				fetchWait(t, done)
				fetchWait(t, stopped)

				// Session policy and caller cancellation keep their distinct classifications.
				if policy {
					require.ErrorIs(t, evaluationErr, aphttp.ErrTimeout)
				} else {
					require.ErrorIs(t, evaluationErr, context.Canceled)
				}
				require.Nil(t, result)
				require.Equal(t, api.ToolExecutionStateMayHaveExecuted, options.Session.ExecutionState())
			})
		}
	}
}

// TestToolFetchCancellationClosesBlockedBody releases a pending read independently of worker progress.
func TestToolFetchCancellationClosesBlockedBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &fetchBlockedBody{started: make(chan struct{}), closed: make(chan struct{}), returned: make(chan struct{})}
	options := fetchOptions(t, ctx, func(*http.Request) (*http.Response, error) {
		return fetchResponse(http.StatusOK, body), nil
	}, fetchLimits())
	program := compileTool(t, `async function execute() {
		return await (await context.connections[0].fetch("https://provider.example/items")).text();
	}`, nil)
	input := fetchInput(t)
	var result common.RawJSON
	var evaluationErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		result, evaluationErr = program.EvaluateWithHTTP(ctx, input, options)
	}()

	// Cancel only after the response body is actively blocking, not merely the request.
	fetchWait(t, body.started)
	cancel()
	fetchWait(t, done)
	fetchWait(t, body.closed)
	fetchWait(t, body.returned)

	// Cleanup unblocks the reader and returns the caller's cancellation without private text.
	require.ErrorIs(t, evaluationErr, context.Canceled)
	require.Nil(t, result)
	require.NotContains(t, evaluationErr.Error(), "private-body-read-failure")
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, options.Session.ExecutionState())
}

// TestToolFetchCleanupDoesNotWaitForBlockedClose prevents transport cleanup from holding the VM owner.
func TestToolFetchCleanupDoesNotWaitForBlockedClose(t *testing.T) {
	body := &fetchBlockedCloseBody{started: make(chan struct{}), release: make(chan struct{}), returned: make(chan struct{})}
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(body.release) }) }
	defer unblock()
	options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
		return fetchResponse(http.StatusOK, body), nil
	}, fetchLimits())
	program := compileTool(t, `async function execute() {
		return (await context.connections[0].fetch("https://provider.example/items")).status;
	}`, nil)
	input := fetchInput(t)
	var result common.RawJSON
	var evaluationErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		result, evaluationErr = program.EvaluateWithHTTP(context.Background(), input, options)
	}()

	// Confirm cleanup is blocked in raw Close, then require evaluation to return independently.
	fetchWait(t, body.started)
	fetchWait(t, done)
	require.NoError(t, evaluationErr)
	require.JSONEq(t, `200`, string(result))
	require.ErrorIs(t, options.Session.Err(), aphttp.ErrClosed)
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, options.Session.ExecutionState())

	// Release the deliberately uncooperative transport after all result assertions.
	unblock()
	fetchWait(t, body.returned)
}

// TestToolFetchAbandonedRequestClosesLateBody handles a client that ignores cancellation until later.
func TestToolFetchAbandonedRequestClosesLateBody(t *testing.T) {
	started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	body := &fetchBody{reader: strings.NewReader("private-late-response")}
	options := fetchOptions(t, context.Background(), func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/late" {
			close(started)
			<-request.Context().Done()
			close(canceled)
			<-release
			return fetchResponse(http.StatusOK, body), nil
		}
		<-started
		return fetchResponse(http.StatusOK, http.NoBody), nil
	}, fetchLimits())
	program := compileTool(t, `async function execute() {
		context.connections[0].fetch("https://provider.example/late");
		await context.connections[0].fetch("https://provider.example/ready");
		return true;
	}`, nil)

	// A settled root preserves its result while cleanup cancels outstanding operations.
	result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
	require.NoError(t, err)
	require.JSONEq(t, `true`, string(result))
	fetchWait(t, canceled)
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, options.Session.ExecutionState())

	// Returning after VM teardown must close the body without consuming or resuming it.
	unblock()
	require.Eventually(t, func() bool { return body.closes.Load() == 1 }, 5*time.Second, time.Millisecond)
	require.Zero(t, body.reads.Load())
}

// TestToolFetchCombinatorsCancelRemainingRequests preserves authored early-settlement behavior.
func TestToolFetchCombinatorsCancelRemainingRequests(t *testing.T) {
	for _, test := range []struct {
		name        string
		source      string
		winnerFails bool
		want        string
	}{
		{"caught all failure", `
			try { await Promise.all([context.connections[0].fetch("https://provider.example/slow"), context.connections[0].fetch("https://provider.example/winner")]); }
			catch (err) { return {handled: err instanceof Error}; }
		`, true, `{"handled":true}`},
		{"race success", `
			return (await Promise.race([context.connections[0].fetch("https://provider.example/slow"), context.connections[0].fetch("https://provider.example/winner")])).status;
		`, false, `200`},
		{"any success", `
			return (await Promise.any([context.connections[0].fetch("https://provider.example/slow"), context.connections[0].fetch("https://provider.example/winner")])).status;
		`, false, `200`},
	} {
		t.Run(test.name, func(t *testing.T) {
			started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			body := &fetchBody{reader: strings.NewReader("late losing response")}
			options := fetchOptions(t, context.Background(), func(request *http.Request) (*http.Response, error) {
				if request.URL.Path == "/slow" {
					close(started)
					<-request.Context().Done()
					close(canceled)
					<-release
					return fetchResponse(http.StatusOK, body), nil
				}
				<-started
				if test.winnerFails {
					return nil, errors.New("private-winner-transport-failure")
				}
				return fetchResponse(http.StatusOK, http.NoBody), nil
			}, fetchLimits())
			program := compileTool(t, "async function execute() {"+test.source+"}", nil)

			// The combinator settles while its losing request remains blocked in the transport.
			result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
			require.NoError(t, err)
			require.JSONEq(t, test.want, string(result))
			fetchWait(t, canceled)
			require.ErrorIs(t, options.Session.Err(), aphttp.ErrClosed)
			require.Equal(t, api.ToolExecutionStateMayHaveExecuted, options.Session.ExecutionState())

			// Cleanup cancels without joining the loser and closes any body it returns later.
			unblock()
			require.Eventually(t, func() bool { return body.closes.Load() == 1 }, 5*time.Second, time.Millisecond)
			require.Zero(t, body.reads.Load())
		})
	}
}

// TestToolFetchAbnormalWorkerExit reports completion even when no transport result returns.
func TestToolFetchAbnormalWorkerExit(t *testing.T) {
	for _, exit := range []string{"panic", "Goexit"} {
		t.Run(exit, func(t *testing.T) {
			var diagnostics bytes.Buffer
			options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
				if exit == "panic" {
					panic("private-panic-payload")
				}
				runtime.Goexit()
				return nil, errors.New("unreachable")
			}, fetchLimits())
			options.Logger = slog.New(slog.NewTextHandler(&diagnostics, nil))
			program := compileTool(t, `async function execute() {
				return await context.connections[0].fetch("https://provider.example/items");
			}`, nil)

			// A worker failure rejects promptly rather than stranding the pending Promise.
			result, err := program.EvaluateWithHTTP(context.Background(), fetchInput(t), options)
			require.ErrorIs(t, err, apjsruntime.ErrEvaluation)
			require.Nil(t, result)
			require.NotContains(t, err.Error(), "private-panic-payload")
			require.NotEmpty(t, diagnostics.String())
			require.NotContains(t, diagnostics.String(), "private-panic-payload")
			require.Equal(t, api.ToolExecutionStateMayHaveExecuted, options.Session.ExecutionState())
		})
	}
}

// TestToolFetchConnectionSnapshot keeps aliases on one copied capability while
// protecting client identity and the connection collection from authored edits.
func TestToolFetchConnectionSnapshot(t *testing.T) {
	input := javascript.Input{Context: expressionContext(t, render.Input{
		ConnectionID: "cxn_captured", ConnectionName: "Captured", ConnectionNamespace: "root.product.owner",
		Cfg:    map[string]any{"tenant": "acme", "nested": map[string]any{"region": "us"}},
		Labels: map[string]string{"team": "blue"}, Annotations: map[string]string{"note": "captured"},
		ToolID: "tol_example", Revision: 7,
	}), Response: common.RawJSON(`{"status":202}`)}
	library := toolLibrary(t, `function tenant() { return cfg.tenant; }`)
	program := compileTool(t, `async function execute(params, context) {
		const [connection] = context.connections;
		const result = {
			count: context.connections.length, globalFetch: typeof fetch,
			identity: context.connection, tool: context.tool, responseStatus: response.status,
			initial: [tenant(), cfg.nested.region, labels.team, annotations.note],
			aliases: cfg === connection.cfg && context.cfg === connection.cfg &&
				labels === connection.labels && context.labels === connection.labels &&
				annotations === connection.annotations && context.annotations === connection.annotations,
			frozen: Object.isFrozen(context.connections) && Object.isFrozen(connection),
			methodHidden: !Object.keys(connection).includes("fetch")
		};
		let refused = 0;
		for (const change of [
			() => context.connections = [], () => context.connections[0] = {},
			() => context.connections.push({}), () => connection.id = "cxn_other",
			() => connection.cfg = {}, () => connection.fetch = undefined
		]) { try { change(); } catch (err) { if (err instanceof TypeError) refused++; } }
		cfg.nested.region = "local";
		labels.team = "local";
		annotations.note = "local";
		result.refused = refused;
		result.local = [connection.cfg.nested.region, connection.labels.team, connection.annotations.note];
		const borrowed = connection.fetch;
		result.status = (await borrowed.call({id: "cxn_other"}, "https://provider.example/items")).status;
		return result;
	}`, library)

	// Reuse the captured input; each invocation gets fresh aliases and its own session.
	for range 2 {
		var calls atomic.Int32
		options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return fetchResponse(http.StatusOK, http.NoBody), nil
		}, fetchLimits())
		result, err := program.EvaluateWithHTTP(context.Background(), input, options)
		require.NoError(t, err)
		require.JSONEq(t, `{
			"count":1,"globalFetch":"undefined",
			"identity":{"id":"cxn_captured","name":"Captured","namespace":"root.product.owner"},
			"tool":{"id":"tol_example","revision":7},"responseStatus":202,
			"initial":["acme","us","blue","captured"],"aliases":true,"frozen":true,
			"methodHidden":true,"refused":6,"local":["local","local","local"],"status":200
		}`, string(result))
		require.Equal(t, int32(1), calls.Load())
	}
}

// TestToolFetchRequiresCapturedConnection rejects missing identity before any
// authored work or I/O, even if the configured session could otherwise dispatch.
func TestToolFetchRequiresCapturedConnection(t *testing.T) {
	program := compileTool(t, `async function execute() {
		await context.connections[0].fetch("https://provider.example/items");
		return true;
	}`, nil)
	var calls atomic.Int32
	options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return fetchResponse(http.StatusOK, http.NoBody), nil
	}, fetchLimits())
	input := javascript.Input{Context: expressionContext(t, render.Input{})}

	// A session supplies transport authority but cannot fill in absent snapshot identity.
	result, err := program.EvaluateWithHTTP(context.Background(), input, options)
	require.ErrorIs(t, err, apjsruntime.ErrInvalidInput)
	require.Nil(t, result)
	require.Zero(t, calls.Load())
	require.ErrorIs(t, options.Session.Err(), aphttp.ErrClosed)
	require.Equal(t, api.ToolExecutionStateNotStarted, options.Session.ExecutionState())
}

// TestToolFetchAliasesShareInputAllowance charges connection configuration once
// even though Tool helper globals and context fields expose the same object.
func TestToolFetchAliasesShareInputAllowance(t *testing.T) {
	input := javascript.Input{Context: expressionContext(t, render.Input{
		ConnectionID: "cxn_example", Cfg: map[string]any{"value": strings.Repeat("<", 400)},
	})}
	limits := expressionLimits()
	limits.MaxInputBytes = 700
	program, err := javascript.CompileTool(`async function execute() {
		return cfg === context.cfg && cfg === context.connections[0].cfg && cfg.value.length === 400;
	}`, nil, limits)
	require.NoError(t, err)
	var calls atomic.Int32
	options := fetchOptions(t, context.Background(), func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return fetchResponse(http.StatusOK, http.NoBody), nil
	}, fetchLimits())

	// A single snapshot fits; duplicating configuration or HTML-escaping its
	// ordinary JSON strings would exceed the cap without adding any input data.
	result, err := program.EvaluateWithHTTP(context.Background(), input, options)
	require.NoError(t, err)
	require.JSONEq(t, `true`, string(result))
	require.Zero(t, calls.Load())
}
