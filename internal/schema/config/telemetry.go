package config

import (
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// TelemetryExporterProtocol identifies the OTLP transport used by the exporter.
type TelemetryExporterProtocol string

const (
	TelemetryExporterProtocolGRPC         TelemetryExporterProtocol = "grpc"
	TelemetryExporterProtocolHTTPProtobuf TelemetryExporterProtocol = "http/protobuf"
)

// DefaultTelemetryServiceNamePrefix is the prefix applied to the service name
// reported in OTel resources (e.g. "authproxy-api"). Configurable via
// telemetry.resource.service_name_prefix.
const DefaultTelemetryServiceNamePrefix = "authproxy"

// DefaultTelemetrySamplingRatio is the default trace sampling ratio when
// telemetry is enabled and no explicit ratio is configured.
const DefaultTelemetrySamplingRatio = 1.0

// DefaultTelemetryHTTPExcludedPaths are the paths excluded from inbound HTTP
// telemetry by default. Avoids drowning telemetry in liveness probes.
var DefaultTelemetryHTTPExcludedPaths = []string{"/ping", "/healthz"}

// Telemetry configures OpenTelemetry signals (traces, metrics, logs) for all
// AuthProxy services. When the block is absent or Enabled is false, all OTel
// providers are no-op and the SDK is not initialised.
type Telemetry struct {
	// Enabled controls whether OpenTelemetry is initialised at all. When
	// nil or false, no exporter is started and no resource is initialised
	// beyond SDK defaults; the application receives no-op providers.
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// Exporter configures the OTLP exporter destination.
	Exporter *TelemetryExporter `json:"exporter,omitempty" yaml:"exporter,omitempty"`

	// Resource configures the OTel resource attributes attached to every
	// emitted telemetry signal.
	Resource *TelemetryResource `json:"resource,omitempty" yaml:"resource,omitempty"`

	// Sampling controls trace sampling behaviour.
	Sampling *TelemetrySampling `json:"sampling,omitempty" yaml:"sampling,omitempty"`

	// Signals controls which OTel signals are emitted.
	Signals *TelemetrySignals `json:"signals,omitempty" yaml:"signals,omitempty"`

	// HTTP configures behaviour of inbound HTTP instrumentation (used by
	// later instrumentation tickets; carried in this config block so the
	// schema is complete).
	HTTP *TelemetryHTTP `json:"http,omitempty" yaml:"http,omitempty"`

	// Proxy configures label projection for proxy telemetry. The two
	// allowlists are applied independently against the request's already-
	// computed effective label set; telemetry does no merging of its own.
	Proxy *TelemetryProxy `json:"proxy,omitempty" yaml:"proxy,omitempty"`

	// Propagation configures W3C trace context injection on outbound calls.
	// Outbound propagation is opt-in.
	Propagation *TelemetryPropagation `json:"propagation,omitempty" yaml:"propagation,omitempty"`
}

// TelemetryExporter configures the OTLP exporter destination shared by all
// signals. The Collector is expected to fan out to backends.
type TelemetryExporter struct {
	// Protocol selects the OTLP transport. One of "grpc" or "http/protobuf".
	Protocol *TelemetryExporterProtocol `json:"protocol,omitempty" yaml:"protocol,omitempty"`

	// Endpoint is the OTLP endpoint URL. Honors env-var fallthrough; falls
	// back to OTEL_EXPORTER_OTLP_ENDPOINT when unset.
	Endpoint *StringValue `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`

	// Headers are additional headers sent with every OTLP request (e.g.
	// auth tokens). Each value supports env-var fallthrough.
	Headers map[string]*StringValue `json:"headers,omitempty" yaml:"headers,omitempty"`

	// Insecure disables TLS on the OTLP connection. Defaults to false.
	Insecure *bool `json:"insecure,omitempty" yaml:"insecure,omitempty"`
}

// TelemetryResource configures static resource attributes attached to every
// emitted signal.
type TelemetryResource struct {
	// ServiceNamePrefix is prepended (with a hyphen) to the service id to
	// produce the OTel service.name. Defaults to "authproxy" so services
	// appear as "authproxy-api", "authproxy-admin-api", etc.
	ServiceNamePrefix *string `json:"service_name_prefix,omitempty" yaml:"service_name_prefix,omitempty"`

	// Attributes are user-supplied resource attributes merged into the
	// resource. OTEL_RESOURCE_ATTRIBUTES env var entries are merged in
	// addition to these (SDK behaviour).
	Attributes map[string]string `json:"attributes,omitempty" yaml:"attributes,omitempty"`
}

// TelemetrySampling configures the sampler used for traces.
type TelemetrySampling struct {
	// Ratio is the parent-based head-sampling ratio in [0, 1]. Sampling
	// decisions made by upstream callers are honored; this ratio applies
	// when there is no parent context.
	Ratio *float64 `json:"ratio,omitempty" yaml:"ratio,omitempty"`
}

// TelemetrySignals enables or disables individual OTel signals.
type TelemetrySignals struct {
	Traces  *bool `json:"traces,omitempty" yaml:"traces,omitempty"`
	Metrics *bool `json:"metrics,omitempty" yaml:"metrics,omitempty"`
	Logs    *bool `json:"logs,omitempty" yaml:"logs,omitempty"`
}

// TelemetryHTTP configures inbound HTTP instrumentation behaviour.
type TelemetryHTTP struct {
	// ExcludedPaths is the list of HTTP request paths to exclude from
	// spans and metrics. Defaults to /ping and /healthz when nil. An empty
	// slice (explicitly configured) disables exclusion entirely.
	ExcludedPaths []string `json:"excluded_paths,omitempty" yaml:"excluded_paths,omitempty"`
}

// TelemetryProxy configures label projection for outbound proxy telemetry.
//
// The two allowlists are independent. Telemetry reads from the request's
// already-computed effective label set (httpf.RequestInfo.Labels) and filters
// by allowlist; it does no merging.
type TelemetryProxy struct {
	// SpanAttributeLabels are label keys whose values are projected as
	// attributes on the proxy span. Cheap; can tolerate higher cardinality.
	SpanAttributeLabels []string `json:"span_attribute_labels,omitempty" yaml:"span_attribute_labels,omitempty"`

	// MetricDimensionLabels are label keys whose values become metric
	// dimensions on proxy metrics. Strictly bounded to control cardinality.
	MetricDimensionLabels []string `json:"metric_dimension_labels,omitempty" yaml:"metric_dimension_labels,omitempty"`

	// MetricDimensionValueCap, when > 0, caps the number of distinct values
	// per metric dimension key. Values beyond the cap collapse to "other"
	// to bound cardinality. Off by default.
	MetricDimensionValueCap *int `json:"metric_dimension_value_cap,omitempty" yaml:"metric_dimension_value_cap,omitempty"`
}

// TelemetryPropagation configures outbound trace context injection.
type TelemetryPropagation struct {
	// InjectOutboundDefault is the global default for injecting W3C
	// traceparent/tracestate on outbound proxy requests. Defaults to false
	// (opt-in). Per-connector settings override this default.
	InjectOutboundDefault *bool `json:"inject_outbound_default,omitempty" yaml:"inject_outbound_default,omitempty"`
}

// IsEnabled reports whether the telemetry block is set and Enabled is true.
// Treats nil and explicit false as disabled.
func (t *Telemetry) IsEnabled() bool {
	if t == nil || t.Enabled == nil {
		return false
	}
	return *t.Enabled
}

// GetExporter returns the exporter sub-block, or a zero value if unset.
func (t *Telemetry) GetExporter() *TelemetryExporter {
	if t == nil || t.Exporter == nil {
		return &TelemetryExporter{}
	}
	return t.Exporter
}

// GetResource returns the resource sub-block, or a zero value if unset.
func (t *Telemetry) GetResource() *TelemetryResource {
	if t == nil || t.Resource == nil {
		return &TelemetryResource{}
	}
	return t.Resource
}

// GetSamplingRatio returns the configured sampling ratio, falling back to
// DefaultTelemetrySamplingRatio when unset. Bounded to [0, 1].
func (t *Telemetry) GetSamplingRatio() float64 {
	if t == nil || t.Sampling == nil || t.Sampling.Ratio == nil {
		return DefaultTelemetrySamplingRatio
	}
	r := *t.Sampling.Ratio
	if r < 0 {
		return 0
	}
	if r > 1 {
		return 1
	}
	return r
}

// TracesEnabled reports whether trace emission is enabled. Defaults to true
// when telemetry is enabled and the signals block is absent.
func (t *Telemetry) TracesEnabled() bool {
	if !t.IsEnabled() {
		return false
	}
	if t.Signals == nil || t.Signals.Traces == nil {
		return true
	}
	return *t.Signals.Traces
}

// MetricsEnabled reports whether metric emission is enabled. Defaults to true
// when telemetry is enabled and the signals block is absent.
func (t *Telemetry) MetricsEnabled() bool {
	if !t.IsEnabled() {
		return false
	}
	if t.Signals == nil || t.Signals.Metrics == nil {
		return true
	}
	return *t.Signals.Metrics
}

// LogsEnabled reports whether log emission is enabled. Defaults to true when
// telemetry is enabled and the signals block is absent.
func (t *Telemetry) LogsEnabled() bool {
	if !t.IsEnabled() {
		return false
	}
	if t.Signals == nil || t.Signals.Logs == nil {
		return true
	}
	return *t.Signals.Logs
}

// GetHTTPExcludedPaths returns the configured exclusion list, falling back to
// DefaultTelemetryHTTPExcludedPaths when nil. An explicitly empty slice in
// config disables exclusion entirely.
func (t *Telemetry) GetHTTPExcludedPaths() []string {
	if t == nil || t.HTTP == nil || t.HTTP.ExcludedPaths == nil {
		out := make([]string, len(DefaultTelemetryHTTPExcludedPaths))
		copy(out, DefaultTelemetryHTTPExcludedPaths)
		return out
	}
	return t.HTTP.ExcludedPaths
}

// GetProxy returns the proxy sub-block, or a zero value if unset.
func (t *Telemetry) GetProxy() *TelemetryProxy {
	if t == nil || t.Proxy == nil {
		return &TelemetryProxy{}
	}
	return t.Proxy
}

// InjectOutboundDefault reports the global default for outbound trace context
// injection. Defaults to false (opt-in).
func (t *Telemetry) InjectOutboundDefault() bool {
	if t == nil || t.Propagation == nil || t.Propagation.InjectOutboundDefault == nil {
		return false
	}
	return *t.Propagation.InjectOutboundDefault
}

// GetProtocol returns the OTLP protocol, defaulting to grpc.
func (e *TelemetryExporter) GetProtocol() TelemetryExporterProtocol {
	if e == nil || e.Protocol == nil {
		return TelemetryExporterProtocolGRPC
	}
	return *e.Protocol
}

// GetInsecure returns the configured insecure flag, defaulting to false.
func (e *TelemetryExporter) GetInsecure() bool {
	if e == nil || e.Insecure == nil {
		return false
	}
	return *e.Insecure
}

// GetServiceNamePrefix returns the configured prefix, falling back to
// DefaultTelemetryServiceNamePrefix.
func (r *TelemetryResource) GetServiceNamePrefix() string {
	if r == nil || r.ServiceNamePrefix == nil || *r.ServiceNamePrefix == "" {
		return DefaultTelemetryServiceNamePrefix
	}
	return *r.ServiceNamePrefix
}

// Validate checks the telemetry block. Validation is permissive when the block
// is disabled; only fields that affect runtime behaviour when enabled are
// strictly checked.
func (t *Telemetry) Validate(vc *common.ValidationContext) error {
	if t == nil {
		return nil
	}

	result := &multierror.Error{}

	if t.Exporter != nil {
		if err := t.Exporter.Validate(vc.PushField("exporter")); err != nil {
			result = multierror.Append(result, err)
		}
	}

	if t.Sampling != nil && t.Sampling.Ratio != nil {
		r := *t.Sampling.Ratio
		if r < 0 || r > 1 {
			result = multierror.Append(result,
				vc.NewErrorfForField("sampling.ratio", "must be in [0, 1], got %v", r))
		}
	}

	if t.Resource != nil && t.Resource.ServiceNamePrefix != nil {
		prefix := *t.Resource.ServiceNamePrefix
		if strings.ContainsAny(prefix, " \t\n\r") {
			result = multierror.Append(result,
				vc.NewErrorForField("resource.service_name_prefix", "must not contain whitespace"))
		}
	}

	if t.HTTP != nil {
		for i, p := range t.HTTP.ExcludedPaths {
			if !strings.HasPrefix(p, "/") {
				result = multierror.Append(result,
					vc.PushField("http.excluded_paths").PushIndex(i).NewErrorf("path must begin with '/', got %q", p))
			}
		}
	}

	if t.Proxy != nil && t.Proxy.MetricDimensionValueCap != nil && *t.Proxy.MetricDimensionValueCap < 0 {
		result = multierror.Append(result,
			vc.NewErrorfForField("proxy.metric_dimension_value_cap", "must be >= 0, got %d", *t.Proxy.MetricDimensionValueCap))
	}

	return result.ErrorOrNil()
}

// Validate checks the exporter block.
func (e *TelemetryExporter) Validate(vc *common.ValidationContext) error {
	if e == nil {
		return nil
	}

	result := &multierror.Error{}

	if e.Protocol != nil {
		switch *e.Protocol {
		case TelemetryExporterProtocolGRPC, TelemetryExporterProtocolHTTPProtobuf:
			// ok
		default:
			result = multierror.Append(result,
				vc.NewErrorfForField("protocol", "must be one of %q or %q, got %q",
					TelemetryExporterProtocolGRPC,
					TelemetryExporterProtocolHTTPProtobuf,
					*e.Protocol))
		}
	}

	return result.ErrorOrNil()
}
