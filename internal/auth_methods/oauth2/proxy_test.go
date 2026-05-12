package oauth2

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gentleman "gopkg.in/h2non/gentleman.v2"
)

// TestClassifyAndRecordRefreshFailure_PermanentFlipsUnhealthy is the
// load-bearing wiring test for this PR: every permanent refresh failure
// category must flip the connection's health_state to unhealthy with a
// reason of "refresh_<category>". A regression here is what user-visible
// reauth surfaces (the marketplace "reconnect" prompt) keys off — if a
// permanent failure stops flipping unhealthy, the user never sees the
// prompt and the integration silently dies.
func TestClassifyAndRecordRefreshFailure_PermanentFlipsUnhealthy(t *testing.T) {
	permanent := []tokenRefreshCategory{
		tokenRefreshNoRefreshToken,
		tokenRefreshInvalidGrant,
		tokenRefreshInvalidClient,
		tokenRefreshProvider4xxOther,
		tokenRefreshMalformedResponse,
	}
	for _, cat := range permanent {
		t.Run(string(cat), func(t *testing.T) {
			logger, read := bufLogger(t)
			conn := &mockCore.Connection{
				Id:          apid.New(apid.PrefixConnection),
				HealthState: database.ConnectionHealthStateHealthy,
			}
			o := &oAuth2Connection{logger: logger, connection: conn}

			err := o.classifyAndRecordRefreshFailure(
				context.Background(),
				cat,
				400,
				"invalid_grant",
				1,
				errors.New("status 400"),
			)
			require.Error(t, err, "the underlying error must be propagated")
			assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState,
				"permanent category must flip the connection to unhealthy")

			records := onlyTokenRefreshFailure(read())
			require.Len(t, records, 1)
			assert.Equal(t, string(cat), records[0]["category"])
		})
	}
}

// TestClassifyAndRecordRefreshFailure_TransientLeavesHealthAlone — the
// other half of the permanent/transient invariant. A 5xx or transport
// failure should *not* flip the connection: the next proxy call gets
// another attempt, and a transient hiccup must not generate a spurious
// "please reconnect" prompt for the end user.
func TestClassifyAndRecordRefreshFailure_TransientLeavesHealthAlone(t *testing.T) {
	transient := []tokenRefreshCategory{
		tokenRefreshNetworkError,
		tokenRefreshProvider5xx,
		tokenRefreshInternalError,
	}
	for _, cat := range transient {
		t.Run(string(cat), func(t *testing.T) {
			logger, read := bufLogger(t)
			conn := &mockCore.Connection{
				Id:          apid.New(apid.PrefixConnection),
				HealthState: database.ConnectionHealthStateHealthy,
			}
			o := &oAuth2Connection{logger: logger, connection: conn}

			err := o.classifyAndRecordRefreshFailure(
				context.Background(),
				cat,
				503,
				"",
				tokenRefreshMaxAttempts,
				errors.New("status 503"),
			)
			require.Error(t, err)
			assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState,
				"transient category must not flip the connection")

			records := onlyTokenRefreshFailure(read())
			require.Len(t, records, 1, "the structured event still emits — only the unhealthy flip is suppressed")
		})
	}
}

// TestClassifyAndRecordRefreshFailure_PermanentOnAlreadyUnhealthyNoop —
// MarkHealthState is idempotent, so a second permanent failure on a
// connection that is already unhealthy should not emit a duplicate
// "connection health state changed" event downstream. We verify here
// that classifyAndRecordRefreshFailure still emits the failure event
// (operators want to see every failed refresh) but the unhealthy state
// is preserved.
func TestClassifyAndRecordRefreshFailure_PermanentOnAlreadyUnhealthyNoop(t *testing.T) {
	logger, read := bufLogger(t)
	conn := &mockCore.Connection{
		Id:          apid.New(apid.PrefixConnection),
		HealthState: database.ConnectionHealthStateUnhealthy,
	}
	o := &oAuth2Connection{logger: logger, connection: conn}

	err := o.classifyAndRecordRefreshFailure(
		context.Background(),
		tokenRefreshInvalidGrant,
		400,
		"invalid_grant",
		1,
		errors.New("status 400"),
	)
	require.Error(t, err)
	assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState)

	records := onlyTokenRefreshFailure(read())
	require.Len(t, records, 1)
}

