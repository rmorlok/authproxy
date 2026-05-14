//go:build integration

package oauth2

import (
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/database"
)

// Scenario 10 from issue #171: concurrent refresh safety. When N proxy
// requests detect an expired access token at the same time, the redis
// mutex in refreshAccessTokenInner must serialize them so:
//
//   - exactly one refresh POST reaches the provider — losers acquire the
//     mutex after the winner persists the new token and find via
//     re-read that the access token is no longer expired, so they return
//     without re-POSTing;
//   - every concurrent proxy request observes the *final* valid access
//     token (all N return 200);
//   - exactly one new oauth2_tokens row is persisted (the winner's);
//   - the rotated refresh_token from the winner is not corrupted by a
//     concurrent stale-write — a TOCTOU bug here would replay the old RT
//     against the provider and 400.
//
// Under the test provider's default rotation policy, the old refresh_token
// is revoked on rotation. If the mutex regressed and a loser POSTed
// /token with the pre-refresh RT after the winner had already rotated it,
// the provider would return 400 invalid_grant and the proxy would emit a
// refresh-failed event. So "exactly one refresh POST + no failure event"
// is the load-bearing signal for both the serialization property and the
// rotation-doesn't-corrupt-credentials property.
//
// Why a delay-only script action
//
// The fixture relies on `ScriptAction{DelayMs: …}` (Status=0, Body=""):
// the script middleware sleeps the configured duration and then falls
// through to the *real* refresh handler. This keeps the response a real
// provider-issued grant (valid bearer credential at /echo) while making
// the winner's refresh slow enough that all concurrent callers reach
// the mutex before the winner releases it. A scripted-body action
// would mint a fake access_token that /echo would 401, triggering the
// proxy's retry-once-after-refresh path and double-counting refreshes
// (the same trap PR A documented for the rotation tests).
//
// The script action is consumed only by the *first* request that reaches
// the script middleware. So if the proxy mutex regressed and N goroutines
// all POSTed concurrently, the first would be delayed, the rest would
// pass through without delay — the second-to-Nth would then try to
// refresh with an RT the provider had already rotated, returning 400.
// That is the exact regression signal: failure events appear, not just
// extra successes.

const concurrentRefreshConcurrency = 8

// TestProxyRefresh_ConcurrentRequestsRefreshExactlyOnce — N proxy
// requests are launched simultaneously against a connection whose
// access_token has been forge-expired. The mutex around
// refreshAccessTokenInner must serialize them so exactly one refresh
// POST hits the provider; the rest read back the freshly-persisted
// token and skip the POST.
func TestProxyRefresh_ConcurrentRequestsRefreshExactlyOnce(t *testing.T) {
	rig := newProxyRefreshRig(t, "refresh-concurrent")
	connID := rig.completeAuthFlow(t)

	preToken := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, preToken)
	preTokenID := preToken.Id
	preRefreshPlaintext := rig.env.DecryptOAuth2RefreshToken(t, preToken)

	rig.forceTokenExpired(t, connID, false)

	// Delay-only action: the winner's refresh POST is slept this long
	// inside the script middleware before falling through to the real
	// handler. Long enough that all N goroutines reach the mutex while
	// the winner still holds it; short enough that the whole test wraps
	// up in well under the 30s default refresh timeout.
	const refreshDelayMs = 500
	rig.provider.Script(rig.clientKey, helpers.EndpointRefresh, helpers.ScriptAction{DelayMs: refreshDelayMs})

	// Launch N concurrent proxy requests. A start barrier (close of a
	// channel) is used instead of WaitGroup.Wait-then-go so all
	// goroutines release ~simultaneously rather than in start-order.
	results := make([]int, concurrentRefreshConcurrency)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(concurrentRefreshConcurrency)
	for i := 0; i < concurrentRefreshConcurrency; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
			results[i] = w.Code
		}()
	}
	close(start)
	wg.Wait()

	// Every concurrent proxy request must observe the final valid access
	// token — the winner's refresh propagated to all losers via the
	// mutex-protected DB re-read.
	for i, code := range results {
		assert.Equalf(t, http.StatusOK, code,
			"concurrent proxy request %d must succeed; got %d", i, code)
	}

	// Exactly one refresh-token grant on the wire. This is the central
	// assertion: serialization means losers re-read the (now fresh)
	// token after acquiring the lock and skip the POST entirely.
	grants := refreshGrantRequests(rig)
	assert.Lenf(t, grants, 1,
		"expected exactly 1 refresh-token grant under concurrent refresh; got %d "+
			"(>1 means the mutex did not serialize concurrent refreshes)", len(grants))

	// Exactly one refresh-succeeded event, no failure events. A failure
	// event here would mean a loser POSTed the rotated-away RT and
	// got 400 invalid_grant — i.e., the rotation-doesn't-corrupt-
	// credentials property is broken.
	succeeded := rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage)
	assert.Lenf(t, succeeded, 1,
		"expected exactly 1 refresh-succeeded event; got %d (>1 means multiple refreshes occurred)", len(succeeded))
	failed := rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage)
	assert.Emptyf(t, failed,
		"concurrent refresh must not emit any refresh-failed events; got %d", len(failed))

	// Exactly one new oauth2_tokens row written: the active token id has
	// rotated forward from preToken.Id, and the plaintext refresh_token
	// has rotated (test provider defaults to rotation on). The retry-
	// once-after-refresh path in ProxyRequest would, if it fired, insert
	// a second token row — but the proactive expiry check forecloses
	// that path here because the refresh succeeds and yields a fresh
	// access_token that /echo accepts.
	postToken := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, postToken)
	assert.NotEqualf(t, preTokenID, postToken.Id,
		"concurrent refresh must persist a new token row; pre id=%s post id=%s",
		preTokenID, postToken.Id)
	postRefreshPlaintext := rig.env.DecryptOAuth2RefreshToken(t, postToken)
	assert.NotEqualf(t, preRefreshPlaintext, postRefreshPlaintext,
		"concurrent refresh must rotate the refresh_token plaintext (provider default policy)")

	// Connection stays healthy. Any permanent failure in a loser-
	// goroutine would have flipped it unhealthy and emitted a
	// connection-health-state-changed event.
	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState,
		"concurrent refresh must leave the connection healthy")
	assert.Emptyf(t, rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage),
		"concurrent refresh must not emit a health-state-changed event")
}
