package httphost_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/stretchr/testify/require"
)

// sessionTestClient exposes the narrow HTTP client capability supplied to an
// invocation, without coupling session tests to a client's proxy internals.
type sessionTestClient struct {
	do func(*http.Request) (*http.Response, error)
}

// Do delegates to the scenario's observed client behavior.
func (c *sessionTestClient) Do(req *http.Request) (*http.Response, error) {
	return c.do(req)
}

// sessionTestLimits leaves room for ordinary responses while bounding every policy.
func sessionTestLimits() httphost.Limits {
	return httphost.Limits{Timeout: 30 * time.Second, MaxRequests: 8, MaxResponseBytes: 8192, MaxStreamBytes: 8192, MaxHeaderBytes: 4096}
}

// sessionTestNew constructs an invocation and guarantees its timer is released.
func sessionTestNew(t *testing.T, ctx context.Context, client iface.HTTPClient, limits httphost.Limits) *httphost.Session {
	t.Helper()
	session, err := httphost.New(ctx, client, limits)
	require.NoError(t, err)
	t.Cleanup(session.Close)
	return session
}

// sessionTestBody records ownership and detects eager response consumption.
// Each instance has at most one reader; close counts may be checked concurrently.
type sessionTestBody struct {
	reader io.Reader
	reads  atomic.Int32
	closes atomic.Int32
}

// Read records a consumer read before forwarding to the fixture's finite source.
func (b *sessionTestBody) Read(p []byte) (int, error) {
	b.reads.Add(1)
	return b.reader.Read(p)
}

// Close records whether ownership was released exactly once.
func (b *sessionTestBody) Close() error { b.closes.Add(1); return nil }

// sessionTestRequest gives each attempted call its own tracked request body.
func sessionTestRequest(ctx context.Context) (*http.Request, *sessionTestBody) {
	body := &sessionTestBody{reader: strings.NewReader("request payload")}
	request := &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Scheme: "https", Host: "provider.example", Path: "/items"},
		Header: make(http.Header), Body: body,
	}
	return request.WithContext(ctx), body
}

// sessionTestResponse is an ordinary empty streaming response owned by its caller.
func sessionTestResponse() *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}
}

// sessionTestWait bounds synchronization without using sleeps to guess progress.
func sessionTestWait(t *testing.T, done <-chan struct{}, cleanup func()) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cleanup()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		t.Fatal("HTTP host operation did not stop before the watchdog expired")
	}
}

// TestSessionConstructor requires a complete positive policy before creating an
// invocation and keeps caller cancellation distinct from invalid arguments.
func TestSessionConstructor(t *testing.T) {
	client := &sessionTestClient{}
	for _, test := range []struct {
		name   string
		change func(*httphost.Limits)
	}{
		{"timeout", func(l *httphost.Limits) { l.Timeout = 0 }},
		{"requests", func(l *httphost.Limits) { l.MaxRequests = 0 }},
		{"response bytes", func(l *httphost.Limits) { l.MaxResponseBytes = 0 }},
		{"stream bytes", func(l *httphost.Limits) { l.MaxStreamBytes = 0 }},
		{"header bytes", func(l *httphost.Limits) { l.MaxHeaderBytes = 0 }},
		{"negative policy", func(l *httphost.Limits) { l.MaxRequests = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A zero or negative value cannot silently disable a guard.
			limits := sessionTestLimits()
			test.change(&limits)
			session, err := httphost.New(context.Background(), client, limits)
			require.ErrorIs(t, err, httphost.ErrInvalidInput)
			require.Nil(t, session)
		})
	}
	_, err := httphost.New(nil, client, sessionTestLimits())
	require.ErrorIs(t, err, httphost.ErrInvalidInput)
	_, err = httphost.New(context.Background(), nil, sessionTestLimits())
	require.ErrorIs(t, err, httphost.ErrInvalidInput)

	// No work starts when the admitted caller was already canceled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	session, err := httphost.New(ctx, client, sessionTestLimits())
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, session)
	session = sessionTestNew(t, context.Background(), client, sessionTestLimits())
	require.Equal(t, api.ToolExecutionStateNotStarted, session.ExecutionState())
	require.NoError(t, session.Context().Err())
	require.NoError(t, session.Err())
	_, hasDeadline := session.Context().Deadline()
	require.True(t, hasDeadline)
}

