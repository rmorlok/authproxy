package http_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	"github.com/stretchr/testify/require"
)

// TestSessionBindValidatesCapabilities rejects unusable owners and clients
// without dispatching, while preserving expired-owner error handling at Do.
func TestSessionBindValidatesCapabilities(t *testing.T) {
	client := &sessionTestClient{do: func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid or expired capability reached its client")
		return nil, nil
	}}
	for _, owner := range []*aphttp.Session{nil, {}} {
		// A zero owner has neither a policy nor an execution lifetime.
		handle, err := owner.Bind(client)
		require.ErrorIs(t, err, aphttp.ErrInvalidInput)
		require.Nil(t, handle)
	}
	owner, err := aphttp.New(context.Background(), sessionTestLimits())
	require.NoError(t, err)
	t.Cleanup(owner.Close)
	handle, err := owner.Bind(nil)
	require.ErrorIs(t, err, aphttp.ErrInvalidInput)
	require.Nil(t, handle)
	require.Equal(t, aphttp.SendStateNotSent, owner.SendState())

	// Binding after close is harmless: it cannot restart the owner's lifetime.
	owner.Close()
	handle, err = owner.Bind(client)
	require.NoError(t, err)
	request, body := sessionTestRequest(context.Background())
	response, state, err := handle.Do(request)
	require.ErrorIs(t, err, aphttp.ErrClosed)
	require.Nil(t, response)
	require.Equal(t, aphttp.SendStateNotSent, state)
	require.EqualValues(t, 1, body.closes.Load())

	// An already expired admission deadline still constructs successfully so
	// the caller can classify the policy failure through the normal request path.
	expired, err := aphttp.NewWithDeadline(context.Background(), sessionTestLimits(), time.Unix(1, 0))
	require.NoError(t, err)
	t.Cleanup(expired.Close)
	handle, err = expired.Bind(client)
	require.NoError(t, err)
	request, body = sessionTestRequest(context.Background())
	response, state, err = handle.Do(request)
	require.ErrorIs(t, err, aphttp.ErrTimeout)
	require.Nil(t, response)
	require.Equal(t, aphttp.SendStateNotSent, state)
	require.EqualValues(t, 1, body.closes.Load())
}

