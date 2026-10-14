package oauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	mockH "github.com/rmorlok/authproxy/internal/httpf/mock"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gentleman "gopkg.in/h2non/gentleman.v2"
)

// rotatingTokenProvider is a fake OAuth2 token endpoint that enforces
// one-time-use refresh tokens: exactly one refresh token is valid at a time,
// and redeeming it issues a replacement and invalidates the old one. Any
// reuse is rejected with invalid_grant, the same way Atlassian, GitHub Apps
// and Box behave. It lets tests prove that concurrent refreshers never
// redeem the same refresh token twice.
type rotatingTokenProvider struct {
	t *testing.T

	mu sync.Mutex
	// current is the only refresh token the provider will accept.
	current string
	// issued counts successful exchanges; it also numbers issued tokens.
	issued int

	// delay is how long each request takes before the provider answers,
	// simulating a slow token endpoint.
	delay time.Duration
	// failFirstWith, when non-zero, is the status returned to the very
	// first request (without consuming the refresh token).
	failFirstWith int
	// extraResponse is merged into every successful response body; used to
	// add provider-specific fields such as refresh_token_expires_in.
	extraResponse map[string]any

	requests   atomic.Int32
	rejections atomic.Int32
}

// ServeHTTP implements the token endpoint described on rotatingTokenProvider.
func (p *rotatingTokenProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := p.requests.Add(1)
	if err := r.ParseForm(); err != nil {
		p.t.Errorf("failed to parse token request: %v", err)
	}
	time.Sleep(p.delay)

	if n == 1 && p.failFirstWith != 0 {
		w.WriteHeader(p.failFirstWith)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if r.PostForm.Get("grant_type") != "refresh_token" || r.PostForm.Get("refresh_token") != p.current {
		p.rejections.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token reused or unknown"}`))
		return
	}

	p.issued++
	p.current = fmt.Sprintf("rt-%d", p.issued)
	body := map[string]any{
		"access_token":  fmt.Sprintf("at-%d", p.issued),
		"refresh_token": p.current,
		"token_type":    "Bearer",
		"expires_in":    3600,
	}
	for k, v := range p.extraResponse {
		body[k] = v
	}
	_ = json.NewEncoder(w).Encode(body)
}

// currentRefreshToken returns the refresh token the provider will accept next.
func (p *rotatingTokenProvider) currentRefreshToken() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}

// healthRecordingConnection wraps the core mock connection with a
// goroutine-safe MarkHealthState that remembers every transition, since
// concurrent refreshers may record health at the same time.
type healthRecordingConnection struct {
	*mockCore.Connection

	mu          sync.Mutex
	transitions []string
}

// MarkHealthState records the transition as "<state>:<reason>".
func (c *healthRecordingConnection) MarkHealthState(_ context.Context, state database.ConnectionHealthState, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.HealthState = state
	c.transitions = append(c.transitions, fmt.Sprintf("%s:%s", state, reason))
	return nil
}

// recordedTransitions returns a copy of the recorded health transitions.
func (c *healthRecordingConnection) recordedTransitions() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.transitions...)
}

// rotationHarness wires an oAuth2Connection to a real (SQLite/Postgres) test
// database, miniredis, the fake encryptor and a rotatingTokenProvider, so
// refresh tests exercise the real lock, persistence and re-read logic.
type rotationHarness struct {
	t          *testing.T
	db         database.DB
	r          apredis.Client
	mr         *miniredis.Miniredis
	encrypt    encrypt.E
	httpf      *mockH.MockF
	provider   *rotatingTokenProvider
	server     *httptest.Server
	connection *healthRecordingConnection
	auth       *cschema.AuthOAuth2
}

// newRotationHarness builds the harness and seeds the connection with an
// access token that has already expired and refresh token "rt-0".
func newRotationHarness(t *testing.T, provider *rotatingTokenProvider, refreshTimeout time.Duration) *rotationHarness {
	t.Helper()
	ctrl := gomock.NewController(t)

	_, db := database.MustApplyBlankTestDbConfig(t, nil)
	_, r, mr := apredis.MustApplyTestConfigWithServer(nil)

	provider.t = t
	provider.current = "rt-0"
	server := httptest.NewServer(provider)
	t.Cleanup(server.Close)

	// A real (non-mocking) gentleman client so requests reach the httptest
	// server over the network.
	h := mockH.NewMockF(ctrl)
	h.EXPECT().ForRequestType(gomock.Any()).Return(h).AnyTimes()
	h.EXPECT().ForConnection(gomock.Any()).Return(h).AnyTimes()
	h.EXPECT().New().DoAndReturn(func() *gentleman.Client { return gentleman.New() }).AnyTimes()

	conn := &healthRecordingConnection{Connection: &mockCore.Connection{
		Id:          apid.New(apid.PrefixConnection),
		Namespace:   "root",
		HealthState: database.ConnectionHealthStateHealthy,
	}}

	hr := &rotationHarness{
		t:          t,
		db:         db,
		r:          r,
		mr:         mr,
		encrypt:    encrypt.NewFakeEncryptService(false),
		httpf:      h,
		provider:   provider,
		server:     server,
		connection: conn,
		auth: &cschema.AuthOAuth2{
			Type:         cschema.AuthTypeOAuth2,
			ClientId:     common.NewStringValueDirect("client-id"),
			ClientSecret: common.NewStringValueDirect("client-secret"),
			Token: cschema.AuthOauth2Token{
				Endpoint:       server.URL + "/token",
				RefreshTimeout: &common.HumanDuration{Duration: refreshTimeout},
			},
		},
	}

	ctx := context.Background()
	encRefresh, err := hr.encrypt.EncryptStringForEntity(ctx, conn, "rt-0")
	require.NoError(t, err)
	encAccess, err := hr.encrypt.EncryptStringForEntity(ctx, conn, "at-0")
	require.NoError(t, err)
	_, err = db.InsertOAuth2Token(ctx, conn.GetId(), nil, encRefresh, encAccess,
		util.ToPtr(time.Now().Add(-time.Minute)), "", "", nil)
	require.NoError(t, err)

	return hr
}

