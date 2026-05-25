//go:build integration

package oauth2

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// Scenario 18 from issue #177: provider-side timeouts. The proxy has
// no time-budgeted HTTP timeout configured — `httpf.factory` wraps
// `http.DefaultTransport` without setting `Timeout` on the underlying
// client, and the inbound gin server is constructed without
// `ReadTimeout` / `WriteTimeout`, so no per-request deadline ever
// lands on the ctx the handler runs under. Cancellation can still
// arrive via context propagation if the *calling client* closes its
// connection: the refresh, proxy, and revocation legs thread `ctx`
// through `gentleman.UseContext(ctx)` so the in-flight upstream
// request aborts in that case. The token-exchange leg
// (`postTokenExchangeWithRetry`) does not call `UseContext` on the
// request itself, but the retry-loop's backoff `select` does check
// `ctx.Done()` between attempts.
//
// What this means for "what happens when the provider hangs":
//   - if the calling client stays connected, there is no source of
//     timeout — the request waits indefinitely on whichever leg is
//     blocked;
//   - if the calling client disconnects, refresh/proxy/revocation
//     abort the upstream request via ctx; token exchange aborts only
//     between attempts.
//
// The closest faithful reproduction of a "provider stopped responding"
// failure that the proxy can actually observe is a connection that
// terminates mid-flight. From the proxy's perspective a server-side
// timeout, a TCP reset, and a process crash all surface identically
// as a transport error on the pending POST/GET. The test provider's
// `ScriptAction{DropConnection: true}` reproduces this by hijacking
// the response writer and closing the underlying connection.
//
// The proxy classifies any transport-layer failure on the token-exchange
// or refresh leg as `network_error` (transient). The token endpoint and
// refresh endpoint both retry transport errors through their shared
// retry helpers (`postTokenExchangeWithRetry`, `postRefreshWithRetry`)
// up to {tokenExchange,tokenRefresh}MaxAttempts. The upstream-API leg
// (`ProxyRequest` → `sendProxyRequest`) does NOT retry on transport
// errors — the 401-retry-after-refresh path is the only retry on the
// proxy leg and it keys on the HTTP status, which doesn't exist when
// the connection dies.
//
// What is NOT covered here (documented in provider_timeouts_test.md):
//
//   - **Authorize endpoint timeouts.** The proxy never POSTs to the
//     authorize endpoint — the browser hits it directly. A timeout
//     there manifests as a UI failure, not a proxy failure.
//   - **Revocation endpoint timeouts.** Revocation runs in the
//     background via the disconnect_connection Asynq task. Coverage
//     for the disconnect path lives with the P2 disconnect tests
//     (#181); folding it in here would require standing up a worker
//     in the integration env.

// TestTokenExchangeTimeout_RetriesAndExhausts — provider drops the
// connection on every /token POST. The proxy must retry the full budget
// (tokenExchangeMaxAttempts = 3), emit exactly one failure event with
// category=network_error and attempts=3, and land in the auth_failed
// terminal state with no token persisted.
//
// FailCount=10 outlasts the retry budget; the proxy gives up after 3
// attempts and the remaining scripted drops sit unconsumed.
func TestTokenExchangeTimeout_RetriesAndExhausts(t *testing.T) {
	rig := newTokenExchangeRetryRig(t, "te-timeout-exhausted")
	connID, stateID, code := rig.initiateAndMintCode(t)

	rig.provider.Script(rig.clientKey, helpers.EndpointToken, helpers.ScriptAction{
		DropConnection: true,
		FailCount:      10,
	})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))

	require.Truef(t, strings.HasPrefix(loc, rig.returnToURL),
		"timeout exhaustion should still 302 to return_to_url with setup=pending; got %q", loc)
	parsed, err := url.Parse(loc)
	require.NoError(t, err)
	assert.Equal(t, "pending", parsed.Query().Get("setup"))
	assert.Equal(t, connID, parsed.Query().Get("connection_id"))

	// Exactly one failure event with category=network_error and
	// attempts=tokenExchangeMaxAttempts. provider_status_code is
	// absent because no response was ever received — the failure is
	// at the transport layer before any HTTP status exists.
	events := rig.logCapture.RecordsWithMessage(t, tokenExchangeFailureMessage)
	require.Lenf(t, events, 1, "exhausted timeout should emit exactly one failure event; got %d (%v)", len(events), events)
	assert.Equal(t, "network_error", events[0]["category"])
	assert.Equal(t, stateID, events[0]["state_id"])
	assert.Equal(t, float64(3), events[0]["attempts"],
		"failure event should record attempts=tokenExchangeMaxAttempts on exhaustion")
	_, hasStatus := events[0]["provider_status_code"]
	assert.Falsef(t, hasStatus,
		"provider_status_code must be absent on a pure transport failure; got %v", events[0]["provider_status_code"])

	// No token row persisted — the exchange never produced a response
	// to parse.
	require.Nil(t, rig.env.GetOAuth2Token(t, connID))

	// Connection in auth_failed terminal state.
	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateSetup, conn.State)
	require.NotNil(t, conn.SetupStep)
	assert.Truef(t, conn.SetupStep.Equals(cschema.SetupStepAuthFailed),
		"exhausted timeout should land in auth_failed; got %q", conn.SetupStep.String())
	require.NotNil(t, conn.SetupError)

	// Exactly tokenExchangeMaxAttempts POSTs to /token. If the proxy
	// gave up early this is < 3; if it ignored the budget, > 3. The
	// recorder counts requests that completed enough of the HTTP
	// preamble for the server to dispatch them — DropConnection runs
	// inside the script middleware after dispatch, so each dropped
	// attempt still increments the recorder.
	assert.Equal(t, 3, rig.tokenCallCount(),
		"proxy should make exactly tokenExchangeMaxAttempts POSTs before giving up")
}

