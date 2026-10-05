package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

// streamAuth supplies attempt-specific credentials while reusing the raw-path
// fixture's recovery accounting and unused authenticator methods.
type streamAuth struct {
	fakeAuth
	resolveFn func(int32) (auth_methods.AuthApplication, error)
}

// Resolve records each attempt before selecting its credentials or failure.
func (a *streamAuth) Resolve(context.Context) (auth_methods.AuthApplication, error) {
	return a.resolveFn(atomic.AddInt32(&a.resolveN, 1))
}

// streamRoundTripper exercises transport failures without socket timing races.
type streamRoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip delegates the attempt to the test's synchronous transport.
func (f streamRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// streamBody records ownership and detects unwanted draining of a response.
type streamBody struct {
	io.Reader
	reads  atomic.Int32
	closed atomic.Bool
}

// Read records consumption independently of closing the body.
func (b *streamBody) Read(p []byte) (int, error) {
	b.reads.Add(1)
	return b.Reader.Read(p)
}

// Close marks the body released without consuming any additional bytes.
func (b *streamBody) Close() error { b.closed.Store(true); return nil }

// streamReaderFunc models an open upstream that must not be read again after
// the downstream disconnects, without letting a regression hang the test.
type streamReaderFunc func([]byte) (int, error)

// Read delegates the next chunk or an unexpected-read error to the fixture.
func (f streamReaderFunc) Read(p []byte) (int, error) { return f(p) }

// streamFailWriter accepts response headers but fails the first body write.
type streamFailWriter struct {
	*recordingResponseWriter
	err error
}

// Write simulates a disconnected downstream while retaining its original error.
func (w *streamFailWriter) Write([]byte) (int, error) { return 0, w.err }

// newStreamTestProxy reuses the raw-path factory with a deterministic transport.
func newStreamTestProxy(auth auth_methods.Authenticator, transport streamRoundTripper) iface.Proxy {
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root/"}
	return New(&stubHttpf{client: &http.Client{Transport: transport}}, conn, auth, nil)
}

// TestProxyRequestStream_IncrementalAndInterruptible proves headers and bytes
// reach the caller before EOF, and neither cancellation nor Close drains SSE.
func TestProxyRequestStream_IncrementalAndInterruptible(t *testing.T) {
	for _, action := range []string{"close", "cancel"} {
		t.Run(action, func(t *testing.T) {
			const event = "data: first\n\n"
			release := make(chan struct{})
			upstreamDone := make(chan struct{})
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("X-Stream", "ready")
				_, _ = io.WriteString(w, event)
				w.(http.Flusher).Flush()
				select {
				case <-r.Context().Done():
					close(upstreamDone)
				case <-release:
				}
			})
			p, srv := newRawTestProxy(t, handler, &fakeAuth{})
			t.Cleanup(func() { close(release) })
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.Cleanup(cancel)
			outbound := mustNewRawRequest(t, srv.URL)
			type result struct {
				response *http.Response
				err      error
			}
			ready := make(chan result, 1)
			go func() {
				r, err := p.ProxyRequestStream(ctx, httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound})
				ready <- result{r, err}
			}()
			var response *http.Response
			select {
			case got := <-ready:
				require.NoError(t, got.err)
				response = got.response
			case <-time.After(2 * time.Second):
				t.Fatal("response headers waited for the open SSE body to finish")
			}
			require.NotNil(t, response)
			t.Cleanup(func() { _ = response.Body.Close() })
			assert.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, "ready", response.Header.Get("X-Stream"))
			assert.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))
			first := make([]byte, len(event))
			_, err := io.ReadFull(response.Body, first)
			require.NoError(t, err)
			assert.Equal(t, event, string(first))

			finished := make(chan error, 1)
			go func() {
				if action == "cancel" {
					cancel()
					_, err := io.ReadAll(response.Body)
					finished <- err
					return
				}
				finished <- response.Body.Close()
			}()
			select {
			case err := <-finished:
				if action == "cancel" {
					require.ErrorIs(t, err, context.Canceled)
				} else {
					require.NoError(t, err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("interrupting an open response blocked waiting for EOF")
			}
			select {
			case <-upstreamDone:
			case <-time.After(2 * time.Second):
				t.Fatal("upstream request remained open after interruption")
			}
		})
	}
}

