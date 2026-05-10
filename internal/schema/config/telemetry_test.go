package config

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTelemetry_Defaults(t *testing.T) {
	t.Run("nil telemetry behaves as disabled", func(t *testing.T) {
		var tel *Telemetry
		require.False(t, tel.IsEnabled())
		require.False(t, tel.TracesEnabled())
		require.False(t, tel.MetricsEnabled())
		require.False(t, tel.LogsEnabled())
		require.False(t, tel.InjectOutboundDefault())
		require.Equal(t, DefaultTelemetrySamplingRatio, tel.GetSamplingRatio())
		require.Equal(t, DefaultTelemetryHTTPExcludedPaths, tel.GetHTTPExcludedPaths())
		require.NotNil(t, tel.GetExporter())
		require.NotNil(t, tel.GetResource())
		require.NotNil(t, tel.GetProxy())
	})

	t.Run("enabled with empty signals defaults to all signals on", func(t *testing.T) {
		on := true
		tel := &Telemetry{Enabled: &on}
		require.True(t, tel.IsEnabled())
		require.True(t, tel.TracesEnabled())
		require.True(t, tel.MetricsEnabled())
		require.True(t, tel.LogsEnabled())
	})

	t.Run("disabled blocks all signals regardless of subblock", func(t *testing.T) {
		off := false
		on := true
		tel := &Telemetry{
			Enabled: &off,
			Signals: &TelemetrySignals{Traces: &on, Metrics: &on, Logs: &on},
		}
		require.False(t, tel.TracesEnabled())
		require.False(t, tel.MetricsEnabled())
		require.False(t, tel.LogsEnabled())
	})

	t.Run("ratio clamps to [0, 1]", func(t *testing.T) {
		over := 5.0
		under := -1.0
		mid := 0.25

		require.Equal(t, 1.0, (&Telemetry{Sampling: &TelemetrySampling{Ratio: &over}}).GetSamplingRatio())
		require.Equal(t, 0.0, (&Telemetry{Sampling: &TelemetrySampling{Ratio: &under}}).GetSamplingRatio())
		require.Equal(t, mid, (&Telemetry{Sampling: &TelemetrySampling{Ratio: &mid}}).GetSamplingRatio())
	})

	t.Run("explicit empty excluded_paths disables exclusion", func(t *testing.T) {
		tel := &Telemetry{HTTP: &TelemetryHTTP{ExcludedPaths: []string{}}}
		require.Equal(t, []string{}, tel.GetHTTPExcludedPaths())
	})

	t.Run("explicit excluded_paths overrides default", func(t *testing.T) {
		tel := &Telemetry{HTTP: &TelemetryHTTP{ExcludedPaths: []string{"/livez"}}}
		require.Equal(t, []string{"/livez"}, tel.GetHTTPExcludedPaths())
	})

	t.Run("service_name_prefix defaults to authproxy", func(t *testing.T) {
		require.Equal(t, DefaultTelemetryServiceNamePrefix, (&TelemetryResource{}).GetServiceNamePrefix())
		empty := ""
		require.Equal(t, DefaultTelemetryServiceNamePrefix, (&TelemetryResource{ServiceNamePrefix: &empty}).GetServiceNamePrefix())
		custom := "myapp"
		require.Equal(t, "myapp", (&TelemetryResource{ServiceNamePrefix: &custom}).GetServiceNamePrefix())
	})

	t.Run("exporter protocol defaults to grpc", func(t *testing.T) {
		var ec *TelemetryExporter
		require.Equal(t, TelemetryExporterProtocolGRPC, ec.GetProtocol())
		require.Equal(t, TelemetryExporterProtocolGRPC, (&TelemetryExporter{}).GetProtocol())
	})

	t.Run("propagation default is opt-in (false)", func(t *testing.T) {
		require.False(t, (&Telemetry{}).InjectOutboundDefault())
		on := true
		tel := &Telemetry{Propagation: &TelemetryPropagation{InjectOutboundDefault: &on}}
		require.True(t, tel.InjectOutboundDefault())
	})
}

