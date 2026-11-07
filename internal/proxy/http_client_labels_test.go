package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

// labelClientMiddleware observes the factory's real snapshots while replacing
// network I/O with a scenario-owned provider. Its counters support concurrent Do.
type labelClientMiddleware struct {
	mu       sync.Mutex // Guards builds and attempts across concurrent requests.
	builds   int
	attempts []httpf.RequestInfo
	handle   func(httpf.RequestInfo, *http.Request) (*http.Response, error)
}

// NewRoundTripper captures one immutable snapshot and records each attempted
// request before the optional provider handles it. No network is contacted.
func (m *labelClientMiddleware) NewRoundTripper(info httpf.RequestInfo, _ http.RoundTripper) http.RoundTripper {
	m.mu.Lock()
	m.builds++
	m.mu.Unlock()
	return streamRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		m.mu.Lock()
		m.attempts = append(m.attempts, info)
		m.mu.Unlock()
		if m.handle != nil {
			return m.handle(info, req)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
}

// labelClientConnection exposes caller-owned policy pointers so the tests can
// verify the configured client's factory detaches them along with metadata.
type labelClientConnection struct {
	mockCore.Connection
	policy *connectors.RateLimiting
	trace  *bool
}

// GetRateLimitConfig supplies the policy captured when the client is built.
func (c *labelClientConnection) GetRateLimitConfig() *connectors.RateLimiting { return c.policy }

// PropagateTraceContext supplies the connector's captured tracing preference.
func (c *labelClientConnection) PropagateTraceContext() *bool { return c.trace }

// TestProxyHTTPClientLabelsSnapshot pins attribution/policy once and applies
// each call's labels on top of connection and client user labels only.
func TestProxyHTTPClientLabelsSnapshot(t *testing.T) {
	actor, _ := httpClientTestActor()
	actor.Labels = map[string]string{"team": "blue"}
	actorID := actor.Id
	trace := true
	conn := &labelClientConnection{
		Connection: mockCore.Connection{
			Id: apid.New(apid.PrefixConnection), Namespace: "root.product.owner",
			ConnectorId: apid.New(apid.PrefixConnector), ConnectorGeneration: 7,
			Labels: map[string]string{"operation": "connection", "region": "west", "apxy/ns/tier": "paid"},
		},
		policy: &connectors.RateLimiting{RetryAfterHeaders: []string{"Retry-After"}},
		trace:  &trace,
	}
	connID, connectorID := conn.Id, conn.ConnectorId
	middleware := &labelClientMiddleware{}
	factory := httpf.CreateFactory(nil, nil, middleware, nil)
	baseLabels := map[string]string{"operation": "client", "job": "sync"}
	client, err := New(factory, conn, &fakeAuth{}, nil).HTTPClient(iface.HTTPClientOptions{
		RequestType: httpf.RequestTypeProbe, Actor: iface.ActingAs(actor), Labels: baseLabels,
	})
	require.NoError(t, err)

	// Changes to the original objects cannot alter subsequent derived stacks.
	actor.Id, actor.Labels["team"] = apid.New(apid.PrefixActor), "red"
	conn.Id, conn.ConnectorId, conn.ConnectorGeneration = apid.New(apid.PrefixConnection), apid.New(apid.PrefixConnector), 8
	conn.Namespace, conn.Labels["region"], baseLabels["job"] = "root.changed", "east", "changed"
	conn.policy.RetryAfterHeaders[0], conn.policy.Disabled, trace = "Changed-Header", true, false
	ctx := apauthcore.NewAuthenticatedRequestAuth(&apauthcore.Actor{Id: actorID}).ContextWith(context.Background())
	labels := map[string]string{"operation": "request", "page": "first"}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test/items", nil)
	require.NoError(t, err)
	response, err := client.DoWithLabels(request, labels)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	labels["operation"] = "caller-edit"

	// Omission and an empty map reuse the base stack without sticky overrides.
	response, err = client.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	response, err = client.DoWithLabels(request, map[string]string{})
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, 2, middleware.builds)
	require.Len(t, middleware.attempts, 3)
	for index, info := range middleware.attempts {
		require.Equal(t, connID, info.ConnectionId)
		require.Equal(t, connectorID, info.ConnectorId)
		require.EqualValues(t, 7, info.ConnectorGeneration)
		require.Equal(t, "root.product.owner", info.Namespace)
		require.Equal(t, httpf.RequestTypeProbe, info.Type)
		require.Equal(t, actorID.String(), info.Labels["apxy/act/-/id"])
		require.Equal(t, "blue", info.Labels["apxy/act/team"])
		require.Equal(t, "paid", info.Labels["apxy/ns/tier"])
		require.Equal(t, "west", info.Labels["region"])
		require.Equal(t, "sync", info.Labels["job"])
		require.False(t, info.RateLimiting.Disabled)
		require.Equal(t, []string{"Retry-After"}, info.RateLimiting.RetryAfterHeaders)
		require.True(t, *info.PropagateTraceContext)
		if index == 0 {
			require.Equal(t, "request", info.Labels["operation"])
			require.Equal(t, "first", info.Labels["page"])
		} else {
			require.Equal(t, "client", info.Labels["operation"])
			require.NotContains(t, info.Labels, "page")
		}
	}
	require.Equal(t, "connection", conn.Labels["operation"])
	require.Equal(t, "client", baseLabels["operation"])
}

