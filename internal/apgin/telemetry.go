package apgin

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	"github.com/rmorlok/authproxy/internal/httperr"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// telemetryInstrumentationName is the instrumentation scope reported on the
// emitted spans and metrics. Useful for filtering by source in dashboards.
const telemetryInstrumentationName = "github.com/rmorlok/authproxy/internal/apgin"

// telemetryMiddleware emits a server span per inbound request plus the
// standard semconv HTTP server metrics (duration histogram + active requests
// up-down counter). Errored 5xx responses are recorded on the span along with
// httperr metadata; panics are recorded as exception events before being
// re-raised for downstream Recovery middleware to handle.
//
// When telemetry is disabled (no providers, providers in no-op mode, or both
// trace and metric signals turned off), telemetryMiddleware returns a nil
// gin.HandlerFunc so the caller can skip registration entirely — keeping the
// hot path zero-cost for unconfigured deployments.
type telemetryMiddleware struct {
	tracer         trace.Tracer
	meter          metric.Meter
	duration       metric.Float64Histogram
	activeRequests metric.Int64UpDownCounter
	excluded       map[string]struct{}
	tracesEnabled  bool
	metricsEnabled bool
	serviceID      string
}

// newTelemetryMiddleware constructs a request-handling middleware from the
// supplied providers and config. When the providers are nil/disabled or both
// the trace and metric signals are off, it returns nil so the caller can skip
// registration.
func newTelemetryMiddleware(
	serviceID string,
	providers *aptelemetry.Providers,
	cfg *sconfig.Telemetry,
) (gin.HandlerFunc, error) {
	if providers == nil || !providers.Enabled {
		return nil, nil
	}

	tracesEnabled := cfg.TracesEnabled()
	metricsEnabled := cfg.MetricsEnabled()
	if !tracesEnabled && !metricsEnabled {
		return nil, nil
	}

	mw := &telemetryMiddleware{
		serviceID:      serviceID,
		tracesEnabled:  tracesEnabled,
		metricsEnabled: metricsEnabled,
		excluded:       buildExcludedSet(cfg.GetHTTPExcludedPaths()),
	}

	if tracesEnabled {
		mw.tracer = providers.TracerProvider.Tracer(telemetryInstrumentationName)
	}

	if metricsEnabled {
		mw.meter = providers.MeterProvider.Meter(telemetryInstrumentationName)

		var err error
		mw.duration, err = mw.meter.Float64Histogram(
			"http.server.request.duration",
			metric.WithUnit("s"),
			metric.WithDescription("Duration of HTTP server requests."),
		)
		if err != nil {
			return nil, fmt.Errorf("apgin: create duration histogram: %w", err)
		}

		mw.activeRequests, err = mw.meter.Int64UpDownCounter(
			"http.server.active_requests",
			metric.WithUnit("{request}"),
			metric.WithDescription("Number of active HTTP server requests."),
		)
		if err != nil {
			return nil, fmt.Errorf("apgin: create active_requests counter: %w", err)
		}
	}

	return mw.handle, nil
}

func buildExcludedSet(paths []string) map[string]struct{} {
	if len(paths) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		out[p] = struct{}{}
	}
	return out
}

// isExcluded reports whether the request path is on the exclusion list.
// Matches on the raw URL path (not the route template) so health checks pinned
// at a specific URL are filtered before route matching has a chance to run.
func (m *telemetryMiddleware) isExcluded(path string) bool {
	if len(m.excluded) == 0 {
		return false
	}
	_, ok := m.excluded[path]
	return ok
}

// handle is the per-request middleware function. It opens a server span
// (when traces are enabled), increments the active-requests gauge, defers
// status / error recording and metric emission, and re-raises any panic so
// the existing Recovery middleware can render a 500.
func (m *telemetryMiddleware) handle(c *gin.Context) {
	if m.isExcluded(c.Request.URL.Path) {
		c.Next()
		return
	}

	req := c.Request
	startAttrs := startAttributes(c, m.serviceID)

	ctx := req.Context()
	var span trace.Span
	if m.tracesEnabled {
		ctx, span = m.tracer.Start(
			ctx,
			"HTTP "+req.Method,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(startAttrs...),
		)
		c.Request = req.WithContext(ctx)
	}

	if m.metricsEnabled {
		m.activeRequests.Add(ctx, 1, metric.WithAttributes(activeRequestAttrs(c, m.serviceID)...))
		defer m.activeRequests.Add(ctx, -1, metric.WithAttributes(activeRequestAttrs(c, m.serviceID)...))
	}

	start := time.Now()

	// Span finalisation runs after the handler chain returns. Panics are
	// captured by telemetryRecovery (registered later in the middleware
	// chain) — by the time finalise runs, the recovery middleware has
	// already attached the exception event to the active span and set the
	// response status to 500.
	defer m.finalise(c, span, start)

	c.Next()
}

