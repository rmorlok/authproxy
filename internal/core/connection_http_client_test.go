package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/stretchr/testify/require"
)

// connectionHTTPClientTestProxy implements only the streaming path exercised by
// the client; unexpected use of another proxy capability cannot silently pass.
type connectionHTTPClientTestProxy struct {
	iface.Proxy
	stream func(context.Context, httpf.RequestType, *iface.RawProxyRequest) (*http.Response, error)
}

// ProxyRequestStream exposes each handoff to the test's programmable proxy.
func (p *connectionHTTPClientTestProxy) ProxyRequestStream(ctx context.Context, kind httpf.RequestType, request *iface.RawProxyRequest) (*http.Response, error) {
	return p.stream(ctx, kind, request)
}

// connectionHTTPClientTestBody tracks ownership and eager consumption without
// involving authentication, network transport, or response buffering fixtures.
type connectionHTTPClientTestBody struct {
	reader io.Reader
	reads  atomic.Int32
	closes atomic.Int32
}

// Read records consumption before forwarding to the finite fixture reader.
func (b *connectionHTTPClientTestBody) Read(p []byte) (int, error) {
	b.reads.Add(1)
	return b.reader.Read(p)
}

// Close records every release so wrapper and proxy ownership cannot overlap.
func (b *connectionHTTPClientTestBody) Close() error { b.closes.Add(1); return nil }

// connectionHTTPClientTestConnection supplies a resolved proxy without setting
// up auth factories; existing proxy tests own actual credential/recovery behavior.
func connectionHTTPClientTestConnection(proxy iface.Proxy) *connection {
	conn := &connection{proxyImpl: proxy}
	conn.proxyImplOnce.Do(func() {})
	return conn
}

// TestConnectionHTTPClientConstructionIsLazy creates a client without resolving
// credentials or calling a proxy, then initializes its connection only for Do.
func TestConnectionHTTPClientConstructionIsLazy(t *testing.T) {
	// Unwired credential dependencies would panic if construction consulted them.
	conn := &connection{}
	client := conn.HTTPClient(iface.HTTPClientOptions{})
	require.NotNil(t, client)
	var calls int
	proxy := &connectionHTTPClientTestProxy{stream: func(_ context.Context, kind httpf.RequestType, _ *iface.RawProxyRequest) (*http.Response, error) {
		calls++
		require.Equal(t, httpf.RequestTypeProxy, kind)
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	}}
	initialized := false
	conn.proxyImplOnce.Do(func() { initialized = true; conn.proxyImpl = proxy })
	require.True(t, initialized, "constructing a client must not initialize the connection proxy")
	require.Zero(t, calls)

	// The later handoff uses the captured connection's now-resolved proxy.
	request, err := http.NewRequest(http.MethodGet, "https://provider.example/items", nil)
	require.NoError(t, err)
	response, err := client.Do(request)
	require.NoError(t, err)
	require.Equal(t, 204, response.StatusCode)
	require.Equal(t, 1, calls)
	require.NoError(t, response.Body.Close())
}

// connectionHTTPClientTestContextKey identifies per-request context values.
type connectionHTTPClientTestContextKey struct{}

// TestConnectionHTTPClientOptionsAndContexts snapshots policy once, isolates
// labels on every handoff, and forwards each request's own context and features.
func TestConnectionHTTPClientOptionsAndContexts(t *testing.T) {
	for _, test := range []struct {
		name             string
		configured, want httpf.RequestType
	}{
		{"default request type", "", httpf.RequestTypeProxy},
		{"explicit request type", httpf.RequestTypeProbe, httpf.RequestTypeProbe},
	} {
		t.Run(test.name, func(t *testing.T) {
			labels := map[string]string{"team": "captured"}
			options := iface.HTTPClientOptions{RequestType: test.configured, Labels: labels}
			var calls int
			proxy := &connectionHTTPClientTestProxy{stream: func(ctx context.Context, kind httpf.RequestType, raw *iface.RawProxyRequest) (*http.Response, error) {
				calls++
				require.Equal(t, test.want, kind)
				require.Equal(t, calls, ctx.Value(connectionHTTPClientTestContextKey{}))
				require.Equal(t, raw.Outbound.Context(), ctx)
				require.Equal(t, map[string]string{"team": "captured"}, raw.Labels)
				raw.Labels["team"] = "proxy mutation"
				require.Equal(t, "tenant.provider.example", raw.Outbound.Host)
				require.Equal(t, []string{"first", "second"}, raw.Outbound.Header["X-Values"])
				defer raw.Outbound.Body.Close()
				payload, err := io.ReadAll(raw.Outbound.Body)
				require.NoError(t, err)
				require.Equal(t, "request data", string(payload))
				replay, err := raw.Outbound.GetBody()
				require.NoError(t, err)
				defer replay.Close()
				payload, err = io.ReadAll(replay)
				require.NoError(t, err)
				require.Equal(t, "request data", string(payload))
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			}}
			client := connectionHTTPClientTestConnection(proxy).HTTPClient(options)

			// Caller edits after construction and proxy edits after a handoff cannot
			// change the request type or labels observed by a later request.
			labels["team"] = "caller mutation"
			options.RequestType = httpf.RequestTypeGlobal
			for i := 1; i <= 2; i++ {
				ctx := context.WithValue(context.Background(), connectionHTTPClientTestContextKey{}, i)
				request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://provider.example/items", strings.NewReader("request data"))
				require.NoError(t, err)
				request.Host = "tenant.provider.example"
				request.Header["X-Values"] = []string{"first", "second"}
				response, err := client.Do(request)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
			}
			require.Equal(t, 2, calls)
			require.Equal(t, "caller mutation", labels["team"])
		})
	}
}

