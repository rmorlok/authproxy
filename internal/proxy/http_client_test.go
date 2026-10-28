package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/golang/mock/gomock"
	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/httpf"
	mockHttpf "github.com/rmorlok/authproxy/internal/httpf/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// httpClientTestActor builds an authenticated actor and a context carrying it.
func httpClientTestActor() (*apauthcore.Actor, context.Context) {
	actor := &apauthcore.Actor{Id: apid.New(apid.PrefixActor), Namespace: "root"}
	return actor, apauthcore.NewAuthenticatedRequestAuth(actor).ContextWith(context.Background())
}

// TestProxyHTTPClient_BuildsMiddlewareOnce binds the actor, labels, and request
// type into one middleware stack that every subsequent Do reuses.
func TestProxyHTTPClient_BuildsMiddlewareOnce(t *testing.T) {
	factory := mockHttpf.NewMockF(gomock.NewController(t))
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root"}
	actor, ctx := httpClientTestActor()
	labels := map[string]string{"operation": "read-account"}
	var sends atomic.Int32
	client := &http.Client{Transport: streamRoundTripper(func(*http.Request) (*http.Response, error) {
		sends.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})}

	// Each factory step runs exactly once, at construction, with the client's
	// fixed attribution rather than anything read from a request context.
	factory.EXPECT().ForRequestType(httpf.RequestTypeProxy).Return(factory).Times(1)
	factory.EXPECT().ForConnection(conn).Return(factory).Times(1)
	factory.EXPECT().ForActor(actor).Return(factory).Times(1)
	factory.EXPECT().ForLabels(labels).Return(factory).Times(1)
	factory.EXPECT().NewHTTPClient().Return(client).Times(1)
	p := New(factory, conn, &fakeAuth{}, nil)
	httpClient, err := p.HTTPClient(iface.HTTPClientOptions{Labels: labels, Actor: iface.ActingAs(actor)})
	require.NoError(t, err)

	// Repeated calls reuse the stack; neither touches the factory again.
	for range 3 {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test", nil)
		require.NoError(t, err)
		resp, err := httpClient.Do(request)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	assert.EqualValues(t, 3, sends.Load())
}

// TestProxyHTTPClient_WithoutActorPassesNoAttribution forwards the explicit
// opt-out to httpf as an untyped nil actor, which skips actor labels.
func TestProxyHTTPClient_WithoutActorPassesNoAttribution(t *testing.T) {
	factory := mockHttpf.NewMockF(gomock.NewController(t))
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root"}
	factory.EXPECT().ForRequestType(httpf.RequestTypeProbe).Return(factory)
	factory.EXPECT().ForConnection(conn).Return(factory)
	factory.EXPECT().ForActor(nil).Return(factory)
	factory.EXPECT().ForLabels(gomock.Nil()).Return(factory)
	factory.EXPECT().NewHTTPClient().Return(&http.Client{Transport: streamRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
	})})
	p := New(factory, conn, &fakeAuth{}, nil)
	httpClient, err := p.HTTPClient(iface.HTTPClientOptions{RequestType: httpf.RequestTypeProbe, Actor: iface.WithoutActor()})
	require.NoError(t, err)

	// A background request with no actor on its context is accepted.
	resp, err := httpClient.Do(mustNewRawRequest(t, "https://example.test"))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

// TestProxyHTTPClient_RejectsInvalidActorChoice fails construction before any
// middleware is built when the caller has not explicitly chosen an actor.
func TestProxyHTTPClient_RejectsInvalidActorChoice(t *testing.T) {
	var typedNil *apauthcore.Actor
	for _, test := range []struct {
		name  string
		actor iface.HTTPClientActor
	}{
		{"zero value", iface.HTTPClientActor{}},
		{"nil actor", iface.ActingAs(nil)},
		{"typed nil actor", iface.ActingAs(typedNil)},
		{"nil actor id", iface.ActingAs(&apauthcore.Actor{Namespace: "root"})},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The strict mock fails the test if any factory method is called.
			factory := mockHttpf.NewMockF(gomock.NewController(t))
			p := New(factory, &mockCore.Connection{Id: apid.New(apid.PrefixConnection)}, &fakeAuth{}, nil)
			httpClient, err := p.HTTPClient(iface.HTTPClientOptions{Actor: test.actor})
			require.ErrorIs(t, err, iface.ErrHTTPClientActorRequired)
			require.Nil(t, httpClient)
		})
	}
}

