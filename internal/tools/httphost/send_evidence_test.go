package httphost_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	coremock "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/proxy"
	"github.com/rmorlok/authproxy/internal/ratelimit"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/stretchr/testify/require"
)

// evidenceAuth permits credential resolution to fail before the target send,
// while retaining the recovery behavior used by the other integration tests.
type evidenceAuth struct {
	integrationAuth
	beforeResolve func(context.Context) error
}

// Resolve invokes the test's credential step before producing usable credentials.
func (a *evidenceAuth) Resolve(ctx context.Context) (auth_methods.AuthApplication, error) {
	if a.beforeResolve != nil {
		if err := a.beforeResolve(ctx); err != nil {
			return auth_methods.AuthApplication{}, err
		}
	}
	return a.integrationAuth.Resolve(ctx)
}

// evidenceFixture supplies a real connection proxy, middleware factory, and
// Redis-backed connector limiter so tests can distinguish local rejection from
// an upstream response with exactly the same status code.
type evidenceFixture struct {
	connection *coremock.Connection
	store      *ratelimit.Store
	factory    httpf.F
}

// newEvidenceFixture creates isolated rate-limit storage and production HTTP
// middleware. Cleanup closes both the Redis client and its in-memory server.
func newEvidenceFixture(t *testing.T) *evidenceFixture {
	t.Helper()
	_, redis, server := apredis.MustApplyTestConfigWithServer(nil)
	t.Cleanup(server.Close)
	t.Cleanup(func() { _ = redis.Close() })
	store := ratelimit.NewStore(redis)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &evidenceFixture{
		connection: &coremock.Connection{
			Id: apid.New(apid.PrefixConnection), Namespace: "root.product.owner",
			ConnectorId: apid.New(apid.PrefixConnector), ConnectorGeneration: 1,
		},
		store: store,
		factory: httpf.CreateFactory(nil, redis, &integrationMiddleware{}, logger,
			ratelimit.NewFactory(store, logger)),
	}
}

// session builds the configured proxy client and gives it a shared call budget.
func (f *evidenceFixture) session(t *testing.T, auth auth_methods.Authenticator, maxRequests int) (*httphost.Session, *integrationClient) {
	t.Helper()
	client, err := proxy.New(f.factory, f.connection, auth, nil).HTTPClient(iface.HTTPClientOptions{
		RequestType: httpf.RequestTypeProxy, Actor: iface.WithoutActor(),
	})
	require.NoError(t, err)
	counted := &integrationClient{connection: f.connection, client: client}
	limits := integrationLimits()
	limits.MaxRequests = maxRequests
	session, err := httphost.New(context.Background(), counted, limits)
	require.NoError(t, err)
	t.Cleanup(session.Close)
	return session, counted
}

// block prevents subsequent target sends through the connector limiter.
func (f *evidenceFixture) block(ctx context.Context) error {
	return f.store.SetRateLimited(ctx, f.connection.Id, time.Minute)
}

// evidenceRequest builds a bodyless request whose credential-recovery replay
// does not depend on upload buffering or a GetBody implementation.
func evidenceRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, target, nil)
	require.NoError(t, err)
	return req
}

// TestSessionSendEvidenceCredentialFailure separates OAuth credential traffic
// from target execution and keeps request-budget accounting independent.
func TestSessionSendEvidenceCredentialFailure(t *testing.T) {
	for _, oauth := range []bool{false, true} {
		name := "local credential error"
		if oauth {
			name = "OAuth request then credential error"
		}
		t.Run(name, func(t *testing.T) {
			// The OAuth endpoint is reachable; the target must never be called.
			var oauthCalls, targetCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth" {
					oauthCalls.Add(1)
				} else {
					targetCalls.Add(1)
				}
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(server.Close)
			fixture := newEvidenceFixture(t)
			auth := &evidenceAuth{beforeResolve: func(ctx context.Context) error {
				if oauth {
					// Preserve the resolver's context exactly, as a token request
					// may inherit it without being a target-operation send.
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/oauth", nil)
					if err != nil {
						return err
					}
					resp, err := fixture.factory.ForRequestType(httpf.RequestTypeOAuth).NewHTTPClient().Do(req)
					if err != nil {
						return err
					}
					_ = resp.Body.Close()
				}
				return errors.New("credential resolution failed")
			}}
			session, client := fixture.session(t, auth, 1)

			// Failure before the target send leaves execution unstarted.
			_, err := session.Do(evidenceRequest(t, server.URL+"/target"))
			require.ErrorIs(t, err, httphost.ErrTransport)
			require.Equal(t, api.ToolExecutionStateNotStarted, session.ExecutionState())
			require.Zero(t, targetCalls.Load())
			if oauth {
				require.Equal(t, int32(1), oauthCalls.Load())
			}

			// The failed logical call still exhausts the configured budget.
			_, err = session.Do(evidenceRequest(t, server.URL+"/target"))
			require.ErrorIs(t, err, httphost.ErrRequestLimit)
			require.Equal(t, int32(1), client.calls.Load())
			require.Equal(t, api.ToolExecutionStateNotStarted, session.ExecutionState())
		})
	}
}

