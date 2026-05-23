package oauth2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gentleman "gopkg.in/h2non/gentleman.v2"
)

// configWithPublicBaseUrl returns a config whose Public.GetBaseUrl()
// resolves to the given domain (no port). Used by getPublicCallbackUrl
// tests so the assertion can pin the rendered URL exactly without
// pulling in TLS / port plumbing.
func configWithPublicBaseUrl(t *testing.T, domain string) config.C {
	t.Helper()
	return config.FromRoot(&sconfig.Root{
		Public: sconfig.ServicePublic{
			ServiceHttp: sconfig.ServiceHttp{
				DomainVal: domain,
				PortVal:   common.NewIntegerValueDirect(80),
			},
		},
	})
}

func TestGetPublicCallbackUrl_NilConfig(t *testing.T) {
	o := &oAuth2Connection{cfg: nil}
	got, err := o.getPublicCallbackUrl()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config is nil")
	assert.Empty(t, got)
}

func TestGetPublicCallbackUrl_NilRoot(t *testing.T) {
	o := &oAuth2Connection{cfg: config.FromRoot(nil)}
	got, err := o.getPublicCallbackUrl()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config root is nil")
	assert.Empty(t, got)
}

func TestGetPublicCallbackUrl_AppendsCallbackPath(t *testing.T) {
	o := &oAuth2Connection{cfg: configWithPublicBaseUrl(t, "proxy.example.com")}
	got, err := o.getPublicCallbackUrl()
	require.NoError(t, err)
	assert.Equal(t, "http://proxy.example.com/oauth2/callback", got)
}

// TestAppendSetupPendingToReturnUrl_AddsExpectedQueryParams pins the
// shape the marketplace UI keys off when re-rendering an auth_failed
// connection. If these param names ever change, every UI consumer
// breaks; the test exists to make that explicit.
func TestAppendSetupPendingToReturnUrl_AddsExpectedQueryParams(t *testing.T) {
	connID := apid.New(apid.PrefixConnection)
	o := &oAuth2Connection{connection: &mockCore.Connection{Id: connID}}

	got := o.appendSetupPendingToReturnUrl("https://app.example.com/return")

	parsed, err := url.Parse(got)
	require.NoError(t, err)
	assert.Equal(t, "pending", parsed.Query().Get("setup"))
	assert.Equal(t, connID.String(), parsed.Query().Get("connection_id"))
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "app.example.com", parsed.Host)
	assert.Equal(t, "/return", parsed.Path)
}

// TestAppendSetupPendingToReturnUrl_PreservesExistingQuery — the
// marketplace passes its own state (e.g. tab selection) on the return
// URL; we must not clobber it when adding our setup-pending markers.
func TestAppendSetupPendingToReturnUrl_PreservesExistingQuery(t *testing.T) {
	connID := apid.New(apid.PrefixConnection)
	o := &oAuth2Connection{connection: &mockCore.Connection{Id: connID}}

	got := o.appendSetupPendingToReturnUrl("https://app.example.com/return?tab=integrations&utm=email")

	parsed, err := url.Parse(got)
	require.NoError(t, err)
	q := parsed.Query()
	assert.Equal(t, "integrations", q.Get("tab"))
	assert.Equal(t, "email", q.Get("utm"))
	assert.Equal(t, "pending", q.Get("setup"))
	assert.Equal(t, connID.String(), q.Get("connection_id"))
}

// TestAppendSetupPendingToReturnUrl_ParseFailureFallback covers the
// defensive `if err != nil { return raw }` branch. url.Parse rejects
// strings containing control characters, so we use one to exercise
// the fallback without needing a stub URL parser.
func TestAppendSetupPendingToReturnUrl_ParseFailureFallback(t *testing.T) {
	o := &oAuth2Connection{connection: &mockCore.Connection{Id: apid.New(apid.PrefixConnection)}}

	raw := "https://app.example.com/\x7f"
	got := o.appendSetupPendingToReturnUrl(raw)
	assert.Equal(t, raw, got, "unparseable URL must be returned untouched so the redirect surface is at least defined")
}

