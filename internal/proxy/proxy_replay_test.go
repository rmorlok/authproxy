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

	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// replayAuth injects recovery outcomes while retaining credential-resolution
// counting from streamAuth and the remaining fake authenticator methods.
type replayAuth struct {
	streamAuth
	recoverFn func(context.Context) error
}

// RecoverFrom401 counts recovery attempts and optionally fails or cancels one.
func (a *replayAuth) RecoverFrom401(ctx context.Context) error {
	atomic.AddInt32(&a.recoverN, 1)
	if a.recoverFn != nil {
		return a.recoverFn(ctx)
	}
	return nil
}

// newReplayRequest creates an upload with tracked ownership and a fixed length;
// individual tests supply GetBody so each reconstruction is observable.
func newReplayRequest(t *testing.T) (*http.Request, *streamBody) {
	t.Helper()
	body := &streamBody{Reader: strings.NewReader("payload")}
	t.Cleanup(func() { _ = body.Close() })
	req, err := http.NewRequest(http.MethodPost, "https://example.test/upload?keep=original", body)
	require.NoError(t, err)
	req.ContentLength = int64(len("payload"))
	req.Header.Set("Authorization", "Bearer caller")
	return req, body
}

// TestProxyRequestStream_ReplaysIndependentBody exercises both public adapters
// while deliberately delaying the first transport's request-body closure.
func TestProxyRequestStream_ReplaysIndependentBody(t *testing.T) {
	for _, adapter := range []string{"stream", "raw"} {
		for _, finalStatus := range []int{http.StatusOK, http.StatusUnauthorized} {
			t.Run(fmt.Sprintf("%s/%d", adapter, finalStatus), func(t *testing.T) {
				outbound, original := newReplayRequest(t)
				fresh := &streamBody{Reader: strings.NewReader("payload")}
				t.Cleanup(func() { _ = fresh.Close() })
				firstResponse := &streamBody{Reader: strings.NewReader("expired")}
				getBodyCalls := 0
				outbound.GetBody = func() (io.ReadCloser, error) {
					getBodyCalls++
					assert.False(t, original.closed.Load(), "Do may return before transport closes its body")
					assert.True(t, firstResponse.closed.Load(), "discard the old response before preparing a retry")
					return fresh, nil
				}
				auth := &replayAuth{streamAuth: streamAuth{resolveFn: func(n int32) (auth_methods.AuthApplication, error) {
					if n == 2 {
						_ = original.Close()
						assert.False(t, fresh.closed.Load(), "closing the original must not close its replacement")
					}
					return auth_methods.AuthApplication{Headers: map[string]string{
						"Authorization": fmt.Sprintf("Bearer attempt-%d", n),
					}}, nil
				}}}
				var sentBodies []io.ReadCloser
				var payloads, credentials []string
				p := newStreamTestProxy(auth, func(r *http.Request) (*http.Response, error) {
					sentBodies = append(sentBodies, r.Body)
					payload, err := io.ReadAll(r.Body)
					if err != nil {
						_ = r.Body.Close()
						return nil, err
					}
					payloads = append(payloads, string(payload))
					credentials = append(credentials, r.Header.Get("Authorization"))
					assert.Equal(t, outbound.ContentLength, r.ContentLength)
					if len(sentBodies) == 1 {
						return &http.Response{StatusCode: http.StatusUnauthorized, Body: firstResponse}, nil
					}
					_ = r.Body.Close()
					return &http.Response{StatusCode: finalStatus, Body: io.NopCloser(strings.NewReader("final"))}, nil
				})
				req := &iface.RawProxyRequest{Outbound: outbound}
				if adapter == "raw" {
					writer := newRecordingResponseWriter()
					require.NoError(t, p.ProxyRequestRaw(context.Background(), httpf.RequestTypeProxy, req, writer))
					assert.Equal(t, finalStatus, writer.status())
					assert.Equal(t, "final", writer.snapshot())
				} else {
					resp, err := p.ProxyRequestStream(context.Background(), httpf.RequestTypeProxy, req)
					require.NoError(t, err)
					defer resp.Body.Close()
					assert.Equal(t, finalStatus, resp.StatusCode)
				}
				assert.Equal(t, []string{"payload", "payload"}, payloads)
				assert.Equal(t, []string{"Bearer attempt-1", "Bearer attempt-2"}, credentials)
				require.Len(t, sentBodies, 2)
				assert.Same(t, original, sentBodies[0])
				assert.Same(t, fresh, sentBodies[1])
				assert.Same(t, original, outbound.Body)
				assert.Equal(t, "Bearer caller", outbound.Header.Get("Authorization"))
				assert.Equal(t, "https://example.test/upload?keep=original", outbound.URL.String())
				assert.Equal(t, 1, getBodyCalls)
				assert.Equal(t, int32(1), atomic.LoadInt32(&auth.recoverN))
				assert.True(t, original.closed.Load())
				assert.True(t, fresh.closed.Load())
				assert.True(t, firstResponse.closed.Load())
				assert.Zero(t, firstResponse.reads.Load(), "discarded 401 must not be drained")
			})
		}
	}
}

