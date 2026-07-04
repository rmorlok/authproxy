package core

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

const (
	migrationNotificationRankHookInfo = iota + 1
	migrationNotificationRankHookWarning
	migrationNotificationRankHookError
	migrationNotificationRankSetupRequired
	migrationNotificationRankAuthRequired
)

// applyProbeMigrationAnalysis computes the set of probes that should be run on
// the candidate based on the set of probes that have already run on the source
// compared to the set of probes needed by the target. It only sets only the
// delta to run.
func applyProbeMigrationAnalysis(candidate *connectionMigrationCandidate) {
	sourceDef := candidate.Connection.cv.GetDefinition()
	targetDef := candidate.Target.GetDefinition()

	if sourceDef == nil || targetDef == nil {
		return
	}

	sourceProbeIDs := map[string]bool{}
	for _, probe := range sourceDef.Probes {
		sourceProbeIDs[probe.Id] = true
	}

	for _, probe := range targetDef.Probes {
		if probe.Id == "" || sourceProbeIDs[probe.Id] {
			// The probe was run previously on the original connector version.
			continue
		}

		candidate.ProbeIdsToRun = append(candidate.ProbeIdsToRun, probe.Id)
	}
}

// applyAuthMigrationAnalysis decides if auth should be refreshed after the
// upgrade. Currently it applies a naive comparison of the auth definition
// to see if there are any changes and refreshes if any are detected.
func applyAuthMigrationAnalysis(log *slog.Logger, candidate *connectionMigrationCandidate) error {
	sourceDef := candidate.Connection.cv.GetDefinition()
	targetDef := candidate.Target.GetDefinition()
	if sourceDef == nil || targetDef == nil || targetDef.Auth == nil {
		return nil
	}
	if targetDef.Auth.GetType() != cschema.AuthTypeOAuth2 {
		return nil
	}

	sourceJSON, err := json.Marshal(sourceDef.Auth)
	if err != nil {
		return err
	}

	targetJSON, err := json.Marshal(targetDef.Auth)
	if err != nil {
		return err
	}

	if string(sourceJSON) != string(targetJSON) {
		log.Info("detected auth migration; will trigger refresh after migration")
		candidate.RefreshAuth = true
	}

	return nil
}

func applySetupFlowMigrationAnalysis(
	log *slog.Logger,
	candidate *connectionMigrationCandidate,
) error {
	// Get the connector definition from where we start and end
	sourceDef := candidate.Connection.cv.GetDefinition()
	targetDef := candidate.Target.GetDefinition()
	if targetDef == nil || targetDef.SetupFlow == nil {
		return nil
	}

	// Get the field names from the setup flow for where we started
	sourceFields := map[string]bool{}
	if sourceDef != nil && sourceDef.SetupFlow != nil {
		sourceFields = sourceDef.SetupFlow.AllConfigFieldNames()
	}

	// If where we are going has preconnect, we need to make sure all those
	// fields are present already, or transition the connection.
	if targetDef.SetupFlow.Preconnect != nil {
		// Get the needed fields
		fields, err := targetDef.SetupFlow.Preconnect.SetupFields()
		if err != nil {
			return fmt.Errorf("inspect target preconnect setup fields: %w", err)
		}

		if err := applySetupFieldMigrationAnalysis(log, candidate, "preconnect", fields, sourceFields); err != nil {
			return err
		}
	}

	if targetDef.SetupFlow.Configure != nil {
		fields, err := targetDef.SetupFlow.Configure.SetupFields()
		if err != nil {
			return fmt.Errorf("inspect target configure setup fields: %w", err)
		}
		if err := applySetupFieldMigrationAnalysis(log, candidate, "configure", fields, sourceFields); err != nil {
			return err
		}
	}
	return nil
}

func applySetupFieldMigrationAnalysis(
	log *slog.Logger,
	candidate *connectionMigrationCandidate, // the candidate connection migration
	phase string, // what phase is this (preconnect or configure)
	fields []cschema.SetupField, // the fields we need on the target version
	sourceFields map[string]bool, // the set of fields that are present on the source version
) error {
	for _, field := range fields {
		if sourceFields[field.Name] {
			// We already have the field; good 👍
			continue
		}

		if _, ok := candidate.Config[field.Name]; ok {
			// The migration covered populating this field; good 👍
			continue
		}

		if field.HasDefault {
			candidate.Config[field.Name] = field.Default
			log.Info(
				"detected setup field added; setting to default",
				"phase", phase,
				"field", field.Name,
				"setup_step_id", field.StepId,
			)
			continue
		}

		if field.Required {
			if phase == "preconnect" {
				log.Info(
					"required preconnection field missing; marking connection as needing reauth",
					"field", field.Name,
					"setup_step_id", field.StepId,
				)
				candidate.HealthState = database.ConnectionHealthStateUnhealthy
				addAuthRequiredNotification(
					candidate,
					migrationNotificationMetadata(candidate, "required_preconnect_field_missing"),
				)
				continue
			}

			if candidate.SetupStep == nil {
				step, err := cschema.NewSetupStep(field.StepId)
				if err != nil {
					return err
				}
				candidate.SetupStep = &step
			}
			log.Info(
				"required configuration field missing; marking connection as needing setup",
				"field", field.Name,
				"setup_step_id", field.StepId,
			)
			addSetupRequiredNotification(
				candidate,
				migrationNotificationMetadata(candidate, "required_configure_field_missing"),
			)
			continue
		}

		log.Info(
			"optional setup field added; no user notification queued",
			"phase", phase,
			"field", field.Name,
			"setup_step_id", field.StepId,
		)
	}
	return nil
}

