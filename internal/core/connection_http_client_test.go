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

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/stretchr/testify/require"
)

// connectionHTTPClientTestProxy implements only the client-building path used
// by the connection wrapper; unexpected use of another proxy capability panics.
type connectionHTTPClientTestProxy struct {
	iface.Proxy
	// builds counts proxy-level clients, which own the middleware stack.
	builds atomic.Int32
	// options records the most recent options passed to HTTPClient.
	options iface.HTTPClientOptions
	// err rejects client construction before do can handle a request.
	err error
	// do handles each request sent through a built client.
	do func(*http.Request) (*http.Response, error)
	// doWithLabels handles explicitly labeled requests without rebuilding a client.
	doWithLabels func(*http.Request, map[string]string) (*http.Response, error)
}

// HTTPClient records the options and returns a client delegating to do.
func (p *connectionHTTPClientTestProxy) HTTPClient(options iface.HTTPClientOptions) (iface.HTTPClient, error) {
	p.builds.Add(1)
	p.options = options
	return connectionHTTPClientTestDoer{do: p.do, doWithLabels: p.doWithLabels}, p.err
}

// connectionHTTPClientTestDoer adapts fixture callbacks to iface.HTTPClient.
type connectionHTTPClientTestDoer struct {
	do           func(*http.Request) (*http.Response, error)
	doWithLabels func(*http.Request, map[string]string) (*http.Response, error)
}

// Do delegates to the fixture function.
func (f connectionHTTPClientTestDoer) Do(r *http.Request) (*http.Response, error) { return f.do(r) }

// DoWithLabels delegates to the labeled fixture function; an unexpected call
// panics rather than silently discarding labels a scenario did not configure.
func (f connectionHTTPClientTestDoer) DoWithLabels(r *http.Request, labels map[string]string) (*http.Response, error) {
	return f.doWithLabels(r, labels)
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
// up auth factories; proxy tests own actual credential and recovery behavior.
func connectionHTTPClientTestConnection(proxy iface.Proxy) *connection {
	conn := &connection{proxyImpl: proxy}
	conn.proxyImplOnce.Do(func() {})
	return conn
}

// TestConnectionHTTPClientRequiresActorChoice rejects options without an
// explicit ActingAs or WithoutActor before any proxy work happens.
func TestConnectionHTTPClientRequiresActorChoice(t *testing.T) {
	conn := &connection{}
	for _, actor := range []iface.HTTPClientActor{{}, iface.ActingAs(nil), iface.ActingAs(&apauthcore.Actor{})} {
		client, err := conn.HTTPClient(iface.HTTPClientOptions{Actor: actor})
		require.ErrorIs(t, err, iface.ErrHTTPClientActorRequired)
		require.Nil(t, client)
	}

	// Rejection never initializes the connection proxy.
	untouched := false
	conn.proxyImplOnce.Do(func() { untouched = true })
	require.True(t, untouched)
}

// TestConnectionHTTPClientConstructionIsLazy creates a client without resolving
// credentials, then builds the proxy-level client once on the first Do and
// reuses it for every later call.
func TestConnectionHTTPClientConstructionIsLazy(t *testing.T) {
	// Unwired credential dependencies would panic if construction consulted them.
	conn := &connection{}
	client, err := conn.HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
	require.NoError(t, err)
	var calls int
	proxy := &connectionHTTPClientTestProxy{do: func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	}}
	initialized := false
	conn.proxyImplOnce.Do(func() { initialized = true; conn.proxyImpl = proxy })
	require.True(t, initialized, "constructing a client must not initialize the connection proxy")
	require.Zero(t, proxy.builds.Load())

	// Two calls share one middleware stack built on first use.
	for range 2 {
		request, err := http.NewRequest(http.MethodGet, "https://provider.example/items", nil)
		require.NoError(t, err)
		response, err := client.Do(request)
		require.NoError(t, err)
		require.Equal(t, 204, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	require.Equal(t, 2, calls)
	require.EqualValues(t, 1, proxy.builds.Load())
}

// TestConnectionHTTPClientOptionsAreDetached normalizes options at
// construction so later caller edits cannot change the built client.
func TestConnectionHTTPClientOptionsAreDetached(t *testing.T) {
	actor := &apauthcore.Actor{Id: apid.New(apid.PrefixActor)}
	for _, test := range []struct {
		name             string
		configured, want httpf.RequestType
	}{
		{"default request type", "", httpf.RequestTypeProxy},
		{"explicit request type", httpf.RequestTypeProbe, httpf.RequestTypeProbe},
	} {
		t.Run(test.name, func(t *testing.T) {
			labels := map[string]string{"team": "captured"}
			options := iface.HTTPClientOptions{RequestType: test.configured, Labels: labels, Actor: iface.ActingAs(actor)}
			proxy := &connectionHTTPClientTestProxy{do: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			}}
			client, err := connectionHTTPClientTestConnection(proxy).HTTPClient(options)
			require.NoError(t, err)

			// Edits after construction are invisible to the lazily built client.
			labels["team"] = "caller mutation"
			options.RequestType = httpf.RequestTypeGlobal
			request, err := http.NewRequest(http.MethodGet, "https://provider.example/items", nil)
			require.NoError(t, err)
			response, err := client.Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, test.want, proxy.options.RequestType)
			require.Equal(t, map[string]string{"team": "captured"}, proxy.options.Labels)
			require.Same(t, actor, proxy.options.Actor.Actor())
		})
	}
}

