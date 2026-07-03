package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestUpdateConnectionForVersionMigration(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	now := time.Date(2026, time.July, 3, 12, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	require.NoError(t, db.CreateNamespace(ctx, &Namespace{
		Path:   "root.tenant",
		State:  NamespaceStateActive,
		Labels: Labels{"tier": "pro"},
	}))

	connectorID := apid.New(apid.PrefixConnectorVersion)
	require.NoError(t, db.UpsertConnectorVersion(ctx, &ConnectorVersion{
		Id:                  connectorID,
		Version:             1,
		Namespace:           "root.tenant",
		State:               ConnectorVersionStateDraft,
		Labels:              Labels{"type": "v1"},
		Hash:                "h1",
		EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000001"), Data: "d1"},
	}))
	require.NoError(t, db.UpsertConnectorVersion(ctx, &ConnectorVersion{
		Id:                  connectorID,
		Version:             2,
		Namespace:           "root.tenant",
		State:               ConnectorVersionStatePrimary,
		Labels:              Labels{"type": "v2"},
		Hash:                "h2",
		EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("dek_test000000000002"), Data: "d2"},
	}))

	connID := apid.New(apid.PrefixConnection)
	require.NoError(t, db.CreateConnection(ctx, &Connection{
		Id:               connID,
		Namespace:        "root.tenant",
		ConnectorId:      connectorID,
		ConnectorVersion: 1,
		State:            ConnectionStateConfigured,
		Labels:           Labels{"subscription": "gold"},
		Annotations:      Annotations{"note": "old"},
	}))

	step := cschema.MustNewSetupStep("configure-team")
	health := ConnectionHealthStateUnhealthy
	updated, err := db.UpdateConnectionForVersionMigration(ctx, ConnectionVersionMigrationUpdate{
		Id:                     connID,
		ConnectorId:            connectorID,
		ConnectorVersion:       2,
		EncryptedConfiguration: &encfield.EncryptedField{ID: apid.MustParse("dek_test000000000003"), Data: "cfg"},
		UserLabels:             map[string]string{"subscription": "platinum"},
		Annotations:            map[string]string{"note": "new"},
		SetupStep:              &step,
		HealthState:            &health,
	})
	require.NoError(t, err)

	require.Equal(t, uint64(2), updated.ConnectorVersion)
	require.Equal(t, "platinum", updated.Labels["subscription"])
	require.Equal(t, "v2", updated.Labels["apxy/cxr/type"])
	require.Equal(t, "pro", updated.Labels["apxy/ns/tier"])
	require.Equal(t, "new", updated.Annotations["note"])
	require.Equal(t, ConnectionHealthStateUnhealthy, updated.HealthState)
	require.NotNil(t, updated.SetupStep)
	require.Equal(t, "configure-team", updated.SetupStep.String())
}
