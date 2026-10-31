package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evidenceMiddleware can reject attempts outside the real factory's transport
// boundary, so a local response is distinguishable from a network response.
type evidenceMiddleware func(http.RoundTripper, *http.Request) (*http.Response, error)

// NewRoundTripper retains the factory-supplied transport for accepted requests.
func (m evidenceMiddleware) NewRoundTripper(_ httpf.RequestInfo, parent http.RoundTripper) http.RoundTripper {
	return streamRoundTripper(func(r *http.Request) (*http.Response, error) {
		if m != nil {
			return m(parent, r)
		}
		return parent.RoundTrip(r)
	})
}

// evidenceFactory uses production tracking and real sockets, with an optional
// local rejection standing in for rate-limiting middleware.
func evidenceFactory(m evidenceMiddleware) httpf.F {
	return httpf.CreateFactory(nil, nil, m, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// evidenceProxy supplies a connection and authenticator without requiring a
// database; all target HTTP traffic still passes through the production factory.
func evidenceProxy(f httpf.F, auth auth_methods.Authenticator) iface.Proxy {
	conn := &mockCore.Connection{Id: apid.New(apid.PrefixConnection), Namespace: "root/"}
	return New(f, conn, auth, nil)
}

// callEvidenceProxy exercises the public orchestrator adapters with replayable
// POST bodies. POST avoids net/http's transparent retries of idempotent requests
// on reused connections. Stream callers consume and close the response body.
func callEvidenceProxy(t *testing.T, ctx context.Context, p iface.Proxy, adapter, target string) (int, error) {
	t.Helper()
	if adapter == "wrapped" {
		resp, err := p.ProxyRequest(ctx, httpf.RequestTypeProxy, &iface.ProxyRequest{
			URL: target, Method: http.MethodPost, BodyRaw: []byte("payload"),
		})
		if resp == nil {
			return 0, err
		}
		return resp.StatusCode, err
	}
	outbound, err := http.NewRequest(http.MethodPost, target, strings.NewReader("payload"))
	require.NoError(t, err)
	req := &iface.RawProxyRequest{Outbound: outbound}
	if adapter == "raw" {
		w := httptest.NewRecorder()
		err := p.ProxyRequestRaw(ctx, httpf.RequestTypeProxy, req, w)
		return w.Code, err
	}
	resp, err := p.ProxyRequestStream(ctx, httpf.RequestTypeProxy, req)
	if resp == nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, readErr := io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, errors.Join(err, readErr)
}

// localEvidence429 constructs a complete middleware response without invoking
// its parent transport, matching the local rate-limiter's short-circuit shape.
func localEvidence429(r *http.Request) *http.Response {
	_ = r.Body.Close()
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(strings.NewReader("locally blocked")),
		Request:    r,
	}
}

// dropEvidenceResponse closes an accepted target request before sending any
// HTTP response. Receipt by this test server does not imply receipt of evidence
// by AuthProxy, which must still report uncertainty.
func dropEvidenceResponse(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		t.Errorf("hijack upstream connection: %v", err)
		return
	}
	_ = conn.Close()
}

// TestProxySendEvidence_Responses distinguishes real upstream status codes
// from a locally synthesized response, independently of HTTP success.
func TestProxySendEvidence_Responses(t *testing.T) {
	for _, adapter := range []string{"wrapped", "stream", "raw"} {
		for _, status := range []int{200, 401, 429, 503} {
			t.Run(fmt.Sprintf("%s/upstream-%d", adapter, status), func(t *testing.T) {
				var received atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					received.Add(1)
					w.WriteHeader(status)
					_, _ = io.WriteString(w, "upstream")
				}))
				defer server.Close()
				ctx, observation := httpf.ObserveSends(context.Background())
				p := evidenceProxy(evidenceFactory(nil), &fakeAuth{})

				// Any actual upstream response proves a send, including errors.
				got, err := callEvidenceProxy(t, ctx, p, adapter, server.URL)
				require.NoError(t, err)
				assert.Equal(t, status, got)
				assert.Equal(t, int32(1), received.Load())
				assert.Equal(t, httpf.SendStateSent, observation.State())
			})
		}
		t.Run(adapter+"/local-429", func(t *testing.T) {
			var received atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Add(1)
			}))
			defer server.Close()
			p := evidenceProxy(evidenceFactory(func(_ http.RoundTripper, r *http.Request) (*http.Response, error) {
				return localEvidence429(r), nil
			}), &fakeAuth{})
			ctx, observation := httpf.ObserveSends(context.Background())

			// The same status from middleware proves no target transport handoff.
			got, err := callEvidenceProxy(t, ctx, p, adapter, server.URL)
			require.NoError(t, err)
			assert.Equal(t, http.StatusTooManyRequests, got)
			assert.Zero(t, received.Load())
			assert.Equal(t, httpf.SendStateNotSent, observation.State())
		})
	}
}

