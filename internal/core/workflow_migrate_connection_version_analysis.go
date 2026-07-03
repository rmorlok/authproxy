package core

import (
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

func (s *service) applyProbeMigrationAnalysis(candidate *connectionMigrationCandidate) {
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
		s.addMigrationSystemNotification(
			candidate,
			database.NotificationLevelInfo,
			fmt.Sprintf("New connection health probe %q will run", probe.Id),
			"The target connector version adds a new health probe. AuthProxy will run it once after migration if no user action is required.",
			fmt.Sprintf("target:%d:probe:%s:added", candidate.Target.Version, probe.Id),
			"", map[string]any{
				"connector_id":    candidate.Connection.ConnectorId.String(),
				"source_version":  candidate.Connection.ConnectorVersion,
				"target_version":  candidate.Target.Version,
				"probe_id":        probe.Id,
				"migration_event": "probe_added",
			},
		)
	}
}

func (s *service) applyAuthMigrationAnalysis(candidate *connectionMigrationCandidate) error {
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
		candidate.RefreshAuth = true
		s.addMigrationSystemNotification(candidate, database.NotificationLevelInfo,
			"Connection credentials will be refreshed",
			"The target connector version changes OAuth settings, so AuthProxy will refresh credentials after migration.",
			fmt.Sprintf("target:%d:oauth:refresh_required", candidate.Target.Version),
			"", map[string]any{
				"connector_id":     candidate.Connection.ConnectorId.String(),
				"source_version":   candidate.Connection.ConnectorVersion,
				"target_version":   candidate.Target.Version,
				"migration_event":  "oauth_refresh_required",
				"auth_method_type": string(cschema.AuthTypeOAuth2),
			})
	}
	return nil
}

func (s *service) applySetupFlowMigrationAnalysis(candidate *connectionMigrationCandidate) error {
	sourceDef := candidate.Connection.cv.GetDefinition()
	targetDef := candidate.Target.GetDefinition()
	if targetDef == nil || targetDef.SetupFlow == nil {
		return nil
	}

	sourceFields := map[string]bool{}
	if sourceDef != nil && sourceDef.SetupFlow != nil {
		sourceFields = sourceDef.SetupFlow.AllConfigFieldNames()
	}

	if targetDef.SetupFlow.Preconnect != nil {
		fields, err := targetDef.SetupFlow.Preconnect.SetupFields()
		if err != nil {
			return fmt.Errorf("inspect target preconnect setup fields: %w", err)
		}
		if err := s.applySetupFieldMigrationAnalysis(candidate, "preconnect", fields, sourceFields); err != nil {
			return err
		}
	}

	if targetDef.SetupFlow.Configure != nil {
		fields, err := targetDef.SetupFlow.Configure.SetupFields()
		if err != nil {
			return fmt.Errorf("inspect target configure setup fields: %w", err)
		}
		if err := s.applySetupFieldMigrationAnalysis(candidate, "configure", fields, sourceFields); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) applySetupFieldMigrationAnalysis(
	candidate *connectionMigrationCandidate,
	phase string,
	fields []cschema.SetupField,
	sourceFields map[string]bool,
) error {
	for _, field := range fields {
		if sourceFields[field.Name] {
			continue
		}
		if _, ok := candidate.Config[field.Name]; ok {
			continue
		}

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
			s.addMigrationSystemNotification(candidate, database.NotificationLevelInfo,
				fmt.Sprintf("Connection setting %q defaulted", field.Name),
				fmt.Sprintf("The connector version migration added the new %s setting %q using the connector default.", phase, field.Name),
				fmt.Sprintf("target:%d:setup:%s:%s:default", candidate.Target.Version, phase, field.Name),
				"", metadata)
			continue
		}

		if field.Required {
			if phase == "preconnect" {
				candidate.HealthState = database.ConnectionHealthStateUnhealthy
				s.addMigrationSystemNotification(candidate, database.NotificationLevelWarning,
					"Connection requires re-authentication",
					fmt.Sprintf("The target connector version requires new preconnect setting %q before credentials can be refreshed.", field.Name),
					fmt.Sprintf("target:%d:setup:%s:%s:required", candidate.Target.Version, phase, field.Name),
					"reauth", metadata)
				continue
			}

			if candidate.SetupStep == nil {
				step, err := cschema.NewSetupStep(field.StepId)
				if err != nil {
					return err
				}
				candidate.SetupStep = &step
			}
			s.addMigrationSystemNotification(candidate, database.NotificationLevelWarning,
				"Connection requires configuration",
				fmt.Sprintf("The target connector version requires new configuration setting %q.", field.Name),
				fmt.Sprintf("target:%d:setup:%s:%s:required", candidate.Target.Version, phase, field.Name),
				"configure", metadata)
			continue
		}

		s.addMigrationSystemNotification(candidate, database.NotificationLevelInfo,
			fmt.Sprintf("New optional connection setting %q is available", field.Name),
			fmt.Sprintf("The target connector version adds optional %s setting %q.", phase, field.Name),
			fmt.Sprintf("target:%d:setup:%s:%s:optional", candidate.Target.Version, phase, field.Name),
			"configure", metadata)
	}
	return nil
}

func (s *service) addMigrationSystemNotification(
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
	candidate.Notifications = append(candidate.Notifications, upsert)
	candidate.NotificationKeys = append(candidate.NotificationKeys, upsert.Key)
}
