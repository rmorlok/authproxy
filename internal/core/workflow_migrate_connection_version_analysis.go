package core

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/util"
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

		// Metadata just tracks information about what triggered a notification
		metadata := map[string]any{
			"connector_id":    candidate.Connection.ConnectorId.String(),
			"source_version":  candidate.Connection.ConnectorVersion,
			"target_version":  candidate.Target.Version,
			"setup_phase":     phase,
			"setup_step_id":   field.StepId,
			"config_field":    field.Name,
			"migration_event": "setup_field_added",
		}

		if field.HasDefault {
			candidate.Config[field.Name] = field.Default
			log.Info("detected setup field added; setting to default", "field", field.Name)
			continue
		}

		if field.Required {
			if phase == "preconnect" {
				log.Info("required preconnection field missing; marking connection as needing reauth", "field", field.Name)
				candidate.HealthState = database.ConnectionHealthStateUnhealthy
				addMigrationSystemNotification(
					candidate,
					database.NotificationLevelWarning,
					"Connection requires re-authentication",
					"The connection requires additional configuration to continue operating.",
					database.NotificationKeyAuthRequired,
					"reauth",
					metadata,
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
			addMigrationSystemNotification(candidate, database.NotificationLevelWarning,
				"Connection requires configuration",
				fmt.Sprintf("The target connector version requires new configuration setting %q.", field.Name),
				fmt.Sprintf("target:%d:setup:%s:%s:required", candidate.Target.Version, phase, field.Name),
				"configure", metadata)
			continue
		}

		addMigrationSystemNotification(candidate, database.NotificationLevelInfo,
			fmt.Sprintf("New optional connection setting %q is available", field.Name),
			fmt.Sprintf("The target connector version adds optional %s setting %q.", phase, field.Name),
			fmt.Sprintf("target:%d:setup:%s:%s:optional", candidate.Target.Version, phase, field.Name),
			"configure", metadata)
	}
	return nil
}

func addMigrationSystemNotification(
	candidate *connectionMigrationCandidate,
	level database.NotificationLevel,
	title string,
	message string,
	keyPart string,
	action string,
	metadata map[string]any,
) {
	key := fmt.Sprintf("%s:%s:%s", connectionMigrationNotificationSource, candidate.Connection.Id, keyPart)
	source := connectionMigrationNotificationSource
	actionURL := ""
	if action != "" {
		actionURL = fmt.Sprintf("/connections/%s?action=%s", candidate.Connection.Id, action)
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
		ActionUrl:    util.ToPtrNonZero(actionURL),
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

	candidate.Notifications = append(candidate.Notifications, upsert)
	candidate.NotificationKeys = append(candidate.NotificationKeys, upsert.Key)
}
