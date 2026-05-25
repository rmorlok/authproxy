package core

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	mockH "github.com/rmorlok/authproxy/internal/httpf/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	genmock "gopkg.in/h2non/gentleman-mock.v2"
)

// stubProxy is a fake iface.Proxy used by probe_http tests. It records the
// last ProxyRequest call and returns a programmable response/error pair.
type stubProxy struct {
	resp  *iface.ProxyResponse
	err   error
	calls []*iface.ProxyRequest
}

func (s *stubProxy) ProxyRequest(ctx context.Context, _ httpf.RequestType, req *iface.ProxyRequest) (*iface.ProxyResponse, error) {
	s.calls = append(s.calls, req)
	return s.resp, s.err
}

func (s *stubProxy) ProxyRequestRaw(ctx context.Context, _ httpf.RequestType, _ *iface.RawProxyRequest, _ http.ResponseWriter) error {
	panic("ProxyRequestRaw should not be invoked by probe_http")
}

// newProbeTestConnection builds a connection wired to a stubProxy so the
// probe code under test exercises the real iface.Proxy contract without
// going through resolveAuthenticator (which would require a fully wired
// auth-method factory).
func newProbeTestConnection(t *testing.T, ctrl *gomock.Controller, def cschema.Connector) (*connection, *stubProxy) {
	t.Helper()
	s, _, _, _, _, _ := FullMockService(t, ctrl)
	cv := NewTestConnectorVersion(def)

	logger := slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	conn := &connection{
		Connection: database.Connection{
			Id:               apid.New(apid.PrefixConnection),
			Namespace:        "root",
			State:            database.ConnectionStateConfigured,
			HealthState:      database.ConnectionHealthStateHealthy,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      s,
		cv:     cv,
		logger: logger,
	}

	proxy := &stubProxy{}
	// Consume the sync.Once so getProxyImpl returns the stub on the first
	// call rather than running the real auth-resolution path.
	conn.proxyImpl = proxy
	conn.proxyImplOnce.Do(func() {})

	return conn, proxy
}

// testWriter routes log lines into t.Log so they appear with the failing
// test rather than on stderr. The Write signature satisfies io.Writer.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}

func TestProbeHttp_ProxyHttp_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	probeCfg := &cschema.Probe{
		Id: "ping",
		ProxyHttp: &cschema.ProbeHttp{
			Method: "GET",
			URL:    "https://example.com/health",
		},
	}
	conn, proxy := newProbeTestConnection(t, ctrl, cschema.Connector{Probes: []cschema.Probe{*probeCfg}})
	proxy.resp = &iface.ProxyResponse{StatusCode: 200}

	probe := NewProbe(probeCfg, conn.s, conn.cv, conn)
	outcome, err := probe.Invoke(context.Background())

	require.NoError(t, err)
	assert.Equal(t, ProbeOutcomeSuccess, outcome)
	require.Len(t, proxy.calls, 1, "exactly one proxied probe request")
	assert.Equal(t, "GET", proxy.calls[0].Method)
	assert.Equal(t, "https://example.com/health", proxy.calls[0].URL)
}

// TestProbeHttp_ProxyHttp_Non2xxIsFailure covers the regression this PR
// fixed: an upstream 401 or 500 must register as a probe failure, not a
// silent success — without that, the probe-driven health signal would
// never flip a connection unhealthy on a credential rejection.
func TestProbeHttp_ProxyHttp_Non2xxIsFailure(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"401 unauthorized", http.StatusUnauthorized},
		{"403 forbidden", http.StatusForbidden},
		{"404 not found", http.StatusNotFound},
		{"500 internal server error", http.StatusInternalServerError},
		{"503 service unavailable", http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			probeCfg := &cschema.Probe{
				Id: "ping",
				ProxyHttp: &cschema.ProbeHttp{
					Method: "GET",
					URL:    "https://example.com/health",
				},
			}
			conn, proxy := newProbeTestConnection(t, ctrl, cschema.Connector{Probes: []cschema.Probe{*probeCfg}})
			proxy.resp = &iface.ProxyResponse{StatusCode: tc.status}

			probe := NewProbe(probeCfg, conn.s, conn.cv, conn)
			outcome, err := probe.Invoke(context.Background())

			require.Errorf(t, err, "status %d must surface as a probe error", tc.status)
			assert.Equal(t, ProbeOutcomeError, outcome)
			// The status code should be embedded in the error so operators
			// can read it from the recorded outcome row without digging
			// into upstream logs.
			assert.Contains(t, err.Error(), "status",
				"recorded probe error should mention status; got %q", err.Error())
		})
	}
}

