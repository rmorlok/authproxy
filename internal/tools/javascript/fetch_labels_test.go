package javascript_test

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/golang/mock/gomock"
	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	authmock "github.com/rmorlok/authproxy/internal/auth_methods/mock"
	"github.com/rmorlok/authproxy/internal/core/iface"
	coremock "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/proxy"
	"github.com/rmorlok/authproxy/internal/tools/httphost"
	"github.com/rmorlok/authproxy/internal/tools/javascript"
	"github.com/rmorlok/authproxy/internal/tools/render"
	"github.com/stretchr/testify/require"
)

// fetchLabelMiddleware records the real configured factory's resolved request
// attribution and returns those user labels without contacting an upstream.
type fetchLabelMiddleware struct {
	mu       sync.Mutex          // Guards concurrent request observations.
	requests []httpf.RequestInfo // Captured after the factory merged per-call labels.
}

// fetchLabelTransport adapts a fake provider to the factory transport boundary.
type fetchLabelTransport func(*http.Request) (*http.Response, error)

// RoundTrip dispatches one request to the in-memory provider.
func (f fetchLabelTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// NewRoundTripper retains each middleware snapshot independently; the returned
// provider owns uploads and exposes only selected labels in its JSON response.
func (m *fetchLabelMiddleware) NewRoundTripper(info httpf.RequestInfo, _ http.RoundTripper) http.RoundTripper {
	return fetchLabelTransport(func(req *http.Request) (*http.Response, error) {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		m.mu.Lock()
		m.requests = append(m.requests, info)
		m.mu.Unlock()
		payload, err := json.Marshal(map[string]string{
			"operation": info.Labels["operation"], "workload": info.Labels["workload"],
		})
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(string(payload))), Request: req,
		}, nil
	})
}

// TestToolFetchLabelsReachConfiguredMiddleware joins JavaScript conversion,
// adapter validation, shared HTTP budgets, and the real proxy/factory snapshot.
func TestToolFetchLabelsReachConfiguredMiddleware(t *testing.T) {
	// Capture one connection and actor before caller-owned metadata is edited.
	actor := &authcore.Actor{Id: apid.New(apid.PrefixActor), Namespace: "root", Labels: map[string]string{"team": "captured"}}
	conn := &coremock.Connection{
		Id: apid.New(apid.PrefixConnection), Namespace: "root.product.owner",
		Labels: map[string]string{"operation": "read", "region": "west"},
	}
	middleware := &fetchLabelMiddleware{}
	factory := httpf.CreateFactory(nil, nil, middleware, nil)
	auth := authmock.NewMockAuthenticator(gomock.NewController(t))
	auth.EXPECT().Resolve(gomock.Any()).Return(auth_methods.AuthApplication{}, nil).Times(3)
	client, err := proxy.New(factory, conn, auth, nil).HTTPClient(iface.HTTPClientOptions{Actor: iface.ActingAs(actor)})
	require.NoError(t, err)
	input := javascript.Input{Context: expressionContext(t, render.Input{
		ConnectionID: conn.Id.String(), ConnectionNamespace: conn.Namespace, Labels: maps.Clone(conn.Labels),
	})}
	actor.Labels["team"], conn.Labels["region"] = "changed", "east"
	limits := fetchLimits()
	limits.MaxRequests = 3
	session, err := httphost.New(context.Background(), client, limits)
	require.NoError(t, err)
	t.Cleanup(session.Close)
	program, err := javascript.CompileTool(`async function execute(params, context) {
		const [connection] = context.connections;
		const labels = {operation: "write", workload: "alpha"};
		const first = connection.fetch("https://provider.example/items", {labels});
		labels.workload = "edited-after-fetch";
		const second = connection.fetch("https://provider.example/items", {labels: {workload: "beta"}});
		let invalid;
		try {
			await connection.fetch("https://provider.example/items", {labels: {"apxy/act/-/id": "forged"}});
		} catch (error) { invalid = error.message; }
		const third = connection.fetch("https://provider.example/items");
		const responses = await Promise.all([first, second, third]);
		const results = await Promise.all(responses.map(response => response.json()));
		return {results, invalid, labels: connection.labels, alias: context.labels === connection.labels, direct: typeof fetch};
	}`, nil, expressionLimits())
	require.NoError(t, err)
	result, err := program.EvaluateWithHTTP(context.Background(), input, javascript.FetchOptions{
		Session: session, MaxRequestBytes: 4096, MaxPendingOperations: 8,
	})
	require.NoError(t, err)

	// Authored edits and concurrent overrides stay private, and rejected labels
	// consume no request allowance. All three valid calls still reach middleware.
	require.JSONEq(t, `{
		"results":[{"operation":"write","workload":"alpha"},{"operation":"read","workload":"beta"},{"operation":"read","workload":""}],
		"invalid":"invalid JavaScript input", "labels":{"operation":"read","region":"west"},
		"alias":true,"direct":"undefined"
	}`, string(result))
	middleware.mu.Lock()
	defer middleware.mu.Unlock()
	require.Len(t, middleware.requests, 3)
	for _, info := range middleware.requests {
		require.Equal(t, conn.Id, info.ConnectionId)
		require.Equal(t, conn.Namespace, info.Namespace)
		require.Equal(t, httpf.RequestTypeProxy, info.Type)
		require.Equal(t, actor.Id.String(), info.Labels["apxy/act/-/id"])
		require.Equal(t, "captured", info.Labels["apxy/act/team"])
		require.Equal(t, "west", info.Labels["region"])
	}
}