// TestTokenRefreshTimeout_RetriesAndExhausts — provider drops the
// connection on every refresh-token POST. The proxy must retry the
// full budget, emit one failure event with category=network_error and
// attempts=tokenRefreshMaxAttempts, return non-200 to the caller, and
// — critically — keep the connection healthy because network_error is
// classified transient. The persisted refresh-token row must not be
// mutated.
func TestTokenRefreshTimeout_RetriesAndExhausts(t *testing.T) {
	rig := newProxyRefreshRig(t, "refresh-timeout-exhausted")
	connID := rig.completeAuthFlow(t)
	rig.forceTokenExpired(t, connID, false)

	// Snapshot the row that exists when the refresh POST is about to
	// fire. A transport failure must leave this exact row in place —
	// no partial response can corrupt the encrypted refresh_token.
	preFailureToken := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, preFailureToken, "forge-expire must leave a token row")
	preFailureTokenID := preFailureToken.Id
	preFailureEncryptedRefresh := preFailureToken.EncryptedRefreshToken

	rig.provider.Script(rig.clientKey, helpers.EndpointRefresh, helpers.ScriptAction{
		DropConnection: true,
		FailCount:      10,
	})

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.NotEqualf(t, 200, w.Code,
		"proxy must fail when refresh exhausts retry budget on transport errors; got 200 body=%s", w.Body.String())

	// Exactly one failure event with category=network_error and
	// attempts=tokenRefreshMaxAttempts. provider_status_code is
	// omitted (no response was ever received).
	failed := rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage)
	require.Lenf(t, failed, 1, "exhausted refresh timeout must emit exactly one refresh-failed event; got %d (%v)", len(failed), failed)
	event := failed[0]
	assert.Equal(t, "network_error", event["category"])
	assert.Equal(t, connID, event["connection_id"])
	assert.Equalf(t, float64(tokenRefreshMaxAttempts), event["attempts"],
		"failure event should record attempts=tokenRefreshMaxAttempts on exhaustion")
	_, hasStatus := event["provider_status_code"]
	assert.Falsef(t, hasStatus,
		"provider_status_code must be absent on a pure transport failure; got %v", event["provider_status_code"])

	// network_error is transient — must NOT flip the connection
	// unhealthy. This is the load-bearing distinction from the
	// permanent categories: the next proxy call gets another chance,
	// so dashboards shouldn't paint the connection as broken on a
	// transient blip.
	conn := rig.env.GetConnection(t, connID)
	assert.Equalf(t, database.ConnectionHealthStateHealthy, conn.HealthState,
		"transient transport failure must not flip the connection unhealthy; got %q", conn.HealthState)
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage),
		"transient transport failure must not emit a health-state-changed event")

	// Exactly tokenRefreshMaxAttempts - 1 retry-warn lines preceded
	// the final failure (the terminal attempt does not schedule a
	// further retry).
	retryWarns := rig.logCapture.RecordsWithMessage(t, tokenRefreshRetryMessage)
	assert.Lenf(t, retryWarns, tokenRefreshMaxAttempts-1,
		"expected %d retry-warn logs (one before each retried attempt); got %d",
		tokenRefreshMaxAttempts-1, len(retryWarns))

	// Token row preservation: a transport failure cannot land partial
	// bytes in the DB because the response body is never parsed. The
	// pre-failure row must remain byte-identical.
	postFailureToken := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, postFailureToken, "transport-error exhaustion must not delete the token row")
	assert.Equalf(t, preFailureTokenID, postFailureToken.Id,
		"transport-error exhaustion must not insert a replacement token row")
	assert.Equalf(t, preFailureEncryptedRefresh, postFailureToken.EncryptedRefreshToken,
		"transport-error exhaustion must not mutate the stored refresh_token")

	// Exactly tokenRefreshMaxAttempts refresh POSTs observed.
	assert.Equalf(t, tokenRefreshMaxAttempts, rig.refreshCallCount(),
		"proxy must make exactly tokenRefreshMaxAttempts refresh POSTs before giving up; got %d",
		rig.refreshCallCount())

	// No success event.
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage),
		"failed refresh must not emit a refresh-succeeded event")
}

