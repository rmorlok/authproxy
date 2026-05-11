package connectors

// ConnectorTelemetry carries per-connector overrides for the OpenTelemetry
// behaviour applied to outbound calls routed through this connector. The
// global defaults for these knobs live in the top-level telemetry: config
// block; values here override the global default for this connector only.
type ConnectorTelemetry struct {
	// PropagateTraceContext, when non-nil, sets the per-connector decision
	// for W3C traceparent / tracestate injection on outbound proxy requests.
	// Use this to opt a single connector in (true) or out (false) of the
	// global telemetry.propagation.inject_outbound_default. Leave nil to
	// inherit the global default.
	//
	// Reason this is opt-in by default: third-party services may reject
	// unknown headers, log them in ways that surface internal trace IDs, or
	// use them in unexpected ways. Only enable when the destination is
	// known to handle W3C trace context gracefully.
	PropagateTraceContext *bool `json:"propagate_trace_context,omitempty" yaml:"propagate_trace_context,omitempty"`
}

// Clone returns a deep copy. Safe to call on nil.
func (ct *ConnectorTelemetry) Clone() *ConnectorTelemetry {
	if ct == nil {
		return nil
	}
	clone := *ct
	if ct.PropagateTraceContext != nil {
		v := *ct.PropagateTraceContext
		clone.PropagateTraceContext = &v
	}
	return &clone
}