// TestProxyHTTPClientLabelsRejected rejects invalid per-call data before stack
// construction, credential resolution, or dispatch, with known-unsent evidence.
func TestProxyHTTPClientLabelsRejected(t *testing.T) {
	middleware := &labelClientMiddleware{}
	auth := &fakeAuth{}
	client, err := New(httpf.CreateFactory(nil, nil, middleware, nil), &mockCore.Connection{}, auth, nil).
		HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
	require.NoError(t, err)
	for _, labels := range []map[string]string{
		{"apxy/act/-/id": "forged"}, {"apxy/cxn/-/id": "forged"}, {"apxy/request/type": "probe"},
		{"": "value"}, {"-invalid": "value"}, {"valid": "invalid value"}, {"valid": strings.Repeat("a", 64)},
	} {
		// Each rejected call owns its unread body and returns a fixed error.
		ctx, observation := httpf.ObserveSends(context.Background())
		body := &streamBody{Reader: strings.NewReader("private payload")}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.test", body)
		require.NoError(t, err)
		response, err := client.DoWithLabels(request, labels)
		require.ErrorIs(t, err, iface.ErrHTTPClientInvalidLabels)
		require.Nil(t, response)
		require.True(t, body.closed.Load())
		require.Zero(t, body.reads.Load())
		require.Equal(t, httpf.SendStateNotSent, observation.State())
	}
	require.Equal(t, 1, middleware.builds)
	require.Empty(t, middleware.attempts)
	require.Zero(t, atomic.LoadInt32(&auth.resolveN))
}

// TestProxyHTTPClientLabelsConcurrent keeps independently labeled calls and an
// ordinary Do isolated while sharing a single captured connection capability.
func TestProxyHTTPClientLabelsConcurrent(t *testing.T) {
	middleware := &labelClientMiddleware{handle: func(info httpf.RequestInfo, req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{"X-Operation": {info.Labels["operation"]}}}, nil
	}}
	client, err := New(httpf.CreateFactory(nil, nil, middleware, nil), &mockCore.Connection{}, &fakeAuth{}, nil).
		HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor(), Labels: map[string]string{"operation": "base"}})
	require.NoError(t, err)
	const count = 16
	results := make(chan error, count)
	var workers sync.WaitGroup
	for index := range count {
		workers.Go(func() {
			want := fmt.Sprint(index)
			labels := map[string]string{"operation": want}
			if index == 0 {
				want, labels = "base", nil
			}
			request, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
			if err == nil {
				var response *http.Response
				response, err = client.DoWithLabels(request, labels)
				if err == nil {
					_ = response.Body.Close()
					if got := response.Header.Get("X-Operation"); got != want {
						err = fmt.Errorf("operation %q got labels for %q", want, got)
					}
				}
			}
			results <- err
		})
	}
	workers.Wait()
	close(results)

	// The race detector checks shared factory access; each response proves the
	// middleware saw the requesting call's own labels, including the base call.
	for err := range results {
		require.NoError(t, err)
	}
	require.Equal(t, count, middleware.builds)
	require.Len(t, middleware.attempts, count)
}

// TestProxyHTTPClientLabelsRecoveryAndRedirect pins the same request snapshot
// through a credential retry and a subsequent redirect without rebuilding it.
func TestProxyHTTPClientLabelsRecoveryAndRedirect(t *testing.T) {
	var credentials []string
	middleware := &labelClientMiddleware{handle: func(_ httpf.RequestInfo, req *http.Request) (*http.Response, error) {
		credentials = append(credentials, req.Header.Get("Authorization"))
		response := &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}
		switch len(credentials) {
		case 1:
			response.StatusCode = http.StatusUnauthorized
		case 2:
			response.StatusCode = http.StatusFound
			response.Header.Set("Location", "/next")
		}
		return response, nil
	}}
	auth := &streamAuth{fakeAuth: fakeAuth{maxRecover: 1}, resolveFn: func(n int32) (auth_methods.AuthApplication, error) {
		return auth_methods.AuthApplication{Headers: map[string]string{"Authorization": fmt.Sprintf("Bearer attempt-%d", n)}}, nil
	}}
	client, err := New(httpf.CreateFactory(nil, nil, middleware, nil), &mockCore.Connection{}, auth, nil).
		HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
	require.NoError(t, err)
	response, err := client.DoWithLabels(mustNewRawRequest(t, "https://example.test/start"), map[string]string{"operation": "sync"})
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	// All three attempts use the same resolved labels; only recovery changes
	// credentials. The redirect retains the recovered credentials on this host.
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, []string{"Bearer attempt-1", "Bearer attempt-2", "Bearer attempt-2"}, credentials)
	require.Equal(t, 2, middleware.builds)
	require.Len(t, middleware.attempts, 3)
	for _, info := range middleware.attempts {
		require.Equal(t, "sync", info.Labels["operation"])
	}
}