// TestProxyHTTPClient_ActorAgreement accepts contexts that agree with the
// configured actor and rejects disagreeing ones before sending or resolving
// credentials, closing the unsent body.
func TestProxyHTTPClient_ActorAgreement(t *testing.T) {
	configured, configuredCtx := httpClientTestActor()
	_, otherCtx := httpClientTestActor()
	// A distinct object with the same identity still agrees: ids are compared.
	sameIdentityCtx := apauthcore.NewAuthenticatedRequestAuth(&apauthcore.Actor{Id: configured.Id, Namespace: "root"}).ContextWith(context.Background())

	for _, test := range []struct {
		name    string
		actor   iface.HTTPClientActor
		ctx     context.Context
		wantErr error
	}{
		{"same actor", iface.ActingAs(configured), configuredCtx, nil},
		{"same identity", iface.ActingAs(configured), sameIdentityCtx, nil},
		{"no context actor", iface.ActingAs(configured), context.Background(), nil},
		{"different actor", iface.ActingAs(configured), otherCtx, iface.ErrHTTPClientActorMismatch},
		{"opt-out with context actor", iface.WithoutActor(), configuredCtx, iface.ErrHTTPClientActorMismatch},
		{"opt-out without context actor", iface.WithoutActor(), context.Background(), nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			auth := &fakeAuth{}
			var sends atomic.Int32
			p := newStreamTestProxy(auth, func(*http.Request) (*http.Response, error) {
				sends.Add(1)
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
			})
			httpClient, err := p.HTTPClient(iface.HTTPClientOptions{Actor: test.actor})
			require.NoError(t, err)
			body := &streamBody{Reader: strings.NewReader("payload")}
			request, err := http.NewRequestWithContext(test.ctx, http.MethodPost, "https://example.test", body)
			require.NoError(t, err)

			resp, err := httpClient.Do(request)
			if test.wantErr == nil {
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				assert.EqualValues(t, 1, sends.Load())
				return
			}

			// Mismatches never resolve credentials or reach the transport, and
			// the client still releases the body it took ownership of.
			require.ErrorIs(t, err, test.wantErr)
			assert.Nil(t, resp)
			assert.Zero(t, sends.Load())
			assert.Zero(t, atomic.LoadInt32(&auth.resolveN))
			assert.True(t, body.closed.Load())
			assert.Zero(t, body.reads.Load())
		})
	}
}

// TestProxyHTTPClient_ActorMutationAfterConstruction keeps the agreement check
// pinned to the actor id the middleware captured, so mutating the actor object
// later cannot admit a request attributed to the original actor's identity.
func TestProxyHTTPClient_ActorMutationAfterConstruction(t *testing.T) {
	factory := mockHttpf.NewMockF(gomock.NewController(t))
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root"}
	actor, _ := httpClientTestActor()
	originalID := actor.Id
	// A separate object carrying A's id, unaffected by the mutation below.
	originalCtx := apauthcore.NewAuthenticatedRequestAuth(&apauthcore.Actor{Id: originalID, Namespace: "root"}).ContextWith(context.Background())
	auth := &fakeAuth{}
	var sends atomic.Int32
	client := &http.Client{Transport: streamRoundTripper(func(*http.Request) (*http.Response, error) {
		sends.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})}

	// The middleware snapshots actor A once, at construction.
	factory.EXPECT().ForRequestType(httpf.RequestTypeProxy).Return(factory)
	factory.EXPECT().ForConnection(conn).Return(factory)
	factory.EXPECT().ForActor(gomock.Any()).DoAndReturn(func(a httpf.Actor) httpf.F {
		require.Equal(t, originalID, a.GetId())
		return factory
	})
	factory.EXPECT().ForLabels(gomock.Nil()).Return(factory)
	factory.EXPECT().NewHTTPClient().Return(client)
	p := New(factory, conn, auth, nil)
	httpClient, err := p.HTTPClient(iface.HTTPClientOptions{Actor: iface.ActingAs(actor)})
	require.NoError(t, err)

	// The caller then rewrites the same actor object's id to B.
	actor.Id = apid.New(apid.PrefixActor)
	mutatedCtx := apauthcore.NewAuthenticatedRequestAuth(&apauthcore.Actor{Id: actor.Id, Namespace: "root"}).ContextWith(context.Background())

	// A request acting as B is refused before credentials or the transport.
	body := &streamBody{Reader: strings.NewReader("payload")}
	request, err := http.NewRequestWithContext(mutatedCtx, http.MethodPost, "https://example.test", body)
	require.NoError(t, err)
	resp, err := httpClient.Do(request)
	require.ErrorIs(t, err, iface.ErrHTTPClientActorMismatch)
	assert.Nil(t, resp)
	assert.True(t, body.closed.Load())
	assert.Zero(t, atomic.LoadInt32(&auth.resolveN))
	assert.Zero(t, sends.Load())

	// A request acting as the captured actor A still succeeds.
	request, err = http.NewRequestWithContext(originalCtx, http.MethodGet, "https://example.test", nil)
	require.NoError(t, err)
	resp, err = httpClient.Do(request)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.EqualValues(t, 1, sends.Load())
}

