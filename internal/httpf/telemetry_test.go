package httpf

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// --- fixtures ---------------------------------------------------------------

type telemetryFixture struct {
	providers *aptelemetry.Providers
	spans     *tracetest.SpanRecorder
	reader    *sdkmetric.ManualReader
}

func newTelemetryFixture(t *testing.T) *telemetryFixture {
	t.Helper()
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})

	return &telemetryFixture{
		providers: &aptelemetry.Providers{
			Enabled:        true,
			TracerProvider: tp,
			MeterProvider:  mp,
			Propagator:     propagation.TraceContext{},
		},
		spans:  spans,
		reader: reader,
	}
}

func (f *telemetryFixture) readMetrics(t *testing.T) metricdata.ResourceMetrics {
	t.Helper()
	rm := metricdata.ResourceMetrics{}
	require.NoError(t, f.reader.Collect(context.Background(), &rm))
	return rm
}

// stubTransport returns a canned response on every RoundTrip call. The body
// is a fixed string so Content-Length is deterministic.
type stubTransport struct {
	status int
	body   string
	err    error
	last   *http.Request
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.last = req
	if s.err != nil {
		return nil, s.err
	}
	body := s.body
	resp := &http.Response{
		StatusCode:    s.status,
		Body:          io.NopCloser(strings.NewReader(body)),
		Header:        http.Header{},
		ContentLength: int64(len(body)),
		Request:       req,
	}
	return resp, nil
}

func newRequest(t *testing.T, method, url, body string) *http.Request {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	return req
}

func enabledPtr(b bool) *bool { return &b }

// --- label projection -------------------------------------------------------

func TestLabelProjector_RequestOverrideWinsInBothAllowlists(t *testing.T) {
	// Acceptance criterion #5 from #232: a request-supplied label override
	// MUST win over a connection-inherited value of the same key in both
	// projected span attrs AND metric dims. The projector itself reads from
	// a single map (httpf.RequestInfo.Labels) — the merging is upstream's
	// job. This test pins that contract by feeding the already-merged map
	// (with the request value present) and asserting the projection takes
	// that value, not whatever was originally on the connection.
	cap := 0
	cfg := &sconfig.TelemetryProxy{
		SpanAttributeLabels:     []string{"tenant_id", "env"},
		MetricDimensionLabels:   []string{"tenant_id", "env"},
		MetricDimensionValueCap: &cap,
	}
	p := newLabelProjector(cfg)

	// Effective set as produced by ForConnection then ForLabels — request
	// value (tenant_id=req-1) has already replaced the connection value.
	effective := map[string]string{
		"tenant_id": "req-1",
		"env":       "prod",
	}

	spanAttrs := p.spanAttrs(effective)
	require.Equal(t, "req-1", findAttr(t, spanAttrs, "tenant_id"))
	require.Equal(t, "prod", findAttr(t, spanAttrs, "env"))

	metricAttrs := p.metricDims(effective)
	require.Equal(t, "req-1", findAttr(t, metricAttrs, "tenant_id"))
	require.Equal(t, "prod", findAttr(t, metricAttrs, "env"))
}

func TestLabelProjector_AllowlistsAreIndependent(t *testing.T) {
	cfg := &sconfig.TelemetryProxy{
		SpanAttributeLabels:   []string{"tenant_id"},
		MetricDimensionLabels: []string{"env"},
	}
	p := newLabelProjector(cfg)

	labels := map[string]string{"tenant_id": "t1", "env": "prod"}

	spanAttrs := p.spanAttrs(labels)
	require.Len(t, spanAttrs, 1, "only span-allowlisted keys appear on spans")
	require.Equal(t, "t1", findAttr(t, spanAttrs, "tenant_id"))

	metricAttrs := p.metricDims(labels)
	require.Len(t, metricAttrs, 1, "only metric-allowlisted keys appear on metrics")
	require.Equal(t, "prod", findAttr(t, metricAttrs, "env"))
}

func TestLabelProjector_UnlistedKeysDropped(t *testing.T) {
	cfg := &sconfig.TelemetryProxy{
		SpanAttributeLabels:   []string{"tenant_id"},
		MetricDimensionLabels: []string{"tenant_id"},
	}
	p := newLabelProjector(cfg)

	labels := map[string]string{"tenant_id": "t1", "secret": "leak"}

	spanAttrs := p.spanAttrs(labels)
	require.Equal(t, "", findAttr(t, spanAttrs, "secret"), "unlisted key must never appear on spans")

	metricAttrs := p.metricDims(labels)
	require.Equal(t, "", findAttr(t, metricAttrs, "secret"), "unlisted key must never appear on metrics")
}

