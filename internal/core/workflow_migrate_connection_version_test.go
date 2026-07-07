package core

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
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

	err := applyMigrationHookPatch(candidate, migrationHookPatch{
		Labels: migrationStringPatch{
			Set: map[string]string{"apxy/cxr/type": "bad"},
		},
	})
	require.Error(t, err)
}

func TestApplyMigrationHookPatchKeepsHighestPriorityNotification(t *testing.T) {
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

	err := applyMigrationHookPatch(candidate, migrationHookPatch{
		Config: migrationAnyPatch{Set: map[string]any{"team": "platform"}},
		Notifications: migrationNotificationPatch{
			Set: []migrationNotificationDef{{
				Key:     "heads-up",
				Level:   string(database.NotificationLevelInfo),
				Title:   "Heads up",
				Message: "A migration happened.",
			}, {
				Key:     "pay-attention",
				Level:   string(database.NotificationLevelWarning),
				Title:   "Pay attention",
				Message: "A migration needs attention.",
			}},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "platform", candidate.Config["team"])
	require.Len(t, candidate.Notifications, 1)
	require.Equal(t, "connection:"+connID.String()+":connector_notice:pay-attention", candidate.Notifications[0].Key)
	require.Equal(t, "pay-attention", candidate.Notifications[0].Metadata["connector_notice_key"])
	require.Equal(t, candidate.Notifications[0].Key, candidate.NotificationKeys[0])
}

func TestApplyMigrationHookPatchUnsetsNotificationByKey(t *testing.T) {
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

	err := applyMigrationHookPatch(candidate, migrationHookPatch{
		Notifications: migrationNotificationPatch{
			Set: []migrationNotificationDef{{
				Key:     "obsolete-notice",
				Level:   string(database.NotificationLevelWarning),
				Title:   "Notice",
				Message: "A notice.",
			}},
			Unset: []migrationNotificationDef{{
				Key: "obsolete-notice",
			}},
		},
	})
	require.NoError(t, err)
	require.Empty(t, candidate.Notifications)
	require.Empty(t, candidate.NotificationKeys)
	require.Equal(t, []string{
		"connection:" + connID.String() + ":connector_notice:obsolete-notice",
	}, candidate.NotificationUnsetKeys)
}

func TestDecodeMigrationHookPatchNotificationSetUnset(t *testing.T) {
	patch, err := decodeMigrationHookPatch(map[string]any{
		"notifications": map[string]any{
			"set": []any{map[string]any{
				"key":     "heads-up",
				"level":   "info",
				"title":   "Heads up",
				"message": "Something changed.",
			}},
			"unset": []any{map[string]any{
				"key": "old-heads-up",
			}},
		},
	})
	require.NoError(t, err)
	require.Len(t, patch.Notifications.Set, 1)
	require.Equal(t, "heads-up", patch.Notifications.Set[0].Key)
	require.Len(t, patch.Notifications.Unset, 1)
	require.Equal(t, "old-heads-up", patch.Notifications.Unset[0].Key)
}

func TestDecodeMigrationHookPatchRejectsBareNotificationArray(t *testing.T) {
	_, err := decodeMigrationHookPatch(map[string]any{
		"notifications": []any{map[string]any{
			"key":     "heads-up",
			"level":   "info",
			"title":   "Heads up",
			"message": "Something changed.",
		}},
	})
	require.Error(t, err)
}

func TestRequiredActionNotificationPrefersAuthOverSetup(t *testing.T) {
	connID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	candidate := &connectionMigrationCandidate{
		Connection: &connection{Connection: database.Connection{
			Id:               connID,
			Namespace:        "root",
			ConnectorId:      connectorID,
			ConnectorVersion: 1,
		}},
		Target: &ConnectorVersion{ConnectorVersion: database.ConnectorVersion{
			Id:      connectorID,
			Version: 2,
		}},
		Config:      map[string]any{},
		UserLabels:  map[string]string{},
		Annotations: map[string]string{},
	}

	addSetupRequiredNotification(candidate, migrationNotificationMetadata(candidate, "setup"))
	addAuthRequiredNotification(candidate, migrationNotificationMetadata(candidate, "auth"))

	require.Len(t, candidate.Notifications, 1)
	require.Equal(t, "connection:"+connID.String()+":auth_required", candidate.Notifications[0].Key)
	require.Equal(t, "/connections/"+connID.String()+"?action=reauth", *candidate.Notifications[0].ActionUrl)
}

func TestNotificationKeysToResolveSkipsQueuedNotification(t *testing.T) {
	connID := apid.New(apid.PrefixConnection)
	connectorID := apid.New(apid.PrefixConnectorVersion)
	candidate := &connectionMigrationCandidate{
		Connection: &connection{Connection: database.Connection{
			Id:               connID,
			Namespace:        "root",
			ConnectorId:      connectorID,
			ConnectorVersion: 1,
		}},
		Target: &ConnectorVersion{ConnectorVersion: database.ConnectorVersion{
			Id:      connectorID,
			Version: 2,
		}},
	}

	addAuthRequiredNotification(candidate, migrationNotificationMetadata(candidate, "auth"))
	candidate.NotificationUnsetKeys = []string{
		"connection:" + connID.String() + ":connector_notice:obsolete",
	}

	require.ElementsMatch(t, []string{
		"connection:" + connID.String() + ":connector_notice:obsolete",
		"connection:" + connID.String() + ":setup_required",
	}, candidate.NotificationKeysToResolve())
}

func TestTargetProbeIDsReturnsAllTargetProbes(t *testing.T) {
	ids := targetProbeIDs(&cschema.Connector{
		Probes: []cschema.Probe{
			{Id: "existing"},
			{Id: "added"},
			{},
		},
	})

	require.Equal(t, []string{"existing", "added"}, ids)
	require.Nil(t, targetProbeIDs(nil))
}

func TestShouldRunMigrationProbesRequiresHealthyMigratedConnection(t *testing.T) {
	step := cschema.MustNewSetupStep("configure")

	tests := []struct {
		name        string
		probeIDs    []string
		setupStep   *cschema.SetupStep
		healthState database.ConnectionHealthState
		want        bool
	}{
		{
			name:        "healthy migrated connection runs probes",
			probeIDs:    []string{"ping"},
			healthState: database.ConnectionHealthStateHealthy,
			want:        true,
		},
		{
			name:        "skips when no probes were selected",
			healthState: database.ConnectionHealthStateHealthy,
			want:        false,
		},
		{
			name:        "skips when setup is pending",
			probeIDs:    []string{"ping"},
			setupStep:   &step,
			healthState: database.ConnectionHealthStateHealthy,
			want:        false,
		},
		{
			name:        "skips when migration made the connection unhealthy",
			probeIDs:    []string{"ping"},
			healthState: database.ConnectionHealthStateUnhealthy,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := &connectionMigrationCandidate{
				ProbeIdsToRun: tt.probeIDs,
				SetupStep:     tt.setupStep,
				HealthState:   tt.healthState,
			}

			require.Equal(t, tt.want, shouldRunMigrationProbes(candidate))
		})
	}
}
