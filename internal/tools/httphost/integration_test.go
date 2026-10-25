package httphost_test

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	coremock "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/proxy"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/stretchr/testify/require"
)

// integrationContextKey identifies admitted context values observed by the real
// httpf transport after the host and proxy have composed request cancellation.
type integrationContextKey struct{}

// integrationConnection supplies resource metadata while delegating streaming
// requests to the existing proxy instead of replacing its authentication flow.
type integrationConnection struct {
	*coremock.Connection
	proxy iface.Proxy
	calls atomic.Int32
}

// ProxyRequestStream counts logical host handoffs; retries remain inside proxy.
func (c *integrationConnection) ProxyRequestStream(ctx context.Context, kind httpf.RequestType, request *iface.RawProxyRequest) (*http.Response, error) {
	c.calls.Add(1)
	return c.proxy.ProxyRequestStream(ctx, kind, request)
}

// integrationAuth exposes deterministic credential rotation without external
// token storage. Counters distinguish logical host calls from proxy attempts.
type integrationAuth struct {
	resolves   atomic.Int32
	recoveries atomic.Int32
	rejected   atomic.Value
}

// Resolve returns fresh header and query credentials on each proxy attempt.
func (a *integrationAuth) Resolve(context.Context) (auth_methods.AuthApplication, error) {
	attempt := a.resolves.Add(1)
	version := fmt.Sprintf("v%d", attempt)
	return auth_methods.AuthApplication{
		Headers:           map[string]string{"Authorization": "Bearer " + version},
		QueryParams:       map[string]string{"api_key": version},
		CredentialVersion: version,
	}, nil
}

// RecoverFrom401 records which credential was rejected before permitting replay.
func (a *integrationAuth) RecoverFrom401(_ context.Context, rejected auth_methods.AuthApplication) error {
	a.recoveries.Add(1)
	a.rejected.Store(rejected.CredentialVersion)
	return nil
}

// Refresh is unused by this credential fixture.
func (*integrationAuth) Refresh(context.Context) error { return nil }

// SupportsRevoke keeps unrelated credential revocation outside these tests.
func (*integrationAuth) SupportsRevoke() bool { return false }

// Revoke is unused by the streaming request path.
func (*integrationAuth) Revoke(context.Context) error { return nil }

// integrationAttempt captures transport attribution without consuming bodies.
type integrationAttempt struct {
	info         httpf.RequestInfo
	contextValue any
	deadline     time.Time
	hasDeadline  bool
}

// integrationMiddleware records the actual factory's resolved metadata before
// forwarding each request to the real network transport.
type integrationMiddleware struct {
	mu       sync.Mutex
	attempts []integrationAttempt
}

// NewRoundTripper retains the factory's metadata snapshot in a recording wrapper.
func (m *integrationMiddleware) NewRoundTripper(info httpf.RequestInfo, next http.RoundTripper) http.RoundTripper {
	return integrationRoundTripper{middleware: m, info: info, next: next}
}

// snapshot returns independent attempt metadata after the request completes.
func (m *integrationMiddleware) snapshot() []integrationAttempt {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]integrationAttempt(nil), m.attempts...)
}

// integrationRoundTripper observes the real transport rather than simulating it.
type integrationRoundTripper struct {
	middleware *integrationMiddleware
	info       httpf.RequestInfo
	next       http.RoundTripper
}

// RoundTrip records admission context and httpf attribution for every wire attempt.
func (r integrationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	info := r.info
	info.Labels = maps.Clone(info.Labels)
	deadline, hasDeadline := request.Context().Deadline()
	r.middleware.mu.Lock()
	r.middleware.attempts = append(r.middleware.attempts, integrationAttempt{
		info: info, contextValue: request.Context().Value(integrationContextKey{}),
		deadline: deadline, hasDeadline: hasDeadline,
	})
	r.middleware.mu.Unlock()
	return r.next.RoundTrip(request)
}