// TestSessionLocalRejectionOwnsRequestBody rejects malformed routing inputs
// before proxy handoff and before consuming the invocation's request allowance.
func TestSessionLocalRejectionOwnsRequestBody(t *testing.T) {
	var calls atomic.Int32
	client := &sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		_ = req.Body.Close()
		return sessionTestResponse(), nil
	}}
	limits := sessionTestLimits()
	limits.MaxRequests = 1
	session := sessionTestNew(t, context.Background(), client, limits)
	for _, test := range []struct {
		name   string
		change func(*http.Request)
	}{
		{"nil URL", func(r *http.Request) { r.URL = nil }},
		{"relative URL", func(r *http.Request) { r.URL.Scheme = "" }},
		{"unsupported scheme", func(r *http.Request) { r.URL.Scheme = "file" }},
		{"missing host", func(r *http.Request) { r.URL.Host = "" }},
		{"opaque URL", func(r *http.Request) { r.URL.Opaque = "private-opaque" }},
		{"fragment", func(r *http.Request) { r.URL.Fragment = "private-fragment" }},
		{"request URI", func(r *http.Request) { r.RequestURI = "/inbound" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, body := sessionTestRequest(context.Background())
			test.change(request)
			response, err := session.Do(request)
			require.ErrorIs(t, err, httphost.ErrInvalidInput)
			require.Nil(t, response)
			require.EqualValues(t, 1, body.closes.Load())
			require.Zero(t, body.reads.Load())
			require.Equal(t, api.ToolExecutionStateNotStarted, session.ExecutionState())
		})
	}
	response, err := session.Do(nil)
	require.ErrorIs(t, err, httphost.ErrInvalidInput)
	require.Nil(t, response)

	// An already canceled request also closes locally without spending a call.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request, body := sessionTestRequest(ctx)
	response, err = session.Do(request)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, response)
	require.EqualValues(t, 1, body.closes.Load())
	require.Zero(t, calls.Load())

	// The one allowed handoff remains available after all local rejections.
	request, body = sessionTestRequest(context.Background())
	response, err = session.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.EqualValues(t, 1, calls.Load())
	require.EqualValues(t, 1, body.closes.Load())
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())
}

// TestSessionUnusableSessionOwnsRequestBody extends entry ownership to nil,
// zero-valued, and already closed sessions without invoking the client.
func TestSessionUnusableSessionOwnsRequestBody(t *testing.T) {
	for _, session := range []*httphost.Session{nil, {}} {
		request, body := sessionTestRequest(context.Background())
		response, err := session.Do(request)
		require.ErrorIs(t, err, httphost.ErrInvalidInput)
		require.Nil(t, response)
		require.EqualValues(t, 1, body.closes.Load())
	}
	client := &sessionTestClient{}
	session := sessionTestNew(t, context.Background(), client, sessionTestLimits())
	session.Close()
	session.Close()
	require.ErrorIs(t, session.Err(), httphost.ErrClosed)
	request, body := sessionTestRequest(context.Background())
	response, err := session.Do(request)
	require.ErrorIs(t, err, httphost.ErrClosed)
	require.Nil(t, response)
	require.EqualValues(t, 1, body.closes.Load())
	require.Equal(t, api.ToolExecutionStateNotStarted, session.ExecutionState())
}

// sessionTestIdentityKey distinguishes admitted identity from request-local values.
type sessionTestIdentityKey struct{}