// TestUpstreamApiTimeout_SurfacedToCaller — provider drops the
// connection on the proxied resource call. The proxy's resource leg
// has no retry budget (only the 401-after-refresh path retries, and
// that keys on HTTP status which is absent here). The transport error
// must propagate as a non-200 to the caller, must not trigger a
// refresh attempt, and must leave both the connection state and its
// health unchanged.
func TestUpstreamApiTimeout_SurfacedToCaller(t *testing.T) {
	rig := newProxyRefreshRig(t, "upstream-timeout")
	connID := rig.completeAuthFlow(t)

	// Token from completeAuthFlow is still valid, so the proxy will
	// not attempt a proactive refresh. Any refresh attempt observed
	// after this point would indicate the transport error was
	// misclassified as a 401.
	//
	// EndpointResource requests carry a Bearer access token, not a
	// client_id form field or Basic auth, so extractClientID on the
	// test provider returns "" for them. Script on the wildcard
	// (empty clientID) queue so the action actually matches the
	// inbound request.
	//
	// The recorder is process-global on the docker test provider, so
	// other tests in the package have already accumulated /test/resource
	// records. Snapshot `since` before firing the proxy request so the
	// "exactly one upstream call" assertion only counts the calls this
	// test produced. completeAuthFlow does not call /test/resource, so
	// `since` only needs to exclude prior tests, not earlier steps in
	// this one.
	since := time.Now()
	rig.provider.Script("", helpers.EndpointResource, helpers.ScriptAction{
		DropConnection: true,
		FailCount:      10,
	})

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.NotEqualf(t, 200, w.Code,
		"upstream transport failure must surface as non-200 to the caller; got 200 body=%s", w.Body.String())

	// Exactly one resource GET observed since the snapshot. The
	// proxy leg does not retry on transport errors —
	// sendProxyRequest returns the error directly and ProxyRequest
	// propagates it without consulting the 401-retry path.
	resourceReqs := rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointResource,
		Since:    since,
	})
	assert.Lenf(t, resourceReqs, 1,
		"proxy must make exactly one upstream call on transport failure (no retry); got %d", len(resourceReqs))

	// No refresh attempted: the transport error must not be confused
	// with a 401. A non-zero refresh count here would mean the proxy
	// reacted to an error that looked like an auth failure when it
	// was actually a connectivity failure.
	assert.Equalf(t, 0, rig.refreshCallCount(),
		"upstream transport failure must not trigger a refresh; got %d refresh POSTs", rig.refreshCallCount())
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage),
		"upstream transport failure must not emit a refresh-failed event")
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage),
		"upstream transport failure must not emit a refresh-succeeded event")

	// Connection and health untouched. An upstream connectivity blip
	// is not a credential problem; flipping unhealthy here would
	// paint a working connection broken.
	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateConfigured, conn.State,
		"upstream transport failure must not change the connection state")
	assert.Equalf(t, database.ConnectionHealthStateHealthy, conn.HealthState,
		"upstream transport failure must not flip the connection unhealthy; got %q", conn.HealthState)
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage),
		"upstream transport failure must not emit a health-state-changed event")
}
