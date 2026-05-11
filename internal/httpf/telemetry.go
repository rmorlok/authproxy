package httpf

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// telemetryInstrumentationName is the instrumentation scope reported on the
// emitted spans and metrics. Useful for filtering by source in dashboards.
const telemetryInstrumentationName = "github.com/rmorlok/authproxy/internal/httpf"

// LabelValueOther is the placeholder substituted for label values that exceed
// the configured metric_dimension_value_cap. Bounds the cardinality of metric
// streams under runaway label-value churn.
const LabelValueOther = "other"

// NewTelemetryFactory returns a RoundTripperFactory that emits an OTel client
// span per outbound request plus the proxy duration / bytes-in / bytes-out
// histograms specified by #232. Labels carried on RequestInfo are projected
// onto telemetry per the two independent allowlists in
// telemetry.proxy.{span_attribute_labels,metric_dimension_labels}; missing
// keys produce no attribute / dimension at all.
//
// When the providers are nil / in no-op mode, or when both trace and metric
// signals are disabled in cfg, NewTelemetryFactory returns (nil, nil) so the
// caller can skip adding the middleware — the outbound path stays a true
// no-op for unconfigured deployments.
func NewTelemetryFactory(providers *aptelemetry.Providers, cfg *sconfig.Telemetry) (RoundTripperFactory, error) {
	if providers == nil || !providers.Enabled {
		return nil, nil
	}

	tracesEnabled := cfg.TracesEnabled()
	metricsEnabled := cfg.MetricsEnabled()
	if !tracesEnabled && !metricsEnabled {
		return nil, nil
	}

	f := &telemetryFactory{
		tracesEnabled:  tracesEnabled,
		metricsEnabled: metricsEnabled,
		projector:      newLabelProjector(cfg.GetProxy()),
	}

	if tracesEnabled {
		f.tracer = providers.TracerProvider.Tracer(telemetryInstrumentationName)
	}

	if metricsEnabled {
		meter := providers.MeterProvider.Meter(telemetryInstrumentationName)

		var err error
		f.duration, err = meter.Float64Histogram(
			"authproxy.client.request.duration",
			metric.WithUnit("s"),
			metric.WithDescription("Duration of outbound HTTP requests issued by AuthProxy."),
		)
		if err != nil {
			return nil, fmt.Errorf("httpf: create client duration histogram: %w", err)
		}
		f.requestBodySize, err = meter.Int64Histogram(
			"authproxy.client.request.body.size",
			metric.WithUnit("By"),
			metric.WithDescription("Bytes sent in outbound HTTP request bodies (when Content-Length is known)."),
		)
		if err != nil {
			return nil, fmt.Errorf("httpf: create request body size histogram: %w", err)
		}
		f.responseBodySize, err = meter.Int64Histogram(
			"authproxy.client.response.body.size",
			metric.WithUnit("By"),
			metric.WithDescription("Bytes received in outbound HTTP response bodies (when Content-Length is known)."),
		)
		if err != nil {
			return nil, fmt.Errorf("httpf: create response body size histogram: %w", err)
		}
	}

	return f, nil
}

type telemetryFactory struct {
	tracesEnabled    bool
	metricsEnabled   bool
	tracer           trace.Tracer
	duration         metric.Float64Histogram
	requestBodySize  metric.Int64Histogram
	responseBodySize metric.Int64Histogram
	projector        *labelProjector
}

// NewRoundTripper wraps transport in a roundtripper that emits a span and
// metric observation per call. ri snapshots the request context (connection /
// connector identity + effective labels) at the time the gentleman client
// was constructed; subsequent calls through this transport reuse that
// snapshot. That matches how other middlewares in the chain operate.
func (f *telemetryFactory) NewRoundTripper(ri RequestInfo, transport http.RoundTripper) http.RoundTripper {
	return &telemetryRoundTripper{
		factory:     f,
		requestInfo: ri,
		transport:   transport,
	}
}

type telemetryRoundTripper struct {
	factory     *telemetryFactory
	requestInfo RequestInfo
	transport   http.RoundTripper
}