// TestProxySendEvidence_ResponseFailures separates absence of a response from
// failures consuming a response that the transport already received.
func TestProxySendEvidence_ResponseFailures(t *testing.T) {
	for _, adapter := range []string{"wrapped", "stream", "raw"} {
		for _, scenario := range []string{"dropped", "truncated", "invalid-json"} {
			t.Run(adapter+"/"+scenario, func(t *testing.T) {
				var received atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					received.Add(1)
					switch scenario {
					case "dropped":
						dropEvidenceResponse(t, w)
					case "truncated":
						w.Header().Set("Content-Length", "100")
						_, _ = io.WriteString(w, "short")
					case "invalid-json":
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, "{invalid")
					}
				}))
				defer server.Close()
				ctx, observation := httpf.ObserveSends(context.Background())
				p := evidenceProxy(evidenceFactory(nil), &fakeAuth{})

				// JSON decoding belongs only to the wrapped adapter. Its existing
				// Bytes() conversion also discards truncation errors; streaming
				// adapters surface those errors. None may erase response evidence.
				_, err := callEvidenceProxy(t, ctx, p, adapter, server.URL)
				if (scenario == "invalid-json" && adapter != "wrapped") ||
					(scenario == "truncated" && adapter == "wrapped") {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				want := httpf.SendStateSent
				if scenario == "dropped" {
					want = httpf.SendStateMayHaveSent
				}
				assert.Equal(t, int32(1), received.Load())
				assert.Equal(t, want, observation.State())
			})
		}
	}
}

// tokenEvidenceAuth resolves credentials through a real HTTP client before
// rejecting them, testing that credential responses never count as target I/O.
type tokenEvidenceAuth struct {
	fakeAuth
	client *http.Client
	url    string
}

// Resolve obtains a token response using the caller's observation-bearing
// context, then returns a credential failure without attempting the target.
func (a *tokenEvidenceAuth) Resolve(ctx context.Context) (auth_methods.AuthApplication, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, nil)
	if err != nil {
		return auth_methods.AuthApplication{}, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return auth_methods.AuthApplication{}, err
	}
	_ = resp.Body.Close()
	return auth_methods.AuthApplication{}, errors.New("token response contains no credentials")
}

// TestProxySendEvidence_CredentialTraffic verifies a real token response does
// not promote an unsent target attempt to sent.
func TestProxySendEvidence_CredentialTraffic(t *testing.T) {
	for _, adapter := range []string{"wrapped", "stream", "raw"} {
		t.Run(adapter, func(t *testing.T) {
			var tokenCalls, targetCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					tokenCalls.Add(1)
				} else {
					targetCalls.Add(1)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			factory := evidenceFactory(nil)
			p := evidenceProxy(factory, &tokenEvidenceAuth{client: factory.NewHTTPClient(), url: server.URL + "/token"})
			ctx, observation := httpf.ObserveSends(context.Background())

			// Only credential HTTP completes; the proxy must fail before target I/O.
			_, err := callEvidenceProxy(t, ctx, p, adapter, server.URL+"/resource")
			require.ErrorContains(t, err, "no credentials")
			assert.Equal(t, int32(1), tokenCalls.Load())
			assert.Zero(t, targetCalls.Load())
			assert.Equal(t, httpf.SendStateNotSent, observation.State())
		})
	}
}