func TestLabelProjector_MissingKeyAbsentNotEmptyString(t *testing.T) {
	cfg := &sconfig.TelemetryProxy{
		SpanAttributeLabels:   []string{"tenant_id", "env"},
		MetricDimensionLabels: []string{"tenant_id", "env"},
	}
	p := newLabelProjector(cfg)

	labels := map[string]string{"tenant_id": "t1"} // env intentionally missing

	spanAttrs := p.spanAttrs(labels)
	require.Len(t, spanAttrs, 1, "missing key must produce no attribute, not an empty-string attribute")
	require.Equal(t, "t1", findAttr(t, spanAttrs, "tenant_id"))

	metricAttrs := p.metricDims(labels)
	require.Len(t, metricAttrs, 1)
	require.Equal(t, "t1", findAttr(t, metricAttrs, "tenant_id"))
}

func TestLabelProjector_ValueCapCollapsesToOther(t *testing.T) {
	cap := 2
	cfg := &sconfig.TelemetryProxy{
		MetricDimensionLabels:   []string{"tenant_id"},
		MetricDimensionValueCap: &cap,
	}
	p := newLabelProjector(cfg)

	// First two distinct values are admitted as-is.
	require.Equal(t, "a", findAttr(t, p.metricDims(map[string]string{"tenant_id": "a"}), "tenant_id"))
	require.Equal(t, "b", findAttr(t, p.metricDims(map[string]string{"tenant_id": "b"}), "tenant_id"))

	// Third distinct value collapses.
	require.Equal(t, LabelValueOther, findAttr(t, p.metricDims(map[string]string{"tenant_id": "c"}), "tenant_id"))

	// Previously-admitted values still pass through verbatim — cap only
	// applies to NEW distinct values.
	require.Equal(t, "a", findAttr(t, p.metricDims(map[string]string{"tenant_id": "a"}), "tenant_id"))
}

func TestLabelProjector_ValueCapAppliesPerKey(t *testing.T) {
	// Each key has its own cap budget.
	cap := 1
	cfg := &sconfig.TelemetryProxy{
		MetricDimensionLabels:   []string{"tenant_id", "env"},
		MetricDimensionValueCap: &cap,
	}
	p := newLabelProjector(cfg)

	one := p.metricDims(map[string]string{"tenant_id": "a", "env": "prod"})
	require.Equal(t, "a", findAttr(t, one, "tenant_id"))
	require.Equal(t, "prod", findAttr(t, one, "env"))

	two := p.metricDims(map[string]string{"tenant_id": "b", "env": "staging"})
	require.Equal(t, LabelValueOther, findAttr(t, two, "tenant_id"), "tenant_id cap is exhausted")
	require.Equal(t, LabelValueOther, findAttr(t, two, "env"), "env cap is exhausted")
}

func TestLabelProjector_NoCapWhenUnset(t *testing.T) {
	cfg := &sconfig.TelemetryProxy{
		MetricDimensionLabels: []string{"tenant_id"},
	}
	p := newLabelProjector(cfg)

	for _, v := range []string{"a", "b", "c", "d", "e"} {
		require.Equal(t, v, findAttr(t, p.metricDims(map[string]string{"tenant_id": v}), "tenant_id"),
			"with no cap configured, every value should pass through verbatim")
	}
}

// --- factory / roundtripper -------------------------------------------------

func TestNewTelemetryFactory_NoOpWhenProvidersDisabled(t *testing.T) {
	cfg := &sconfig.Telemetry{Enabled: enabledPtr(true)}
	f, err := NewTelemetryFactory(aptelemetry.NoopProviders(), cfg)
	require.NoError(t, err)
	require.Nil(t, f, "no-op providers must skip middleware registration entirely")
}

func TestNewTelemetryFactory_NoOpWhenAllSignalsOff(t *testing.T) {
	fx := newTelemetryFixture(t)
	off := false
	on := true
	cfg := &sconfig.Telemetry{
		Enabled: &on,
		Signals: &sconfig.TelemetrySignals{Traces: &off, Metrics: &off, Logs: &off},
	}
	f, err := NewTelemetryFactory(fx.providers, cfg)
	require.NoError(t, err)
	require.Nil(t, f)
}

