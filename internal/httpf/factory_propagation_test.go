package httpf

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
)

// connectionWithPropagation is a stubConnection that also implements
// httpf.TracePropagationProvider — modelling a core.connection whose
// connector definition specifies telemetry.propagate_trace_context.
type connectionWithPropagation struct {
	stubConnection
	propagate *bool
}

func (c *connectionWithPropagation) PropagateTraceContext() *bool { return c.propagate }

func TestForConnectionPlumbsPropagationOverride(t *testing.T) {
	// When the Connection implements TracePropagationProvider, ForConnection
	// must copy the returned pointer into RequestInfo so downstream
	// middlewares (telemetryRoundTripper) can resolve the decision against
	// the global default.
	yes := true
	conn := &connectionWithPropagation{
		stubConnection: stubConnection{
			id:        apid.MustParse("cxn_test1234567890ab"),
			namespace: "root.foo",
			cvID:      apid.MustParse("cxr_test1234567890ab"),
			cvVersion: 1,
		},
		propagate: &yes,
	}

	f := newTestFactory().ForConnection(conn).(*clientFactory)
	require.NotNil(t, f.requestInfo.PropagateTraceContext)
	require.True(t, *f.requestInfo.PropagateTraceContext)
}

func TestForConnectionLeavesPropagationNilWhenNotProvided(t *testing.T) {
	// Connections that don't implement TracePropagationProvider, or that
	// return nil, must produce a nil RequestInfo.PropagateTraceContext —
	// i.e. "use the global default".
	conn := &stubConnection{
		id:        apid.MustParse("cxn_test1234567890ab"),
		namespace: "root.foo",
		cvID:      apid.MustParse("cxr_test1234567890ab"),
		cvVersion: 1,
	}

	f := newTestFactory().ForConnection(conn).(*clientFactory)
	require.Nil(t, f.requestInfo.PropagateTraceContext)
}

func TestForConnectionPropagationOverrideRespectsExplicitFalse(t *testing.T) {
	// Connections opting OUT (explicit false) must be preserved through to
	// RequestInfo so the roundtripper can distinguish "use global default"
	// (nil) from "explicit opt out" (false).
	no := false
	conn := &connectionWithPropagation{
		stubConnection: stubConnection{
			id:        apid.MustParse("cxn_test1234567890ab"),
			namespace: "root.foo",
		},
		propagate: &no,
	}

	f := newTestFactory().ForConnection(conn).(*clientFactory)
	require.NotNil(t, f.requestInfo.PropagateTraceContext)
	require.False(t, *f.requestInfo.PropagateTraceContext)
}
