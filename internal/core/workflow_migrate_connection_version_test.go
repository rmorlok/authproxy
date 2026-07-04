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
	require.Equal(t, connectionConnectorNoticeNotificationSource, *candidate.Notifications[0].Source)
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
	require.Equal(t, connectionRequiredActionNotificationSource, *candidate.Notifications[0].Source)
	require.Equal(t, "/connections/"+connID.String()+"?action=reauth", *candidate.Notifications[0].ActionUrl)
}