func TestTelemetryRoundTripper_EmitsClientSpanAndMetrics(t *testing.T) {
	fx := newTelemetryFixture(t)
	cfg := &sconfig.Telemetry{
		Enabled: enabledPtr(true),
		Proxy: &sconfig.TelemetryProxy{
			SpanAttributeLabels:   []string{"tenant_id"},
			MetricDimensionLabels: []string{"tenant_id"},
		},
	}

	factory, err := NewTelemetryFactory(fx.providers, cfg)
	require.NoError(t, err)
	require.NotNil(t, factory)

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connectionID := apid.New(apid.PrefixConnection)

	ri := RequestInfo{
		Namespace:        "root.team-a",
		Type:             RequestTypeProxy,
		ConnectorId:      connectorID,
		ConnectorVersion: 7,
		ConnectionId:     connectionID,
		Labels:           map[string]string{"tenant_id": "t1"},
	}
	upstream := &stubTransport{status: http.StatusOK, body: "hello"}
	rt := factory.NewRoundTripper(ri, upstream)

	req := newRequest(t, http.MethodPost, "https://api.example.com/v1/things", `{"x":1}`)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Span content
	gotSpans := fx.spans.Ended()
	require.Len(t, gotSpans, 1)
	span := gotSpans[0]
	require.Equal(t, "HTTP POST", span.Name())

	attrs := attrMap(span.Attributes())
	require.Equal(t, http.MethodPost, attrs["http.request.method"])
	require.Equal(t, "api.example.com", attrs["server.address"])
	require.Equal(t, "/v1/things", attrs["url.path"])
	require.EqualValues(t, http.StatusOK, attrs["http.response.status_code"])
	require.Equal(t, string(RequestTypeProxy), attrs["authproxy.request.type"])
	require.Equal(t, "root.team-a", attrs["authproxy.namespace"])
	require.Equal(t, connectorID.String(), attrs["authproxy.connector_id"])
	require.EqualValues(t, 7, attrs["authproxy.connector_version"])
	require.Equal(t, connectionID.String(), attrs["authproxy.connection_id"])
	require.Equal(t, "t1", attrs["tenant_id"])
	require.Equal(t, codes.Unset, span.Status().Code, "2xx must leave span status Unset")

	// Metrics
	rm := fx.readMetrics(t)
	dur := findMetric(t, rm, "authproxy.client.request.duration")
	requireDimEqual(t, dur, "tenant_id", "t1")

	reqBytes := findMetric(t, rm, "authproxy.client.request.body.size")
	requireDimEqual(t, reqBytes, "tenant_id", "t1")

	respBytes := findMetric(t, rm, "authproxy.client.response.body.size")
	requireDimEqual(t, respBytes, "tenant_id", "t1")
}

func TestTelemetryRoundTripper_5xxMarksSpanErrored(t *testing.T) {
	fx := newTelemetryFixture(t)
	cfg := &sconfig.Telemetry{Enabled: enabledPtr(true)}

	factory, err := NewTelemetryFactory(fx.providers, cfg)
	require.NoError(t, err)

	upstream := &stubTransport{status: http.StatusInternalServerError, body: ""}
	rt := factory.NewRoundTripper(RequestInfo{Type: RequestTypeProxy}, upstream)

	req := newRequest(t, http.MethodGet, "https://api.example.com/v1/things", "")
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	gotSpans := fx.spans.Ended()
	require.Len(t, gotSpans, 1)
	require.Equal(t, codes.Error, gotSpans[0].Status().Code)
}

func TestTelemetryRoundTripper_4xxLeavesSpanUnset(t *testing.T) {
	fx := newTelemetryFixture(t)
	cfg := &sconfig.Telemetry{Enabled: enabledPtr(true)}

	factory, err := NewTelemetryFactory(fx.providers, cfg)
	require.NoError(t, err)

	upstream := &stubTransport{status: http.StatusForbidden, body: ""}
	rt := factory.NewRoundTripper(RequestInfo{Type: RequestTypeProxy}, upstream)

	req := newRequest(t, http.MethodGet, "https://api.example.com/v1/things", "")
	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	gotSpans := fx.spans.Ended()
	require.Len(t, gotSpans, 1)
	require.Equal(t, codes.Unset, gotSpans[0].Status().Code,
		"4xx client errors are not server errors per OTel HTTP semconv")
}

func TestTelemetryRoundTripper_TransportErrorRecordsErrorOnSpan(t *testing.T) {
	fx := newTelemetryFixture(t)
	cfg := &sconfig.Telemetry{Enabled: enabledPtr(true)}

	factory, err := NewTelemetryFactory(fx.providers, cfg)
	require.NoError(t, err)

	wantErr := errors.New("dial tcp: connection refused")
	upstream := &stubTransport{err: wantErr}
	rt := factory.NewRoundTripper(RequestInfo{Type: RequestTypeProxy}, upstream)

	req := newRequest(t, http.MethodGet, "https://api.example.com/v1/things", "")
	_, err = rt.RoundTrip(req)
	require.ErrorIs(t, err, wantErr)

	gotSpans := fx.spans.Ended()
	require.Len(t, gotSpans, 1)
	require.Equal(t, codes.Error, gotSpans[0].Status().Code)
	require.NotEmpty(t, gotSpans[0].Events(), "transport error must produce an exception event")
}

