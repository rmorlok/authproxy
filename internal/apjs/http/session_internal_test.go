package http

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// panicTestClient fails before it can return settled target-send evidence.
// It captures the request context so the test can verify cleanup after recovery.
type panicTestClient struct {
	ctx context.Context
}

// Do imitates a client that fails before returning its response and evidence.
func (c *panicTestClient) Do(req *http.Request) (*http.Response, SendState, error) {
	c.ctx = req.Context()
	panic("client failure")
}

// TestSessionPanicSettlesRequest ensures an outer recovery does not leave an
// in-flight reservation behind or trust evidence from an incomplete client call.
func TestSessionPanicSettlesRequest(t *testing.T) {
	// The caller can recover a client panic while continuing the invocation.
	client := &panicTestClient{}
	session, err := New(context.Background(), Limits{
		Timeout: time.Minute, MaxRequests: 1, MaxResponseBytes: 1024,
		MaxStreamBytes: 1024, MaxHeaderBytes: 1024,
	})
	require.NoError(t, err)
	t.Cleanup(session.Close)
	handle, err := session.Bind(client)
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodGet, "https://provider.example/items", nil)
	require.NoError(t, err)
	require.PanicsWithValue(t, "client failure", func() { _, _, _ = handle.Do(request) })

	// The request has ended, but its allowance stays spent and its uncertainty
	// stays sticky. Per-request cancellation resources are released on unwind.
	require.Zero(t, session.inFlight)
	require.Equal(t, 1, session.requests)
	require.Equal(t, SendStateMayHaveSent, session.sendState)
	require.Equal(t, SendStateMayHaveSent, session.SendState())
	require.ErrorIs(t, client.ctx.Err(), context.Canceled)
	require.NoError(t, session.Err())

	// Recovering the panic cannot make the exhausted request allowance reusable.
	response, state, err := handle.Do(request)
	require.ErrorIs(t, err, ErrRequestLimit)
	require.Equal(t, SendStateNotSent, state)
	require.Nil(t, response)
	require.Zero(t, session.inFlight)
}
