package httpf

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// LabelValueOther mirrors aptelemetry.LabelValueOther for callers that
// only import this package. Kept as a constant alias so existing
// references continue to compile.
const LabelValueOther = aptelemetry.LabelValueOther

// telemetryInstrumentationName is the instrumentation scope reported on the
// emitted spans and metrics. Useful for filtering by source in dashboards.
const telemetryInstrumentationName = "github.com/rmorlok/authproxy/internal/httpf"

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
		tracesEnabled:         tracesEnabled,
		metricsEnabled:        metricsEnabled,
		projector:             aptelemetry.NewLabelProjectorFromProxyConfig(cfg.GetProxy()),
		propagator:            providers.Propagator,
		injectOutboundDefault: cfg.InjectOutboundDefault(),
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
	projector        *aptelemetry.LabelProjector

	// propagator is the W3C TraceContext + Baggage composite from the
	// providers. Always present (even with no-op providers) but only used
	// when the resolved propagation decision is true.
	propagator propagation.TextMapPropagator
	// injectOutboundDefault is the global default for outbound W3C trace
	// context injection. Per-connection / per-connector overrides on
	// RequestInfo.PropagateTraceContext take precedence over this default.
	injectOutboundDefault bool
}

// shouldPropagate resolves the outbound-injection decision for a given
// RequestInfo. The per-request override (sourced from the connector's
// telemetry.propagate_trace_context) wins when present; otherwise the global
// telemetry.propagation.inject_outbound_default applies.
func (f *telemetryFactory) shouldPropagate(ri RequestInfo) bool {
	if ri.PropagateTraceContext != nil {
		return *ri.PropagateTraceContext
	}
	return f.injectOutboundDefault
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

	// Inject W3C trace context onto outbound headers when the resolved
	// decision is true. Decision priority: per-request override > global
	// default. Injection is unconditionally a no-op when the active span is
	// invalid (no provider, sampling drop, etc.), so this is safe to call
	// even with no-op providers.
	if rt.factory.propagator != nil && rt.factory.shouldPropagate(rt.requestInfo) {
		rt.factory.propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))
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
	attrs = append(attrs, rt.factory.projector.SpanAttrs(rt.requestInfo.Labels)...)
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
	attrs = append(attrs, rt.factory.projector.MetricDims(rt.requestInfo.Labels)...)
	return attrs
}

