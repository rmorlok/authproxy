package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/core/iface"
	mockCore "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAccelerator records every EnqueueProbeNow call so tests can assert on
// "fired" vs "not fired" for status codes without standing up the full
// service + Redis + asynq stack.
type stubAccelerator struct {
	calls  atomic.Int32
	lastId atomic.Value // apid.ID
	retErr error
}

func (s *stubAccelerator) EnqueueProbeNow(ctx context.Context, id apid.ID) error {
	s.calls.Add(1)
	s.lastId.Store(id)
	return s.retErr
}

// staticHandler returns a handler that always responds with the given status.
// Body content is irrelevant — the proxy never inspects it for the
// acceleration decision (only the status code matters).
func staticHandler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	})
}

// newTestProxyWithAcceleration builds a proxy wired to a stub accelerator
// and a single-handler upstream. Mirrors newRawTestProxy but exposes the
// accelerator so each test can read its call count, and returns the
// upstream URL so the test can construct an outbound request that
// resolves correctly through the stub httpf's client.
func newTestProxyWithAcceleration(t *testing.T, h http.Handler, auth auth_methods.Authenticator) (iface.Proxy, *stubAccelerator, string) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	conn := &mockCore.Connection{
		Id:        apid.New(apid.PrefixConnection),
		Namespace: "root/",
	}
	acc := &stubAccelerator{}
	p := New(&stubHttpf{client: srv.Client()}, conn, auth, acc)
	return p, acc, srv.URL
}

// authNeverRecovers makes RecoverFrom401 always return ErrCannotRecover, so
// a 401 upstream lands as a 401 to the customer without any retry loop —
// the canonical api-key shape.
func authNeverRecovers() *fakeAuth {
	return &fakeAuth{maxRecover: 0}
}

// TestProxyRequestRaw_AccelerationOn401 — the central invariant of #257:
// an upstream 401 on a user-initiated request fires EnqueueProbeNow.
func TestProxyRequestRaw_AccelerationOn401(t *testing.T) {
	p, acc, srvURL := newTestProxyWithAcceleration(t, staticHandler(http.StatusUnauthorized), authNeverRecovers())

	req := mustNewRawRequest(t, srvURL+"/x")
	w := httptest.NewRecorder()
	require.NoError(t, p.ProxyRequestRaw(context.Background(), common.RequestTypeProxy, &iface.RawProxyRequest{Outbound: req}, w))

	assert.Equal(t, int32(1), acc.calls.Load(),
		"401 on a proxied user request must enqueue a probe-now task")
}

// TestProxyRequestRaw_AccelerationOn403 — 403 is the other status the
// issue spec calls out. Same observable: probe-now is enqueued.
func TestProxyRequestRaw_AccelerationOn403(t *testing.T) {
	p, acc, srvURL := newTestProxyWithAcceleration(t, staticHandler(http.StatusForbidden), authNeverRecovers())

	req := mustNewRawRequest(t, srvURL+"/x")
	w := httptest.NewRecorder()
	require.NoError(t, p.ProxyRequestRaw(context.Background(), common.RequestTypeProxy, &iface.RawProxyRequest{Outbound: req}, w))

	assert.Equal(t, int32(1), acc.calls.Load(),
		"403 on a proxied user request must enqueue a probe-now task")
}

// TestProxyRequestRaw_NoAccelerationOnSuccessOrOther4xx — only 401/403 are
// "credential failed" signals. 200 obviously doesn't fire; 400/404/429
// also must not fire — those are bad-request / not-found / rate-limit
// signals where a probe-now would be wasted work.
func TestProxyRequestRaw_NoAccelerationOnSuccessOrOther4xx(t *testing.T) {
	cases := []int{
		http.StatusOK,
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
	}
	for _, status := range cases {
		t.Run(http.StatusText(status), func(t *testing.T) {
			p, acc, srvURL := newTestProxyWithAcceleration(t, staticHandler(status), authNeverRecovers())

			req := mustNewRawRequest(t, srvURL+"/x")
			w := httptest.NewRecorder()
			require.NoError(t, p.ProxyRequestRaw(context.Background(), common.RequestTypeProxy, &iface.RawProxyRequest{Outbound: req}, w))

			assert.Equalf(t, int32(0), acc.calls.Load(),
				"status %d (%q) must not enqueue a probe-now task", status, http.StatusText(status))
		})
	}
}

// TestProxyRequestRaw_NoAccelerationOnProbeTraffic — when the probe itself
// receives a 401, the proxy must NOT re-enqueue probe-now. Even with
// throttling, the recursion is wasteful (the probe outcome already feeds
// the health signal directly).
func TestProxyRequestRaw_NoAccelerationOnProbeTraffic(t *testing.T) {
	p, acc, srvURL := newTestProxyWithAcceleration(t, staticHandler(http.StatusUnauthorized), authNeverRecovers())

	req := mustNewRawRequest(t, srvURL+"/x")
	w := httptest.NewRecorder()
	require.NoError(t, p.ProxyRequestRaw(context.Background(), common.RequestTypeProbe, &iface.RawProxyRequest{Outbound: req}, w))

	assert.Equal(t, int32(0), acc.calls.Load(),
		"probe-typed traffic must not trigger probe-now (would loop)")
}

// TestProxyRequestRaw_AccelerationToleratesAcceleratorErrors — the
// accelerator's return error must not surface to the customer. The
// upstream response (here: 401) is what they see; acceleration is a
// background signal and failing it must not bleed into the user-facing
// response path.
func TestProxyRequestRaw_AccelerationToleratesAcceleratorErrors(t *testing.T) {
	p, acc, srvURL := newTestProxyWithAcceleration(t, staticHandler(http.StatusUnauthorized), authNeverRecovers())
	acc.retErr = assertErr("accelerator down")

	req := mustNewRawRequest(t, srvURL+"/x")
	w := httptest.NewRecorder()
	require.NoError(t, p.ProxyRequestRaw(context.Background(), common.RequestTypeProxy, &iface.RawProxyRequest{Outbound: req}, w),
		"proxy response path must not surface an accelerator error to the customer")
	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"the customer-visible status is the upstream's, unchanged by the accelerator")
}

// TestProxyRequestRaw_NoAcceleratorIsAllowed — a connection constructed
// without an accelerator (e.g. legacy callers, or tests not exercising the
// 401-acceleration path) must still pass through 401/403 cleanly.
func TestProxyRequestRaw_NoAcceleratorIsAllowed(t *testing.T) {
	srv := httptest.NewServer(staticHandler(http.StatusUnauthorized))
	t.Cleanup(srv.Close)
	conn := &mockCore.Connection{
		Id:        apid.New(apid.PrefixConnection),
		Namespace: "root/",
	}
	p := New(&stubHttpf{client: srv.Client()}, conn, authNeverRecovers(), nil)

	req := mustNewRawRequest(t, srv.URL+"/x")
	w := httptest.NewRecorder()
	require.NoError(t, p.ProxyRequestRaw(context.Background(), common.RequestTypeProxy, &iface.RawProxyRequest{Outbound: req}, w))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// mustNewRawRequest builds an *http.Request for the raw-proxy path. The
// URL must point at the httptest.Server backing the stubHttpf — otherwise
// the request would try to actually dial that hostname.
func mustNewRawRequest(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	return req
}

// assertErr is a tiny error helper to avoid importing errors-pkg gymnastics
// for one synthetic value.
type assertErr string

func (e assertErr) Error() string { return string(e) }