// TestSessionClientContract preserves request features while the HTTP client
// receives the admitted caller's identity through the effective request context.
func TestSessionClientContract(t *testing.T) {
	parent := context.WithValue(context.Background(), sessionTestIdentityKey{}, "admitted identity")
	requestCtx := context.WithValue(context.Background(), sessionTestIdentityKey{}, "request identity")
	var calls, replays int
	var responseBodies []*sessionTestBody
	client := &sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
		ctx := req.Context()
		calls++
		require.Equal(t, "admitted identity", ctx.Value(sessionTestIdentityKey{}))
		require.Equal(t, "https://provider.example/items", req.URL.String())
		require.Equal(t, "tenant.provider.example", req.Host)
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, []string{"first", "second"}, req.Header["X-Custom"])
		payload, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, "request payload", string(payload))
		require.NoError(t, req.Body.Close())
		replay, err := req.GetBody()
		require.NoError(t, err)
		payload, err = io.ReadAll(replay)
		require.NoError(t, err)
		require.Equal(t, "request payload", string(payload))
		require.NoError(t, replay.Close())
		body := &sessionTestBody{reader: strings.NewReader("upstream content")}
		responseBodies = append(responseBodies, body)
		return &http.Response{StatusCode: 201, Header: http.Header{"X-Upstream": {"first", "second"}}, Body: body}, nil
	}}
	session, err := httphost.New(parent, client, sessionTestLimits())
	require.NoError(t, err)
	t.Cleanup(session.Close)
	require.Equal(t, "admitted identity", session.Context().Value(sessionTestIdentityKey{}))

	// Each request retains its headers, host override, and replay capability.
	for range 2 {
		request, body := sessionTestRequest(requestCtx)
		request.Host = "tenant.provider.example"
		request.Header["X-Custom"] = []string{"first", "second"}
		request.GetBody = func() (io.ReadCloser, error) {
			replays++
			return io.NopCloser(strings.NewReader("request payload")), nil
		}
		response, err := session.Do(request)
		require.NoError(t, err)
		require.Equal(t, 201, response.StatusCode)
		require.Equal(t, []string{"first", "second"}, response.Header["X-Upstream"])
		require.EqualValues(t, 1, body.closes.Load())
		require.Zero(t, responseBodies[len(responseBodies)-1].reads.Load())
		require.Equal(t, "request identity", request.Context().Value(sessionTestIdentityKey{}))
		payload, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, "upstream content", string(payload))
		require.NoError(t, response.Body.Close())
	}
	require.Equal(t, 2, calls)
	require.Equal(t, 2, replays)
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())
}

// TestSessionClientDeadlinePrecedence exposes the earliest effective deadline to
// transport helpers without allowing request-local values to replace identity.
func TestSessionClientDeadlinePrecedence(t *testing.T) {
	for _, test := range []struct {
		name           string
		requestTimeout time.Duration
	}{
		{"earlier request", time.Minute},
		{"earlier session", 4 * time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Long deadlines let the test compare captured timestamps directly,
			// with no waiting or dependence on wall-clock sampling precision.
			parent := context.WithValue(context.Background(), sessionTestIdentityKey{}, "admitted identity")
			requestCtx, cancel := context.WithTimeout(
				context.WithValue(context.Background(), sessionTestIdentityKey{}, "request identity"),
				test.requestTimeout,
			)
			t.Cleanup(cancel)
			requestDeadline, ok := requestCtx.Deadline()
			require.True(t, ok)
			var wantDeadline time.Time
			client := &sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
				ctx := req.Context()
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.Equal(t, wantDeadline, deadline)
				require.Equal(t, "admitted identity", ctx.Value(sessionTestIdentityKey{}))
				_ = req.Body.Close()
				return sessionTestResponse(), nil
			}}
			limits := sessionTestLimits()
			limits.Timeout = 2 * time.Minute
			session := sessionTestNew(t, parent, client, limits)
			sessionDeadline, ok := session.Context().Deadline()
			require.True(t, ok)
			wantDeadline = sessionDeadline
			if requestDeadline.Before(sessionDeadline) {
				wantDeadline = requestDeadline
			}

			// Do preserves exactly the earlier timestamp and leaves the shared
			// invocation's deadline and lifetime unchanged after the body closes.
			request, _ := sessionTestRequest(requestCtx)
			response, err := session.Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			deadline, ok := session.Context().Deadline()
			require.True(t, ok)
			require.Equal(t, sessionDeadline, deadline)
			require.NoError(t, session.Err())
		})
	}
}

