package proxy

import (
	"bufio"
	"context"
	"io"
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
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gentleman "gopkg.in/h2non/gentleman.v2"
)

// fakeAuth is a hand-written Authenticator that records call sequencing
// without pulling in mockgen for this single fixture. RecoverFrom401 is
// allowed exactly maxRecover times before returning ErrCannotRecover.
type fakeAuth struct {
	headers     map[string]string
	maxRecover  int
	resolveN    int32
	recoverN    int32
	failResolve error
}

func (a *fakeAuth) Resolve(ctx context.Context) (auth_methods.AuthApplication, error) {
	atomic.AddInt32(&a.resolveN, 1)
	if a.failResolve != nil {
		return auth_methods.AuthApplication{}, a.failResolve
	}
	hs := make(map[string]string, len(a.headers))
	for k, v := range a.headers {
		hs[k] = v
	}
	return auth_methods.AuthApplication{Headers: hs}, nil
}

func (a *fakeAuth) RecoverFrom401(ctx context.Context) error {
	if atomic.LoadInt32(&a.recoverN) >= int32(a.maxRecover) {
		return auth_methods.ErrCannotRecover
	}
	atomic.AddInt32(&a.recoverN, 1)
	return nil
}

func (a *fakeAuth) SupportsRevoke() bool          { return false }
func (a *fakeAuth) Revoke(ctx context.Context) error { return nil }

// stubHttpf returns a single supplied http.Client through NewHTTPClient,
// ignoring all chain methods. Sufficient for testing the orchestrator —
// the real chain semantics are covered by httpf's own tests.
type stubHttpf struct {
	client *http.Client
}

func (s *stubHttpf) New() *gentleman.Client { return gentleman.New() }
func (s *stubHttpf) NewHTTPClient() *http.Client {
	return s.client
}
func (s *stubHttpf) ForRequestInfo(httpf.RequestInfo) httpf.F      { return s }
func (s *stubHttpf) ForRequestType(httpf.RequestType) httpf.F      { return s }
func (s *stubHttpf) ForConnectorVersion(httpf.ConnectorVersion) httpf.F {
	return s
}
func (s *stubHttpf) ForConnection(httpf.Connection) httpf.F        { return s }
func (s *stubHttpf) ForActor(httpf.Actor) httpf.F                  { return s }
func (s *stubHttpf) ForLabels(map[string]string) httpf.F           { return s }

func newRawTestProxy(t *testing.T, h http.Handler, auth *fakeAuth) (iface.Proxy, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	conn := &mockCore.Connection{
		Id:        apid.New(apid.PrefixConnection),
		Namespace: "root/",
	}
	p := New(&stubHttpf{client: srv.Client()}, conn, auth, nil)
	return p, srv
}

// TestProxyRequestRaw_StreamsChunkedBytes — the critical streaming
// invariant: an upstream that writes a chunk, flushes, pauses, writes
// another chunk, must surface as two separately-observable arrivals on
// the downstream writer. A regression where the orchestrator buffered
// the whole body before flushing would break SSE consumers, LLM token
// streaming, and large S3 downloads.
func TestProxyRequestRaw_StreamsChunkedBytes(t *testing.T) {
	const chunkA = "event: alpha\ndata: 1\n\n"
	const chunkB = "event: beta\ndata: 2\n\n"

	releaseB := make(chan struct{})
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(chunkA))
		w.(http.Flusher).Flush()
		<-releaseB
		_, _ = w.Write([]byte(chunkB))
		w.(http.Flusher).Flush()
	})
	auth := &fakeAuth{headers: map[string]string{"Authorization": "Bearer test"}}
	p, srv := newRawTestProxy(t, upstream, auth)

	outbound, err := http.NewRequest(http.MethodGet, srv.URL+"/stream", nil)
	require.NoError(t, err)

	rec := newRecordingResponseWriter()
	done := make(chan error, 1)
	go func() {
		done <- p.ProxyRequestRaw(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound}, rec)
	}()

	// First chunk must arrive before we release the second — proves no
	// end-of-response buffering.
	require.Eventually(t, func() bool {
		return rec.containsString(chunkA)
	}, 2*time.Second, 10*time.Millisecond, "first chunk did not arrive before second was released")
	assert.NotContains(t, rec.snapshot(), chunkB, "second chunk arrived before being released — server-side serialization broken")

	close(releaseB)
	require.NoError(t, <-done)
	assert.Equal(t, chunkA+chunkB, rec.snapshot())
	assert.GreaterOrEqual(t, rec.flushCount(), 2, "must flush at least once per chunk")
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
}

// TestProxyRequestRaw_AttachesCredentialHeader — auth.Resolve's headers
// must end up on the outbound request before it hits the upstream, and
// they must overwrite any caller-supplied value (credential always wins).
func TestProxyRequestRaw_AttachesCredentialHeader(t *testing.T) {
	var receivedAuth string
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})
	auth := &fakeAuth{headers: map[string]string{"Authorization": "Bearer correct"}}
	p, srv := newRawTestProxy(t, upstream, auth)

	outbound, err := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
	require.NoError(t, err)
	outbound.Header.Set("Authorization", "Bearer should-be-overwritten")

	rec := newRecordingResponseWriter()
	require.NoError(t, p.ProxyRequestRaw(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound}, rec))
	assert.Equal(t, "Bearer correct", receivedAuth)
}