// TestProxyRequestStream_AuthDoesNotMutateCaller verifies both credential
// precedence and the independent header/URL copies required by retry callers.
func TestProxyRequestStream_AuthDoesNotMutateCaller(t *testing.T) {
	auth := &streamAuth{resolveFn: func(int32) (auth_methods.AuthApplication, error) {
		return auth_methods.AuthApplication{
			Headers:     map[string]string{"Authorization": "Bearer resolved"},
			QueryParams: map[string]string{"api_key": "resolved"},
		}, nil
	}}
	var received *http.Request
	p := newStreamTestProxy(auth, func(r *http.Request) (*http.Response, error) {
		received = r.Clone(r.Context())
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	outbound := mustNewRawRequest(t, "https://example.test/path?api_key=caller&keep=a%2Fb")
	outbound.Header.Set("Authorization", "Bearer caller")
	outbound.Header.Add("X-Custom", "one")
	outbound.Header.Add("X-Custom", "two")
	originalURL, originalHeaders := outbound.URL.String(), outbound.Header.Clone()
	resp, err := p.ProxyRequestStream(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "Bearer resolved", received.Header.Get("Authorization"))
	assert.Equal(t, "resolved", received.URL.Query().Get("api_key"))
	assert.Equal(t, "a/b", received.URL.Query().Get("keep"))
	assert.Equal(t, []string{"one", "two"}, received.Header.Values("X-Custom"))
	assert.Equal(t, originalURL, outbound.URL.String())
	assert.Equal(t, originalHeaders, outbound.Header)
}

// TestProxyRequestStream_RecoversOnceWithFreshCredentials covers recovery even
// when the second attempt remains unauthorized; its live body belongs to us.
func TestProxyRequestStream_RecoversOnceWithFreshCredentials(t *testing.T) {
	for _, finalStatus := range []int{http.StatusOK, http.StatusUnauthorized} {
		t.Run(http.StatusText(finalStatus), func(t *testing.T) {
			auth := &streamAuth{fakeAuth: fakeAuth{maxRecover: 10}, resolveFn: func(n int32) (auth_methods.AuthApplication, error) {
				return auth_methods.AuthApplication{Headers: map[string]string{"Authorization": fmt.Sprintf("Bearer attempt-%d", n)}}, nil
			}}
			first := &streamBody{Reader: strings.NewReader("expired")}
			last := &streamBody{Reader: strings.NewReader("final")}
			var credentials []string
			p := newStreamTestProxy(auth, func(r *http.Request) (*http.Response, error) {
				credentials = append(credentials, r.Header.Get("Authorization"))
				if len(credentials) == 1 {
					return &http.Response{StatusCode: http.StatusUnauthorized, Body: first}, nil
				}
				return &http.Response{StatusCode: finalStatus, Body: last}, nil
			})
			resp, err := p.ProxyRequestStream(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: mustNewRawRequest(t, "https://example.test")})
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, finalStatus, resp.StatusCode)
			assert.Equal(t, []string{"Bearer attempt-1", "Bearer attempt-2"}, credentials)
			assert.Equal(t, int32(1), atomic.LoadInt32(&auth.recoverN))
			assert.True(t, first.closed.Load())
			assert.Zero(t, first.reads.Load(), "discarded 401 must close without draining")
			assert.False(t, last.closed.Load(), "final response belongs to the caller")
		})
	}
}

// TestProxyRequestStream_DoesNotReplayPost preserves the current bodyless-only
// policy even when net/http could reconstruct the POST body through GetBody.
func TestProxyRequestStream_DoesNotReplayPost(t *testing.T) {
	auth := &fakeAuth{maxRecover: 1}
	var payloads []string
	p := newStreamTestProxy(auth, func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, string(body))
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("denied"))}, nil
	})
	outbound, err := http.NewRequest(http.MethodPost, "https://example.test", strings.NewReader("payload"))
	require.NoError(t, err)
	require.NotNil(t, outbound.GetBody)
	resp, err := p.ProxyRequestStream(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, []string{"payload"}, payloads)
	assert.Zero(t, atomic.LoadInt32(&auth.recoverN))
}