// newIntegrationConnection uses the production factory and proxy while replacing
// only persistent connection metadata and credential acquisition with fixtures.
func newIntegrationConnection(auth *integrationAuth) (*integrationConnection, *integrationMiddleware) {
	connection := &integrationConnection{Connection: &coremock.Connection{
		Id: apid.New(apid.PrefixConnection), Namespace: "root.product.owner",
		ConnectorId: apid.New(apid.PrefixConnector), ConnectorGeneration: 7,
		Labels: map[string]string{"connection-label": "captured"},
	}}
	middleware := &integrationMiddleware{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	factory := httpf.CreateFactory(nil, nil, middleware, logger)
	connection.proxy = proxy.New(factory, connection, auth, nil)
	return connection, middleware
}

// integrationLimits keeps deadlines bounded and leaves room for real HTTP headers.
func integrationLimits() httphost.Limits {
	return httphost.Limits{
		Timeout: 5 * time.Second, MaxRequests: 1,
		MaxResponseBytes: 4096, MaxStreamBytes: 4096, MaxHeaderBytes: 4096,
	}
}

// TestSessionIntegrationProxyRecoveryAndAttribution verifies the real proxy keeps
// authentication recovery inside one attributed, connection-scoped host request.
func TestSessionIntegrationProxyRecoveryAndAttribution(t *testing.T) {
	for _, finalStatus := range []int{http.StatusOK, http.StatusUnauthorized} {
		t.Run(http.StatusText(finalStatus), func(t *testing.T) {
			// Record complete upstream requests without assertions in handler goroutines.
			type receivedRequest struct {
				host, method, authorization, credential, keep, body string
				custom                                              []string
				err                                                 error
			}
			received := make(chan receivedRequest, 3)
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				received <- receivedRequest{
					host: r.Host, method: r.Method, authorization: r.Header.Get("Authorization"),
					credential: r.URL.Query().Get("api_key"), keep: r.URL.Query().Get("keep"),
					body: string(body), custom: r.Header.Values("X-Custom"), err: err,
				}
				w.Header().Set("Content-Type", "application/json")
				if attempts.Add(1) == 1 {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = io.WriteString(w, `{"expired":true}`)
					return
				}
				w.WriteHeader(finalStatus)
				_, _ = io.WriteString(w, `{"final":true}`)
			}))
			t.Cleanup(server.Close)

			// One captured connection and one logical request permit the proxy's
			// existing credential-recovery replay, even for a replayable POST.
			auth := &integrationAuth{}
			connection, transport := newIntegrationConnection(auth)
			labels := map[string]string{"tool-label": "captured"}
			ctx := context.WithValue(context.Background(), integrationContextKey{}, "admitted")
			session, err := httphost.New(ctx, connection, integrationLimits(), labels)
			require.NoError(t, err)
			t.Cleanup(session.Close)
			labels["tool-label"] = "changed after capture"
			request, err := http.NewRequest(http.MethodPost, server.URL+"/record?api_key=caller&keep=a%2Fb", strings.NewReader(`{"name":"Alice"}`))
			require.NoError(t, err)
			request.Host = "tenant.example.test"
			request.Header.Set("Authorization", "Bearer caller")
			request.Header.Add("X-Custom", "first")
			request.Header.Add("X-Custom", "second")
			originalURL, originalHeaders := request.URL.String(), request.Header.Clone()
			response, err := session.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, finalStatus, response.StatusCode)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, `{"final":true}`, string(body))
			require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())

			// Both attempts preserve the upload, Host override, and repeated headers;
			// freshly resolved credentials override only the authenticated copies.
			for attempt := 1; attempt <= 2; attempt++ {
				got := <-received
				require.NoError(t, got.err)
				require.Equal(t, "tenant.example.test", got.host)
				require.Equal(t, http.MethodPost, got.method)
				require.Equal(t, fmt.Sprintf("Bearer v%d", attempt), got.authorization)
				require.Equal(t, fmt.Sprintf("v%d", attempt), got.credential)
				require.Equal(t, "a/b", got.keep)
				require.Equal(t, `{"name":"Alice"}`, got.body)
				require.Equal(t, []string{"first", "second"}, got.custom)
			}
			require.Equal(t, int32(2), auth.resolves.Load())
			require.Equal(t, int32(1), auth.recoveries.Load())
			require.Equal(t, "v1", auth.rejected.Load())
			require.Equal(t, int32(1), connection.calls.Load())
			require.Equal(t, originalURL, request.URL.String())
			require.Equal(t, originalHeaders, request.Header)
			require.Equal(t, "tenant.example.test", request.Host)

			// The real httpf chain receives the connection, captured request labels,
			// admitted context value, and the shared invocation deadline.
			observed := transport.snapshot()
			require.Len(t, observed, 2)
			deadline, ok := session.Context().Deadline()
			require.True(t, ok)
			for _, attempt := range observed {
				require.Equal(t, httpf.RequestTypeProxy, attempt.info.Type)
				require.Equal(t, connection.GetId(), attempt.info.ConnectionId)
				require.Equal(t, connection.GetConnectorId(), attempt.info.ConnectorId)
				require.Equal(t, uint64(7), attempt.info.ConnectorGeneration)
				require.Equal(t, "captured", attempt.info.Labels["tool-label"])
				require.Equal(t, "captured", attempt.info.Labels["connection-label"])
				require.Equal(t, "admitted", attempt.contextValue)
				require.True(t, attempt.hasDeadline)
				require.Equal(t, deadline, attempt.deadline)
			}

			// A second logical request is rejected without another proxy or wire attempt.
			next, err := http.NewRequest(http.MethodGet, server.URL+"/second", nil)
			require.NoError(t, err)
			response, err = session.Do(next)
			require.ErrorIs(t, err, httphost.ErrRequestLimit)
			require.Nil(t, response)
			require.Equal(t, int32(1), connection.calls.Load())
			require.Equal(t, int32(2), attempts.Load())
		})
	}
}

