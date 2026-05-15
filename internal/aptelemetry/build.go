package aptelemetry

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	lognoop "go.opentelemetry.io/otel/log/noop"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// New constructs Providers for the named service from the supplied telemetry
// configuration. When cfg is nil or disabled, New returns no-op providers
// without dialling any exporter or initialising a resource beyond SDK
// defaults.
//
// serviceID is the per-service identifier (e.g. "api", "admin-api"); it is
// joined with the configured resource.service_name_prefix to produce the
// service.name resource attribute. version is the application build version
// reported as service.version; pass an empty string to derive it from the
// embedded debug build info.
func New(ctx context.Context, serviceID string, version string, cfg *sconfig.Telemetry) (*Providers, error) {
	if !cfg.IsEnabled() {
		return NoopProviders(), nil
	}

	// Endpoint-gated soft-disable: when the operator marks telemetry
	// enabled but the exporter.endpoint resolves to empty (env-var with
	// blank default, no explicit value), fall through to no-op providers.
	// This is the gating mechanism dev_config/default.yaml uses to ship a
	// "telemetry block present, but inert unless AUTHPROXY_OTEL_ENDPOINT
	// is set" UX — flip on the env var to point at a real OTLP collector,
	// otherwise the SDK never tries to dial anywhere.
	if endpoint, ok, err := exporterEndpoint(ctx, cfg.GetExporter()); err != nil {
		return nil, fmt.Errorf("aptelemetry: resolve exporter endpoint: %w", err)
	} else if !ok || endpoint == "" {
		return NoopProviders(), nil
	}

	res, err := buildResource(ctx, serviceID, version, cfg.GetResource())
	if err != nil {
		return nil, fmt.Errorf("aptelemetry: build resource: %w", err)
	}

	providers := &Providers{
		Enabled:    true,
		Propagator: defaultPropagator(),
	}

	if cfg.TracesEnabled() {
		tp, err := buildTracerProvider(ctx, res, cfg)
		if err != nil {
			_ = providers.Shutdown(ctx)
			return nil, fmt.Errorf("aptelemetry: build tracer provider: %w", err)
		}
		providers.TracerProvider = tp
		providers.addShutdown(tp.Shutdown)
	} else {
		providers.TracerProvider = tracenoop.NewTracerProvider()
	}

	if cfg.MetricsEnabled() {
		mp, err := buildMeterProvider(ctx, res, cfg)
		if err != nil {
			_ = providers.Shutdown(ctx)
			return nil, fmt.Errorf("aptelemetry: build meter provider: %w", err)
		}
		providers.MeterProvider = mp
		providers.addShutdown(mp.Shutdown)
	} else {
		providers.MeterProvider = metricnoop.NewMeterProvider()
	}

	if cfg.LogsEnabled() {
		lp, err := buildLoggerProvider(ctx, res, cfg)
		if err != nil {
			_ = providers.Shutdown(ctx)
			return nil, fmt.Errorf("aptelemetry: build logger provider: %w", err)
		}
		providers.LoggerProvider = lp
		providers.addShutdown(lp.Shutdown)
	} else {
		providers.LoggerProvider = lognoop.NewLoggerProvider()
	}

	return providers, nil
}

// buildResource constructs the OTel resource from the service id, build
// version, and configured resource block. Standard OTEL_RESOURCE_ATTRIBUTES
// env-var entries are merged in by resource.Default() / resource.New
// detectors.
func buildResource(
	ctx context.Context,
	serviceID string,
	version string,
	rcfg *sconfig.TelemetryResource,
) (*resource.Resource, error) {
	prefix := rcfg.GetServiceNamePrefix()
	serviceName := prefix
	if serviceID != "" {
		serviceName = prefix + "-" + serviceID
	}

	if version == "" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			version = info.Main.Version
		}
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceName(serviceName),
		semconv.ServiceInstanceID(uuid.NewString()),
	}
	if version != "" {
		attrs = append(attrs, semconv.ServiceVersion(version))
	}
	for k, v := range rcfg.Attributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	return resource.New(
		ctx,
		resource.WithFromEnv(),    // honors OTEL_RESOURCE_ATTRIBUTES
		resource.WithTelemetrySDK(),
		resource.WithProcess(),
		resource.WithHost(),
		resource.WithAttributes(attrs...),
	)
}

func buildTracerProvider(
	ctx context.Context,
	res *resource.Resource,
	cfg *sconfig.Telemetry,
) (*sdktrace.TracerProvider, error) {
	exp, err := buildTraceExporter(ctx, cfg.GetExporter())
	if err != nil {
		return nil, err
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.GetSamplingRatio()))

	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
		sdktrace.WithBatcher(exp),
	), nil
}

func buildTraceExporter(ctx context.Context, ec *sconfig.TelemetryExporter) (sdktrace.SpanExporter, error) {
	switch ec.GetProtocol() {
	case sconfig.TelemetryExporterProtocolHTTPProtobuf:
		opts, err := traceHTTPOptions(ctx, ec)
		if err != nil {
			return nil, err
		}
		return otlptrace.New(ctx, otlptracehttp.NewClient(opts...))
	default:
		opts, err := traceGRPCOptions(ctx, ec)
		if err != nil {
			return nil, err
		}
		return otlptrace.New(ctx, otlptracegrpc.NewClient(opts...))
	}
}

func traceGRPCOptions(ctx context.Context, ec *sconfig.TelemetryExporter) ([]otlptracegrpc.Option, error) {
	var opts []otlptracegrpc.Option
	if endpoint, ok, err := exporterEndpoint(ctx, ec); err != nil {
		return nil, err
	} else if ok {
		opts = append(opts, otlptracegrpc.WithEndpointURL(endpoint))
	}
	if ec.GetInsecure() {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	if headers, err := exporterHeaders(ctx, ec); err != nil {
		return nil, err
	} else if len(headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(headers))
	}
	return opts, nil
}

