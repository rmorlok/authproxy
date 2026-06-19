package core

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeKeepMinimum_UsesLargerThreshold(t *testing.T) {
	p := &cschema.Probe{
		FailureThreshold:  intPtr(5),
		RecoveryThreshold: intPtr(2),
	}
	assert.Equal(t, 5, probeKeepMinimum(p))

	p = &cschema.Probe{
		FailureThreshold:  intPtr(1),
		RecoveryThreshold: intPtr(10),
	}
	assert.Equal(t, 10, probeKeepMinimum(p))
}

func TestProbeKeepMinimum_UsesDefaultsWhenUnset(t *testing.T) {
	p := &cschema.Probe{}
	want := cschema.DefaultProbeFailureThreshold
	if cschema.DefaultProbeRecoveryThreshold > want {
		want = cschema.DefaultProbeRecoveryThreshold
	}
	assert.Equal(t, want, probeKeepMinimum(p))
}

func TestEnabledProbeThresholdsForConnection_ExcludesDisabledProbes(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "cleanup-conditional",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		Probes: []cschema.Probe{
			{
				Id:               "enabled",
				FailureThreshold: intPtr(5),
			},
			{
				Id:               "disabled",
				If:               &common.Predicate{Javascript: `cfg.keep_disabled === true`},
				FailureThreshold: intPtr(10),
			},
		},
	}
	conn := &database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateConfigured,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
	}

	svc, _, _, ctrl := setupVerifyTest(t, connectionId, conn, connector)
	defer ctrl.Finish()

	thresholds, err := svc.enabledProbeThresholdsForConnection(context.Background(), connectionId)
	require.NoError(t, err)
	require.Contains(t, thresholds, "enabled")
	assert.NotContains(t, thresholds, "disabled")
	assert.Equal(t, 5, probeKeepMinimum(thresholds["enabled"]))
}
