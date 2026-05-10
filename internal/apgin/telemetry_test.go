package apgin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	"github.com/rmorlok/authproxy/internal/httperr"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// telemetryFixture wires up an OTel SDK with a span recorder + manual metric
// reader and returns the providers + accessors so tests can introspect
// emissions without needing a live exporter.
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

// readMetrics collects everything currently observed by the manual reader.
func (f *telemetryFixture) readMetrics(t *testing.T) metricdata.ResourceMetrics {
	t.Helper()
	rm := metricdata.ResourceMetrics{}
	require.NoError(t, f.reader.Collect(context.Background(), &rm))
	return rm
}

func enabled(b bool) *bool { return &b }

func TestTelemetryMiddleware_EmitsSpanAndMetrics(t *testing.T) {
	f := newTelemetryFixture(t)
	cfg := &sconfig.Telemetry{Enabled: enabled(true)}

	engine := ForService(stubService{}, nil, false, WithTelemetry(f.providers, cfg, "api"))
	engine.GET("/api/v1/things/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.Param("id")})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/things/abc", nil)
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	gotSpans := f.spans.Ended()
	require.Len(t, gotSpans, 1)
	span := gotSpans[0]

	require.Equal(t, "HTTP GET", span.Name())

	attrs := attrMap(span.Attributes())
	require.Equal(t, "/api/v1/things/:id", attrs["http.route"], "route template (not raw path) should be the http.route attribute")
	require.EqualValues(t, http.StatusOK, attrs["http.response.status_code"])
	require.Equal(t, http.MethodGet, attrs["http.request.method"])
	require.Equal(t, "/api/v1/things/abc", attrs["url.path"])
	require.Equal(t, "api", attrs["authproxy.service"])

	rm := f.readMetrics(t)
	requireMetric(t, rm, "http.server.request.duration")
	requireMetric(t, rm, "http.server.active_requests")
}

func TestTelemetryMiddleware_ExcludesConfiguredPaths(t *testing.T) {
	f := newTelemetryFixture(t)
	cfg := &sconfig.Telemetry{Enabled: enabled(true)} // default exclusions: /ping, /healthz

	engine := ForService(stubService{}, nil, false, WithTelemetry(f.providers, cfg, "api"))
	engine.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/observed", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, p := range []string{"/ping", "/healthz"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		engine.ServeHTTP(rec, req)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/observed", nil)
	engine.ServeHTTP(rec, req)

	got := f.spans.Ended()
	require.Len(t, got, 1, "only the non-excluded path should produce a span")
	require.Equal(t, "/observed", attrMap(got[0].Attributes())["url.path"])
}