// TestConnectionHTTPClientResponseIsLazy transfers the response stream to the
// caller without reading, closing, or replacing it during Do.
func TestConnectionHTTPClientResponseIsLazy(t *testing.T) {
	requestBody := &connectionHTTPClientTestBody{reader: strings.NewReader("request data")}
	responseBody := &connectionHTTPClientTestBody{reader: strings.NewReader("response data")}
	upstream := &http.Response{StatusCode: 201, Header: http.Header{"X-Upstream": {"value"}}, Body: responseBody}
	proxy := &connectionHTTPClientTestProxy{do: func(r *http.Request) (*http.Response, error) {
		// Proxy ownership starts at handoff, including unread request bodies.
		require.Same(t, requestBody, r.Body)
		require.NoError(t, r.Body.Close())
		return upstream, nil
	}}
	client, err := connectionHTTPClientTestConnection(proxy).HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
	require.NoError(t, err)
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

// TestConnectionHTTPClientInitializationFailure reports no target send, closes
// every unsent body, and keeps returning the cached error on later calls.
func TestConnectionHTTPClientInitializationFailure(t *testing.T) {
	for _, stage := range []string{"proxy resolution", "client construction"} {
		t.Run(stage, func(t *testing.T) {
			// Either initialization stage can fail before a target call is made.
			initializationError := errors.New("proxy initialization failed")
			proxy := &connectionHTTPClientTestProxy{err: initializationError, do: func(*http.Request) (*http.Response, error) {
				t.Fatal("initialization failure must prevent proxy client calls")
				return nil, nil
			}}
			conn := connectionHTTPClientTestConnection(proxy)
			if stage == "proxy resolution" {
				conn.proxyImplErr = initializationError
			}
			client, err := conn.HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
			require.NoError(t, err)

			// Initial and cached failures each report no send and release their
			// own request body exactly once without consuming it.
			for range 2 {
				ctx, observation := httpf.ObserveSends(context.Background())
				body := &connectionHTTPClientTestBody{reader: strings.NewReader("request data")}
				request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://provider.example/items", body)
				require.NoError(t, err)
				response, err := client.Do(request)
				require.ErrorIs(t, err, initializationError)
				require.Nil(t, response)
				require.False(t, observation.MayHaveSent())
				require.Zero(t, body.reads.Load())
				require.EqualValues(t, 1, body.closes.Load())
			}

			// A cached local failure cannot erase an earlier possible send in
			// the same observation, and a bodyless request needs no cleanup.
			ctx, observation := httpf.ObserveSends(context.Background())
			httpf.ReportSendEvidence(ctx, true)
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://provider.example/items", nil)
			require.NoError(t, err)
			response, err := client.Do(request)
			require.ErrorIs(t, err, initializationError)
			require.Nil(t, response)
			require.True(t, observation.MayHaveSent())
			if stage == "client construction" {
				require.EqualValues(t, 1, proxy.builds.Load(), "failed initialization is not retried")
			} else {
				require.Zero(t, proxy.builds.Load(), "proxy resolution fails before client construction")
			}
		})
	}
}

// TestConnectionHTTPClientNilRequest rejects a missing request before
// attempting to resolve connection authentication.
func TestConnectionHTTPClientNilRequest(t *testing.T) {
	conn := &connection{}
	client, err := conn.HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
	require.NoError(t, err)
	response, err := client.Do(nil)
	require.Error(t, err)
	require.Nil(t, response)
	untouched := false
	conn.proxyImplOnce.Do(func() { untouched = true })
	require.True(t, untouched)
}

// connectionHTTPClientTestContextKey identifies per-request context values.
type connectionHTTPClientTestContextKey struct{}

// TestConnectionHTTPClientConcurrentCalls builds the proxy client once under
// concurrent first use while each request keeps its own context.
func TestConnectionHTTPClientConcurrentCalls(t *testing.T) {
	const count = 16
	proxy := &connectionHTTPClientTestProxy{do: func(r *http.Request) (*http.Response, error) {
		value := fmt.Sprint(r.Context().Value(connectionHTTPClientTestContextKey{}))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(value))}, nil
	}}
	client, err := connectionHTTPClientTestConnection(proxy).HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
	require.NoError(t, err)
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

	// All calls keep their own context, and the stack was built only once.
	for err := range results {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, proxy.builds.Load())
}