// finalise records the response status, errors, and metric observations
// after the handler chain has run. Safe to call when traces or metrics are
// disabled.
func (m *telemetryMiddleware) finalise(c *gin.Context, span trace.Span, start time.Time) {
	status := c.Writer.Status()
	route := routeName(c)

	if span != nil {
		span.SetAttributes(finishAttributes(status, route)...)
		recordSpanErrors(span, c, status)
		span.End()
	}

	if m.metricsEnabled {
		m.duration.Record(
			c.Request.Context(),
			time.Since(start).Seconds(),
			metric.WithAttributes(metricAttributes(c, route, status, m.serviceID)...),
		)
	}
}

// telemetryRecovery replaces gin.Recovery() in the middleware chain when
// telemetry is enabled. It records the panic as an exception event on the
// active span (started by telemetryMiddleware further out in the chain) and
// writes a 500 response — same response behaviour as gin.Recovery().
func telemetryRecovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(gin.DefaultErrorWriter, func(c *gin.Context, rec any) {
		span := trace.SpanFromContext(c.Request.Context())
		if span.IsRecording() {
			span.RecordError(panicError(rec), trace.WithStackTrace(true))
			span.SetStatus(codes.Error, "panic recovered")
		}
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}

func panicError(rec any) error {
	if err, ok := rec.(error); ok {
		return err
	}
	return fmt.Errorf("panic: %v", rec)
}

// startAttributes returns the attribute set known at request start.
func startAttributes(c *gin.Context, serviceID string) []attribute.KeyValue {
	req := c.Request
	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(req.Method),
		semconv.URLPath(req.URL.Path),
		semconv.URLScheme(schemeFor(req.TLS != nil)),
	}
	if proto := req.Proto; proto != "" {
		attrs = append(attrs, attribute.String("network.protocol.version", proto))
	}
	if host := req.Host; host != "" {
		attrs = append(attrs, semconv.ServerAddress(host))
	}
	if ua := req.UserAgent(); ua != "" {
		attrs = append(attrs, semconv.UserAgentOriginal(ua))
	}
	if ip := c.ClientIP(); ip != "" {
		attrs = append(attrs, semconv.ClientAddress(ip))
	}
	if serviceID != "" {
		attrs = append(attrs, attribute.String("authproxy.service", serviceID))
	}
	return attrs
}

// activeRequestAttrs are a smaller set used for the active-requests counter:
// keeping cardinality low avoids exploding the gauge by URL path.
func activeRequestAttrs(c *gin.Context, serviceID string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(c.Request.Method),
	}
	if serviceID != "" {
		attrs = append(attrs, attribute.String("authproxy.service", serviceID))
	}
	return attrs
}

// finishAttributes returns the attribute set known once the handler has
// returned: the matched route template (if any) and the response status.
func finishAttributes(status int, route string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.HTTPResponseStatusCode(status),
	}
	if route != "" {
		attrs = append(attrs, semconv.HTTPRoute(route))
	}
	return attrs
}

// metricAttributes for the duration histogram. Limited to bounded-cardinality
// labels (method, route template, status code, service) to avoid metric
// explosion by URL path.
func metricAttributes(c *gin.Context, route string, status int, serviceID string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(c.Request.Method),
		semconv.HTTPResponseStatusCode(status),
	}
	if route != "" {
		attrs = append(attrs, semconv.HTTPRoute(route))
	}
	if serviceID != "" {
		attrs = append(attrs, attribute.String("authproxy.service", serviceID))
	}
	return attrs
}

// routeName resolves the matched route template for the request. Falls back
// to the URL path when no route matched (e.g., 404s) so the span still has a
// useful identifier.
func routeName(c *gin.Context) string {
	if fp := c.FullPath(); fp != "" {
		return fp
	}
	return c.Request.URL.Path
}

func schemeFor(tls bool) string {
	if tls {
		return "https"
	}
	return "http"
}

// recordSpanErrors marks the span errored on 5xx and attaches httperr
// metadata where available. 4xx client errors leave the span status Unset
// per OpenTelemetry HTTP semantic conventions: client mistakes are not
// "the server malfunctioned" and should not show up as service errors.
func recordSpanErrors(span trace.Span, c *gin.Context, status int) {
	if status < 500 {
		return
	}

	span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))

	for _, ginErr := range c.Errors {
		err := ginErr.Err
		if err == nil {
			continue
		}

		var httpErr *httperr.Error
		if errors.As(err, &httpErr) {
			attrs := []attribute.KeyValue{
				attribute.Int("authproxy.httperr.status", httpErr.Status),
			}
			if msg := httpErr.ResponseMsgOrDefault(); msg != "" {
				attrs = append(attrs, attribute.String("authproxy.httperr.response_msg", msg))
			}
			span.SetAttributes(attrs...)
			if httpErr.InternalErr != nil {
				span.RecordError(httpErr.InternalErr)
			} else {
				span.RecordError(httpErr)
			}
			continue
		}

		span.RecordError(err)
	}
}