// TestSessionSendEvidenceLocalRateLimit exercises both a fresh local rejection
// and a rejection following a completed call within the same Tool invocation.
func TestSessionSendEvidenceLocalRateLimit(t *testing.T) {
	for _, previousStatus := range []int{0, http.StatusOK, http.StatusTooManyRequests} {
		name := "no previous target send"
		if previousStatus != 0 {
			name = "previous upstream " + http.StatusText(previousStatus)
		}
		t.Run(name, func(t *testing.T) {
			// Use the actual limiter to synthesize a response without network I/O.
			var targetCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				targetCalls.Add(1)
				if previousStatus != 0 {
					w.Header().Set("Retry-After", "60")
					w.WriteHeader(previousStatus)
				}
			}))
			t.Cleanup(server.Close)
			fixture := newEvidenceFixture(t)
			session, _ := fixture.session(t, &integrationAuth{}, 2)
			wantState := api.ToolExecutionStateNotStarted
			if previousStatus != 0 {
				response, err := session.Do(evidenceRequest(t, server.URL))
				require.NoError(t, err)
				require.Equal(t, previousStatus, response.StatusCode)
				require.NoError(t, response.Body.Close())
				wantState = api.ToolExecutionStateMayHaveExecuted
			}
			require.NoError(t, fixture.block(context.Background()))

			// A local 429 adds no execution uncertainty, but cannot erase a send.
			response, err := session.Do(evidenceRequest(t, server.URL))
			require.NoError(t, err)
			require.Equal(t, http.StatusTooManyRequests, response.StatusCode)
			require.NoError(t, response.Body.Close())
			require.Equal(t, wantState, session.ExecutionState())
			if previousStatus != 0 {
				require.Equal(t, int32(1), targetCalls.Load())
			} else {
				require.Zero(t, targetCalls.Load())
			}
		})
	}
}

// TestSessionSendEvidenceReplayAndRedirect keeps evidence from the first target
// response even when credential replay or an automatic redirect is rejected.
func TestSessionSendEvidenceReplayAndRedirect(t *testing.T) {
	for _, scenario := range []string{"401 then resolve failure", "401 then local 429", "redirect then local 429"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newEvidenceFixture(t)
			var targetCalls atomic.Int32
			blockErrors := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// A regression that sends again should fail the assertions,
				// without blocking the handler or causing a redirect loop.
				if targetCalls.Add(1) > 1 {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				// Install the limiter before releasing the first response so
				// redirects and authentication replay deterministically hit it.
				if scenario != "401 then resolve failure" {
					blockErrors <- fixture.block(r.Context())
				}
				if scenario == "redirect then local 429" {
					w.Header().Set("Location", "/redirected")
					w.WriteHeader(http.StatusFound)
				} else {
					w.WriteHeader(http.StatusUnauthorized)
				}
			}))
			t.Cleanup(server.Close)
			auth := &evidenceAuth{}
			if scenario == "401 then resolve failure" {
				auth.beforeResolve = func(context.Context) error {
					if auth.resolves.Load() > 0 {
						return errors.New("replacement credentials unavailable")
					}
					return nil
				}
			}
			session, client := fixture.session(t, auth, 1)

			// Every case reaches the target once; later rejection must not
			// turn that already possible upstream effect into notStarted.
			response, err := session.Do(evidenceRequest(t, server.URL))
			if scenario == "401 then resolve failure" {
				require.ErrorIs(t, err, httphost.ErrTransport)
			} else {
				require.NoError(t, err)
				require.Equal(t, http.StatusTooManyRequests, response.StatusCode)
				require.NoError(t, response.Body.Close())
				require.NoError(t, <-blockErrors)
			}
			require.Equal(t, int32(1), targetCalls.Load())
			require.Equal(t, int32(1), client.calls.Load())
			require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())
		})
	}
}

// unknownEvidenceFactory retains fluent factory selection while substituting
// an uninstrumented transport whose response cannot prove whether a send ran.
type unknownEvidenceFactory struct{ httpf.F }

// ForRequestType preserves the uninstrumented client through proxy setup.
func (f *unknownEvidenceFactory) ForRequestType(httpf.RequestType) httpf.F { return f }

// ForConnection preserves the uninstrumented client through proxy setup.
func (f *unknownEvidenceFactory) ForConnection(httpf.Connection) httpf.F { return f }

// ForActor preserves the uninstrumented client through proxy setup.
func (f *unknownEvidenceFactory) ForActor(httpf.Actor) httpf.F { return f }

// ForLabels preserves the uninstrumented client through proxy setup.
func (f *unknownEvidenceFactory) ForLabels(map[string]string) httpf.F { return f }

// NewHTTPClient returns a transport without the production send observer.
func (*unknownEvidenceFactory) NewHTTPClient() *http.Client {
	return &http.Client{Transport: unknownEvidenceTransport{}}
}

// unknownEvidenceTransport produces a status alone, without trusted evidence.
type unknownEvidenceTransport struct{}

// RoundTrip returns a synthetic 429 indistinguishable from an upstream one.
func (unknownEvidenceTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}, Body: http.NoBody}, nil
}

// TestSessionSendEvidenceUnknownFactory keeps custom transports conservative:
// status codes alone cannot establish that an upstream request was never sent.
func TestSessionSendEvidenceUnknownFactory(t *testing.T) {
	// Replace only the configured transport; retain the real proxy call path.
	fixture := newEvidenceFixture(t)
	fixture.factory = &unknownEvidenceFactory{F: fixture.factory}
	session, _ := fixture.session(t, &integrationAuth{}, 1)
	response, err := session.Do(evidenceRequest(t, "https://unused.example.test/target"))
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, response.StatusCode)
	require.NoError(t, response.Body.Close())
	require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())
}