// TestConnectionHTTPClientResponseIsLazy transfers the response stream to the
// caller without reading, closing, or replacing it during Do.
func TestConnectionHTTPClientResponseIsLazy(t *testing.T) {
	requestBody := &connectionHTTPClientTestBody{reader: strings.NewReader("request data")}
	responseBody := &connectionHTTPClientTestBody{reader: strings.NewReader("response data")}
	upstream := &http.Response{StatusCode: 201, Header: http.Header{"X-Upstream": {"value"}}, Body: responseBody}
	proxy := &connectionHTTPClientTestProxy{stream: func(_ context.Context, _ httpf.RequestType, raw *iface.RawProxyRequest) (*http.Response, error) {
		// Proxy ownership starts at handoff, including unread request bodies.
		require.Same(t, requestBody, raw.Outbound.Body)
		require.NoError(t, raw.Outbound.Body.Close())
		return upstream, nil
	}}
	client := connectionHTTPClientTestConnection(proxy).HTTPClient(iface.HTTPClientOptions{})
	request, err := http.NewRequest(http.MethodPost, "https://provider.example/items", requestBody)
	require.NoError(t, err)
	response, err := client.Do(request)
	require.NoError(t, err)
	require.Same(t, upstream, response)
	require.EqualValues(t, 1, requestBody.closes.Load())
	require.Zero(t, responseBody.reads.Load())
	require.Zero(t, responseBody.closes.Load())

	// Only the response consumer reads and releases the original transport body.
	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "response data", string(payload))
	require.NoError(t, response.Body.Close())
	require.EqualValues(t, 1, responseBody.closes.Load())
}

// TestConnectionHTTPClientInitializationFailure retains the streaming proxy's
// ownership guarantee even when credentials cannot be initialized before I/O.
func TestConnectionHTTPClientInitializationFailure(t *testing.T) {
	initializationError := errors.New("proxy initialization failed")
	conn := &connection{proxyImplErr: initializationError}
	conn.proxyImplOnce.Do(func() {})
	client := conn.HTTPClient(iface.HTTPClientOptions{})
	body := &connectionHTTPClientTestBody{reader: strings.NewReader("request data")}
	request, err := http.NewRequest(http.MethodPost, "https://provider.example/items", body)
	require.NoError(t, err)

	// Failure still closes exactly once through the existing connection path.
	response, err := client.Do(request)
	require.ErrorIs(t, err, initializationError)
	require.Nil(t, response)
	require.Zero(t, body.reads.Load())
	require.EqualValues(t, 1, body.closes.Load())
}

// TestConnectionHTTPClientNilRequest rejects a missing request before accessing
// its context or attempting to resolve connection authentication.
func TestConnectionHTTPClientNilRequest(t *testing.T) {
	conn := &connection{}
	client := conn.HTTPClient(iface.HTTPClientOptions{})
	response, err := client.Do(nil)
	require.Error(t, err)
	require.Nil(t, response)
	untouched := false
	conn.proxyImplOnce.Do(func() { untouched = true })
	require.True(t, untouched)
}

// TestConnectionHTTPClientConcurrentCalls reuses one client without sharing
// mutable per-request labels or capturing any request's context for another.
func TestConnectionHTTPClientConcurrentCalls(t *testing.T) {
	const count = 16
	proxy := &connectionHTTPClientTestProxy{stream: func(ctx context.Context, _ httpf.RequestType, raw *iface.RawProxyRequest) (*http.Response, error) {
		if raw.Labels["team"] != "captured" {
			return nil, errors.New("another request mutated the labels")
		}
		value := fmt.Sprint(ctx.Value(connectionHTTPClientTestContextKey{}))
		raw.Labels["team"] = value
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(value))}, nil
	}}
	client := connectionHTTPClientTestConnection(proxy).HTTPClient(iface.HTTPClientOptions{Labels: map[string]string{"team": "captured"}})
	results := make(chan error, count)
	var workers sync.WaitGroup
	for i := range count {
		ctx := context.WithValue(context.Background(), connectionHTTPClientTestContextKey{}, i)
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://provider.example/items", nil)
		require.NoError(t, err)
		workers.Go(func() {
			response, err := client.Do(request)
			if err == nil {
				payload, readErr := io.ReadAll(response.Body)
				_ = response.Body.Close()
				err = readErr
				if err == nil && string(payload) != fmt.Sprint(i) {
					err = errors.New("response used another request's context")
				}
			}
			results <- err
		})
	}
	workers.Wait()
	close(results)

	// All calls retain their own context and isolated label map under the race detector.
	for err := range results {
		require.NoError(t, err)
	}
}