// TestProxyRequestStream_ReplayFailuresCloseOwnedBodies pins ownership at every
// recovery boundary, including cancellation by a host callback before sending.
func TestProxyRequestStream_ReplayFailuresCloseOwnedBodies(t *testing.T) {
	for _, stage := range []string{
		"recover unavailable", "recover error", "cancel before recover", "cancel in recover", "cancel recover error",
		"factory error", "factory body and error", "factory nil", "cancel in factory", "resolve error", "cancel in resolve", "transport error",
	} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wantErr := errors.New(stage)
			outbound, original := newReplayRequest(t)
			fresh := &streamBody{Reader: strings.NewReader("payload")}
			firstResponse := &streamBody{Reader: strings.NewReader("expired")}
			factoryCalls, transportCalls := 0, 0
			outbound.GetBody = func() (io.ReadCloser, error) {
				factoryCalls++
				switch stage {
				case "factory error":
					return nil, wantErr
				case "factory body and error":
					return fresh, wantErr
				case "factory nil":
					return nil, nil
				case "cancel in factory":
					cancel()
				}
				return fresh, nil
			}
			auth := &replayAuth{
				streamAuth: streamAuth{resolveFn: func(n int32) (auth_methods.AuthApplication, error) {
					if n == 2 {
						if stage == "resolve error" {
							return auth_methods.AuthApplication{}, wantErr
						}
						if stage == "cancel in resolve" {
							cancel()
						}
					}
					return auth_methods.AuthApplication{}, nil
				}},
				recoverFn: func(context.Context) error {
					switch stage {
					case "recover unavailable":
						return auth_methods.ErrCannotRecover
					case "recover error":
						return wantErr
					case "cancel in recover", "cancel recover error":
						cancel()
						if stage == "cancel recover error" {
							return wantErr
						}
					}
					return nil
				},
			}
			p := newStreamTestProxy(auth, func(r *http.Request) (*http.Response, error) {
				transportCalls++
				_ = r.Body.Close()
				if transportCalls == 1 {
					if stage == "cancel before recover" {
						cancel()
					}
					return &http.Response{StatusCode: http.StatusUnauthorized, Body: firstResponse}, nil
				}
				return nil, wantErr
			})
			resp, err := p.ProxyRequestStream(ctx, httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound})
			if stage == "recover unavailable" || stage == "recover error" {
				require.NoError(t, err)
				require.NotNil(t, resp)
				defer resp.Body.Close()
				assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
				assert.False(t, firstResponse.closed.Load())
				data, readErr := io.ReadAll(resp.Body)
				require.NoError(t, readErr)
				assert.Equal(t, "expired", string(data))
			} else {
				require.Error(t, err)
				assert.Nil(t, resp)
				if strings.HasPrefix(stage, "cancel") {
					assert.ErrorIs(t, err, context.Canceled)
				} else if stage != "factory nil" {
					assert.ErrorIs(t, err, wantErr)
				}
				assert.True(t, firstResponse.closed.Load())
				assert.Zero(t, firstResponse.reads.Load())
			}
			wantFactoryCalls := 1
			if strings.Contains(stage, "recover") {
				wantFactoryCalls = 0
			}
			assert.Equal(t, wantFactoryCalls, factoryCalls)
			wantRecoveryCalls := int32(1)
			if stage == "cancel before recover" {
				wantRecoveryCalls = 0
			}
			assert.Equal(t, wantRecoveryCalls, atomic.LoadInt32(&auth.recoverN))
			wantTransportCalls := 1
			if stage == "transport error" {
				wantTransportCalls = 2
			}
			assert.Equal(t, wantTransportCalls, transportCalls)
			if factoryCalls > 0 && stage != "factory error" && stage != "factory nil" {
				assert.True(t, fresh.closed.Load(), "unused replay bodies must be released")
				if stage != "transport error" {
					assert.Zero(t, fresh.reads.Load(), "cleanup must not drain an unsent replay body")
				}
			}
			assert.True(t, original.closed.Load())
		})
	}
}