// TestClassifyAndRecordRefreshFailure_NilConnectionDoesNotPanic — the
// internal-error code paths in refreshAccessToken can fire before the
// connection field is fully populated (defensive coding around the
// constructor). Calling classifyAndRecordRefreshFailure with
// connection==nil must still emit the event and not panic.
func TestClassifyAndRecordRefreshFailure_NilConnectionDoesNotPanic(t *testing.T) {
	logger, read := bufLogger(t)
	o := &oAuth2Connection{logger: logger, connection: nil}

	err := o.classifyAndRecordRefreshFailure(
		context.Background(),
		tokenRefreshInternalError,
		0,
		"",
		0,
		errors.New("decrypt failed"),
	)
	require.Error(t, err)
	records := onlyTokenRefreshFailure(read())
	require.Len(t, records, 1)
	assert.Equal(t, string(tokenRefreshInternalError), records[0]["category"])
}

// TestClassifyAndRecordRefreshFailure_ReasonStringFormat pins the
// "refresh_<category>" reason string that lands on the
// "connection health state changed" event. Dashboards correlate this
// with the failure event by category — if the format ever drifts, the
// correlation breaks silently.
func TestClassifyAndRecordRefreshFailure_ReasonStringFormat(t *testing.T) {
	logger, _ := bufLogger(t)
	captured := &reasonCapturingConnection{
		Connection: mockCore.Connection{Id: apid.New(apid.PrefixConnection)},
	}
	o := &oAuth2Connection{logger: logger, connection: captured}

	_ = o.classifyAndRecordRefreshFailure(
		context.Background(),
		tokenRefreshInvalidGrant,
		400,
		"invalid_grant",
		1,
		errors.New("status 400"),
	)

	assert.Equal(t, "refresh_invalid_grant", captured.lastReason,
		"reason must be refresh_<category> so dashboards can correlate")
}

// reasonCapturingConnection wraps mockCore.Connection to record the
// reason argument passed to MarkHealthState. The embedded type provides
// the rest of the iface.Connection methods so we only override the one
// we want to observe.
type reasonCapturingConnection struct {
	mockCore.Connection
	lastReason string
}

func (c *reasonCapturingConnection) MarkHealthState(ctx context.Context, state database.ConnectionHealthState, reason string) error {
	c.lastReason = reason
	return c.Connection.MarkHealthState(ctx, state, reason)
}

// TestClassifyAndRecordRefreshFailure_AttemptsEmittedWhenSet pins that
// the attempts field plumbed in from postRefreshWithRetry actually lands
// on the structured event. PR C's integration tests will assert
// attempts=tokenRefreshMaxAttempts on the exhausted-retry path; if the
// wiring here regressed, those assertions would silently see attempts=0
// and pass for the wrong reason.
func TestClassifyAndRecordRefreshFailure_AttemptsEmittedWhenSet(t *testing.T) {
	logger, read := bufLogger(t)
	conn := &mockCore.Connection{
		Id:          apid.New(apid.PrefixConnection),
		HealthState: database.ConnectionHealthStateHealthy,
	}
	o := &oAuth2Connection{logger: logger, connection: conn}

	_ = o.classifyAndRecordRefreshFailure(
		context.Background(),
		tokenRefreshProvider5xx,
		503,
		"",
		tokenRefreshMaxAttempts,
		errors.New("status 503"),
	)

	records := onlyTokenRefreshFailure(read())
	require.Len(t, records, 1)
	assert.EqualValues(t, tokenRefreshMaxAttempts, records[0]["attempts"],
		"attempts must round-trip through the structured event so retry exhaustion is observable")
}