func (rt *telemetryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	spanAttrs := rt.spanStartAttributes(req)

	var span trace.Span
	if rt.factory.tracesEnabled {
		ctx, span = rt.factory.tracer.Start(
			ctx,
			"HTTP "+req.Method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(spanAttrs...),
		)
		req = req.WithContext(ctx)
	}

	start := time.Now()
	resp, err := rt.transport.RoundTrip(req)
	elapsed := time.Since(start)

	status := 0
	if resp != nil {
		status = resp.StatusCode
	}

	if span != nil {
		rt.finaliseSpan(span, status, err)
	}

	if rt.factory.metricsEnabled {
		metricAttrs := rt.metricAttributes(req, status)
		rt.factory.duration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(metricAttrs...))
		if req.ContentLength > 0 {
			rt.factory.requestBodySize.Record(ctx, req.ContentLength, metric.WithAttributes(metricAttrs...))
		}
		if resp != nil && resp.ContentLength > 0 {
			rt.factory.responseBodySize.Record(ctx, resp.ContentLength, metric.WithAttributes(metricAttrs...))
		}
	}

	return resp, err
}

// spanStartAttributes returns the attributes known at request start: HTTP
// method, URL parts, request-type, connector/connection identity (when
// present), and the projected span-attribute labels.
func (rt *telemetryRoundTripper) spanStartAttributes(req *http.Request) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(req.Method),
	}
	if req.URL != nil {
		if host := req.URL.Host; host != "" {
			attrs = append(attrs, semconv.ServerAddress(host))
		}
		if scheme := req.URL.Scheme; scheme != "" {
			attrs = append(attrs, semconv.URLScheme(scheme))
		}
		if path := req.URL.Path; path != "" {
			attrs = append(attrs, semconv.URLPath(path))
		}
	}
	attrs = append(attrs, rt.identityAttributes()...)
	attrs = append(attrs, rt.factory.projector.spanAttrs(rt.requestInfo.Labels)...)
	return attrs
}

// identityAttributes are the AuthProxy-specific identity attributes attached
// to every emitted signal. Keep cardinality bounded: namespace and connector
// id are low-cardinality, connection id is per-entity but still bounded by
// total connection count.
func (rt *telemetryRoundTripper) identityAttributes() []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("authproxy.request.type", string(rt.requestInfo.Type)),
	}
	if ns := rt.requestInfo.Namespace; ns != "" {
		attrs = append(attrs, attribute.String("authproxy.namespace", ns))
	}
	if !rt.requestInfo.ConnectorId.IsNil() {
		attrs = append(attrs, attribute.String("authproxy.connector_id", rt.requestInfo.ConnectorId.String()))
	}
	if rt.requestInfo.ConnectorVersion != 0 {
		attrs = append(attrs, attribute.Int64("authproxy.connector_version", int64(rt.requestInfo.ConnectorVersion)))
	}
	if !rt.requestInfo.ConnectionId.IsNil() {
		attrs = append(attrs, attribute.String("authproxy.connection_id", rt.requestInfo.ConnectionId.String()))
	}
	return attrs
}

// finaliseSpan records the response status and ends the span. 5xx responses
// and transport errors mark the span errored; 4xx leaves status Unset per
// OTel HTTP semantic conventions (client mistakes are not service errors).
func (rt *telemetryRoundTripper) finaliseSpan(span trace.Span, status int, err error) {
	defer span.End()

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "transport error")
		return
	}

	span.SetAttributes(semconv.HTTPResponseStatusCode(status))
	if status >= 500 {
		span.SetStatus(codes.Error, "HTTP "+strconv.Itoa(status))
	}
}

// metricAttributes returns the dimension set for metric observations. Limited
// to bounded-cardinality dimensions: method, status, request type, identity,
// and explicitly allowlisted projected labels (with optional value cap).
func (rt *telemetryRoundTripper) metricAttributes(req *http.Request, status int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(req.Method),
		semconv.HTTPResponseStatusCode(status),
		attribute.String("authproxy.request.type", string(rt.requestInfo.Type)),
	}
	if !rt.requestInfo.ConnectorId.IsNil() {
		attrs = append(attrs, attribute.String("authproxy.connector_id", rt.requestInfo.ConnectorId.String()))
	}
	attrs = append(attrs, rt.factory.projector.metricDims(rt.requestInfo.Labels)...)
	return attrs
}

