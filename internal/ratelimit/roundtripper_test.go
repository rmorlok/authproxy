package ratelimit

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"log/slog"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTransport struct {
	resp *http.Response
	err  error
	// called tracks whether RoundTrip was invoked
	called bool
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.called = true
	return m.resp, m.err
}

func newTestStore(t *testing.T) (*Store, apredis.Client) {
	_, r := apredis.MustApplyTestConfig(nil)
	return NewStore(r), r
}

func testConnectionID() apid.ID {
	return apid.New(apid.PrefixConnection)
}

func testCtx() context.Context {
	return apctx.WithFixedClock(context.Background(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}

func testLogger() *slog.Logger {
	return slog.Default()
}

func TestFactory_SkipsNonProxyRequests(t *testing.T) {
	store, _ := newTestStore(t)
	factory := NewFactory(store, testLogger())

	rt := factory.NewRoundTripper(httpf.RequestInfo{
		ConnectionId: testConnectionID(),
		Type:         httpf.RequestTypeOAuth,
	}, http.DefaultTransport)

	assert.Nil(t, rt)
}

func TestFactory_SkipsNoConnectionId(t *testing.T) {
	store, _ := newTestStore(t)
	factory := NewFactory(store, testLogger())

	rt := factory.NewRoundTripper(httpf.RequestInfo{
		Type: httpf.RequestTypeProxy,
	}, http.DefaultTransport)

	assert.Nil(t, rt)
}

func TestFactory_SkipsDisabled(t *testing.T) {
	store, _ := newTestStore(t)
	factory := NewFactory(store, testLogger())

	rt := factory.NewRoundTripper(httpf.RequestInfo{
		ConnectionId: testConnectionID(),
		Type:         httpf.RequestTypeProxy,
		RateLimiting: &connectors.RateLimiting{Disabled: true},
	}, http.DefaultTransport)

	assert.Nil(t, rt)
}

func TestFactory_CreatesForProxy(t *testing.T) {
	store, _ := newTestStore(t)
	factory := NewFactory(store, testLogger())

	rt := factory.NewRoundTripper(httpf.RequestInfo{
		ConnectionId: testConnectionID(),
		Type:         httpf.RequestTypeProxy,
	}, http.DefaultTransport)

	assert.NotNil(t, rt)
}

func TestFactory_CreatesForProbe(t *testing.T) {
	store, _ := newTestStore(t)
	factory := NewFactory(store, testLogger())

	rt := factory.NewRoundTripper(httpf.RequestInfo{
		ConnectionId: testConnectionID(),
		Type:         httpf.RequestTypeProbe,
	}, http.DefaultTransport)

	assert.NotNil(t, rt)
}

func TestRoundTripper_PassthroughOnSuccess(t *testing.T) {
	store, _ := newTestStore(t)
	connID := testConnectionID()

	transport := &mockTransport{
		resp: &http.Response{StatusCode: 200, Header: http.Header{}},
	}

	rt := &RoundTripper{
		connectionId: connID,
		store:        store,
		transport:    transport,
		logger:       testLogger(),
	}

	req, _ := http.NewRequestWithContext(testCtx(), "GET", "http://example.com", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, transport.called)
}

func TestRoundTripper_429SetsRateLimit(t *testing.T) {
	store, _ := newTestStore(t)
	connID := testConnectionID()
	ctx := testCtx()

	transport := &mockTransport{
		resp: &http.Response{
			StatusCode: 429,
			Header:     http.Header{"Retry-After": {"30"}},
		},
	}

	rt := &RoundTripper{
		connectionId: connID,
		store:        store,
		transport:    transport,
		logger:       testLogger(),
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)

	// Verify the connection is now rate-limited
	remaining, limited, err := store.IsRateLimited(ctx, connID)
	require.NoError(t, err)
	assert.True(t, limited)
	assert.True(t, remaining > 0)
}

func TestRoundTripper_BlocksSubsequentRequests(t *testing.T) {
	store, _ := newTestStore(t)
	connID := testConnectionID()
	ctx := testCtx()

	// Pre-set a rate limit
	err := store.SetRateLimited(ctx, connID, 30*time.Second)
	require.NoError(t, err)

	transport := &mockTransport{
		resp: &http.Response{StatusCode: 200, Header: http.Header{}},
	}

	rt := &RoundTripper{
		connectionId: connID,
		store:        store,
		transport:    transport,
		logger:       testLogger(),
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)
	assert.Equal(t, "true", resp.Header.Get("X-Authproxy-Ratelimited"))
	assert.False(t, transport.called, "should not have called upstream transport")

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "rate limited")
}

func TestRoundTripper_ClearsCounterOnSuccess(t *testing.T) {
	store, _ := newTestStore(t)
	connID := testConnectionID()
	ctx := testCtx()

	// Set a consecutive count
	_, err := store.IncrementConsecutive429Count(ctx, connID)
	require.NoError(t, err)
	_, err = store.IncrementConsecutive429Count(ctx, connID)
	require.NoError(t, err)

	count, err := store.GetConsecutive429Count(ctx, connID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	transport := &mockTransport{
		resp: &http.Response{StatusCode: 200, Header: http.Header{}},
	}

	rt := &RoundTripper{
		connectionId: connID,
		store:        store,
		transport:    transport,
		logger:       testLogger(),
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	// Counter should be cleared
	count, err = store.GetConsecutive429Count(ctx, connID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestRoundTripper_DefaultRetryAfterWhenNoHeader(t *testing.T) {
	store, _ := newTestStore(t)
	connID := testConnectionID()
	ctx := testCtx()

	transport := &mockTransport{
		resp: &http.Response{
			StatusCode: 429,
			Header:     http.Header{}, // No Retry-After
		},
	}

	rt := &RoundTripper{
		connectionId: connID,
		store:        store,
		transport:    transport,
		logger:       testLogger(),
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	require.NoError(t, err)

	// Should be rate-limited with default duration
	_, limited, err := store.IsRateLimited(ctx, connID)
	require.NoError(t, err)
	assert.True(t, limited)
}

func TestRoundTripper_CustomRetryAfterHeaders(t *testing.T) {
	store, _ := newTestStore(t)
	connID := testConnectionID()
	ctx := testCtx()

	transport := &mockTransport{
		resp: &http.Response{
			StatusCode: 429,
			Header: http.Header{
				"X-Custom-Retry": {"45"},
			},
		},
	}

	cfg := &connectors.RateLimiting{
		RetryAfterHeaders: []string{"X-Custom-Retry"},
	}

	rt := &RoundTripper{
		connectionId: connID,
		config:       cfg,
		store:        store,
		transport:    transport,
		logger:       testLogger(),
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	require.NoError(t, err)

	_, limited, err := store.IsRateLimited(ctx, connID)
	require.NoError(t, err)
	assert.True(t, limited)
}

func TestRoundTripper_MaxRetryAfterCap(t *testing.T) {
	store, _ := newTestStore(t)
	connID := testConnectionID()
	ctx := testCtx()

	transport := &mockTransport{
		resp: &http.Response{
			StatusCode: 429,
			Header:     http.Header{"Retry-After": {"999999"}}, // Very large
		},
	}

	rt := &RoundTripper{
		connectionId: connID,
		store:        store,
		transport:    transport,
		logger:       testLogger(),
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	require.NoError(t, err)

	remaining, limited, err := store.IsRateLimited(ctx, connID)
	require.NoError(t, err)
	assert.True(t, limited)
	// Should be capped at default max (15 minutes)
	assert.LessOrEqual(t, remaining, connectors.DefaultMaxRetryAfter)
}

func TestRoundTripper_IncrementsConsecutiveCount(t *testing.T) {
	store, _ := newTestStore(t)
	connID := testConnectionID()
	ctx := testCtx()

	transport := &mockTransport{
		resp: &http.Response{
			StatusCode: 429,
			Header:     http.Header{"Retry-After": {"10"}},
		},
	}

	rt := &RoundTripper{
		connectionId: connID,
		store:        store,
		transport:    transport,
		logger:       testLogger(),
	}

	// First 429
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	require.NoError(t, err)

	count, err := store.GetConsecutive429Count(ctx, connID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Clear the rate limit to allow second request through
	store.r.Del(ctx, blockedKey(connID))

	// Second 429
	req, _ = http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	count, err = store.GetConsecutive429Count(ctx, connID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}