// TestConnectionHTTPClientRequestLabelsRejectBeforeInitialization prevents
// invalid request metadata from resolving a connection or consuming its body.
func TestConnectionHTTPClientRequestLabelsRejectBeforeInitialization(t *testing.T) {
	for _, labels := range []map[string]string{
		{"apxy/act/-/id": "forged"},
		{"invalid key": "value"},
		{"valid-key": "invalid value"},
	} {
		// An unwired connection would fail if label validation initialized it.
		conn := &connection{}
		client, err := conn.HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
		require.NoError(t, err)
		ctx, observation := httpf.ObserveSends(context.Background())
		body := &connectionHTTPClientTestBody{reader: strings.NewReader("request data")}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://provider.example/items", body)
		require.NoError(t, err)

		response, err := client.DoWithLabels(request, labels)
		require.ErrorIs(t, err, iface.ErrHTTPClientInvalidLabels)
		require.Nil(t, response)
		require.Equal(t, httpf.SendStateNotSent, observation.State())
		require.EqualValues(t, 1, body.closes.Load())
		require.Zero(t, body.reads.Load())

		// Rejection neither initializes nor poisons the lazy client's state.
		untouched := false
		conn.proxyImplOnce.Do(func() { untouched = true })
		require.True(t, untouched)
	}
}

// TestConnectionHTTPClientRequestLabelsRemainPerCall forwards concurrent labels
// through one initialized proxy client without replacing its baseline options.
func TestConnectionHTTPClientRequestLabelsRemainPerCall(t *testing.T) {
	const count = 16
	proxy := &connectionHTTPClientTestProxy{
		do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
		},
		doWithLabels: func(r *http.Request, labels map[string]string) (*http.Response, error) {
			if labels["operation"] != r.Header.Get("X-Operation") {
				return nil, errors.New("request used another call's labels")
			}
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		},
	}
	baseline := map[string]string{"operation": "baseline"}
	client, err := connectionHTTPClientTestConnection(proxy).HTTPClient(iface.HTTPClientOptions{
		Actor: iface.WithoutActor(), Labels: baseline,
	})
	require.NoError(t, err)

	// Concurrent first use builds one proxy client; each call carries its own map.
	results := make(chan error, count)
	var workers sync.WaitGroup
	for index := range count {
		operation := fmt.Sprintf("operation-%d", index)
		request, err := http.NewRequest(http.MethodGet, "https://provider.example/items", nil)
		require.NoError(t, err)
		request.Header.Set("X-Operation", operation)
		workers.Go(func() {
			response, err := client.DoWithLabels(request, map[string]string{"operation": operation})
			if response != nil {
				_ = response.Body.Close()
				if err == nil && response.StatusCode != http.StatusOK {
					err = errors.New("labeled request used the baseline path")
				}
			}
			results <- err
		})
	}
	workers.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}

	// Omitted and empty labels preserve the baseline path after labeled calls.
	for _, labels := range []map[string]string{nil, {}} {
		request, err := http.NewRequest(http.MethodGet, "https://provider.example/items", nil)
		require.NoError(t, err)
		response, err := client.DoWithLabels(request, labels)
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	require.EqualValues(t, 1, proxy.builds.Load())
	require.Equal(t, baseline, proxy.options.Labels)
	request, err := http.NewRequest(http.MethodGet, "https://provider.example/items", nil)
	require.NoError(t, err)
	response, err := client.Do(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, response.Body.Close())
}