// scriptedTokenServer returns an httptest.Server that serves the
// supplied response sequence one entry per request, then keeps
// returning the final entry. Lets retry tests assert on attempt
// counts deterministically without orchestrating a real provider.
type scriptedResponse struct {
	status int
	body   string
}

func scriptedTokenServer(t *testing.T, responses []scriptedResponse) (*httptest.Server, *int32, *[]url.Values) {
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

func readBody(t *testing.T, r *http.Request) string {
	t.Helper()
	defer r.Body.Close()
	buf := make([]byte, 4096)
	n, _ := r.Body.Read(buf)
	return string(buf[:n])
}

func newRetryTestConn(t *testing.T) *oAuth2Connection {
	t.Helper()
	logger, _ := bufLogger(t)
	return &oAuth2Connection{
		auth:   &sconfig.AuthOAuth2{Token: cschema.AuthOauth2Token{}},
		logger: logger,
	}
}

func TestPostTokenExchangeWithRetry_SuccessFirstAttempt(t *testing.T) {
	srv, calls, bodies := scriptedTokenServer(t, []scriptedResponse{
		{status: 200, body: `{"access_token":"a"}`},
	})
	o := newRetryTestConn(t)

	resp, attempts, err := o.postTokenExchangeWithRetry(
		context.Background(),
		gentleman.New(),
		srv.URL,
		url.Values{"grant_type": {"authorization_code"}, "code": {"abc"}},
		"",
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, attempts, "200 must not retry")
	assert.Equal(t, int32(1), atomic.LoadInt32(calls))
	require.Len(t, *bodies, 1)
	assert.Equal(t, "abc", (*bodies)[0].Get("code"),
		"posted body must include the form values verbatim")
}

// TestPostTokenExchangeWithRetry_4xxNotRetried is the load-bearing
// invariant of the policy: 4xx responses are non-retryable because
// resubmitting the same authorization code would burn it. A
// regression here (e.g. retrying on 400) would silently break every
// 4xx integration test by changing the observed call count.
func TestPostTokenExchangeWithRetry_4xxNotRetried(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 422, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, calls, _ := scriptedTokenServer(t, []scriptedResponse{
				{status: status, body: `{"error":"invalid_grant"}`},
			})
			o := newRetryTestConn(t)

			resp, attempts, err := o.postTokenExchangeWithRetry(
				context.Background(),
				gentleman.New(),
				srv.URL,
				url.Values{"grant_type": {"authorization_code"}},
				"",
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

func TestPostTokenExchangeWithRetry_5xxThenSuccess(t *testing.T) {
	srv, calls, _ := scriptedTokenServer(t, []scriptedResponse{
		{status: 503, body: `{"error":"temporarily_unavailable"}`},
		{status: 200, body: `{"access_token":"a"}`},
	})
	o := newRetryTestConn(t)

	start := time.Now()
	resp, attempts, err := o.postTokenExchangeWithRetry(
		context.Background(),
		gentleman.New(),
		srv.URL,
		url.Values{},
		"",
	)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, int32(2), atomic.LoadInt32(calls))
	assert.GreaterOrEqual(t, elapsed, tokenExchangeBackoffStep,
		"second attempt must wait at least one backoff step")
}

// TestPostTokenExchangeWithRetry_5xxExhausted asserts the exhaustion
// shape contracted with the failure event path: after the budget is
// burned, the helper returns the *last* response so the caller can
// classify on its status code, plus an attempts count equal to
// tokenExchangeMaxAttempts so the failure event can record budget
// exhaustion.
func TestPostTokenExchangeWithRetry_5xxExhausted(t *testing.T) {
	srv, calls, _ := scriptedTokenServer(t, []scriptedResponse{
		{status: 503, body: `{"error":"temporarily_unavailable"}`},
	})
	o := newRetryTestConn(t)

	resp, attempts, err := o.postTokenExchangeWithRetry(
		context.Background(),
		gentleman.New(),
		srv.URL,
		url.Values{},
		"",
	)

	require.NoError(t, err, "5xx is an HTTP-level failure, not a transport error")
	require.NotNil(t, resp, "exhausted retry must return the last response so the caller can classify")
	assert.Equal(t, 503, resp.StatusCode)
	assert.Equal(t, tokenExchangeMaxAttempts, attempts)
	assert.Equal(t, int32(tokenExchangeMaxAttempts), atomic.LoadInt32(calls),
		"helper must make exactly tokenExchangeMaxAttempts calls and stop")
}

// TestPostTokenExchangeWithRetry_TransportErrorRetried — when the
// dial fails (server closed, DNS fails, etc.) the helper has no
// response to inspect, so it falls into the same retry branch as a
// 5xx. We close the test server before calling so every attempt hits
// "connection refused".
func TestPostTokenExchangeWithRetry_TransportErrorRetried(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := srv.URL
	srv.Close() // ensure dials fail with "connection refused"
	o := newRetryTestConn(t)

	resp, attempts, err := o.postTokenExchangeWithRetry(
		context.Background(),
		gentleman.New(),
		closedURL,
		url.Values{},
		"",
	)

	require.Error(t, err, "transport failure must surface as an error")
	if resp != nil {
		assert.Equal(t, 0, resp.StatusCode,
			"transport-error response carries no status code; downstream classifier keys on err, not resp")
	}
	assert.Equal(t, tokenExchangeMaxAttempts, attempts,
		"transport errors are retried like 5xx — full budget should be consumed")
}

// TestPostTokenExchangeWithRetry_ContextCancelledBeforeRetry — the
// caller's context is honored *between* attempts. Cancelling after
// the first 5xx must short-circuit the loop with ctx.Err and an
// attempt count reflecting only the calls actually completed.
func TestPostTokenExchangeWithRetry_ContextCancelledBeforeRetry(t *testing.T) {
	srv, calls, _ := scriptedTokenServer(t, []scriptedResponse{
		{status: 503, body: `{"error":"temporarily_unavailable"}`},
	})
	o := newRetryTestConn(t)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately after the first response would arrive; the
	// helper checks ctx.Done() inside the backoff select.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	resp, attempts, err := o.postTokenExchangeWithRetry(
		ctx,
		gentleman.New(),
		srv.URL,
		url.Values{},
		"",
	)

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, resp, "cancellation in the backoff window returns nil response")
	assert.Equal(t, 1, attempts,
		"attempts records only completed calls — the in-flight cancellation was during backoff")
	assert.Equal(t, int32(1), atomic.LoadInt32(calls),
		"server should have observed exactly one call before cancellation")
}

// TestPostTokenExchangeWithRetry_QueryOverridesAppliedEachAttempt —
// the retry loop rebuilds the gentleman.Request each iteration
// (single-use API). This test pins the contract that connector-level
// QueryOverrides are reapplied on every retry, not silently dropped
// after the first attempt.
func TestPostTokenExchangeWithRetry_QueryOverridesAppliedEachAttempt(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":"temporarily_unavailable"}`))
	}))
	t.Cleanup(srv.Close)

	logger, _ := bufLogger(t)
	o := &oAuth2Connection{
		auth: &sconfig.AuthOAuth2{Token: cschema.AuthOauth2Token{
			QueryOverrides: map[string]string{"audience": "api.example.com"},
		}},
		logger: logger,
	}

	_, attempts, err := o.postTokenExchangeWithRetry(
		context.Background(),
		gentleman.New(),
		srv.URL,
		url.Values{},
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, tokenExchangeMaxAttempts, attempts)

	require.Len(t, queries, tokenExchangeMaxAttempts)
	for i, q := range queries {
		assert.Truef(t, strings.Contains(q, "audience=api.example.com"),
			"attempt %d should carry connector QueryOverrides; got raw query %q", i+1, q)
	}
}
