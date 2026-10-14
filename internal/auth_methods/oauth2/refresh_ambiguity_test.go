package oauth2

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAmbiguousTransportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"dns failure", &url.Error{Op: "Post", URL: "https://x", Err: &net.DNSError{Err: "no such host", Name: "x"}}, false},
		{"connect refused", &url.Error{Op: "Post", URL: "https://x", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}}, false},
		{"untrusted certificate", &url.Error{Op: "Post", URL: "https://x", Err: x509.UnknownAuthorityError{}}, false},
		{"connection reset while reading", &url.Error{Op: "Post", URL: "https://x", Err: &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")}}, true},
		{"deadline mid-request", &url.Error{Op: "Post", URL: "https://x", Err: context.DeadlineExceeded}, true},
		{"unexpected EOF", fmt.Errorf("reading body: %w", errors.New("unexpected EOF")), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isAmbiguousTransportError(tt.err))
		})
	}
}

func TestIsAmbiguousRefreshOutcome(t *testing.T) {
	assert.True(t, isAmbiguousRefreshOutcome(502, nil))
	assert.True(t, isAmbiguousRefreshOutcome(503, nil))
	assert.False(t, isAmbiguousRefreshOutcome(400, nil))
	assert.False(t, isAmbiguousRefreshOutcome(200, nil))
}

// refreshFailureCategory runs a forced refresh and returns the category of
// the single token_refresh_failure event it emitted.
func refreshFailureCategory(t *testing.T, hr *rotationHarness) string {
	t.Helper()
	o := hr.newConn()
	logger, read := bufLogger(t)
	o.logger = logger
	_, err := o.refreshAccessToken(context.Background(), nil, refreshModeAlways)
	require.Error(t, err)
	records := onlyTokenRefreshFailure(read())
	require.Len(t, records, 1)
	return records[0]["category"].(string)
}

// currentTokenId returns the id of the connection's current token row.
func (hr *rotationHarness) currentTokenId() apid.ID {
	hr.t.Helper()
	token, err := hr.db.GetOAuth2Token(context.Background(), hr.connection.GetId())
	require.NoError(hr.t, err)
	return token.Id
}

// TestRefresh_LostResponseThenInvalidGrantIsFlaggedAmbiguous: the provider
// rotates the refresh token but the response is lost behind a 502; our
// in-refresh retry then redeems the consumed token and gets invalid_grant.
// That must be distinguishable from a user revoking access.
func TestRefresh_LostResponseThenInvalidGrantIsFlaggedAmbiguous(t *testing.T) {
	provider := &rotatingTokenProvider{consumeThenFailFirstWith: http.StatusBadGateway}
	hr := newRotationHarness(t, provider, time.Second)

	assert.Equal(t, string(tokenRefreshInvalidGrantAfterAmbiguousAttempt), refreshFailureCategory(t, hr))
	assert.Equal(t, int32(2), provider.requests.Load())
	assert.Contains(t, hr.connection.recordedTransitions(),
		"unhealthy:refresh_invalid_grant_after_ambiguous_attempt",
		"the refresh token is unusable either way, so the connection still needs re-auth")
}

// TestRefresh_AmbiguousMarkerFromEarlierRefreshUpgradesInvalidGrant covers
// the cross-refresh case: an earlier refresh (e.g. a previous asynq attempt)
// ended ambiguously for this token, and now the provider says invalid_grant.
func TestRefresh_AmbiguousMarkerFromEarlierRefreshUpgradesInvalidGrant(t *testing.T) {
	provider := &rotatingTokenProvider{}
	hr := newRotationHarness(t, provider, time.Second)
	provider.current = "rt-consumed-elsewhere"

	hr.newConn().recordAmbiguousRefresh(context.Background(), hr.currentTokenId())

	assert.Equal(t, string(tokenRefreshInvalidGrantAfterAmbiguousAttempt), refreshFailureCategory(t, hr))
}

// TestRefresh_PlainInvalidGrantStaysInvalidGrant: without any ambiguous
// attempt (or with a marker for a different token row), invalid_grant keeps
// its existing category.
func TestRefresh_PlainInvalidGrantStaysInvalidGrant(t *testing.T) {
	t.Run("no marker", func(t *testing.T) {
		provider := &rotatingTokenProvider{}
		hr := newRotationHarness(t, provider, time.Second)
		provider.current = "rt-revoked"
		assert.Equal(t, string(tokenRefreshInvalidGrant), refreshFailureCategory(t, hr))
	})
	t.Run("marker for another token", func(t *testing.T) {
		provider := &rotatingTokenProvider{}
		hr := newRotationHarness(t, provider, time.Second)
		provider.current = "rt-revoked"
		hr.newConn().recordAmbiguousRefresh(context.Background(), apid.New(apid.PrefixOAuth2Token))
		assert.Equal(t, string(tokenRefreshInvalidGrant), refreshFailureCategory(t, hr))
	})
}

// TestRefresh_AmbiguousOutcomeRecordsMarkerAndSuccessClearsIt: a refresh
// that exhausts its budget on 5xx leaves a marker for the token it tried to
// redeem; a later successful refresh clears it.
func TestRefresh_AmbiguousOutcomeRecordsMarkerAndSuccessClearsIt(t *testing.T) {
	provider := &rotatingTokenProvider{}
	hr := newRotationHarness(t, provider, time.Second)
	tokenId := hr.currentTokenId()

	// Point at an endpoint that always answers 503.
	unavailable := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	srv := newHTTPTestServer(t, unavailable)
	failing := *hr.auth
	failing.Token.Endpoint = srv.URL
	o := hr.newConn()
	o.auth = &failing
	_, err := o.refreshAccessToken(context.Background(), nil, refreshModeAlways)
	require.Error(t, err)
	assert.True(t, hr.newConn().hadAmbiguousRefresh(context.Background(), tokenId))

	require.NoError(t, hr.newConn().Refresh(context.Background()))
	assert.False(t, hr.newConn().hadAmbiguousRefresh(context.Background(), tokenId))
}

// TestRefresh_ConnectFailureIsNotAmbiguous: a request that never left the
// building cannot have consumed the refresh token, so no marker is recorded.
func TestRefresh_ConnectFailureIsNotAmbiguous(t *testing.T) {
	provider := &rotatingTokenProvider{}
	hr := newRotationHarness(t, provider, time.Second)
	tokenId := hr.currentTokenId()

	closed := newHTTPTestServer(t, http.NotFoundHandler())
	endpoint := closed.URL
	closed.Close()

	unreachable := *hr.auth
	unreachable.Token.Endpoint = endpoint
	o := hr.newConn()
	o.auth = &unreachable
	_, err := o.refreshAccessToken(context.Background(), nil, refreshModeAlways)
	require.Error(t, err)
	assert.False(t, hr.newConn().hadAmbiguousRefresh(context.Background(), tokenId))
}
