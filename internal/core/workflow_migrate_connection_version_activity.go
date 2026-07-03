package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

func (s *service) applyMigrateConnectionVersionV1(
	ctx context.Context,
	connectionID apid.ID,
	targetVersion uint64,
) error {
	logger := s.logger.With(
		"workflow", WorkflowNameMigrateConnectionVersionV1,
		"activity", ActivityNameMigrateConnectionVersionApplyV1,
		"connection_id", connectionID,
		"target_version", targetVersion,
	)
	logger.Info("connection version migration apply activity started")
	defer logger.Info("connection version migration apply activity completed")

	candidate, err := s.buildConnectionMigrationCandidate(ctx, connectionID, targetVersion)
	if err != nil {
		return err
	}

	encryptedConfig, err := s.encryptMigrationConfig(ctx, candidate.Connection.Namespace, candidate.Config)
	if err != nil {
		return err
	}

	health := candidate.HealthState
	if health == "" {
		health = candidate.Connection.GetHealthState()
	}
	updated, err := s.db.UpdateConnectionForVersionMigration(ctx, database.ConnectionVersionMigrationUpdate{
		Id:                     connectionID,
		ConnectorId:            candidate.Connection.ConnectorId,
		ConnectorVersion:       targetVersion,
		EncryptedConfiguration: encryptedConfig,
		UserLabels:             candidate.UserLabels,
		Annotations:            candidate.Annotations,
		SetupStep:              candidate.SetupStep,
		SetupError:             candidate.SetupError,
		HealthState:            &health,
	})
	if err != nil {
		return err
	}

	if candidate.RefreshAuth {
		if err := s.refreshAuthAfterConnectionMigration(ctx, updated, candidate); err != nil {
			candidate.HealthState = database.ConnectionHealthStateUnhealthy
			s.addMigrationSystemNotification(candidate, database.NotificationLevelWarning,
				"Connection requires re-authentication",
				"An upgrade to the connector changed OAuth settings and credentials could not be refreshed automatically.",
				fmt.Sprintf("target:%d:oauth:refresh_failed", candidate.Target.Version),
				"reauth",
				map[string]any{
					"connector_id":     candidate.Connection.ConnectorId.String(),
					"source_version":   candidate.Connection.ConnectorVersion,
					"target_version":   candidate.Target.Version,
					"migration_event":  "oauth_refresh_failed",
					"refresh_error":    err.Error(),
					"requires_reauth":  true,
					"auth_method_type": string(cschema.AuthTypeOAuth2),
				})
			if markErr := s.db.SetConnectionHealthState(ctx, connectionID, database.ConnectionHealthStateUnhealthy); markErr != nil {
				return markErr
			}
		}
	}
	if len(candidate.ProbeIdsToRun) > 0 &&
		candidate.SetupStep == nil &&
		candidate.HealthState != database.ConnectionHealthStateUnhealthy {
		for _, probeID := range candidate.ProbeIdsToRun {
			if err := s.RunProbe(ctx, connectionID, probeID); err != nil {
				logger.Warn("new target probe failed after migration", "probe_id", probeID, "error", err)
			}
		}
	}

	if err := s.db.ResolveNotificationsForResource(
		ctx,
		"connection", // resource type
		connectionID,
		connectionMigrationNotificationSource,
		candidate.NotificationKeys,
	); err != nil {
		return err
	}
	for _, notification := range candidate.Notifications {
		notification.Labels = map[string]string(updated.Labels)
		if _, err := s.db.UpsertNotification(ctx, notification); err != nil {
			return err
		}
	}

	logger.Info(
		"connection version migration applied",
		"source_version", candidate.Connection.ConnectorVersion,
		"target_version", updated.ConnectorVersion,
	)
	return nil
}

func (s *service) refreshAuthAfterConnectionMigration(
	ctx context.Context,
	updated *database.Connection,
	candidate *connectionMigrationCandidate,
) error {
	conn, err := s.getConnectionForDb(ctx, updated)
	if err != nil {
		return err
	}

	factory := s.getAuthMethodFactory(candidate.Target.GetDefinition())
	if factory == nil {
		return fmt.Errorf("auth method factory is not configured")
	}

	authenticator := factory.NewAuthenticator(conn)
	if authenticator == nil {
		return fmt.Errorf("authenticator is not configured")
	}

	return authenticator.RecoverFrom401(ctx)
}

func (s *service) encryptMigrationConfig(
	ctx context.Context,
	namespace string, cfg map[string]any,
) (*encfield.EncryptedField, error) {
	if cfg == nil {
		return nil, nil
	}

	jsonBytes, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal migrated connection configuration: %w", err)
	}

	encrypted, err := s.encrypt.EncryptStringForNamespace(ctx, namespace, string(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("encrypt migrated connection configuration: %w", err)
	}

	return &encrypted, nil
}
