// Package aptelemetry initialises and tears down the OpenTelemetry SDK for
// AuthProxy services.
//
// When the telemetry config block is absent or disabled, New returns Providers
// backed by no-op SDK implementations. No exporter is dialled, no resource is
// initialised beyond SDK defaults, and there is zero runtime cost beyond a
// handful of no-op interface calls. Existing deployments are unaffected by
// upgrading.
//
// When telemetry is enabled, New constructs OTLP exporters (gRPC or
// HTTP/protobuf) feeding TracerProvider, MeterProvider and LoggerProvider
// instances configured per the supplied config block. Providers are not
// installed as globals — callers receive the configured providers and are
// expected to plumb them through their dependency graph. Standard OTEL_*
// environment variables fill in any unset fields per SDK defaults.
//
// This package contains the bootstrap only. Subsystem instrumentation
// (HTTP, outbound proxy, SQL, Redis, Asynq, OAuth2 lifecycle, slog logs
// bridge) lives in subsequent tickets and consumes the providers exposed
// here.
package aptelemetry
