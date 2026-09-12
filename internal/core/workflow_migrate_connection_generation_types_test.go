package core

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/require"
)

func TestNotificationKeysToResolveIncludesExplicitUnsetsAndStaleRequiredActions(t *testing.T) {
	candidate := newMigrationTestCandidate(t)
	candidate.NotificationUnsetKeys = []string{
		connectionNotificationKey(candidate, "connector_notice:obsolete"),
	}

	require.ElementsMatch(t, []string{
		connectionNotificationKey(candidate, "connector_notice:obsolete"),
		connectionNotificationKey(candidate, database.NotificationKeyAuthRequired),
		connectionNotificationKey(candidate, database.NotificationKeySetupRequired),
	}, candidate.NotificationKeysToResolve())
}

func TestNotificationKeysToResolveSkipsQueuedNotification(t *testing.T) {
	candidate := newMigrationTestCandidate(t)
	addAuthRequiredNotification(candidate, migrationNotificationMetadata(candidate, "auth"))
	candidate.NotificationUnsetKeys = []string{
		connectionNotificationKey(candidate, "connector_notice:obsolete"),
	}

	require.ElementsMatch(t, []string{
		connectionNotificationKey(candidate, "connector_notice:obsolete"),
		connectionNotificationKey(candidate, database.NotificationKeySetupRequired),
	}, candidate.NotificationKeysToResolve())
}

func TestNotificationKeysToResolveDedupesExplicitUnset(t *testing.T) {
	candidate := newMigrationTestCandidate(t)
	setupKey := connectionNotificationKey(candidate, database.NotificationKeySetupRequired)
	candidate.NotificationUnsetKeys = []string{setupKey}

	require.ElementsMatch(t, []string{
		setupKey,
		connectionNotificationKey(candidate, database.NotificationKeyAuthRequired),
	}, candidate.NotificationKeysToResolve())
}