func TestTelemetryRoundTripper_ConnectorIdentityFromRequestInfo(t *testing.T) {
	// Even when no labels are configured, the connector/connection identity
	// from RequestInfo must appear on the span and on metric dimensions
	// (for breaking down RED by connector).
	fx := newTelemetryFixture(t)
	cfg := &sconfig.Telemetry{Enabled: enabledPtr(true)}

	factory, err := NewTelemetryFactory(fx.providers, cfg)
	require.NoError(t, err)

	connectorID := apid.New(apid.PrefixConnectorVersion)
	ri := RequestInfo{
		Type:        RequestTypeProxy,
		ConnectorId: connectorID,
	}
	upstream := &stubTransport{status: http.StatusOK, body: "ok"}
	rt := factory.NewRoundTripper(ri, upstream)

	req := newRequest(t, http.MethodGet, "https://api.example.com/v1/things", "")
	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	gotSpans := fx.spans.Ended()
	require.Len(t, gotSpans, 1)
	require.Equal(t, connectorID.String(), attrMap(gotSpans[0].Attributes())["authproxy.connector_id"])

	rm := fx.readMetrics(t)
	dur := findMetric(t, rm, "authproxy.client.request.duration")
	requireDimEqual(t, dur, "authproxy.connector_id", connectorID.String())
}

func TestTelemetryRoundTripper_NoBodySizeWhenContentLengthUnknown(t *testing.T) {
	fx := newTelemetryFixture(t)
	cfg := &sconfig.Telemetry{Enabled: enabledPtr(true)}

	factory, err := NewTelemetryFactory(fx.providers, cfg)
	require.NoError(t, err)

	// Body size = 0 (no body); response Content-Length will be 0.
	upstream := &stubTransport{status: http.StatusNoContent, body: ""}
	rt := factory.NewRoundTripper(RequestInfo{Type: RequestTypeProxy}, upstream)

	req := newRequest(t, http.MethodGet, "https://api.example.com/v1/things", "")
	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	rm := fx.readMetrics(t)
	// Duration is always emitted, but body-size histograms are skipped when
	// Content-Length isn't a positive value — avoiding noisy zero
	// observations.
	requireMetricEmitted(t, rm, "authproxy.client.request.duration")
	requireMetricNotEmitted(t, rm, "authproxy.client.request.body.size")
	requireMetricNotEmitted(t, rm, "authproxy.client.response.body.size")
}

// --- helpers ----------------------------------------------------------------

func attrMap(kvs []attribute.KeyValue) map[string]any {
	out := make(map[string]any, len(kvs))
	for _, kv := range kvs {
		out[string(kv.Key)] = kv.Value.AsInterface()
	}
	return out
}

func findAttr(t *testing.T, kvs []attribute.KeyValue, key string) string {
	t.Helper()
	for _, kv := range kvs {
		if string(kv.Key) == key {
			return kv.Value.AsString()
		}
	}
	return ""
}

func findMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	names := []string{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			names = append(names, m.Name)
		}
	}
	t.Fatalf("metric %q not emitted; got: %v", name, names)
	return metricdata.Metrics{}
}

func requireMetricEmitted(t *testing.T, rm metricdata.ResourceMetrics, name string) {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return
			}
		}
	}
	t.Fatalf("metric %q was expected but not emitted", name)
}

func requireMetricNotEmitted(t *testing.T, rm metricdata.ResourceMetrics, name string) {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				t.Fatalf("metric %q should not have been emitted but was", name)
			}
		}
	}
}

// requireDimEqual asserts that the metric m has at least one data point with
// an attribute matching key=value. Works for both float64 and int64
// histograms, which are the only metric kinds emitted here.
func requireDimEqual(t *testing.T, m metricdata.Metrics, key, value string) {
	t.Helper()
	switch agg := m.Data.(type) {
	case metricdata.Histogram[float64]:
		for _, dp := range agg.DataPoints {
			if hasAttrValue(dp.Attributes, key, value) {
				return
			}
		}
	case metricdata.Histogram[int64]:
		for _, dp := range agg.DataPoints {
			if hasAttrValue(dp.Attributes, key, value) {
				return
			}
		}
	default:
		t.Fatalf("metric %q has unsupported aggregation type %T", m.Name, m.Data)
	}
	t.Fatalf("metric %q has no data point with %s=%q", m.Name, key, value)
}

func hasAttrValue(set attribute.Set, key, value string) bool {
	v, ok := set.Value(attribute.Key(key))
	return ok && v.AsString() == value
}
