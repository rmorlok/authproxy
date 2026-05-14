package aptelemetry

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

func TestLabelProjector_RequestOverrideWinsInBothAllowlists(t *testing.T) {
	// Acceptance criterion #5 from #232: a request-supplied label override
	// MUST win over a connection-inherited value of the same key in both
	// projected span attrs AND metric dims. The projector itself reads from
	// a single map — the merging is upstream's job. This test pins that
	// contract by feeding the already-merged map (with the request value
	// present) and asserting the projection takes that value, not whatever
	// was originally on the connection.
	cap := 0
	cfg := &sconfig.TelemetryProxy{
		SpanAttributeLabels:     []string{"tenant_id", "env"},
		MetricDimensionLabels:   []string{"tenant_id", "env"},
		MetricDimensionValueCap: &cap,
	}
	p := NewLabelProjectorFromProxyConfig(cfg)

	// Effective set as produced by the upstream merge — request value
	// (tenant_id=req-1) has already replaced the connection value.
	effective := map[string]string{
		"tenant_id": "req-1",
		"env":       "prod",
	}

	spanAttrs := p.SpanAttrs(effective)
	require.Equal(t, "req-1", findAttr(t, spanAttrs, "tenant_id"))
	require.Equal(t, "prod", findAttr(t, spanAttrs, "env"))

	metricAttrs := p.MetricDims(effective)
	require.Equal(t, "req-1", findAttr(t, metricAttrs, "tenant_id"))
	require.Equal(t, "prod", findAttr(t, metricAttrs, "env"))
}

func TestLabelProjector_AllowlistsAreIndependent(t *testing.T) {
	cfg := &sconfig.TelemetryProxy{
		SpanAttributeLabels:   []string{"tenant_id"},
		MetricDimensionLabels: []string{"env"},
	}
	p := NewLabelProjectorFromProxyConfig(cfg)

	labels := map[string]string{"tenant_id": "t1", "env": "prod"}

	spanAttrs := p.SpanAttrs(labels)
	require.Len(t, spanAttrs, 1, "only span-allowlisted keys appear on spans")
	require.Equal(t, "t1", findAttr(t, spanAttrs, "tenant_id"))

	metricAttrs := p.MetricDims(labels)
	require.Len(t, metricAttrs, 1, "only metric-allowlisted keys appear on metrics")
	require.Equal(t, "prod", findAttr(t, metricAttrs, "env"))
}

func TestLabelProjector_UnlistedKeysDropped(t *testing.T) {
	cfg := &sconfig.TelemetryProxy{
		SpanAttributeLabels:   []string{"tenant_id"},
		MetricDimensionLabels: []string{"tenant_id"},
	}
	p := NewLabelProjectorFromProxyConfig(cfg)

	labels := map[string]string{"tenant_id": "t1", "secret": "leak"}

	spanAttrs := p.SpanAttrs(labels)
	require.Equal(t, "", findAttr(t, spanAttrs, "secret"), "unlisted key must never appear on spans")

	metricAttrs := p.MetricDims(labels)
	require.Equal(t, "", findAttr(t, metricAttrs, "secret"), "unlisted key must never appear on metrics")
}

func TestLabelProjector_MissingKeyAbsentNotEmptyString(t *testing.T) {
	cfg := &sconfig.TelemetryProxy{
		SpanAttributeLabels:   []string{"tenant_id", "env"},
		MetricDimensionLabels: []string{"tenant_id", "env"},
	}
	p := NewLabelProjectorFromProxyConfig(cfg)

	labels := map[string]string{"tenant_id": "t1"} // env intentionally missing

	spanAttrs := p.SpanAttrs(labels)
	require.Len(t, spanAttrs, 1, "missing key must produce no attribute, not an empty-string attribute")
	require.Equal(t, "t1", findAttr(t, spanAttrs, "tenant_id"))

	metricAttrs := p.MetricDims(labels)
	require.Len(t, metricAttrs, 1)
	require.Equal(t, "t1", findAttr(t, metricAttrs, "tenant_id"))
}

func TestLabelProjector_ValueCapCollapsesToOther(t *testing.T) {
	cap := 2
	cfg := &sconfig.TelemetryProxy{
		MetricDimensionLabels:   []string{"tenant_id"},
		MetricDimensionValueCap: &cap,
	}
	p := NewLabelProjectorFromProxyConfig(cfg)

	// First two distinct values are admitted as-is.
	require.Equal(t, "a", findAttr(t, p.MetricDims(map[string]string{"tenant_id": "a"}), "tenant_id"))
	require.Equal(t, "b", findAttr(t, p.MetricDims(map[string]string{"tenant_id": "b"}), "tenant_id"))

	// Third distinct value collapses.
	require.Equal(t, LabelValueOther, findAttr(t, p.MetricDims(map[string]string{"tenant_id": "c"}), "tenant_id"))

	// Previously-admitted values still pass through verbatim — cap only
	// applies to NEW distinct values.
	require.Equal(t, "a", findAttr(t, p.MetricDims(map[string]string{"tenant_id": "a"}), "tenant_id"))
}

func TestLabelProjector_ValueCapAppliesPerKey(t *testing.T) {
	// Each key has its own cap budget.
	cap := 1
	cfg := &sconfig.TelemetryProxy{
		MetricDimensionLabels:   []string{"tenant_id", "env"},
		MetricDimensionValueCap: &cap,
	}
	p := NewLabelProjectorFromProxyConfig(cfg)

	one := p.MetricDims(map[string]string{"tenant_id": "a", "env": "prod"})
	require.Equal(t, "a", findAttr(t, one, "tenant_id"))
	require.Equal(t, "prod", findAttr(t, one, "env"))

	two := p.MetricDims(map[string]string{"tenant_id": "b", "env": "staging"})
	require.Equal(t, LabelValueOther, findAttr(t, two, "tenant_id"), "tenant_id cap is exhausted")
	require.Equal(t, LabelValueOther, findAttr(t, two, "env"), "env cap is exhausted")
}

func TestLabelProjector_NoCapWhenUnset(t *testing.T) {
	cfg := &sconfig.TelemetryProxy{
		MetricDimensionLabels: []string{"tenant_id"},
	}
	p := NewLabelProjectorFromProxyConfig(cfg)

	for _, v := range []string{"a", "b", "c", "d", "e"} {
		require.Equal(t, v, findAttr(t, p.MetricDims(map[string]string{"tenant_id": v}), "tenant_id"),
			"with no cap configured, every value should pass through verbatim")
	}
}

func TestLabelProjector_NilReceiverIsSafe(t *testing.T) {
	// Defensive: both projection methods must tolerate a nil receiver so
	// subsystems that hold an optional *LabelProjector don't need to nil-
	// guard every call site.
	var p *LabelProjector
	require.Nil(t, p.SpanAttrs(map[string]string{"x": "y"}))
	require.Nil(t, p.MetricDims(map[string]string{"x": "y"}))
}

func TestNewLabelProjectorFromProxyConfig_NilConfigSafe(t *testing.T) {
	// Constructing from a nil proxy config produces a non-nil projector
	// that emits no attributes / dimensions — the same shape as a fully-
	// disabled allowlist.
	p := NewLabelProjectorFromProxyConfig(nil)
	require.NotNil(t, p)
	require.Nil(t, p.SpanAttrs(map[string]string{"x": "y"}))
	require.Nil(t, p.MetricDims(map[string]string{"x": "y"}))
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
