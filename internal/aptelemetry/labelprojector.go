package aptelemetry

import (
	"sync"

	"go.opentelemetry.io/otel/attribute"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// LabelValueOther is the placeholder substituted for label values that exceed
// the configured per-key cap. Bounds the cardinality of metric streams under
// runaway label-value churn.
const LabelValueOther = "other"

// LabelProjector applies the two independent allowlists from #232 +
// optional value-cap to an arbitrary set of labels, producing the
// span-attribute / metric-dimension projections used by subsystem
// telemetry (httpf outbound proxy, oauth2 lifecycle, etc.).
//
// The projector itself is stateful only for the value-cap counters
// (per-key bounded set of admitted values). Each subsystem holds its own
// projector instance — caps are per-subsystem-per-key, which keeps the
// type self-contained and avoids cross-subsystem coupling.
type LabelProjector struct {
	spanKeys   []string
	metricKeys []string
	valueCap   int

	// valueSeen tracks the bounded set of values observed per metric
	// dimension key for cap enforcement. Once a key's set reaches
	// valueCap, further distinct values collapse to LabelValueOther.
	// The cap is per process; in a multi-replica deployment the cap
	// applies independently on each replica.
	valueSeenMu sync.RWMutex
	valueSeen   map[string]map[string]struct{}
}

// NewLabelProjector constructs a projector from the explicit allowlists and
// optional cap. Pass cap=0 to disable the cap (every value passes through).
// Safe to call with empty / nil allowlists — the corresponding projection
// method returns nil with no work.
func NewLabelProjector(spanKeys, metricKeys []string, valueCap int) *LabelProjector {
	p := &LabelProjector{
		valueCap: valueCap,
	}
	p.spanKeys = append(p.spanKeys, spanKeys...)
	p.metricKeys = append(p.metricKeys, metricKeys...)
	if valueCap > 0 {
		p.valueSeen = make(map[string]map[string]struct{}, len(p.metricKeys))
	}
	return p
}

// NewLabelProjectorFromProxyConfig is a convenience constructor that pulls
// the allowlists + cap directly from the telemetry.proxy.* config block.
// The proxy block is the project-wide source of truth for metric-dimension
// cardinality controls — see telemetry.proxy.{span_attribute_labels,
// metric_dimension_labels, metric_dimension_value_cap} in #225.
func NewLabelProjectorFromProxyConfig(cfg *sconfig.TelemetryProxy) *LabelProjector {
	if cfg == nil {
		return NewLabelProjector(nil, nil, 0)
	}
	cap := 0
	if cfg.MetricDimensionValueCap != nil && *cfg.MetricDimensionValueCap > 0 {
		cap = *cfg.MetricDimensionValueCap
	}
	return NewLabelProjector(cfg.SpanAttributeLabels, cfg.MetricDimensionLabels, cap)
}

// SpanAttrs projects allowlisted labels onto span attributes. Keys not
// present in labels produce no attribute. The label key is reported verbatim
// (no namespacing) — applications choose label keys that won't collide with
// reserved OTel attributes.
//
// Safe to call on a nil receiver — returns nil.
func (p *LabelProjector) SpanAttrs(labels map[string]string) []attribute.KeyValue {
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

// MetricDims projects allowlisted labels onto metric dimensions, applying
// the configured value cap. Keys not present in labels produce no dimension.
//
// Safe to call on a nil receiver — returns nil.
func (p *LabelProjector) MetricDims(labels map[string]string) []attribute.KeyValue {
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
func (p *LabelProjector) cappedValue(key, value string) string {
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