func TestProbeHttp_ProxyHttp_2xxRangeIsSuccess(t *testing.T) {
	cases := []int{200, 201, 204, 299}
	for _, status := range cases {
		t.Run(http.StatusText(status), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			probeCfg := &cschema.Probe{
				Id: "ping",
				ProxyHttp: &cschema.ProbeHttp{
					Method: "GET",
					URL:    "https://example.com/health",
				},
			}
			conn, proxy := newProbeTestConnection(t, ctrl, cschema.Connector{Probes: []cschema.Probe{*probeCfg}})
			proxy.resp = &iface.ProxyResponse{StatusCode: status}

			probe := NewProbe(probeCfg, conn.s, conn.cv, conn)
			outcome, err := probe.Invoke(context.Background())

			require.NoErrorf(t, err, "status %d must be treated as success", status)
			assert.Equal(t, ProbeOutcomeSuccess, outcome)
		})
	}
}

func TestProbeHttp_ProxyHttp_TransportErrorIsFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	probeCfg := &cschema.Probe{
		Id: "ping",
		ProxyHttp: &cschema.ProbeHttp{
			Method: "GET",
			URL:    "https://example.com/health",
		},
	}
	conn, proxy := newProbeTestConnection(t, ctrl, cschema.Connector{Probes: []cschema.Probe{*probeCfg}})
	proxy.err = errors.New("dial tcp: connection refused")

	probe := NewProbe(probeCfg, conn.s, conn.cv, conn)
	outcome, err := probe.Invoke(context.Background())

	require.Error(t, err)
	assert.Equal(t, ProbeOutcomeError, outcome)
	assert.Contains(t, err.Error(), "connection refused")
}

// TestProbeHttp_RawHttp_Success covers the non-proxied branch — the probe
// hits the upstream directly via httpf without going through the proxy
// orchestrator. gentleman-mock intercepts the request and replies 200.
func TestProbeHttp_RawHttp_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h := mockH.NewFactoryWithMockingClient(ctrl)
	s, _, _, _, _, _ := FullMockService(t, ctrl)
	s.httpf = h
	cv := NewTestConnectorVersion(cschema.Connector{})
	logger := slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	conn := &connection{
		Connection: database.Connection{
			Id:               apid.New(apid.PrefixConnection),
			Namespace:        "root",
			State:            database.ConnectionStateConfigured,
			HealthState:      database.ConnectionHealthStateHealthy,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      s,
		cv:     cv,
		logger: logger,
	}

	genmock.New("https://raw.example.com").Get("/health").Reply(200)

	probeCfg := &cschema.Probe{
		Id: "ping-raw",
		Http: &cschema.ProbeHttp{
			Method: "GET",
			URL:    "https://raw.example.com/health",
		},
	}

	probe := NewProbe(probeCfg, s, cv, conn)
	outcome, err := probe.Invoke(context.Background())

	require.NoError(t, err)
	assert.Equal(t, ProbeOutcomeSuccess, outcome)
}

func TestProbeHttp_RawHttp_Non2xxIsFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h := mockH.NewFactoryWithMockingClient(ctrl)
	s, _, _, _, _, _ := FullMockService(t, ctrl)
	s.httpf = h
	cv := NewTestConnectorVersion(cschema.Connector{})
	logger := slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	conn := &connection{
		Connection: database.Connection{
			Id:               apid.New(apid.PrefixConnection),
			Namespace:        "root",
			State:            database.ConnectionStateConfigured,
			HealthState:      database.ConnectionHealthStateHealthy,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      s,
		cv:     cv,
		logger: logger,
	}

	genmock.New("https://raw.example.com").Get("/health").Reply(500)

	probeCfg := &cschema.Probe{
		Id: "ping-raw",
		Http: &cschema.ProbeHttp{
			Method: "GET",
			URL:    "https://raw.example.com/health",
		},
	}

	probe := NewProbe(probeCfg, s, cv, conn)
	outcome, err := probe.Invoke(context.Background())

	require.Error(t, err, "raw HTTP probe must treat 500 as failure")
	assert.Equal(t, ProbeOutcomeError, outcome)
	assert.Contains(t, err.Error(), "status")
}