// TestMultipleClientsShareRequestBudget keeps fixed dispatch identities under
// one concurrent request cap, deadline, caller context, and evidence owner.
func TestMultipleClientsShareRequestBudget(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		name := "all unsent"
		if confirmed {
			name = "one confirmed"
		}
		t.Run(name, func(t *testing.T) {
			parent := context.WithValue(context.Background(), sessionTestIdentityKey{}, "admitted")
			limits := sessionTestLimits()
			limits.MaxRequests = 2
			owner, err := aphttp.New(parent, limits)
			require.NoError(t, err)
			t.Cleanup(owner.Close)
			deadline, ok := owner.Context().Deadline()
			require.True(t, ok)

			// Each configured client records the effective context before the
			// test allows either call to return its independent send evidence.
			type observation struct {
				client   string
				identity any
				deadline time.Time
				host     string
			}
			entered := make(chan observation, 2)
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			t.Cleanup(unblock)
			var calls atomic.Int32
			newClient := func(name string, state aphttp.SendState) aphttp.Client {
				return &sessionTestClient{doWithState: func(req *http.Request) (*http.Response, aphttp.SendState, error) {
					calls.Add(1)
					_ = req.Body.Close()
					actualDeadline, _ := req.Context().Deadline()
					entered <- observation{name, req.Context().Value(sessionTestIdentityKey{}), actualDeadline, req.Host}
					select {
					case <-release:
					case <-req.Context().Done():
						return nil, state, req.Context().Err()
					}
					return sessionTestResponse(), state, nil
				}}
			}
			secondState := aphttp.SendStateNotSent
			if confirmed {
				secondState = aphttp.SendStateSent
			}
			selected := newClient("first", aphttp.SendStateNotSent)
			first, err := owner.Bind(selected)
			require.NoError(t, err)
			selected = newClient("second", secondState)
			second, err := owner.Bind(selected)
			require.NoError(t, err)
			borrowedDo := first.Do
			require.Zero(t, calls.Load())
			require.Equal(t, aphttp.SendStateNotSent, owner.SendState())

			// Changing the selected Go variable or request Host cannot retarget
			// a previously bound method; both handles consume the same two slots.
			type outcome struct {
				state aphttp.SendState
				err   error
			}
			finished := make(chan outcome, 2)
			for _, do := range []func(*http.Request) (*aphttp.Response, aphttp.SendState, error){borrowedDo, second.Do} {
				request, _ := sessionTestRequest(context.WithValue(context.Background(), sessionTestIdentityKey{}, "request-local"))
				request.Host = "overridden.example"
				go func() {
					response, state, err := do(request)
					if response != nil {
						_ = response.Body.Close()
					}
					finished <- outcome{state, err}
				}()
			}
			seen := make(map[string]bool)
			for range 2 {
				select {
				case call := <-entered:
					seen[call.client] = true
					require.Equal(t, "admitted", call.identity)
					require.Equal(t, deadline, call.deadline)
					require.Equal(t, "overridden.example", call.host)
				case <-time.After(5 * time.Second):
					t.Fatal("bound clients did not enter before watchdog")
				}
			}
			require.Equal(t, map[string]bool{"first": true, "second": true}, seen)
			require.Equal(t, aphttp.SendStateMayHaveSent, owner.SendState())

			// Even a newly bound handle cannot reset the exhausted shared cap.
			third, err := owner.Bind(selected)
			require.NoError(t, err)
			request, body := sessionTestRequest(context.Background())
			response, state, err := third.Do(request)
			require.ErrorIs(t, err, aphttp.ErrRequestLimit)
			require.Nil(t, response)
			require.Equal(t, aphttp.SendStateNotSent, state)
			require.EqualValues(t, 1, body.closes.Load())
			require.EqualValues(t, 2, calls.Load())
			unblock()
			states := make([]aphttp.SendState, 0, 2)
			for range 2 {
				select {
				case result := <-finished:
					require.NoError(t, result.err)
					states = append(states, result.state)
				case <-time.After(5 * time.Second):
					t.Fatal("bound clients did not finish before watchdog")
				}
			}
			require.ElementsMatch(t, []aphttp.SendState{aphttp.SendStateNotSent, secondState}, states)
			require.Equal(t, secondState, owner.SendState())
		})
	}
}

