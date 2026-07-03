package core

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/require"
)

func TestApplyMigrationHookPatchRejectsSystemLabels(t *testing.T) {
	connID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	candidate := &connectionMigrationCandidate{
		Connection: &connection{Connection: database.Connection{
			Id:               connID,
			Namespace:        "root",
			ConnectorId:      connectorID,
			ConnectorVersion: 1,
		}},
		Config:      map[string]any{},
		UserLabels:  map[string]string{},
		Annotations: map[string]string{},
	}

	err := applyMigrationHookPatch(candidate, nil, 1, 2, migrationHookPatch{
		Labels: migrationStringPatch{
			Set: map[string]string{"apxy/cxr/type": "bad"},
		},
	})
	require.Error(t, err)
}

func TestApplyMigrationHookPatchNotificationKeys(t *testing.T) {
	connID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	version := &ConnectorVersion{ConnectorVersion: database.ConnectorVersion{
		Id:      connectorID,
		Version: 2,
	}}
	candidate := &connectionMigrationCandidate{
		Connection: &connection{Connection: database.Connection{
			Id:               connID,
			Namespace:        "root",
			ConnectorId:      connectorID,
			ConnectorVersion: 1,
		}},
		Config:      map[string]any{},
		UserLabels:  map[string]string{},
		Annotations: map[string]string{},
	}

	err := applyMigrationHookPatch(candidate, version, 1, 2, migrationHookPatch{
		Config: migrationAnyPatch{Set: map[string]any{"team": "platform"}},
		Notifications: []migrationNotificationDef{{
			Key:     "heads-up",
			Level:   string(database.NotificationLevelInfo),
			Title:   "Heads up",
			Message: "A migration happened.",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, "platform", candidate.Config["team"])
	require.Len(t, candidate.Notifications, 1)
	require.Equal(t, "connector_migration:"+connID.String()+":1:2:heads-up", candidate.Notifications[0].Key)
	require.Equal(t, candidate.Notifications[0].Key, candidate.NotificationKeys[0])
}
