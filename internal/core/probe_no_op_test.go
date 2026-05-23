package core

import (
	"context"
	"log/slog"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProbeNoOp_Invoke covers the no-op probe variant: NewProbe falls back
// to probeNoOp when the connector declares neither http nor proxy_http.
// Every invocation should be reported as success with no error so the
// connector's health signal stays healthy by default when no real probe is
// configured.
func TestProbeNoOp_Invoke(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	s, _, _, _, _, _ := FullMockService(t, ctrl)
	cv := NewTestConnectorVersion(cschema.Connector{})
	logger := slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	conn := &connection{
		Connection: database.Connection{
			Id:               apid.New(apid.PrefixConnection),
			Namespace:        "root",
			State:            database.ConnectionStateReady,
			HealthState:      database.ConnectionHealthStateHealthy,
			ConnectorId:      cv.GetId(),
			ConnectorVersion: cv.GetVersion(),
		},
		s:      s,
		cv:     cv,
		logger: logger,
	}

	// No Http or ProxyHttp on the probe definition → NewProbe returns the
	// no-op probe variant.
	probeCfg := &cschema.Probe{Id: "noop"}
	probe := NewProbe(probeCfg, s, cv, conn)

	outcome, err := probe.Invoke(context.Background())
	require.NoError(t, err)
	assert.Equal(t, ProbeOutcomeSuccess, outcome)

	// Invariant should hold across repeated invocations — the no-op probe
	// has no state that could drift.
	for i := 0; i < 3; i++ {
		outcome, err := probe.Invoke(context.Background())
		require.NoError(t, err)
		assert.Equal(t, ProbeOutcomeSuccess, outcome)
	}
}
