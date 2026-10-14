//go:build integration

package oauth2

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// One-time-use (rotating) refresh tokens, issue #1002. Providers such as
// Atlassian, GitHub Apps, Box and Intuit issue a new refresh token on every
// refresh and invalidate the one that was redeemed. The test provider does
// the same with rotation on (its default): a replayed refresh token gets
// 400 "Refresh token revoked". Any code path that redeems the same refresh
// token twice therefore surfaces as a refresh failure and an unhealthy
// connection, which is what these end-to-end tests guard against.
//
// The unit tests in internal/auth_methods/oauth2 cover the same properties
// against an in-process fake provider; these exercise them against the real
// HTTP stack, Redis and database.

const oneTimeUseConcurrency = 8

// runConcurrentProxyRequests releases n proxy requests to /echo at once and
// returns the upstream status each one observed.
func runConcurrentProxyRequests(t *testing.T, rig *proxyRefreshRig, connID string, n int) []int {
	t.Helper()
	upstreamStatus := make([]int, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
			upstreamStatus[i] = parseRevocationProxyResponse(t, w).StatusCode
		}()
	}
	close(start)
	wg.Wait()
	return upstreamStatus
}

// assertSingleCleanRotation checks that exactly one refresh succeeded, none
// failed, and the connection stayed healthy.
func assertSingleCleanRotation(t *testing.T, rig *proxyRefreshRig, connID string) {
	t.Helper()
	assert.Lenf(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage), 1,
		"exactly one refresh may redeem the one-time-use refresh token")
	assert.Emptyf(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage),
		"no refresh may replay the rotated-away refresh token")
	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState)
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage))
}

// TestOneTimeUseRefresh_ConcurrentUpstream401sRotateOnce: the provider
// revokes the access token while it still looks valid locally, so every
// in-flight request gets 401 from the upstream at once. Each request calls
// RecoverFrom401 with the token it used; only the first may refresh. The
// rest must see the rejected token was already replaced and simply retry.
// Before RecoverFrom401 knew which token was rejected, each 401 forced its
// own refresh and one revocation burned N rotations.
func TestOneTimeUseRefresh_ConcurrentUpstream401sRotateOnce(t *testing.T) {
	rig := newProxyRefreshRig(t, "one-time-use-401s")
	connID := rig.completeAuthFlow(t)

	accessToken := rig.env.DecryptOAuth2AccessToken(t, rig.env.GetOAuth2Token(t, connID))
	rig.provider.RevokeToken(accessToken)

	// Slow the refresh slightly so the 401s pile up behind the token mutex
	// while the first recovery is still in flight.
	rig.provider.Script(rig.clientKey, helpers.EndpointRefresh, helpers.ScriptAction{DelayMs: 300})

	for i, status := range runConcurrentProxyRequests(t, rig, connID, oneTimeUseConcurrency) {
		assert.Equalf(t, http.StatusOK, status, "request %d must succeed after recovery", i)
	}

	assert.Lenf(t, refreshGrantRequests(rig), 1,
		"one revoked access token must cause exactly one refresh, not one per 401")
	assertSingleCleanRotation(t, rig, connID)
}

// TestOneTimeUseRefresh_RefreshSlowerThanLockLease: the refresh takes longer
// than token.refreshTimeout (the refresh lock's lease) because the provider is
// slow and the first attempt is retried after a 503. The lease must be renewed
// for the whole refresh; if it lapsed, a waiting request would take the lock,
// re-read the not-yet-replaced token and replay its refresh token.
func TestOneTimeUseRefresh_RefreshSlowerThanLockLease(t *testing.T) {
	const lease = time.Second
	rig := newProxyRefreshRigWithOptions(t, "one-time-use-slow", func(o *helpers.OAuth2ConnectorOptions) {
		o.RefreshTimeout = lease
	})
	connID := rig.completeAuthFlow(t)
	rig.forceTokenExpired(t, connID, false)

	// 600ms → 503, ~200ms retry backoff, 600ms → real rotation: ~1.4s total,
	// comfortably past the 1s lease while each attempt stays inside the 1s
	// per-attempt deadline.
	rig.provider.Script(rig.clientKey, helpers.EndpointRefresh,
		helpers.ScriptAction{Status: http.StatusServiceUnavailable, Body: `{"error":"temporarily_unavailable"}`, DelayMs: 600},
		helpers.ScriptAction{DelayMs: 600},
	)

	for i, status := range runConcurrentProxyRequests(t, rig, connID, oneTimeUseConcurrency) {
		assert.Equalf(t, http.StatusOK, status, "request %d must succeed", i)
	}

	require.Lenf(t, refreshGrantRequests(rig), 2,
		"one refresh: the 503 attempt and its successful retry; any more means a waiter replayed the refresh token")
	assertSingleCleanRotation(t, rig, connID)
}