// TestProxyHTTPClient_SharesStreamRecovery runs the same once-only 401
// recovery as ProxyRequestStream, because both delegate to one implementation.
func TestProxyHTTPClient_SharesStreamRecovery(t *testing.T) {
	auth := &streamAuth{fakeAuth: fakeAuth{maxRecover: 10}, resolveFn: func(n int32) (auth_methods.AuthApplication, error) {
		return auth_methods.AuthApplication{Headers: map[string]string{"Authorization": fmt.Sprintf("Bearer attempt-%d", n)}}, nil
	}}
	var credentials []string
	p := newStreamTestProxy(auth, func(r *http.Request) (*http.Response, error) {
		credentials = append(credentials, r.Header.Get("Authorization"))
		if len(credentials) == 1 {
			return &http.Response{StatusCode: http.StatusUnauthorized, Body: http.NoBody}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	httpClient, err := p.HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
	require.NoError(t, err)

	// The bodyless request is replayed once with freshly resolved credentials.
	resp, err := httpClient.Do(mustNewRawRequest(t, "https://example.test"))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, []string{"Bearer attempt-1", "Bearer attempt-2"}, credentials)
	assert.EqualValues(t, 1, atomic.LoadInt32(&auth.recoverN))
}

// TestProxyHTTPClient_InvalidRequests rejects nil requests and requests with
// no URL before credentials are resolved, releasing any owned body.
func TestProxyHTTPClient_InvalidRequests(t *testing.T) {
	auth := &fakeAuth{}
	p := newStreamTestProxy(auth, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("unexpected transport call")
	})
	httpClient, err := p.HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
	require.NoError(t, err)

	// A nil request fails without touching anything.
	resp, err := httpClient.Do(nil)
	require.Error(t, err)
	assert.Nil(t, resp)

	// A request without a URL is rejected and its body closed unread.
	body := &streamBody{Reader: strings.NewReader("payload")}
	request, err := http.NewRequest(http.MethodPost, "https://example.test", body)
	require.NoError(t, err)
	request.URL = nil
	resp, err = httpClient.Do(request)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, body.closed.Load())
	assert.Zero(t, body.reads.Load())
	assert.Zero(t, atomic.LoadInt32(&auth.resolveN))
}

// TestProxyHTTPClient_ConcurrentCalls shares one prebuilt stack across
// goroutines while each request keeps its own context and response.
func TestProxyHTTPClient_ConcurrentCalls(t *testing.T) {
	const count = 16
	p := newStreamTestProxy(&fakeAuth{}, func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Path": {r.URL.Path}}, Body: http.NoBody}, nil
	})
	httpClient, err := p.HTTPClient(iface.HTTPClientOptions{Actor: iface.WithoutActor()})
	require.NoError(t, err)

	results := make(chan error, count)
	var workers sync.WaitGroup
	for i := range count {
		workers.Go(func() {
			path := fmt.Sprintf("/items/%d", i)
			request, err := http.NewRequest(http.MethodGet, "https://example.test"+path, nil)
			if err != nil {
				results <- err
				return
			}
			resp, err := httpClient.Do(request)
			if err == nil {
				_ = resp.Body.Close()
				if got := resp.Header.Get("X-Path"); got != path {
					err = fmt.Errorf("response for %s carried %s", path, got)
				}
			}
			results <- err
		})
	}
	workers.Wait()
	close(results)

	// Every call completes independently under the race detector.
	for err := range results {
		require.NoError(t, err)
	}
}