// newConn returns a fresh oAuth2Connection for the harness's connection, as
// a separate process or request would construct one.
func (hr *rotationHarness) newConn() *oAuth2Connection {
	logger, _ := bufLogger(hr.t)
	return &oAuth2Connection{
		db:         hr.db,
		r:          hr.r,
		encrypt:    hr.encrypt,
		logger:     logger,
		auth:       hr.auth,
		httpf:      hr.httpf,
		connection: hr.connection,
	}
}

// advanceRedisClockWithWallTime makes miniredis key TTLs elapse in step with
// real time (miniredis only expires keys on FastForward). Without this a
// lapsed mutex lease would never actually expire in tests.
func (hr *rotationHarness) advanceRedisClockWithWallTime() {
	const tick = 10 * time.Millisecond
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				hr.mr.FastForward(tick)
			}
		}
	}()
	hr.t.Cleanup(func() {
		close(done)
		<-stopped
	})
}

// storedRefreshToken decrypts the refresh token on the connection's current
// token row.
func (hr *rotationHarness) storedRefreshToken() string {
	hr.t.Helper()
	ctx := context.Background()
	token, err := hr.db.GetOAuth2Token(ctx, hr.connection.GetId())
	require.NoError(hr.t, err)
	rt, err := hr.encrypt.DecryptString(ctx, token.EncryptedRefreshToken)
	require.NoError(hr.t, err)
	return rt
}

// TestRefreshAccessToken_ConcurrentRefreshersDoNotReuseRotatedToken is the
// regression test for issue #1002. The provider is slower than the mutex
// lease (one 503 plus a successful retry takes ~2× the lease), and several
// refreshers race for the same expired token. Before the lease was renewed
// for the duration of the refresh, the lock lapsed mid-refresh, a waiter
// re-read the not-yet-replaced token and redeemed the same one-time refresh
// token, and the provider's invalid_grant flipped the connection unhealthy.
//
// With lease renewal, waiters block until the holder persists the rotated
// token, re-read it, see a valid access token and return without calling the
// provider at all.
func TestRefreshAccessToken_ConcurrentRefreshersDoNotReuseRotatedToken(t *testing.T) {
	const lease = 300 * time.Millisecond
	provider := &rotatingTokenProvider{
		delay:         lease * 2 / 3,
		failFirstWith: http.StatusServiceUnavailable,
	}
	hr := newRotationHarness(t, provider, lease)
	hr.advanceRedisClockWithWallTime()

	const refreshers = 4
	var wg sync.WaitGroup
	errs := make([]error, refreshers)
	for i := 0; i < refreshers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Stagger slightly so one refresher reliably takes the lock first.
			time.Sleep(time.Duration(i) * 10 * time.Millisecond)
			_, errs[i] = hr.newConn().getValidToken(context.Background())
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "refresher %d", i)
	}
	assert.Zero(t, provider.rejections.Load(), "no refresh token may be redeemed twice")
	assert.Equal(t, int32(2), provider.requests.Load(),
		"exactly one refresher talks to the provider: one 503 and one successful retry")
	assert.Equal(t, provider.currentRefreshToken(), hr.storedRefreshToken(),
		"the rotated refresh token must be the one persisted")
	for _, tr := range hr.connection.recordedTransitions() {
		assert.NotContains(t, tr, string(database.ConnectionHealthStateUnhealthy))
	}
}

// TestRefreshAccessToken_SequentialForcedRefreshesFollowRotation checks the
// steady-state rotation contract: every forced refresh (background task /
// 401 recovery path) redeems the refresh token returned by the previous one.
func TestRefreshAccessToken_SequentialForcedRefreshesFollowRotation(t *testing.T) {
	provider := &rotatingTokenProvider{}
	hr := newRotationHarness(t, provider, time.Second)

	for i := 1; i <= 3; i++ {
		require.NoError(t, hr.newConn().Refresh(context.Background()), "refresh %d", i)
		assert.Equal(t, fmt.Sprintf("rt-%d", i), hr.storedRefreshToken())
	}
	assert.Zero(t, provider.rejections.Load())
}

// TestPostRefreshWithRetry_AttemptTimeout verifies token.refreshTimeout
// bounds each POST: a provider that never answers within the deadline yields
// a transport error after the full retry budget rather than hanging.
func TestPostRefreshWithRetry_AttemptTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	o := newRefreshRetryTestConn(t)
	start := time.Now()
	resp, attempts, err := o.postRefreshWithRetry(
		context.Background(),
		gentleman.New(),
		srv.URL,
		nil,
		"",
		50*time.Millisecond,
	)

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, tokenRefreshMaxAttempts, attempts, "timeouts are transport errors and are retried")
	assert.Less(t, time.Since(start), 5*time.Second, "per-attempt deadline must bound the refresh")
}

// TestRefreshLockWaitBudget pins the waiter budget to the holder's worst case.
func TestRefreshLockWaitBudget(t *testing.T) {
	// 3 attempts + 1 slack = 4 × 30s, plus 200ms + 400ms of backoff.
	assert.Equal(t, 120*time.Second+600*time.Millisecond, refreshLockWaitBudget(30*time.Second))
}