// TestProxyRequestStream_RetryFailureNeverReturnsClosedResponse verifies both
// failure points after recovery instead of returning the discarded first 401.
func TestProxyRequestStream_RetryFailureNeverReturnsClosedResponse(t *testing.T) {
	for _, failure := range []string{"resolve", "transport"} {
		t.Run(failure, func(t *testing.T) {
			wantErr := errors.New("retry failed")
			auth := &streamAuth{fakeAuth: fakeAuth{maxRecover: 1}, resolveFn: func(n int32) (auth_methods.AuthApplication, error) {
				if n == 2 && failure == "resolve" {
					return auth_methods.AuthApplication{}, wantErr
				}
				return auth_methods.AuthApplication{}, nil
			}}
			first := &streamBody{Reader: strings.NewReader("expired")}
			calls := 0
			p := newStreamTestProxy(auth, func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return &http.Response{StatusCode: http.StatusUnauthorized, Body: first}, nil
				}
				return nil, wantErr
			})
			resp, err := p.ProxyRequestStream(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: mustNewRawRequest(t, "https://example.test")})
			require.ErrorIs(t, err, wantErr)
			assert.Nil(t, resp)
			assert.True(t, first.closed.Load())
			assert.Zero(t, first.reads.Load())
		})
	}
}

// TestProxyRequestStream_ClosesUnsentBody covers ownership on failures before
// http.Client takes responsibility for closing a streamed request body.
func TestProxyRequestStream_ClosesUnsentBody(t *testing.T) {
	for _, failure := range []string{"invalid URL", "auth"} {
		t.Run(failure, func(t *testing.T) {
			wantErr := errors.New("credentials unavailable")
			auth := &fakeAuth{failResolve: wantErr}
			calls := 0
			p := newStreamTestProxy(auth, func(*http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("unexpected transport call")
			})
			body := &streamBody{Reader: strings.NewReader("payload")}
			outbound, err := http.NewRequest(http.MethodPost, "https://example.test", body)
			require.NoError(t, err)
			if failure == "invalid URL" {
				outbound.URL = nil
			}
			resp, err := p.ProxyRequestStream(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound})
			require.Error(t, err)
			if failure == "auth" {
				assert.ErrorIs(t, err, wantErr)
			}
			assert.Nil(t, resp)
			assert.True(t, body.closed.Load())
			assert.Zero(t, body.reads.Load())
			assert.Zero(t, calls)
		})
	}
}

// TestProxyRequestStream_PreservesAttribution guards the factory boundary that
// supplies connection/actor policy, label projection, and request telemetry.
func TestProxyRequestStream_PreservesAttribution(t *testing.T) {
	factory := mockHttpf.NewMockF(gomock.NewController(t))
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root"}
	actor := &apauthcore.Actor{Id: apid.New(apid.PrefixActor), Namespace: "root"}
	ctx := apauthcore.NewAuthenticatedRequestAuth(actor).ContextWith(context.Background())
	labels := map[string]string{"purpose": "stream-test"}
	client := &http.Client{Transport: streamRoundTripper(func(r *http.Request) (*http.Response, error) {
		assert.Same(t, actor, apauthcore.ActorFromContext(r.Context()))
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})}
	factory.EXPECT().ForRequestType(httpf.RequestTypeProxy).Return(factory)
	factory.EXPECT().ForConnection(conn).Return(factory)
	factory.EXPECT().ForActor(actor).Return(factory)
	factory.EXPECT().ForLabels(labels).Return(factory)
	factory.EXPECT().NewHTTPClient().Return(client)
	p := New(factory, conn, &fakeAuth{}, nil)
	resp, err := p.ProxyRequestStream(ctx, httpf.RequestTypeProxy, &iface.RawProxyRequest{
		Outbound: mustNewRawRequest(t, "https://example.test"), Labels: labels,
	})
	require.NoError(t, err)
	defer resp.Body.Close()
}

// TestProxyRequestRaw_WriteFailureClosesWithoutDraining guards the adapter's
// cleanup when the upstream has produced one chunk but has not reached EOF.
func TestProxyRequestRaw_WriteFailureClosesWithoutDraining(t *testing.T) {
	wantErr := errors.New("downstream disconnected")
	body := &streamBody{}
	body.Reader = streamReaderFunc(func(p []byte) (int, error) {
		if body.reads.Load() > 1 {
			return 0, errors.New("unexpected read after downstream write failure")
		}
		return copy(p, "data: first\n\n"), nil
	})
	p := newStreamTestProxy(&fakeAuth{}, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})
	writer := &streamFailWriter{recordingResponseWriter: newRecordingResponseWriter(), err: wantErr}
	err := p.ProxyRequestRaw(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{
		Outbound: mustNewRawRequest(t, "https://example.test/stream"),
	}, writer)
	require.ErrorIs(t, err, wantErr)
	assert.True(t, body.closed.Load(), "failed downstream write must release the response")
	assert.Equal(t, int32(1), body.reads.Load(), "cleanup must not drain an open upstream")
}
