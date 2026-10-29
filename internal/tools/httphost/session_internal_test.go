package httphost

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/stretchr/testify/require"
)

// panicTestClient reports partial evidence before a client or middleware panic.
// It captures the request context so the test can verify cleanup after recovery.
type panicTestClient struct {
	ctx context.Context
}

// Do imitates a client that reports local rejection but fails before returning.
func (c *panicTestClient) Do(req *http.Request) (*http.Response, error) {
	c.ctx = req.Context()
	httpf.ReportSendEvidence(c.ctx, false)
	panic("client failure")
}

// TestSessionPanicSettlesRequest ensures an outer recovery does not leave an
// in-flight reservation behind or trust evidence from an incomplete client call.
func TestSessionPanicSettlesRequest(t *testing.T) {
	// The caller can recover a client panic while continuing the invocation.
	client := &panicTestClient{}
	session, err := New(context.Background(), client, Limits{
		Timeout: time.Minute, MaxRequests: 1, MaxResponseBytes: 1024,
		MaxStreamBytes: 1024, MaxHeaderBytes: 1024,
	})
	require.NoError(t, err)
	t.Cleanup(session.Close)
	request, err := http.NewRequest(http.MethodGet, "https://provider.example/items", nil)
	require.NoError(t, err)
	require.PanicsWithValue(t, "client failure", func() { _, _ = session.Do(request) })

	// The request has ended, but its allowance stays spent and its uncertainty
	// stays sticky. Per-request cancellation resources are released on unwind.
	require.Zero(t, session.inFlight)
	require.Equal(t, 1, session.requests)
	require.True(t, session.mayHaveExecuted)
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())
	require.ErrorIs(t, client.ctx.Err(), context.Canceled)
	require.NoError(t, session.Err())

	// Recovering the panic cannot make the exhausted request allowance reusable.
	response, err := session.Do(request)
	require.ErrorIs(t, err, ErrRequestLimit)
	require.Nil(t, response)
	require.Zero(t, session.inFlight)
}
