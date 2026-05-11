package httpf

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// TestPropagationDecisionMatrix pins the full {global default, per-connector
// override} matrix from the spec in #233. Outbound W3C trace context
// injection is opt-in: it must NOT happen by default, must inject when the
// global default is true, and the per-connector override always wins.
func TestPropagationDecisionMatrix(t *testing.T) {
	cases := []struct {
		name              string
		globalDefault     bool
		perConnector      *bool
		expectInjected    bool
	}{
		{
			name:           "global default false, no override → no injection (default opt-in case)",
			globalDefault:  false,
			perConnector:   nil,
			expectInjected: false,
		},
		{
			name:           "global default true, no override → injected",
			globalDefault:  true,
			perConnector:   nil,
			expectInjected: true,
		},
		{
			name:           "global default false, per-connector true → injected (opt-in this connector)",
			globalDefault:  false,
			perConnector:   ptrBool(true),
			expectInjected: true,
		},
		{
			name:           "global default true, per-connector false → no injection (opt-out this connector)",
			globalDefault:  true,
			perConnector:   ptrBool(false),
			expectInjected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newTelemetryFixture(t)
			on := true
			cfg := &sconfig.Telemetry{
				Enabled: &on,
				Propagation: &sconfig.TelemetryPropagation{
					InjectOutboundDefault: &tc.globalDefault,
				},
			}

			factory, err := NewTelemetryFactory(fx.providers, cfg)
			require.NoError(t, err)
			require.NotNil(t, factory)

			ri := RequestInfo{
				Type:                  RequestTypeProxy,
				PropagateTraceContext: tc.perConnector,
			}
			upstream := &stubTransport{status: http.StatusOK, body: "ok"}
			rt := factory.NewRoundTripper(ri, upstream)

			req := newRequest(t, http.MethodGet, "https://api.example.com/v1/things", "")
			_, err = rt.RoundTrip(req)
			require.NoError(t, err)

			tp := upstream.last.Header.Get("traceparent")
			if tc.expectInjected {
				require.NotEmpty(t, tp, "traceparent must be injected when the resolved decision is true")
			} else {
				require.Empty(t, tp, "traceparent must NOT be injected when the resolved decision is false")
			}
		})
	}
}

// TestPropagationInheritsIncomingParent verifies the inbound side: when a
// request arrives at AuthProxy with a traceparent already present in the
// context (extracted by Gin middleware), the outbound span emitted by the
// httpf telemetry roundtripper participates in that trace regardless of the
// outbound injection setting. This pins the spec line:
//
//	"Inbound services accept incoming W3C headers via parent-based sampler
//	regardless of the outbound setting."
func TestPropagationInheritsIncomingParent(t *testing.T) {
	fx := newTelemetryFixture(t)
	on := true
	off := false
	// Outbound injection OFF — we should still inherit the inbound parent.
	cfg := &sconfig.Telemetry{
		Enabled:     &on,
		Propagation: &sconfig.TelemetryPropagation{InjectOutboundDefault: &off},
	}

	factory, err := NewTelemetryFactory(fx.providers, cfg)
	require.NoError(t, err)

	// Simulate an inbound traceparent arriving on the request and being
	// extracted into the context — the same shape ginhttp / our gin
	// middleware would produce.
	inbound := http.Header{}
	inbound.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	parentCtx := propagation.TraceContext{}.Extract(req(t).Context(), propagation.HeaderCarrier(inbound))

	rt := factory.NewRoundTripper(RequestInfo{Type: RequestTypeProxy}, &stubTransport{status: http.StatusOK, body: "ok"})
	r := newRequest(t, http.MethodGet, "https://api.example.com/v1/things", "")
	r = r.WithContext(parentCtx)
	_, err = rt.RoundTrip(r)
	require.NoError(t, err)

	got := fx.spans.Ended()
	require.Len(t, got, 1)
	// The outbound span's trace id must match the inbound traceparent's
	// trace id, proving the parent was honoured.
	require.Equal(t, "0af7651916cd43dd8448eb211c80319c", got[0].SpanContext().TraceID().String())
}

func req(t *testing.T) *http.Request {
	t.Helper()
	r, err := http.NewRequest(http.MethodGet, "https://example/", nil)
	require.NoError(t, err)
	return r
}

func ptrBool(b bool) *bool { return &b }