// TestProxySendEvidence_RetryRetainsFirstResponse pins cumulative evidence
// while preserving the adapters' existing difference on a failed second send.
func TestProxySendEvidence_RetryRetainsFirstResponse(t *testing.T) {
	for _, adapter := range []string{"wrapped", "stream", "raw"} {
		for _, failure := range []string{"local-429", "dropped"} {
			t.Run(adapter+"/"+failure, func(t *testing.T) {
				var received atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if received.Add(1) == 1 {
						w.WriteHeader(http.StatusUnauthorized)
						_, _ = io.WriteString(w, "expired")
						return
					}
					dropEvidenceResponse(t, w)
				}))
				defer server.Close()
				var attempts atomic.Int32
				factory := evidenceFactory(func(parent http.RoundTripper, r *http.Request) (*http.Response, error) {
					if attempts.Add(1) == 2 && failure == "local-429" {
						return localEvidence429(r), nil
					}
					return parent.RoundTrip(r)
				})
				auth := &fakeAuth{maxRecover: 1}
				p := evidenceProxy(factory, auth)
				ctx, observation := httpf.ObserveSends(context.Background())

				// The first 401 is definite evidence even if the retry is blocked
				// locally or its response disappears after target receipt.
				status, err := callEvidenceProxy(t, ctx, p, adapter, server.URL)
				if failure == "local-429" {
					require.NoError(t, err)
					assert.Equal(t, http.StatusTooManyRequests, status)
					assert.Equal(t, int32(1), received.Load())
				} else if adapter == "wrapped" {
					require.NoError(t, err)
					assert.Equal(t, http.StatusUnauthorized, status, "wrapped proxy preserves the original 401")
					assert.Equal(t, int32(2), received.Load())
				} else {
					require.Error(t, err, "streaming adapters return the retry failure")
					assert.Equal(t, int32(2), received.Load())
				}
				assert.Equal(t, int32(2), attempts.Load())
				assert.Equal(t, int32(1), atomic.LoadInt32(&auth.recoverN))
				assert.Equal(t, httpf.SendStateSent, observation.State())
			})
		}
	}
}

// TestProxySendEvidence_RedirectThenLocalFailure keeps the initial redirect's
// response evidence when middleware rejects the next hop before sending it.
func TestProxySendEvidence_RedirectThenLocalFailure(t *testing.T) {
	for _, adapter := range []string{"wrapped", "stream", "raw"} {
		t.Run(adapter, func(t *testing.T) {
			var received atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Add(1)
				http.Redirect(w, r, "/blocked", http.StatusFound)
			}))
			defer server.Close()
			factory := evidenceFactory(func(parent http.RoundTripper, r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/blocked" {
					return nil, errors.New("redirect denied locally")
				}
				return parent.RoundTrip(r)
			})
			ctx, observation := httpf.ObserveSends(context.Background())

			// A redirect is already an upstream response, even without a final
			// successful response or any network traffic to the redirect target.
			_, err := callEvidenceProxy(t, ctx, evidenceProxy(factory, &fakeAuth{}), adapter, server.URL)
			require.ErrorContains(t, err, "redirect denied locally")
			assert.Equal(t, int32(1), received.Load())
			assert.Equal(t, httpf.SendStateSent, observation.State())
		})
	}
}

// TestProxySendEvidence_ObservationIsolationAndPanic verifies fresh calls do
// not inherit earlier certainty and a panic cannot leave stale unsent evidence.
func TestProxySendEvidence_ObservationIsolationAndPanic(t *testing.T) {
	for _, adapter := range []string{"wrapped", "stream", "raw"} {
		t.Run(adapter, func(t *testing.T) {
			auth := &streamAuth{resolveFn: func(n int32) (auth_methods.AuthApplication, error) {
				if n == 2 {
					panic("credential callback panicked")
				}
				return auth_methods.AuthApplication{}, errors.New("credentials unavailable")
			}}
			p := evidenceProxy(evidenceFactory(nil), auth)
			ctx, earlier := httpf.ObserveSends(context.Background())

			// A normal credential rejection establishes known-unsent evidence.
			_, err := callEvidenceProxy(t, ctx, p, adapter, "http://127.0.0.1/unused")
			require.Error(t, err)
			require.Equal(t, httpf.SendStateNotSent, earlier.State())
			independentCtx, independent := httpf.ObserveSends(ctx)
			assert.Equal(t, httpf.SendStateMayHaveSent, independent.State())

			// Reusing an observation for another attempt must become conservative
			// if that attempt aborts without reporting its transport outcome.
			require.Panics(t, func() {
				_, _ = callEvidenceProxy(t, ctx, p, adapter, "http://127.0.0.1/unused")
			})
			assert.Equal(t, httpf.SendStateMayHaveSent, earlier.State())
			assert.Equal(t, httpf.SendStateMayHaveSent, independent.State())

			// A later explicit report belongs only to its fresh observation,
			// even when that context descends from an earlier observed call.
			_, err = callEvidenceProxy(t, independentCtx, p, adapter, "http://127.0.0.1/unused")
			require.Error(t, err)
			assert.Equal(t, httpf.SendStateNotSent, independent.State())
			assert.Equal(t, httpf.SendStateMayHaveSent, earlier.State())
		})
	}
}