func traceHTTPOptions(ctx context.Context, ec *sconfig.TelemetryExporter) ([]otlptracehttp.Option, error) {
	var opts []otlptracehttp.Option
	if endpoint, ok, err := exporterEndpoint(ctx, ec); err != nil {
		return nil, err
	} else if ok {
		opts = append(opts, otlptracehttp.WithEndpointURL(endpoint))
	}
	if ec.GetInsecure() {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	if headers, err := exporterHeaders(ctx, ec); err != nil {
		return nil, err
	} else if len(headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(headers))
	}
	return opts, nil
}

func buildMeterProvider(
	ctx context.Context,
	res *resource.Resource,
	cfg *sconfig.Telemetry,
) (*sdkmetric.MeterProvider, error) {
	exp, err := buildMetricExporter(ctx, cfg.GetExporter())
	if err != nil {
		return nil, err
	}

	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)),
	), nil
}

func buildMetricExporter(ctx context.Context, ec *sconfig.TelemetryExporter) (sdkmetric.Exporter, error) {
	switch ec.GetProtocol() {
	case sconfig.TelemetryExporterProtocolHTTPProtobuf:
		var opts []otlpmetrichttp.Option
		if endpoint, ok, err := exporterEndpoint(ctx, ec); err != nil {
			return nil, err
		} else if ok {
			opts = append(opts, otlpmetrichttp.WithEndpointURL(endpoint))
		}
		if ec.GetInsecure() {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}
		if headers, err := exporterHeaders(ctx, ec); err != nil {
			return nil, err
		} else if len(headers) > 0 {
			opts = append(opts, otlpmetrichttp.WithHeaders(headers))
		}
		return otlpmetrichttp.New(ctx, opts...)
	default:
		var opts []otlpmetricgrpc.Option
		if endpoint, ok, err := exporterEndpoint(ctx, ec); err != nil {
			return nil, err
		} else if ok {
			opts = append(opts, otlpmetricgrpc.WithEndpointURL(endpoint))
		}
		if ec.GetInsecure() {
			opts = append(opts, otlpmetricgrpc.WithInsecure())
		}
		if headers, err := exporterHeaders(ctx, ec); err != nil {
			return nil, err
		} else if len(headers) > 0 {
			opts = append(opts, otlpmetricgrpc.WithHeaders(headers))
		}
		return otlpmetricgrpc.New(ctx, opts...)
	}
}

func buildLoggerProvider(
	ctx context.Context,
	res *resource.Resource,
	cfg *sconfig.Telemetry,
) (*sdklog.LoggerProvider, error) {
	exp, err := buildLogExporter(ctx, cfg.GetExporter())
	if err != nil {
		return nil, err
	}

	return sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
	), nil
}

func buildLogExporter(ctx context.Context, ec *sconfig.TelemetryExporter) (sdklog.Exporter, error) {
	switch ec.GetProtocol() {
	case sconfig.TelemetryExporterProtocolHTTPProtobuf:
		var opts []otlploghttp.Option
		if endpoint, ok, err := exporterEndpoint(ctx, ec); err != nil {
			return nil, err
		} else if ok {
			opts = append(opts, otlploghttp.WithEndpointURL(endpoint))
		}
		if ec.GetInsecure() {
			opts = append(opts, otlploghttp.WithInsecure())
		}
		if headers, err := exporterHeaders(ctx, ec); err != nil {
			return nil, err
		} else if len(headers) > 0 {
			opts = append(opts, otlploghttp.WithHeaders(headers))
		}
		return otlploghttp.New(ctx, opts...)
	default:
		var opts []otlploggrpc.Option
		if endpoint, ok, err := exporterEndpoint(ctx, ec); err != nil {
			return nil, err
		} else if ok {
			opts = append(opts, otlploggrpc.WithEndpointURL(endpoint))
		}
		if ec.GetInsecure() {
			opts = append(opts, otlploggrpc.WithInsecure())
		}
		if headers, err := exporterHeaders(ctx, ec); err != nil {
			return nil, err
		} else if len(headers) > 0 {
			opts = append(opts, otlploggrpc.WithHeaders(headers))
		}
		return otlploggrpc.New(ctx, opts...)
	}
}

// exporterEndpoint returns the configured endpoint URL. The bool reports
// whether an endpoint was supplied; when false, the SDK falls back to its own
// default (e.g. OTEL_EXPORTER_OTLP_ENDPOINT or localhost:4317).
func exporterEndpoint(ctx context.Context, ec *sconfig.TelemetryExporter) (string, bool, error) {
	if ec == nil || ec.Endpoint == nil || !ec.Endpoint.HasValue(ctx) {
		return "", false, nil
	}
	v, err := ec.Endpoint.GetValue(ctx)
	if err != nil {
		return "", false, fmt.Errorf("resolve exporter.endpoint: %w", err)
	}
	if v == "" {
		return "", false, nil
	}
	return v, true, nil
}

// exporterHeaders resolves env-var fallthrough on each header value.
func exporterHeaders(ctx context.Context, ec *sconfig.TelemetryExporter) (map[string]string, error) {
	if ec == nil || len(ec.Headers) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(ec.Headers))
	for k, sv := range ec.Headers {
		if sv == nil || !sv.HasValue(ctx) {
			continue
		}
		v, err := sv.GetValue(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve exporter.headers[%s]: %w", k, err)
		}
		out[k] = v
	}
	return out, nil
}

// ShutdownTimeout is the recommended deadline for a clean shutdown call.
const ShutdownTimeout = 10 * time.Second