// TestProxyRequestRaw_401TriggersRecoverAndRetry — for bodyless
// requests (the case where retry is safe) a 401 must call
// RecoverFrom401, re-Resolve, and replay the request. A regression here
// would mean expired-token retries stop working on the streaming path.
func TestProxyRequestRaw_401TriggersRecoverAndRetry(t *testing.T) {
	var calls int32
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	auth := &fakeAuth{
		headers:    map[string]string{"Authorization": "Bearer t"},
		maxRecover: 1,
	}
	p, srv := newRawTestProxy(t, upstream, auth)

	outbound, err := http.NewRequest(http.MethodGet, srv.URL+"/y", nil)
	require.NoError(t, err)

	rec := newRecordingResponseWriter()
	require.NoError(t, p.ProxyRequestRaw(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound}, rec))

	assert.Equal(t, int32(2), atomic.LoadInt32(&calls), "expected exactly one retry after 401")
	assert.Equal(t, int32(1), atomic.LoadInt32(&auth.recoverN), "RecoverFrom401 must be called exactly once")
	assert.Equal(t, int32(2), atomic.LoadInt32(&auth.resolveN), "Resolve must be called once per attempt")
	assert.Equal(t, http.StatusOK, rec.status())
	assert.Equal(t, "ok", rec.snapshot())
}

// TestProxyRequestRaw_401SurfacedWhenInboundBodyWasConsumed — when the
// inbound body is non-empty and not rewindable, the orchestrator must
// NOT retry. Retrying with an exhausted reader would send an empty
// body and look success-y to the upstream, masking the real 401 from
// the caller's perspective.
func TestProxyRequestRaw_401SurfacedWhenInboundBodyWasConsumed(t *testing.T) {
	var calls int32
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		// Drain to simulate the body being consumed.
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusUnauthorized)
	})
	auth := &fakeAuth{headers: map[string]string{"Authorization": "Bearer t"}, maxRecover: 1}
	p, srv := newRawTestProxy(t, upstream, auth)

	body := io.NopCloser(strings.NewReader("payload"))
	outbound, err := http.NewRequest(http.MethodPost, srv.URL+"/z", body)
	require.NoError(t, err)

	rec := newRecordingResponseWriter()
	require.NoError(t, p.ProxyRequestRaw(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound}, rec))

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "no retry when inbound body was already consumed")
	assert.Equal(t, int32(0), atomic.LoadInt32(&auth.recoverN), "RecoverFrom401 must not be called when retry is unsafe")
	assert.Equal(t, http.StatusUnauthorized, rec.status())
}

// TestProxyRequestRaw_HopByHopResponseHeadersStripped — the orchestrator
// is the trust boundary for what the upstream is allowed to send back
// to the caller's TCP connection. Hop-by-hop headers must be dropped
// per RFC 7230 §6.1; otherwise an upstream Transfer-Encoding announcement
// would conflict with the downstream's actual framing.
func TestProxyRequestRaw_HopByHopResponseHeadersStripped(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "X-Hop-Custom")
		w.Header().Set("X-Hop-Custom", "leak")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Keep-Alive", "timeout=5")
		w.Header().Set("X-Allowed", "passes-through")
		w.WriteHeader(http.StatusOK)
	})
	auth := &fakeAuth{headers: map[string]string{"Authorization": "Bearer t"}}
	p, srv := newRawTestProxy(t, upstream, auth)

	outbound, err := http.NewRequest(http.MethodGet, srv.URL+"/h", nil)
	require.NoError(t, err)

	rec := newRecordingResponseWriter()
	require.NoError(t, p.ProxyRequestRaw(context.Background(), httpf.RequestTypeProxy, &iface.RawProxyRequest{Outbound: outbound}, rec))

	assert.Empty(t, rec.Header().Get("Connection"), "hop-by-hop must not be forwarded")
	assert.Empty(t, rec.Header().Get("X-Hop-Custom"), "header named by Connection: must not be forwarded")
	assert.Empty(t, rec.Header().Get("Transfer-Encoding"), "hop-by-hop must not be forwarded")
	assert.Empty(t, rec.Header().Get("Keep-Alive"), "hop-by-hop must not be forwarded")
	assert.Equal(t, "passes-through", rec.Header().Get("X-Allowed"))
}

// recordingResponseWriter captures everything sent to the writer so the
// tests can assert on body bytes, flush counts, status, and headers
// without spinning up a second httptest.Server.
type recordingResponseWriter struct {
	mu      sync.Mutex
	hdr     http.Header
	buf     strings.Builder
	flushes int
	st      int
}

func newRecordingResponseWriter() *recordingResponseWriter {
	return &recordingResponseWriter{hdr: http.Header{}, st: 200}
}

func (r *recordingResponseWriter) Header() http.Header { return r.hdr }

func (r *recordingResponseWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

func (r *recordingResponseWriter) WriteHeader(s int) {
	r.mu.Lock()
	r.st = s
	r.mu.Unlock()
}

func (r *recordingResponseWriter) Flush() {
	r.mu.Lock()
	r.flushes++
	r.mu.Unlock()
}

func (r *recordingResponseWriter) status() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.st
}

func (r *recordingResponseWriter) snapshot() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

func (r *recordingResponseWriter) containsString(s string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Contains(r.buf.String(), s)
}

func (r *recordingResponseWriter) flushCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.flushes
}

// silence unused import lint if bufio gets dropped during edits
var _ = bufio.NewReader