// TestProxyRequestStream_InitiallyCanceledClosesUnreadBody rejects work before
// credential resolution while honoring ownership of the supplied upload body.
func TestProxyRequestStream_InitiallyCanceledClosesUnreadBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outbound, original := newReplayRequest(t)
	factoryCalls, transportCalls := 0, 0
	outbound.GetBody = func() (io.ReadCloser, error) {
		factoryCalls++
		return nil, errors.New("unexpected reconstruction after cancellation")
	}
	auth := &fakeAuth{maxRecover: 1}
	p := newStreamTestProxy(auth, func(*http.Request) (*http.Response, error) {
		transportCalls++
		return nil, errors.New("unexpected transport after cancellation")
	})
	resp, err := p.ProxyRequestStream(ctx, httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound})
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, resp)
	assert.True(t, original.closed.Load())
	assert.Zero(t, original.reads.Load())
	assert.Zero(t, atomic.LoadInt32(&auth.resolveN))
	assert.Zero(t, atomic.LoadInt32(&auth.recoverN))
	assert.Zero(t, factoryCalls)
	assert.Zero(t, transportCalls)
}

// TestProxyRequestStream_GetBodyDoesNotEnableGeneralRetries excludes non-401
// statuses and network failures from the proxy's credential-recovery policy.
func TestProxyRequestStream_GetBodyDoesNotEnableGeneralRetries(t *testing.T) {
	for _, status := range []int{0, http.StatusOK, http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			outbound, _ := newReplayRequest(t)
			factoryCalls, transportCalls := 0, 0
			outbound.GetBody = func() (io.ReadCloser, error) {
				factoryCalls++
				return nil, errors.New("unexpected body reconstruction")
			}
			auth := &fakeAuth{maxRecover: 1}
			wantErr := errors.New("connection failed")
			p := newStreamTestProxy(auth, func(r *http.Request) (*http.Response, error) {
				transportCalls++
				_ = r.Body.Close()
				if status == 0 {
					return nil, wantErr
				}
				return &http.Response{StatusCode: status, Body: http.NoBody}, nil
			})
			resp, err := p.ProxyRequestStream(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound})
			if status == 0 {
				require.ErrorIs(t, err, wantErr)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				defer resp.Body.Close()
				assert.Equal(t, status, resp.StatusCode)
			}
			assert.Equal(t, 1, transportCalls)
			assert.Zero(t, factoryCalls)
			assert.Zero(t, atomic.LoadInt32(&auth.recoverN))
		})
	}
}

// TestProxyRequestStream_NoBodyNeedsNoFactory preserves bodyless recovery when
// the request uses net/http's sentinel and its irrelevant factory would fail.
func TestProxyRequestStream_NoBodyNeedsNoFactory(t *testing.T) {
	outbound := mustNewRawRequest(t, "https://example.test")
	outbound.Body = http.NoBody
	factoryCalls, transportCalls := 0, 0
	outbound.GetBody = func() (io.ReadCloser, error) {
		factoryCalls++
		return nil, errors.New("bodyless request must not need reconstruction")
	}
	p := newStreamTestProxy(&fakeAuth{maxRecover: 1}, func(*http.Request) (*http.Response, error) {
		transportCalls++
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: http.NoBody}, nil
	})
	resp, err := p.ProxyRequestStream(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, 2, transportCalls)
	assert.Zero(t, factoryCalls)
}

// replaySeekBody is seekable but deliberately provides no GetBody contract.
type replaySeekBody struct{ *strings.Reader }

// Close satisfies request-body ownership without making the reader replayable.
func (*replaySeekBody) Close() error { return nil }

// TestProxyRequestStream_DoesNotInferReplayability prevents length or seekability
// from enabling retries of unknown-length uploads and raw inbound streams.
func TestProxyRequestStream_DoesNotInferReplayability(t *testing.T) {
	for _, length := range []int64{0, -1} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			outbound, _ := newReplayRequest(t)
			outbound.Body = &replaySeekBody{Reader: strings.NewReader("payload")}
			outbound.ContentLength = length
			require.Nil(t, outbound.GetBody)
			auth := &fakeAuth{maxRecover: 1}
			calls := 0
			p := newStreamTestProxy(auth, func(r *http.Request) (*http.Response, error) {
				calls++
				_, err := io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()
				return &http.Response{StatusCode: http.StatusUnauthorized, Body: http.NoBody}, err
			})
			writer := newRecordingResponseWriter()
			require.NoError(t, p.ProxyRequestRaw(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound}, writer))
			assert.Equal(t, http.StatusUnauthorized, writer.status())
			assert.Equal(t, 1, calls)
			assert.Zero(t, atomic.LoadInt32(&auth.recoverN))
		})
	}
}