// TestSessionTransportFailureConsumesCall makes uncertainty and request counts
// cumulative, without leaking private transport details or retrying the call.
func TestSessionTransportFailureConsumesCall(t *testing.T) {
	var calls atomic.Int32
	upstreamBody := &sessionTestBody{reader: strings.NewReader("private-upstream-body")}
	client := &sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		_ = req.Body.Close()
		return &http.Response{StatusCode: 502, Body: upstreamBody}, errors.New("private-transport-url-and-credentials")
	}}
	limits := sessionTestLimits()
	limits.MaxRequests = 1
	session := sessionTestNew(t, context.Background(), client, limits)
	request, body := sessionTestRequest(context.Background())
	response, err := session.Do(request)
	require.ErrorIs(t, err, httphost.ErrTransport)
	require.NotContains(t, err.Error(), "private-")
	require.Nil(t, response)
	require.EqualValues(t, 1, calls.Load())
	require.EqualValues(t, 1, body.closes.Load())
	require.EqualValues(t, 1, upstreamBody.closes.Load())
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())

	// Failed transport spends its slot; the next body is rejected and closed locally.
	request, body = sessionTestRequest(context.Background())
	response, err = session.Do(request)
	require.ErrorIs(t, err, httphost.ErrRequestLimit)
	require.Nil(t, response)
	require.EqualValues(t, 1, body.closes.Load())
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())
}

// TestSessionStatusesDoNotTriggerHostRetries leaves credential recovery and
// status interpretation to the existing proxy and higher-level Tool executor.
func TestSessionStatusesDoNotTriggerHostRetries(t *testing.T) {
	for _, status := range []int{401, 429, 503} {
		var calls int
		client := &sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
			calls++
			_ = req.Body.Close()
			response := sessionTestResponse()
			response.StatusCode = status
			return response, nil
		}}
		session := sessionTestNew(t, context.Background(), client, sessionTestLimits())
		request, _ := sessionTestRequest(context.Background())
		response, err := session.Do(request)
		require.NoError(t, err)
		require.Equal(t, status, response.StatusCode)
		require.Equal(t, 1, calls)
		require.NoError(t, response.Body.Close())
	}
}

// TestSessionConcurrentRequestLimit reserves logical calls atomically and
// closes every rejected body while admitted calls are still in flight.
func TestSessionConcurrentRequestLimit(t *testing.T) {
	const attempts, allowed = 24, 4
	entered := make(chan struct{}, attempts)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var calls atomic.Int32
	client := &sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
		ctx := req.Context()
		calls.Add(1)
		_ = req.Body.Close()
		entered <- struct{}{}
		select {
		case <-release:
			return sessionTestResponse(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	limits := sessionTestLimits()
	limits.MaxRequests = allowed
	session := sessionTestNew(t, context.Background(), client, limits)
	t.Cleanup(unblock)
	start := make(chan struct{})
	results := make(chan error, attempts)
	bodies := make([]*sessionTestBody, 0, attempts)
	var workers sync.WaitGroup
	for range attempts {
		request, body := sessionTestRequest(context.Background())
		bodies = append(bodies, body)
		workers.Go(func() {
			<-start
			response, err := session.Do(request)
			if response != nil {
				_ = response.Body.Close()
			}
			results <- err
		})
	}
	close(start)
	for range allowed {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			session.Close()
			unblock()
			t.Fatal("allowed proxy calls did not enter")
		}
	}
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())
	unblock()
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	sessionTestWait(t, done, func() { session.Close(); unblock() })
	close(results)

	// Exactly the configured number reach the proxy, with no refunded slots.
	succeeded, rejected := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			require.ErrorIs(t, err, httphost.ErrRequestLimit)
			rejected++
		}
	}
	require.Equal(t, allowed, succeeded)
	require.Equal(t, attempts-allowed, rejected)
	require.EqualValues(t, allowed, calls.Load())
	for _, body := range bodies {
		require.EqualValues(t, 1, body.closes.Load())
	}
}