// TestClassifyAndRecordRefreshFailure_AttemptsOmittedWhenZero — the
// no-HTTP-call paths (no_refresh_token, internal_error before the POST)
// supply attempts=0. The event must omit the field entirely rather than
// emitting attempts=0, which would be observably confusing on dashboards
// (looks like the helper claims it didn't try at all).
func TestClassifyAndRecordRefreshFailure_AttemptsOmittedWhenZero(t *testing.T) {
	logger, read := bufLogger(t)
	conn := &mockCore.Connection{
		Id:          apid.New(apid.PrefixConnection),
		HealthState: database.ConnectionHealthStateHealthy,
	}
	o := &oAuth2Connection{logger: logger, connection: conn}

	_ = o.classifyAndRecordRefreshFailure(
		context.Background(),
		tokenRefreshNoRefreshToken,
		0,
		"",
		0,
		errNoRefreshToken,
	)

	records := onlyTokenRefreshFailure(read())
	require.Len(t, records, 1)
	_, present := records[0]["attempts"]
	assert.False(t, present, "attempts=0 must be omitted, not emitted as the literal zero value")
}

// scriptedRefreshServer is the refresh-path analog of scriptedTokenServer
// in callback_test.go — serves the supplied responses one per request,
// returning the final entry for further calls. Lets the retry tests pin
// attempt counts deterministically.
func scriptedRefreshServer(t *testing.T, responses []scriptedResponse) (*httptest.Server, *int32, *[]url.Values) {
	t.Helper()
	var calls int32
	var bodies []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := int(atomic.AddInt32(&calls, 1)) - 1
		body, _ := url.ParseQuery(readBody(t, r))
		bodies = append(bodies, body)
		resp := responses[len(responses)-1]
		if idx < len(responses) {
			resp = responses[idx]
		}
		w.WriteHeader(resp.status)
		_, _ = w.Write([]byte(resp.body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &bodies
}

func newRefreshRetryTestConn(t *testing.T) *oAuth2Connection {
	t.Helper()
	logger, _ := bufLogger(t)
	return &oAuth2Connection{logger: logger}
}

func TestPostRefreshWithRetry_SuccessFirstAttempt(t *testing.T) {
	srv, calls, bodies := scriptedRefreshServer(t, []scriptedResponse{
		{status: 200, body: `{"access_token":"a"}`},
	})
	o := newRefreshRetryTestConn(t)

	resp, attempts, err := o.postRefreshWithRetry(
		context.Background(),
		gentleman.New(),
		srv.URL,
		url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"rt"}},
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, attempts, "200 must not retry")
	assert.Equal(t, int32(1), atomic.LoadInt32(calls))
	require.Len(t, *bodies, 1)
	assert.Equal(t, "rt", (*bodies)[0].Get("refresh_token"),
		"posted body must include the refresh_token form value verbatim")
	assert.Equal(t, "refresh_token", (*bodies)[0].Get("grant_type"))
}

// TestPostRefreshWithRetry_4xxNotRetried is the load-bearing invariant
// of the refresh retry policy: 4xx responses are non-retryable. The
// provider has classified the refresh token as invalid/expired/revoked
// and resubmitting won't change that — with rotating-refresh providers
// it can actively make things worse. A regression here would silently
// break PR B's integration tests by inflating the observed call count.
func TestPostRefreshWithRetry_4xxNotRetried(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 422, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, calls, _ := scriptedRefreshServer(t, []scriptedResponse{
				{status: status, body: `{"error":"invalid_grant"}`},
			})
			o := newRefreshRetryTestConn(t)

			resp, attempts, err := o.postRefreshWithRetry(
				context.Background(),
				gentleman.New(),
				srv.URL,
				url.Values{"grant_type": {"refresh_token"}},
			)

			require.NoError(t, err, "transport-layer call succeeded — 4xx is not a transport error")
			require.NotNil(t, resp)
			assert.Equal(t, status, resp.StatusCode)
			assert.Equal(t, 1, attempts)
			assert.Equal(t, int32(1), atomic.LoadInt32(calls),
				"4xx must terminate the loop on the first attempt")
		})
	}
}