func TestTelemetryMiddleware_ExclusionsConfigurable(t *testing.T) {
	f := newTelemetryFixture(t)

	// Override: exclude /livez instead of the default /ping + /healthz.
	cfg := &sconfig.Telemetry{
		Enabled: enabled(true),
		HTTP:    &sconfig.TelemetryHTTP{ExcludedPaths: []string{"/livez"}},
	}

	engine := ForService(stubService{}, nil, false, WithTelemetry(f.providers, cfg, "api"))
	engine.GET("/livez", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, p := range []string{"/livez", "/ping"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		engine.ServeHTTP(rec, req)
	}

	got := f.spans.Ended()
	require.Len(t, got, 1, "only /ping should be observed when /livez is the override")
	require.Equal(t, "/ping", attrMap(got[0].Attributes())["url.path"])
}

func TestTelemetryMiddleware_5xxMarksSpanErroredWithHttperrMetadata(t *testing.T) {
	f := newTelemetryFixture(t)
	cfg := &sconfig.Telemetry{Enabled: enabled(true)}

	engine := ForService(stubService{}, nil, false, WithTelemetry(f.providers, cfg, "api"))
	engine.GET("/boom", func(c *gin.Context) {
		// Mirror the WriteError pattern: attach an httperr to gin's error
		// stack and write the response. The middleware should pick the
		// httperr up from c.Errors and project its metadata onto the span.
		err := httperr.InternalServerError(httperr.WithInternalErr(errors.New("downstream failure")))
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.ResponseMsgOrDefault()})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	got := f.spans.Ended()
	require.Len(t, got, 1)
	span := got[0]

	require.Equal(t, codes.Error, span.Status().Code, "5xx must mark the span errored")

	attrs := attrMap(span.Attributes())
	require.EqualValues(t, http.StatusInternalServerError, attrs["authproxy.httperr.status"])

	require.NotEmpty(t, span.Events(), "RecordError should emit an exception event")
}

func TestTelemetryMiddleware_4xxLeavesSpanStatusUnset(t *testing.T) {
	f := newTelemetryFixture(t)
	cfg := &sconfig.Telemetry{Enabled: enabled(true)}

	engine := ForService(stubService{}, nil, false, WithTelemetry(f.providers, cfg, "api"))
	engine.GET("/forbidden", func(c *gin.Context) {
		c.JSON(http.StatusForbidden, gin.H{})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/forbidden", nil)
	engine.ServeHTTP(rec, req)

	got := f.spans.Ended()
	require.Len(t, got, 1)
	require.Equal(t, codes.Unset, got[0].Status().Code,
		"4xx client errors must not be reported as service errors")
}

func TestTelemetryMiddleware_PanicEmitsExceptionEvent(t *testing.T) {
	f := newTelemetryFixture(t)
	cfg := &sconfig.Telemetry{Enabled: enabled(true)}

	engine := ForService(stubService{}, nil, false, WithTelemetry(f.providers, cfg, "api"))
	engine.GET("/panic", func(c *gin.Context) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code, "gin.Recovery should still convert panics to 500")

	got := f.spans.Ended()
	require.Len(t, got, 1)
	span := got[0]

	require.Equal(t, codes.Error, span.Status().Code, "panic must mark the span errored")

	events := span.Events()
	require.NotEmpty(t, events, "panic must produce an exception event")
}

func TestTelemetryMiddleware_NoOpWhenProvidersDisabled(t *testing.T) {
	disabled := aptelemetry.NoopProviders()
	cfg := &sconfig.Telemetry{Enabled: enabled(true)} // even though config says enabled, providers report disabled

	mw, err := newTelemetryMiddleware("api", disabled, cfg)
	require.NoError(t, err)
	require.Nil(t, mw, "no-op providers must skip middleware registration entirely")
}

func TestTelemetryMiddleware_NoOpWhenSignalsAllOff(t *testing.T) {
	f := newTelemetryFixture(t)
	off := false
	on := true
	cfg := &sconfig.Telemetry{
		Enabled: &on,
		Signals: &sconfig.TelemetrySignals{Traces: &off, Metrics: &off, Logs: &off},
	}

	mw, err := newTelemetryMiddleware("api", f.providers, cfg)
	require.NoError(t, err)
	require.Nil(t, mw, "with both traces and metrics off there is no work for the middleware")
}

func TestForService_NoTelemetry_NoOpFastPath(t *testing.T) {
	// Without WithTelemetry, the engine is identical to the historic build:
	// no extra middleware and no panic from a nil providers pointer.
	engine := ForService(stubService{}, nil, false)
	require.NotNil(t, engine)

	engine.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

// --- helpers -----------------------------------------------------------------

// stubService satisfies config.Service for ForService's signature without
// pulling in real config. The middleware doesn't read fields from it.
type stubService struct{}

func (stubService) GetId() sconfig.ServiceId { return sconfig.ServiceIdApi }
func (stubService) HealthCheckPort() uint64  { return 0 }

// attrMap flattens an attribute slice into a map keyed by string. Convenient
// for require.Equal assertions on individual attributes.
func attrMap(kvs []attribute.KeyValue) map[string]any {
	out := make(map[string]any, len(kvs))
	for _, kv := range kvs {
		out[string(kv.Key)] = kv.Value.AsInterface()
	}
	return out
}

// requireMetric asserts that a metric with the given name was emitted.
func requireMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return
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
}
