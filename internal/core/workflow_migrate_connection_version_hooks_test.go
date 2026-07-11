package core

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

func TestApplyMigrationHookForVersionSelectsUpAndDown(t *testing.T) {
	version := NewTestConnectorVersion(cschema.Connector{
		Javascript: `
			function migrateUp() {
				return { config: { set: { direction: "up", from_cfg: cfg.seed } } };
			}
			function migrateDown() {
				return { config: { set: { direction: "down", from_label: labels.team } } };
			}
		`,
		Migrations: &cschema.Migrations{
			Up:   &cschema.MigrationHook{Javascript: `migrateUp()`},
			Down: &cschema.MigrationHook{Javascript: `migrateDown()`},
		},
	})

	up := newMigrationTestCandidate(t)
	up.Config["seed"] = "source"
	require.NoError(t, version.s.applyMigrationHookForVersion(context.Background(), up, version, 1, 2))
	require.Equal(t, "up", up.Config["direction"])
	require.Equal(t, "source", up.Config["from_cfg"])

	down := newMigrationTestCandidate(t)
	down.UserLabels["team"] = "platform"
	require.NoError(t, version.s.applyMigrationHookForVersion(context.Background(), down, version, 2, 1))
	require.Equal(t, "down", down.Config["direction"])
	require.Equal(t, "platform", down.Config["from_label"])
}

func TestApplyMigrationHookForVersionSkipsMissingHook(t *testing.T) {
	version := NewTestConnectorVersion(cschema.Connector{})
	candidate := newMigrationTestCandidate(t)

	require.NoError(t, version.s.applyMigrationHookForVersion(context.Background(), candidate, version, 1, 2))
	require.Empty(t, candidate.Config)
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

func TestApplyMigrationHookPatchAppliesMapSetAndUnset(t *testing.T) {
	candidate := newMigrationTestCandidate(t)
	candidate.Config = map[string]any{"keep": "yes", "remove": "old"}
	candidate.UserLabels = map[string]string{"team": "platform", "old": "gone"}
	candidate.Annotations = map[string]string{"note": "keep", "old": "gone"}

	err := applyMigrationHookPatch(candidate, migrationHookPatch{
		Config: migrationAnyPatch{
			Set:   map[string]any{"added": "value"},
			Unset: []string{"remove"},
		},
		Labels: migrationStringPatch{
			Set:   map[string]string{"tier": "gold"},
			Unset: []string{"old"},
		},
		Annotations: migrationStringPatch{
			Set:   map[string]string{"mode": "migration"},
			Unset: []string{"old"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"keep": "yes", "added": "value"}, candidate.Config)
	require.Equal(t, map[string]string{"team": "platform", "tier": "gold"}, candidate.UserLabels)
	require.Equal(t, map[string]string{"note": "keep", "mode": "migration"}, candidate.Annotations)
}

func TestApplyMigrationHookPatchRejectsInvalidLabelsAndAnnotations(t *testing.T) {
	t.Run("system label", func(t *testing.T) {
		candidate := newMigrationTestCandidate(t)
		err := applyMigrationHookPatch(candidate, migrationHookPatch{
			Labels: migrationStringPatch{
				Set: map[string]string{"apxy/cxr/type": "bad"},
			},
		})
		require.Error(t, err)
	})

	t.Run("invalid label", func(t *testing.T) {
		candidate := newMigrationTestCandidate(t)
		err := applyMigrationHookPatch(candidate, migrationHookPatch{
			Labels: migrationStringPatch{
				Set: map[string]string{"-bad": "value"},
			},
		})
		require.Error(t, err)
	})

	t.Run("invalid annotation", func(t *testing.T) {
		candidate := newMigrationTestCandidate(t)
		err := applyMigrationHookPatch(candidate, migrationHookPatch{
			Annotations: migrationStringPatch{
				Set: map[string]string{"-bad": "value"},
			},
		})
		require.Error(t, err)
	})
}

func TestApplyMigrationHookPatchKeepsHighestPriorityNotification(t *testing.T) {
	candidate := newMigrationTestCandidate(t)

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
	require.Equal(t, connectionNotificationKey(candidate, "connector_notice:pay-attention"), candidate.Notifications[0].Key)
	require.Equal(t, "pay-attention", candidate.Notifications[0].Metadata["connector_notice_key"])
	require.Equal(t, candidate.Notifications[0].Key, candidate.NotificationKeys[0])
}

func TestApplyMigrationHookPatchUnsetsNotificationByKey(t *testing.T) {
	candidate := newMigrationTestCandidate(t)

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
		connectionNotificationKey(candidate, "connector_notice:obsolete-notice"),
	}, candidate.NotificationUnsetKeys)
}

func TestMigrationNotificationUpsertValidationAndPermissions(t *testing.T) {
	candidate := newMigrationTestCandidate(t)

	_, _, err := migrationNotificationUpsert(candidate, migrationNotificationDef{
		Key:     "missing-title",
		Message: "message",
	})
	require.Error(t, err)

	_, _, err = migrationNotificationUpsert(candidate, migrationNotificationDef{
		Key:     "bad-level",
		Level:   "critical",
		Title:   "Title",
		Message: "message",
	})
	require.Error(t, err)

	upsert, rank, err := migrationNotificationUpsert(candidate, migrationNotificationDef{
		Key:       "resume",
		Level:     string(database.NotificationLevelError),
		Title:     "Needs attention",
		Message:   "Review the connection.",
		ActionURL: "/connections/cxn_test?action=review",
		Metadata:  map[string]any{"from": "hook"},
	})
	require.NoError(t, err)
	require.Equal(t, migrationNotificationRankHookError, rank)
	require.Equal(t, connectionNotificationKey(candidate, "connector_notice:resume"), upsert.Key)
	require.Equal(t, "resume", upsert.Metadata["connector_notice_key"])
	require.Equal(t, "hook", upsert.Metadata["from"])
	require.NotNil(t, upsert.ActionUrl)
	require.Equal(t, "/connections/cxn_test?action=review", *upsert.ActionUrl)
	require.NotEmpty(t, upsert.ViewPermissions)
	require.NotEmpty(t, upsert.ActionPermissions)
}
