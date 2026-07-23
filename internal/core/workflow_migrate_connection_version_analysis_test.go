package core

import (
	"log/slog"
	"testing"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/require"
)

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

func TestApplyAuthMigrationAnalysisFlagsOAuthChangesOnly(t *testing.T) {
	tests := []struct {
		name       string
		sourceAuth *cschema.Auth
		targetAuth *cschema.Auth
		want       bool
	}{
		{
			name:       "same oauth does not refresh",
			sourceAuth: migrationTestOAuthAuth("read"),
			targetAuth: migrationTestOAuthAuth("read"),
		},
		{
			name:       "changed oauth refreshes",
			sourceAuth: migrationTestOAuthAuth("read"),
			targetAuth: migrationTestOAuthAuth("write"),
			want:       true,
		},
		{
			name:       "non oauth target does not refresh",
			sourceAuth: migrationTestOAuthAuth("read"),
			targetAuth: cschema.NewNoAuth(),
		},
		{
			name:       "missing target auth does not refresh",
			sourceAuth: migrationTestOAuthAuth("read"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := newMigrationTestCandidate(t)
			candidate.Connection.cv = NewTestConnectorVersion(cschema.Connector{Auth: tt.sourceAuth})
			candidate.Target = NewTestConnectorVersion(cschema.Connector{Auth: tt.targetAuth})

			require.NoError(t, applyAuthMigrationAnalysis(slog.Default(), candidate))
			require.Equal(t, tt.want, candidate.RefreshAuth)
		})
	}
}

func TestApplySetupFieldMigrationAnalysisHandlesConfigGaps(t *testing.T) {
	candidate := newMigrationTestCandidate(t)
	candidate.Config = map[string]any{
		"provided_by_hook": "already present",
	}

	err := applySetupFieldMigrationAnalysis(
		slog.Default(),
		candidate,
		"preconnect",
		[]cschema.SetupField{
			{Name: "from_source", StepId: "source", Required: true},
			{Name: "provided_by_hook", StepId: "hook", Required: true},
			{Name: "with_default", StepId: "defaults", HasDefault: true, Default: "us"},
			{Name: "optional", StepId: "optional"},
			{Name: "tenant", StepId: "tenant", Required: true},
		},
		map[string]bool{"from_source": true},
	)
	require.NoError(t, err)
	require.Equal(t, "us", candidate.Config["with_default"])
	require.Equal(t, database.ConnectionHealthStateUnhealthy, candidate.HealthState)
	require.Nil(t, candidate.SetupStep)
	require.Len(t, candidate.Notifications, 1)
	require.Equal(t, connectionNotificationKey(candidate, database.NotificationKeyAuthRequired), candidate.Notifications[0].Key)
}

func TestApplySetupFieldMigrationAnalysisConfigureUsesFirstMissingRequiredStep(t *testing.T) {
	candidate := newMigrationTestCandidate(t)

	err := applySetupFieldMigrationAnalysis(
		slog.Default(),
		candidate,
		"configure",
		[]cschema.SetupField{
			{Name: "workspace", StepId: "workspace", Required: true},
			{Name: "project", StepId: "project", Required: true},
		},
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, candidate.SetupStep)
	require.Equal(t, "workspace", candidate.SetupStep.Id())
	require.Equal(t, database.ConnectionHealthStateHealthy, candidate.HealthState)
	require.Len(t, candidate.Notifications, 1)
	require.Equal(t, connectionNotificationKey(candidate, database.NotificationKeySetupRequired), candidate.Notifications[0].Key)
}

func TestApplySetupFieldMigrationAnalysisConfigureRejectsInvalidStep(t *testing.T) {
	candidate := newMigrationTestCandidate(t)

	err := applySetupFieldMigrationAnalysis(
		slog.Default(),
		candidate,
		"configure",
		[]cschema.SetupField{{Name: "workspace", Required: true}},
		nil,
	)
	require.Error(t, err)
	require.Nil(t, candidate.SetupStep)
}

func TestApplySetupFlowMigrationAnalysisPropagatesSchemaErrors(t *testing.T) {
	candidate := newMigrationTestCandidate(t)
	candidate.Connection.cv = &ConnectorVersion{def: &cschema.Connector{Auth: cschema.NewNoAuth()}}
	candidate.Target = &ConnectorVersion{def: &cschema.Connector{
		Auth: cschema.NewNoAuth(),
		SetupFlow: &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{{
					Id:         "broken",
					JsonSchema: common.RawJSON(`{not-json`),
				}},
			},
		},
	}}

	err := applySetupFlowMigrationAnalysis(slog.Default(), candidate)
	require.Error(t, err)
	require.Contains(t, err.Error(), "inspect target configure setup fields")
}

func TestApplyRequiredActionNotificationPrefersAuthOverSetup(t *testing.T) {
	candidate := newMigrationTestCandidate(t)
	step := cschema.MustNewSetupStep("configure")
	candidate.SetupStep = &step
	candidate.HealthState = database.ConnectionHealthStateUnhealthy

	applyRequiredActionNotification(candidate)

	require.Len(t, candidate.Notifications, 1)
	require.Equal(t, connectionNotificationKey(candidate, database.NotificationKeyAuthRequired), candidate.Notifications[0].Key)
	require.Equal(t, "/connections/"+candidate.Connection.Id.String()+"?action=reauth", *candidate.Notifications[0].ActionUrl)
}

func TestApplySuccessfulMigrationAuthRefreshClearsStaleAuthNotification(t *testing.T) {
	candidate := newMigrationTestCandidate(t)
	candidate.HealthState = database.ConnectionHealthStateUnhealthy
	addAuthRequiredNotification(candidate, migrationNotificationMetadata(candidate, "connection_requires_reauth"))

	applySuccessfulMigrationAuthRefresh(candidate)

	require.Equal(t, database.ConnectionHealthStateHealthy, candidate.HealthState)
	require.Empty(t, candidate.Notifications)
	require.Empty(t, candidate.NotificationKeys)
	require.Zero(t, candidate.NotificationRank)
	require.Contains(
		t,
		candidate.NotificationKeysToResolve(),
		connectionNotificationKey(candidate, database.NotificationKeyAuthRequired),
	)
}

func TestSetCandidateNotificationRankAndUnsetHandling(t *testing.T) {
	candidate := newMigrationTestCandidate(t)
	authKey := connectionNotificationKey(candidate, database.NotificationKeyAuthRequired)
	setupKey := connectionNotificationKey(candidate, database.NotificationKeySetupRequired)
	candidate.NotificationUnsetKeys = []string{authKey}

	setCandidateNotification(candidate, migrationNotificationRankAuthRequired, database.NotificationUpsert{Key: authKey})
	require.Equal(t, []database.NotificationUpsert{{Key: authKey}}, candidate.Notifications)
	require.Empty(t, candidate.NotificationUnsetKeys)

	setCandidateNotification(candidate, migrationNotificationRankSetupRequired, database.NotificationUpsert{Key: setupKey})
	require.Equal(t, []database.NotificationUpsert{{Key: authKey}}, candidate.Notifications)
	require.Equal(t, []string{authKey}, candidate.NotificationKeys)
}

func TestMigrationNotificationRankForLevel(t *testing.T) {
	require.Equal(t, migrationNotificationRankHookError, migrationNotificationRankForLevel(database.NotificationLevelError))
	require.Equal(t, migrationNotificationRankHookWarning, migrationNotificationRankForLevel(database.NotificationLevelWarning))
	require.Equal(t, migrationNotificationRankHookInfo, migrationNotificationRankForLevel(database.NotificationLevelInfo))
}