// TestMultipleClientsShareResponseBudget charges discarded bytes across clients
// and cancels both outstanding requests and body reads when the owner exhausts.
func TestMultipleClientsShareResponseBudget(t *testing.T) {
	limits := sessionTestLimits()
	limits.MaxResponseBytes = 3
	owner, err := aphttp.New(context.Background(), limits)
	require.NoError(t, err)
	t.Cleanup(owner.Close)
	var calls atomic.Int32
	bindBody := func(body io.ReadCloser) *aphttp.ClientHandle {
		handle, err := owner.Bind(&sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			_ = req.Body.Close()
			return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
		}})
		require.NoError(t, err)
		return handle
	}

	// A partially consumed response still spends its accepted bytes after close.
	discardedBody := &sessionTestBody{reader: strings.NewReader("abcd")}
	first := bindBody(discardedBody)
	request, _ := sessionTestRequest(context.Background())
	discarded, _, err := first.Do(request)
	require.NoError(t, err)
	prefix := make([]byte, 2)
	_, err = io.ReadFull(discarded.Body, prefix)
	require.NoError(t, err)
	require.Equal(t, "ab", string(prefix))
	require.NoError(t, discarded.Body.Close())
	require.EqualValues(t, 1, discardedBody.closes.Load())

	// Another client's stream blocks, while a third client's request waits for
	// cancellation before it can return any response at all.
	blockedBody := &sessionTestBlockingBody{entered: make(chan struct{}), closed: make(chan struct{})}
	second := bindBody(blockedBody)
	request, _ = sessionTestRequest(context.Background())
	blocked, _, err := second.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocked.Body.Close() })
	readDone := make(chan struct{})
	var readErr error
	go func() { _, readErr = blocked.Body.Read(make([]byte, 1)); close(readDone) }()
	sessionTestWait(t, blockedBody.entered, owner.Close)
	requestEntered := make(chan struct{})
	third, err := owner.Bind(&sessionTestClient{do: func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		_ = req.Body.Close()
		close(requestEntered)
		<-req.Context().Done()
		return nil, req.Context().Err()
	}})
	require.NoError(t, err)
	request, _ = sessionTestRequest(context.Background())
	requestDone := make(chan struct{})
	var requestErr error
	go func() { _, _, requestErr = third.Do(request); close(requestDone) }()
	sessionTestWait(t, requestEntered, owner.Close)

	// A fourth client cannot read two bytes after the discarded prefix spent
	// two of the owner's three bytes; the accepted suffix remains partial data.
	overflowBody := &sessionTestBody{reader: strings.NewReader("xy")}
	fourth := bindBody(overflowBody)
	request, _ = sessionTestRequest(context.Background())
	overflow, _, err := fourth.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = overflow.Body.Close() })
	partial, err := io.ReadAll(overflow.Body)
	require.ErrorIs(t, err, aphttp.ErrResponseLimit)
	require.Equal(t, "x", string(partial))
	sessionTestWait(t, readDone, owner.Close)
	sessionTestWait(t, requestDone, owner.Close)
	require.ErrorIs(t, readErr, aphttp.ErrResponseLimit)
	require.ErrorIs(t, requestErr, aphttp.ErrResponseLimit)
	require.ErrorIs(t, owner.Err(), aphttp.ErrResponseLimit)
	require.EqualValues(t, 1, blockedBody.closes.Load())
	require.EqualValues(t, 1, overflowBody.closes.Load())

	// Binding after exhaustion grants no fresh byte allowance or dispatch.
	later := bindBody(http.NoBody)
	request, upload := sessionTestRequest(context.Background())
	response, state, err := later.Do(request)
	require.ErrorIs(t, err, aphttp.ErrResponseLimit)
	require.Nil(t, response)
	require.Equal(t, aphttp.SendStateNotSent, state)
	require.EqualValues(t, 1, upload.closes.Load())
	require.EqualValues(t, 4, calls.Load())
	require.Equal(t, aphttp.SendStateMayHaveSent, owner.SendState())
}

// reusableTestClient makes unwanted client-lifetime ownership observable while
// still supplying ordinary successful target responses to separate sessions.
type reusableTestClient struct {
	calls  int
	closes int
}

// Do records the shared client's use without acquiring a new transport.
func (c *reusableTestClient) Do(req *http.Request) (*http.Response, aphttp.SendState, error) {
	c.calls++
	_ = req.Body.Close()
	return sessionTestResponse(), aphttp.SendStateSent, nil
}

// Close records a client close, which session shutdown must never call.
func (c *reusableTestClient) Close() error { c.closes++; return nil }

// TestClientReuseKeepsExecutionLifetimesSeparate allows a reusable client under a
// fresh owner without reviving an old handle or closing the caller's client.
func TestClientReuseKeepsExecutionLifetimesSeparate(t *testing.T) {
	client := &reusableTestClient{}
	first, err := aphttp.New(context.Background(), sessionTestLimits())
	require.NoError(t, err)
	t.Cleanup(first.Close)
	oldHandle, err := first.Bind(client)
	require.NoError(t, err)
	request, _ := sessionTestRequest(context.Background())
	response, _, err := oldHandle.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	first.Close()

	// Closing the first execution leaves the caller-owned client reusable.
	second, err := aphttp.New(context.Background(), sessionTestLimits())
	require.NoError(t, err)
	t.Cleanup(second.Close)
	newHandle, err := second.Bind(client)
	require.NoError(t, err)
	require.Equal(t, aphttp.SendStateNotSent, second.SendState())
	request, _ = sessionTestRequest(context.Background())
	response, _, err = newHandle.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	// The old method remains bound to its canceled owner, regardless of reuse.
	request, upload := sessionTestRequest(context.Background())
	response, state, err := oldHandle.Do(request)
	require.ErrorIs(t, err, aphttp.ErrClosed)
	require.Nil(t, response)
	require.Equal(t, aphttp.SendStateNotSent, state)
	require.EqualValues(t, 1, upload.closes.Load())
	require.Equal(t, 2, client.calls)
	second.Close()
	require.Zero(t, client.closes)
}