func TestTelemetry_YamlParse(t *testing.T) {
	yamlData := `
enabled: true
exporter:
  protocol: grpc
  endpoint:
    env_var: OTEL_EXPORTER_OTLP_ENDPOINT
    default: http://localhost:4317
  insecure: true
  headers:
    authorization:
      env_var: OTEL_HEADER_AUTH
      default: Bearer dev-token
resource:
  service_name_prefix: authproxy
  attributes:
    deployment.environment: dev
sampling:
  ratio: 0.5
signals:
  traces: true
  metrics: true
  logs: false
http:
  excluded_paths:
    - /ping
    - /healthz
    - /metrics
proxy:
  span_attribute_labels:
    - type
    - env
    - tenant_id
  metric_dimension_labels:
    - type
    - env
  metric_dimension_value_cap: 50
propagation:
  inject_outbound_default: false
`

	var tel Telemetry
	err := yaml.Unmarshal([]byte(yamlData), &tel)
	require.NoError(t, err)

	require.True(t, tel.IsEnabled())
	require.Equal(t, TelemetryExporterProtocolGRPC, tel.GetExporter().GetProtocol())
	require.True(t, tel.GetExporter().GetInsecure())

	require.NotNil(t, tel.Exporter)
	require.NotNil(t, tel.Exporter.Endpoint)
	endpoint, err := tel.Exporter.Endpoint.GetValue(context.Background())
	require.NoError(t, err)
	require.Equal(t, "http://localhost:4317", endpoint)

	require.Equal(t, "authproxy", tel.GetResource().GetServiceNamePrefix())
	require.Equal(t, "dev", tel.Resource.Attributes["deployment.environment"])

	require.Equal(t, 0.5, tel.GetSamplingRatio())

	require.True(t, tel.TracesEnabled())
	require.True(t, tel.MetricsEnabled())
	require.False(t, tel.LogsEnabled())

	require.Equal(t, []string{"/ping", "/healthz", "/metrics"}, tel.GetHTTPExcludedPaths())

	require.Equal(t, []string{"type", "env", "tenant_id"}, tel.GetProxy().SpanAttributeLabels)
	require.Equal(t, []string{"type", "env"}, tel.GetProxy().MetricDimensionLabels)
	require.NotNil(t, tel.GetProxy().MetricDimensionValueCap)
	require.Equal(t, 50, *tel.GetProxy().MetricDimensionValueCap)

	require.False(t, tel.InjectOutboundDefault())
}

func TestTelemetry_EnvVarOverride(t *testing.T) {
	t.Setenv("OTEL_TEST_ENDPOINT", "http://otel-collector:4317")

	yamlData := `
enabled: true
exporter:
  endpoint:
    env_var: OTEL_TEST_ENDPOINT
    default: http://localhost:4317
`
	var tel Telemetry
	require.NoError(t, yaml.Unmarshal([]byte(yamlData), &tel))

	endpoint, err := tel.Exporter.Endpoint.GetValue(context.Background())
	require.NoError(t, err)
	require.Equal(t, "http://otel-collector:4317", endpoint)
}

func TestTelemetry_EnvVarFallsBackToDefault(t *testing.T) {
	yamlData := `
enabled: true
exporter:
  endpoint:
    env_var: OTEL_TELEMETRY_TEST_UNSET_VAR
    default: http://localhost:4317
`
	var tel Telemetry
	require.NoError(t, yaml.Unmarshal([]byte(yamlData), &tel))

	endpoint, err := tel.Exporter.Endpoint.GetValue(context.Background())
	require.NoError(t, err)
	require.Equal(t, "http://localhost:4317", endpoint)
}

func TestTelemetry_Validate(t *testing.T) {
	vc := &common.ValidationContext{Path: "$.telemetry"}

	t.Run("nil is valid", func(t *testing.T) {
		var tel *Telemetry
		require.NoError(t, tel.Validate(vc))
	})

	t.Run("empty is valid", func(t *testing.T) {
		require.NoError(t, (&Telemetry{}).Validate(vc))
	})

	t.Run("ratio out of range fails", func(t *testing.T) {
		over := 2.0
		err := (&Telemetry{Sampling: &TelemetrySampling{Ratio: &over}}).Validate(vc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "sampling.ratio")
	})

	t.Run("invalid protocol fails", func(t *testing.T) {
		bad := TelemetryExporterProtocol("not-a-protocol")
		err := (&Telemetry{Exporter: &TelemetryExporter{Protocol: &bad}}).Validate(vc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "protocol")
	})

	t.Run("excluded path missing leading slash fails", func(t *testing.T) {
		err := (&Telemetry{HTTP: &TelemetryHTTP{ExcludedPaths: []string{"healthz"}}}).Validate(vc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "must begin with")
	})

	t.Run("negative metric dimension value cap fails", func(t *testing.T) {
		neg := -1
		err := (&Telemetry{Proxy: &TelemetryProxy{MetricDimensionValueCap: &neg}}).Validate(vc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "metric_dimension_value_cap")
	})

	t.Run("whitespace in service_name_prefix fails", func(t *testing.T) {
		bad := "auth proxy"
		err := (&Telemetry{Resource: &TelemetryResource{ServiceNamePrefix: &bad}}).Validate(vc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "service_name_prefix")
	})

	t.Run("valid full config passes", func(t *testing.T) {
		on := true
		ratio := 0.5
		cap := 50
		grpc := TelemetryExporterProtocolGRPC
		prefix := "authproxy"

		tel := &Telemetry{
			Enabled:  &on,
			Exporter: &TelemetryExporter{Protocol: &grpc, Insecure: &on},
			Resource: &TelemetryResource{ServiceNamePrefix: &prefix, Attributes: map[string]string{"env": "test"}},
			Sampling: &TelemetrySampling{Ratio: &ratio},
			Signals:  &TelemetrySignals{Traces: &on, Metrics: &on, Logs: &on},
			HTTP:     &TelemetryHTTP{ExcludedPaths: []string{"/ping", "/healthz"}},
			Proxy: &TelemetryProxy{
				SpanAttributeLabels:     []string{"type", "env"},
				MetricDimensionLabels:   []string{"type"},
				MetricDimensionValueCap: &cap,
			},
			Propagation: &TelemetryPropagation{InjectOutboundDefault: &on},
		}
		require.NoError(t, tel.Validate(vc))
	})
}