// TestSessionPendingRequestCancellation distinguishes every lifetime source
// after proxy handoff, while withholding the proxy's private cancellation error.
func TestSessionPendingRequestCancellation(t *testing.T) {
	for _, kind := range []string{"parent cancel", "request cancel", "parent custom cause", "request custom cause", "parent deadline", "request deadline", "policy timeout", "close"} {
		t.Run(kind, func(t *testing.T) {
			parent, cancelParent := context.WithCancel(context.Background())
			requestCtx, cancelRequest := context.WithCancel(context.Background())
			t.Cleanup(cancelParent)
			t.Cleanup(cancelRequest)
			limits := sessionTestLimits()
			want := error(context.Canceled)
			switch kind {
			case "parent custom cause":
				cancelParent()
				var cancelCause context.CancelCauseFunc
				parent, cancelCause = context.WithCancelCause(context.Background())
				cancelParent = func() { cancelCause(errors.New("private-caller-cause")) }
				t.Cleanup(cancelParent)
			case "request custom cause":
				cancelRequest()
				var cancelCause context.CancelCauseFunc
				requestCtx, cancelCause = context.WithCancelCause(context.Background())
				cancelRequest = func() { cancelCause(errors.New("private-request-cause")) }
				t.Cleanup(cancelRequest)
			case "parent deadline":
				cancelParent()
				parent, cancelParent = context.WithTimeout(context.Background(), 100*time.Millisecond)
				t.Cleanup(cancelParent)
				want = context.DeadlineExceeded
			case "request deadline":
				cancelRequest()
				requestCtx, cancelRequest = context.WithTimeout(context.Background(), 100*time.Millisecond)
				t.Cleanup(cancelRequest)
				want = context.DeadlineExceeded
			case "policy timeout":
				limits.Timeout = 100 * time.Millisecond
				want = httphost.ErrTimeout
			case "close":
				want = httphost.ErrClosed
			}
			entered := make(chan struct{})
			client := &sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
				ctx := req.Context()
				_ = req.Body.Close()
				close(entered)
				<-ctx.Done()
				return nil, errors.New("private-transport-cancellation")
			}}
			session := sessionTestNew(t, parent, client, limits)
			request, body := sessionTestRequest(requestCtx)
			var response *httphost.Response
			var err error
			done := make(chan struct{})
			go func() { response, err = session.Do(request); close(done) }()
			sessionTestWait(t, entered, session.Close)
			switch kind {
			case "parent cancel", "parent custom cause":
				cancelParent()
			case "request cancel", "request custom cause":
				cancelRequest()
			case "close":
				session.Close()
			}
			sessionTestWait(t, done, session.Close)

			// Every interruption preserves uncertainty and the public classification.
			require.ErrorIs(t, err, want)
			require.NotContains(t, err.Error(), "private-")
			require.Nil(t, response)
			require.EqualValues(t, 1, body.closes.Load())
			require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())
			if strings.HasPrefix(kind, "request ") {
				require.NoError(t, session.Context().Err())
				require.NoError(t, session.Err())
			} else {
				require.ErrorIs(t, session.Err(), want)
				require.NotContains(t, session.Err().Error(), "private-")
			}
		})
	}
}