func TestPostRefreshWithRetry_5xxThenSuccess(t *testing.T) {
	srv, calls, _ := scriptedRefreshServer(t, []scriptedResponse{
		{status: 503, body: `{"error":"temporarily_unavailable"}`},
		{status: 200, body: `{"access_token":"a"}`},
	})
	o := newRefreshRetryTestConn(t)

	start := time.Now()
	resp, attempts, err := o.postRefreshWithRetry(
		context.Background(),
		gentleman.New(),
		srv.URL,
		url.Values{},
	)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, int32(2), atomic.LoadInt32(calls))
	assert.GreaterOrEqual(t, elapsed, tokenRefreshBackoffStep,
		"second attempt must wait at least one backoff step")
}

// TestPostRefreshWithRetry_5xxExhausted pins the exhaustion shape: the
// helper returns the last response (so the caller can classify on its
// status code), with attempts=tokenRefreshMaxAttempts so the failure
// event can record budget exhaustion.
func TestPostRefreshWithRetry_5xxExhausted(t *testing.T) {
	srv, calls, _ := scriptedRefreshServer(t, []scriptedResponse{
		{status: 503, body: `{"error":"temporarily_unavailable"}`},
	})
	o := newRefreshRetryTestConn(t)

	resp, attempts, err := o.postRefreshWithRetry(
		context.Background(),
		gentleman.New(),
		srv.URL,
		url.Values{},
	)

	require.NoError(t, err, "5xx is an HTTP-level failure, not a transport error")
	require.NotNil(t, resp, "exhausted retry must return the last response so the caller can classify")
	assert.Equal(t, 503, resp.StatusCode)
	assert.Equal(t, tokenRefreshMaxAttempts, attempts)
	assert.Equal(t, int32(tokenRefreshMaxAttempts), atomic.LoadInt32(calls),
		"helper must make exactly tokenRefreshMaxAttempts calls and stop")
}

// TestPostRefreshWithRetry_TransportErrorRetried — when the dial fails
// (server closed, DNS, etc.) the helper has no response to inspect, so
// it falls into the same retry branch as a 5xx. Closing the test server
// before the call ensures every attempt hits "connection refused".
func TestPostRefreshWithRetry_TransportErrorRetried(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := srv.URL
	srv.Close()
	o := newRefreshRetryTestConn(t)

	resp, attempts, err := o.postRefreshWithRetry(
		context.Background(),
		gentleman.New(),
		closedURL,
		url.Values{},
	)

	require.Error(t, err, "transport failure must surface as an error")
	if resp != nil {
		assert.Equal(t, 0, resp.StatusCode,
			"transport-error response carries no status code; classifier keys on err, not resp")
	}
	assert.Equal(t, tokenRefreshMaxAttempts, attempts,
		"transport errors are retried like 5xx — full budget should be consumed")
}

// TestPostRefreshWithRetry_ContextCancelledBeforeRetry — caller context
// is honored between attempts. Cancellation after the first 5xx must
// short-circuit with ctx.Err and an attempt count reflecting only the
// calls actually completed before cancellation.
func TestPostRefreshWithRetry_ContextCancelledBeforeRetry(t *testing.T) {
	srv, calls, _ := scriptedRefreshServer(t, []scriptedResponse{
		{status: 503, body: `{"error":"temporarily_unavailable"}`},
	})
	o := newRefreshRetryTestConn(t)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	resp, attempts, err := o.postRefreshWithRetry(
		ctx,
		gentleman.New(),
		srv.URL,
		url.Values{},
	)

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, resp, "cancellation in the backoff window returns nil response")
	assert.Equal(t, 1, attempts,
		"attempts records only completed calls — the in-flight cancellation was during backoff")
	assert.Equal(t, int32(1), atomic.LoadInt32(calls),
		"server should have observed exactly one call before cancellation")
}