func applyRequiredActionNotification(candidate *connectionMigrationCandidate) {
	if candidate.HealthState == database.ConnectionHealthStateUnhealthy {
		addAuthRequiredNotification(
			candidate,
			migrationNotificationMetadata(candidate, "connection_requires_reauth"),
		)
		return
	}
	if candidate.SetupStep != nil {
		addSetupRequiredNotification(
			candidate,
			migrationNotificationMetadata(candidate, "connection_requires_setup"),
		)
	}
}

func addAuthRequiredNotification(candidate *connectionMigrationCandidate, metadata map[string]any) {
	addConnectionRequiredActionNotification(
		candidate,
		migrationNotificationRankAuthRequired,
		database.NotificationKeyAuthRequired,
		database.NotificationLevelWarning,
		"Connection requires re-authentication",
		"Reconnect this connection to continue using it.",
		"reauth",
		metadata,
	)
}

func addSetupRequiredNotification(candidate *connectionMigrationCandidate, metadata map[string]any) {
	addConnectionRequiredActionNotification(
		candidate,
		migrationNotificationRankSetupRequired,
		database.NotificationKeySetupRequired,
		database.NotificationLevelWarning,
		"Connection requires setup",
		"Review this connection's setup before using it.",
		"configure",
		metadata,
	)
}

func addConnectionRequiredActionNotification(
	candidate *connectionMigrationCandidate,
	rank int,
	keyPart string,
	level database.NotificationLevel,
	title string,
	message string,
	action string,
	metadata map[string]any,
) {
	key := connectionNotificationKey(candidate, keyPart)
	source := connectionRequiredActionNotificationSource
	actionURL := ""
	if action != "" {
		actionURL = fmt.Sprintf("/connections/%s?action=%s", candidate.Connection.Id, action)
	}
	var actionURLPtr *string
	if actionURL != "" {
		actionURLPtr = &actionURL
	}

	actionPermissions := aschema.NoPermissions()
	if action != "" {
		actionPermissions = aschema.PermissionsSingleWithResourceIds(
			candidate.Connection.Namespace,
			"connections",
			"update",
			candidate.Connection.Id.String(),
		)
	}

	upsert := database.NotificationUpsert{
		Key:          key,
		Level:        level,
		ResourceType: "connection",
		ResourceId:   candidate.Connection.Id,
		Namespace:    candidate.Connection.Namespace,
		Title:        title,
		Message:      message,
		ActionUrl:    actionURLPtr,
		ViewPermissions: aschema.PermissionsSingleWithResourceIds(
			candidate.Connection.Namespace,
			"connections",
			"get",
			candidate.Connection.Id.String(),
		),
		ActionPermissions: actionPermissions,
		Source:            &source,
		Metadata:          metadata,
	}

	setCandidateNotification(candidate, rank, upsert)
}

func setCandidateNotification(candidate *connectionMigrationCandidate, rank int, upsert database.NotificationUpsert) {
	if rank <= candidate.NotificationRank {
		return
	}
	candidate.NotificationRank = rank
	candidate.Notifications = []database.NotificationUpsert{upsert}
	candidate.NotificationKeys = []string{upsert.Key}
	candidate.NotificationUnsetKeys = removeString(candidate.NotificationUnsetKeys, upsert.Key)
}

func connectionNotificationKey(candidate *connectionMigrationCandidate, keyPart string) string {
	return fmt.Sprintf("connection:%s:%s", candidate.Connection.Id, keyPart)
}

func migrationNotificationMetadata(candidate *connectionMigrationCandidate, event string) map[string]any {
	return map[string]any{
		"connector_id":    candidate.Connection.ConnectorId.String(),
		"source_version":  candidate.Connection.ConnectorVersion,
		"target_version":  candidate.Target.Version,
		"migration_event": event,
	}
}

func migrationNotificationRankForLevel(level database.NotificationLevel) int {
	switch level {
	case database.NotificationLevelError:
		return migrationNotificationRankHookError
	case database.NotificationLevelWarning:
		return migrationNotificationRankHookWarning
	default:
		return migrationNotificationRankHookInfo
	}
}