// TestSessionTimeoutIncludesIdleTime starts the shared lifetime at construction,
// even when no proxy handoff or response read has occurred yet.
func TestSessionTimeoutIncludesIdleTime(t *testing.T) {
	limits := sessionTestLimits()
	limits.Timeout = 100 * time.Millisecond
	session := sessionTestNew(t, context.Background(), &sessionTestClient{}, limits)

	// Observe the actual policy deadline instead of sleeping for a guessed delay.
	sessionTestWait(t, session.Context().Done(), session.Close)
	require.ErrorIs(t, session.Err(), httphost.ErrTimeout)
	request, body := sessionTestRequest(context.Background())
	response, err := session.Do(request)
	require.ErrorIs(t, err, httphost.ErrTimeout)
	require.Nil(t, response)
	require.EqualValues(t, 1, body.closes.Load())
	require.Equal(t, api.ToolExecutionStateNotStarted, session.ExecutionState())
}

// sessionTestBlockingBody models a transport read that unblocks only when its
// owner closes the stream, with deterministic signals for read and close events.
type sessionTestBlockingBody struct {
	entered   chan struct{}
	closed    chan struct{}
	enterOnce sync.Once
	closeOnce sync.Once
	closes    atomic.Int32
}

// Read waits for ownership cleanup rather than polling a cancellation flag.
func (b *sessionTestBlockingBody) Read([]byte) (int, error) {
	b.enterOnce.Do(func() { close(b.entered) })
	<-b.closed
	return 0, errors.New("private-upstream-read-error")
}

// Close unblocks a pending read and records the release count.
func (b *sessionTestBlockingBody) Close() error {
	b.closes.Add(1)
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

// TestSessionCancellationClosesActiveResponse cancels reads that have already
// begun, without draining bodies or exposing the resulting transport error.
func TestSessionCancellationClosesActiveResponse(t *testing.T) {
	for _, kind := range []string{"parent", "request", "close"} {
		t.Run(kind, func(t *testing.T) {
			parent, cancelParent := context.WithCancel(context.Background())
			requestCtx, cancelRequest := context.WithCancel(context.Background())
			t.Cleanup(cancelParent)
			t.Cleanup(cancelRequest)
			body := &sessionTestBlockingBody{entered: make(chan struct{}), closed: make(chan struct{})}
			client := &sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
				_ = req.Body.Close()
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
			}}
			session := sessionTestNew(t, parent, client, sessionTestLimits())
			request, _ := sessionTestRequest(requestCtx)
			response, err := session.Do(request)
			require.NoError(t, err)
			t.Cleanup(func() { _ = response.Body.Close() })
			var n int
			var readErr error
			done := make(chan struct{})
			go func() { n, readErr = response.Body.Read(make([]byte, 16)); close(done) }()
			sessionTestWait(t, body.entered, session.Close)
			want := error(context.Canceled)
			switch kind {
			case "parent":
				cancelParent()
			case "request":
				cancelRequest()
			case "close":
				session.Close()
				want = httphost.ErrClosed
			}
			sessionTestWait(t, done, session.Close)

			// Closing is prompt, idempotent, and leaves no public body bytes behind.
			require.Zero(t, n)
			require.ErrorIs(t, readErr, want)
			require.NotContains(t, readErr.Error(), "private-")
			require.EqualValues(t, 1, body.closes.Load())
			_ = response.Body.Close()
			require.EqualValues(t, 1, body.closes.Load())
			require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())
		})
	}
}

// TestSessionRequestCancellationDoesNotPoisonInvocation keeps request-local
// lifetime restrictions separate from the reusable invocation context and budget.
func TestSessionRequestCancellationDoesNotPoisonInvocation(t *testing.T) {
	var calls int
	entered := make(chan struct{})
	client := &sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
		ctx := req.Context()
		calls++
		_ = req.Body.Close()
		if calls == 1 {
			close(entered)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return sessionTestResponse(), nil
	}}
	limits := sessionTestLimits()
	limits.MaxRequests = 2
	session := sessionTestNew(t, context.Background(), client, limits)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	request, _ := sessionTestRequest(ctx)
	var firstErr error
	done := make(chan struct{})
	go func() { _, firstErr = session.Do(request); close(done) }()
	sessionTestWait(t, entered, session.Close)
	cancel()
	sessionTestWait(t, done, session.Close)
	require.ErrorIs(t, firstErr, context.Canceled)
	require.NoError(t, session.Context().Err())

	// A later call still succeeds, but the canceled handoff consumed its slot.
	request, _ = sessionTestRequest(context.Background())
	response, err := session.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	request, body := sessionTestRequest(context.Background())
	response, err = session.Do(request)
	require.ErrorIs(t, err, httphost.ErrRequestLimit)
	require.Nil(t, response)
	require.EqualValues(t, 1, body.closes.Load())
	require.Equal(t, 2, calls)
}