// TestSessionIntegrationGzipStreams verifies headers-first delivery and decoded
// budgets through both net/http's transparent gzip and explicit host decoding.
func TestSessionIntegrationGzipStreams(t *testing.T) {
	for _, explicitEncoding := range []bool{false, true} {
		for _, tooLarge := range []bool{false, true} {
			name := fmt.Sprintf("explicit=%t/limited=%t", explicitEncoding, tooLarge)
			t.Run(name, func(t *testing.T) {
				// Flush headers before emitting any gzip bytes. The host must return
				// metadata promptly regardless of which layer will decompress the body.
				payload := strings.Repeat("decoded response ", 64)
				release := make(chan struct{})
				unblock := sync.OnceFunc(func() { close(release) })
				acceptEncoding := make(chan string, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					acceptEncoding <- r.Header.Get("Accept-Encoding")
					w.Header().Set("Content-Type", "text/plain")
					w.Header().Set("Content-Encoding", "gzip")
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					writer := gzip.NewWriter(w)
					_, _ = io.WriteString(writer, payload)
					_ = writer.Close()
				}))
				t.Cleanup(server.Close)
				t.Cleanup(unblock)
				connection, _ := newIntegrationConnection(&integrationAuth{})
				limits := integrationLimits()
				limits.MaxResponseBytes = int64(len(payload))
				if tooLarge {
					limits.MaxResponseBytes--
				}
				session, err := httphost.New(context.Background(), connection, limits, nil)
				require.NoError(t, err)
				t.Cleanup(session.Close)
				request, err := http.NewRequest(http.MethodGet, server.URL, nil)
				require.NoError(t, err)
				if explicitEncoding {
					request.Header.Set("Accept-Encoding", "gzip")
				}
				type readyResponse struct {
					response *httphost.Response
					err      error
				}
				ready := make(chan readyResponse, 1)
				go func() {
					response, err := session.Do(request)
					ready <- readyResponse{response, err}
				}()
				var response *httphost.Response
				select {
				case got := <-ready:
					require.NoError(t, got.err)
					response = got.response
				case <-time.After(2 * time.Second):
					t.Fatal("host waited for gzip bytes before returning response headers")
				}
				defer response.Body.Close()
				require.Equal(t, http.StatusOK, response.StatusCode)
				require.Empty(t, response.Header.Get("Content-Encoding"))
				require.Equal(t, "text/plain", response.Header.Get("Content-Type"))
				require.Equal(t, "gzip", <-acceptEncoding)

				// Releasing the body exercises either transport-transparent gzip or
				// explicit host decoding, with the same aggregate decoded-byte cap.
				unblock()
				body, err := io.ReadAll(response.Body)
				if tooLarge {
					require.ErrorIs(t, err, httphost.ErrResponseLimit)
					require.LessOrEqual(t, int64(len(body)), limits.MaxResponseBytes)
				} else {
					require.NoError(t, err)
					require.Equal(t, payload, string(body))
				}
				require.Equal(t, api.ToolExecutionStateMayHaveExecuted, session.ExecutionState())
			})
		}
	}
}

// TestSessionIntegrationBodylessEncodedResponses preserves representation
// metadata without trying to decompress content that HTTP forbids sending.
func TestSessionIntegrationBodylessEncodedResponses(t *testing.T) {
	for _, tc := range []struct {
		method string
		status int
	}{
		{http.MethodHead, http.StatusOK},
		{http.MethodGet, http.StatusNoContent},
		{http.MethodGet, http.StatusNotModified},
	} {
		t.Run(fmt.Sprintf("%s/%d", tc.method, tc.status), func(t *testing.T) {
			// HEAD and bodyless statuses can describe a gzip representation while
			// carrying no encoded bytes. The actual transport preserves that header.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodHead {
					w.Header().Set("Content-Length", "123")
				}
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(server.Close)
			connection, _ := newIntegrationConnection(&integrationAuth{})
			session, err := httphost.New(context.Background(), connection, integrationLimits(), nil)
			require.NoError(t, err)
			t.Cleanup(session.Close)
			request, err := http.NewRequest(tc.method, server.URL, nil)
			require.NoError(t, err)
			response, err := session.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()

			// Reading a complete empty response succeeds and does not discard the
			// representation headers by pretending that decompression took place.
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Empty(t, body)
			require.Equal(t, tc.status, response.StatusCode)
			require.Equal(t, "gzip", response.Header.Get("Content-Encoding"))
			if tc.method == http.MethodHead {
				require.Equal(t, "123", response.Header.Get("Content-Length"))
			}
			require.NoError(t, session.Err())
		})
	}
}