// labelProjector encapsulates the two independent allowlists and optional
// value-cap state. It reads directly from RequestInfo.Labels (the already-
// computed effective set produced by httpf.F.ForConnection / ForLabels). It
// does no merging of its own: that work happens upstream.
type labelProjector struct {
	spanKeys   []string
	metricKeys []string
	valueCap   int

	// valueSeen tracks the bounded set of values observed per metric
	// dimension key for cap enforcement. Once a key's set reaches valueCap,
	// further distinct values collapse to LabelValueOther. The cap is per
	// process; in a multi-replica deployment the cap applies independently
	// on each replica, which is the conservative behaviour we want.
	valueSeenMu sync.RWMutex
	valueSeen   map[string]map[string]struct{}
}

func newLabelProjector(cfg *sconfig.TelemetryProxy) *labelProjector {
	p := &labelProjector{}
	if cfg == nil {
		return p
	}
	p.spanKeys = append(p.spanKeys, cfg.SpanAttributeLabels...)
	p.metricKeys = append(p.metricKeys, cfg.MetricDimensionLabels...)
	if cfg.MetricDimensionValueCap != nil && *cfg.MetricDimensionValueCap > 0 {
		p.valueCap = *cfg.MetricDimensionValueCap
		p.valueSeen = make(map[string]map[string]struct{}, len(p.metricKeys))
	}
	return p
}

// spanAttrs projects allowlisted labels onto span attributes. Keys not
// present in labels produce no attribute. The label key is reported verbatim
// (no namespacing) — applications choose label keys that won't collide with
// reserved OTel attributes.
func (p *labelProjector) spanAttrs(labels map[string]string) []attribute.KeyValue {
	if p == nil || len(p.spanKeys) == 0 || len(labels) == 0 {
		return nil
	}
	out := make([]attribute.KeyValue, 0, len(p.spanKeys))
	for _, k := range p.spanKeys {
		v, ok := labels[k]
		if !ok {
			continue
		}
		out = append(out, attribute.String(k, v))
	}
	return out
}

// metricDims projects allowlisted labels onto metric dimensions, applying the
// configured value cap. Keys not present in labels produce no dimension.
func (p *labelProjector) metricDims(labels map[string]string) []attribute.KeyValue {
	if p == nil || len(p.metricKeys) == 0 || len(labels) == 0 {
		return nil
	}
	out := make([]attribute.KeyValue, 0, len(p.metricKeys))
	for _, k := range p.metricKeys {
		v, ok := labels[k]
		if !ok {
			continue
		}
		out = append(out, attribute.String(k, p.cappedValue(k, v)))
	}
	return out
}

// cappedValue returns v unchanged when the configured value cap allows it,
// or LabelValueOther when v would push a key's distinct-value count over the
// cap. When no cap is configured, v is always returned verbatim.
func (p *labelProjector) cappedValue(key, value string) string {
	if p.valueCap <= 0 {
		return value
	}

	// Hot path: value already accepted for this key.
	p.valueSeenMu.RLock()
	if seen, ok := p.valueSeen[key]; ok {
		if _, present := seen[value]; present {
			p.valueSeenMu.RUnlock()
			return value
		}
	}
	p.valueSeenMu.RUnlock()

	// Slow path: take a write lock and admit the value if there's room.
	p.valueSeenMu.Lock()
	defer p.valueSeenMu.Unlock()

	seen := p.valueSeen[key]
	if seen == nil {
		seen = make(map[string]struct{}, p.valueCap)
		p.valueSeen[key] = seen
	}
	if _, present := seen[value]; present {
		return value
	}
	if len(seen) >= p.valueCap {
		return LabelValueOther
	}
	seen[value] = struct{}{}
	return value
}