// TestSessionResponseOverflowCancelsOtherReads enforces the decoded-byte limit
// across the whole invocation, including responses already waiting for content.
func TestSessionResponseOverflowCancelsOtherReads(t *testing.T) {
	completedBody := &sessionTestBody{reader: strings.NewReader("ok")}
	pendingBody := &sessionTestBlockingBody{entered: make(chan struct{}), closed: make(chan struct{})}
	overflowBody := &sessionTestBody{reader: strings.NewReader("abc")}
	var calls atomic.Int32
	client := &sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		_ = req.Body.Close()
		var body io.ReadCloser
		switch req.URL.Path {
		case "/completed":
			body = completedBody
		case "/pending":
			body = pendingBody
		case "/overflow":
			body = overflowBody
		default:
			return nil, errors.New("unexpected proxy call after response exhaustion")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
	}}
	limits := sessionTestLimits()
	limits.MaxResponseBytes = 4
	session := sessionTestNew(t, context.Background(), client, limits)

	// A fully consumed result uses two bytes and remains caller-owned data.
	request, _ := sessionTestRequest(context.Background())
	request.URL.Path = "/completed"
	completed, err := session.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = completed.Body.Close() })
	completedData, err := io.ReadAll(completed.Body)
	require.NoError(t, err)
	require.Equal(t, "ok", string(completedData))

	// Start another response read and wait until its transport is blocked.
	request, _ = sessionTestRequest(context.Background())
	request.URL.Path = "/pending"
	pending, err := session.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pending.Body.Close() })
	var pendingBytes int
	var pendingErr error
	pendingDone := make(chan struct{})
	go func() { pendingBytes, pendingErr = pending.Body.Read(make([]byte, 8)); close(pendingDone) }()
	sessionTestWait(t, pendingBody.entered, session.Close)

	// A third body consumes the remaining allowance and then proves excess.
	// Its accepted prefix is partial data, accompanied by the aggregate failure.
	request, _ = sessionTestRequest(context.Background())
	request.URL.Path = "/overflow"
	overflow, err := session.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = overflow.Body.Close() })
	partial, err := io.ReadAll(overflow.Body)
	require.ErrorIs(t, err, httphost.ErrResponseLimit)
	require.Equal(t, "ab", string(partial))
	require.ErrorIs(t, session.Err(), httphost.ErrResponseLimit)

	// Exceeding the shared cap closes the other stream and unblocks its read.
	sessionTestWait(t, pendingDone, session.Close)
	require.Zero(t, pendingBytes)
	require.ErrorIs(t, pendingErr, httphost.ErrResponseLimit)
	require.EqualValues(t, 1, pendingBody.closes.Load())
	require.EqualValues(t, 1, overflowBody.closes.Load())

	// Further requests are rejected locally, while prior completed bytes and
	// their terminal EOF remain intact. Upstream effects stay conservatively known.
	request, rejectedBody := sessionTestRequest(context.Background())
	request.URL.Path = "/later"
	response, err := session.Do(request)
	require.ErrorIs(t, err, httphost.ErrResponseLimit)
	require.Nil(t, response)
	require.EqualValues(t, 1, rejectedBody.closes.Load())
	require.EqualValues(t, 3, calls.Load())
	require.Equal(t, "ok", string(completedData))
	n, err := completed.Body.Read(make([]byte, 1))
	require.Zero(t, n)
	require.ErrorIs(t, err, io.EOF)
	require.EqualValues(t, 1, completedBody.closes.Load())
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())
}
