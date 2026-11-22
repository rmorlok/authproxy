package proxy_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	authmock "github.com/rmorlok/authproxy/internal/auth_methods/mock"
	"github.com/rmorlok/authproxy/internal/core/iface"
	coremock "github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/proxy"
	"github.com/rmorlok/authproxy/internal/ratelimit"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	clock "k8s.io/utils/clock/testing"
)

// labelMiddleware supplies a test transport wrapper at the normal factory boundary.
type labelMiddleware func(httpf.RequestInfo, http.RoundTripper) http.RoundTripper

// NewRoundTripper delegates to the fixture while retaining real factory composition.
func (f labelMiddleware) NewRoundTripper(info httpf.RequestInfo, next http.RoundTripper) http.RoundTripper {
	return f(info, next)
}

// labelTransport adapts an in-memory upstream or request observer to HTTP transport.
type labelTransport func(*http.Request) (*http.Response, error)

// RoundTrip delegates to the fixture without creating a network connection.
func (f labelTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// TestHTTPClientRequestLabelsReachMiddleware verifies the actual rate-limit and
// telemetry middleware consume the same resolved labels on each logical call.
func TestHTTPClientRequestLabelsReachMiddleware(t *testing.T) {
	// Use a fixed clock and isolated in-memory Redis so the second request must
	// hit the same window. Only Redis uses a local listener; the upstream is fake.
	_, redis, server := apredis.MustApplyTestConfigWithServer(nil)
	t.Cleanup(server.Close)
	t.Cleanup(func() { require.NoError(t, redis.Close()) })
	now := clock.NewFakeClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
	ctx := apctx.NewBuilderBackground().WithClock(now).Build()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cache := ratelimit.NewCache()
	ruleID := apid.New(apid.PrefixRateLimit)
	cache.Replace([]*database.RateLimit{{
		Id: ruleID, Namespace: "root",
		Definition: rlschema.RateLimitSpec{
			Selector: rlschema.Selector{LabelSelector: "operation=write"},
			Bucket:   rlschema.Bucket{Dimensions: []string{rlschema.DimensionActor, "labels/workload"}},
			Algorithm: rlschema.Algorithm{FixedWindow: &rlschema.FixedWindow{
				Window: common.HumanDuration{Duration: time.Minute}, Limit: 1,
			}},
		},
	}}, now.Now())

	// Both telemetry signals stay in memory, with the same label allowlist.
	spans := tracetest.NewSpanRecorder()
	traces := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	reader := sdkmetric.NewManualReader()
	metrics := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, traces.Shutdown(context.Background())) })
	t.Cleanup(func() { require.NoError(t, metrics.Shutdown(context.Background())) })
	enabled := true
	telemetry, err := httpf.NewTelemetryFactory(&aptelemetry.Providers{
		Enabled: true, TracerProvider: traces, MeterProvider: metrics, Propagator: propagation.TraceContext{},
	}, &sconfig.Telemetry{Enabled: &enabled, Proxy: &sconfig.TelemetryProxy{
		SpanAttributeLabels: []string{"operation", "workload"}, MetricDimensionLabels: []string{"operation", "workload"},
	}})
	require.NoError(t, err)

	// Observe request-event attribution outside the enforcer, including its
	// synthetic 429; the innermost fake records actual upstream handoffs only.
	var infos []httpf.RequestInfo
	var sent int
	observer := labelMiddleware(func(info httpf.RequestInfo, next http.RoundTripper) http.RoundTripper {
		return labelTransport(func(req *http.Request) (*http.Response, error) {
			infos = append(infos, info)
			return next.RoundTrip(req)
		})
	})
	target := labelMiddleware(func(_ httpf.RequestInfo, _ http.RoundTripper) http.RoundTripper {
		return labelTransport(func(req *http.Request) (*http.Response, error) {
			sent++
			if req.Body != nil {
				_ = req.Body.Close()
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody, Request: req}, nil
		})
	})
	factory := httpf.CreateFactory(nil, redis, observer, logger, target, ratelimit.NewEnforcerFactory(cache, redis, logger), telemetry)
	auth := authmock.NewMockAuthenticator(gomock.NewController(t))
	auth.EXPECT().Resolve(gomock.Any()).Return(auth_methods.AuthApplication{}, nil).Times(4)
	actor := &authcore.Actor{Id: apid.New(apid.PrefixActor), Namespace: "root", Labels: map[string]string{"team": "captured"}}
	connection := &coremock.Connection{
		Id: apid.New(apid.PrefixConnection), Namespace: "root.tenant", Labels: map[string]string{"operation": "connection", "tenant": "captured"},
	}
	client, err := proxy.New(factory, connection, auth, nil).HTTPClient(iface.HTTPClientOptions{
		Actor: iface.ActingAs(actor), Labels: map[string]string{"operation": "read"},
	})
	require.NoError(t, err)

	// Caller mutations cannot refresh captured metadata when request labels
	// build their own middleware stack. Per-call labels affect only their call.
	connection.Labels["tenant"] = "changed"
	actor.Labels["team"] = "changed"
	for index, test := range []struct {
		labels map[string]string
		status int
	}{
		{map[string]string{"operation": "write", "workload": "alpha"}, http.StatusOK},
		{map[string]string{"operation": "write", "workload": "alpha"}, http.StatusTooManyRequests},
		{map[string]string{"operation": "write", "workload": "beta"}, http.StatusOK},
		{nil, http.StatusOK},
	} {
		attribution := &app_metrics.Attribution{}
		requestCtx, evidence := httpf.ObserveSends(app_metrics.ContextWithAttribution(ctx, attribution))
		request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, "https://upstream.example.test/items", nil)
		require.NoError(t, err)
		response, err := client.DoWithLabels(request, test.labels)
		require.NoError(t, err)
		require.Equal(t, test.status, response.StatusCode)
		require.NoError(t, response.Body.Close())
		if index == 1 {
			// A repeated workload is rejected locally and records the exact
			// actor/workload bucket derived from captured identity + request labels.
			require.Equal(t, httpf.SendStateNotSent, evidence.State())
			require.Equal(t, app_metrics.ResponseSourceRateLimit, attribution.Source)
			require.Equal(t, ruleID, attribution.RateLimitId)
			require.Equal(t, map[string]string{"actor": actor.Id.String(), "labels/workload": "alpha"}, attribution.RateLimitBucket)
		}
	}
	require.Equal(t, 3, sent)
	require.Len(t, infos, 4)
	for index, info := range infos {
		require.Equal(t, connection.Id, info.ConnectionId)
		require.Equal(t, "root.tenant", info.Namespace)
		require.Equal(t, httpf.RequestTypeProxy, info.Type)
		require.Equal(t, actor.Id.String(), info.Labels["apxy/act/-/id"])
		require.Equal(t, "captured", info.Labels["apxy/act/team"])
		require.Equal(t, "captured", info.Labels["tenant"])
		require.Equal(t, []string{"write", "write", "write", "read"}[index], info.Labels["operation"])
	}
	require.NotContains(t, infos[3].Labels, "workload", "unlabelled calls keep the original stack")

	// Spans and metric dimensions see the same final labels, including local
	// rejection, while an unlabelled call retains the client-level default.
	ended := spans.Ended()
	require.Len(t, ended, 4)
	for index, span := range ended {
		attrs := attribute.NewSet(span.Attributes()...)
		operation, ok := attrs.Value("operation")
		require.True(t, ok)
		require.Equal(t, infos[index].Labels["operation"], operation.AsString())
		workload, present := attrs.Value("workload")
		require.Equal(t, index < 3, present)
		require.Equal(t, infos[index].Labels["workload"], workload.AsString())
	}
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &collected))
	counts := make(map[string]uint64)
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "authproxy.client.request.duration" {
				continue
			}
			for _, point := range metric.Data.(metricdata.Histogram[float64]).DataPoints {
				operation, _ := point.Attributes.Value("operation")
				workload, _ := point.Attributes.Value("workload")
				counts[operation.AsString()+"/"+workload.AsString()] += point.Count
			}
		}
	}
	require.Equal(t, map[string]uint64{"write/alpha": 2, "write/beta": 1, "read/": 1}, counts)
}
