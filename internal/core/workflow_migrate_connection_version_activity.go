package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
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

	// Compute the set of changes that are needed to move the connection from
	// the current version to the taget version. This call is the primary
	// operation of the migration, as it computes the changes to the config
	// labels, annotations, and other fields like connection state as well
	// as computes notifications. The output candidate object is the migration
	// that will be performed.
	candidate, err := s.buildConnectionMigrationCandidate(
		ctx,
		connectionID,
		targetVersion,
	)
	if err != nil {
		return err
	}

	// Encrypt the computed updated configuration
	encryptedConfig, err := s.encryptMigrationConfig(
		ctx,
		candidate.Connection.Namespace,
		candidate.Config,
	)
	if err != nil {
		return err
	}

	// If health state wasn't supplied as part of the candidate update, it
	// should remain the same from the current state of the connection.
	health := candidate.HealthState
	if health == "" {
		health = candidate.Connection.GetHealthState()
	}

	// Apply the migration to the actual data in the databse
	updated, err := s.db.UpdateConnectionForVersionMigration(
		ctx,
		database.ConnectionVersionMigrationUpdate{
			Id:                     connectionID,
			ConnectorId:            candidate.Connection.ConnectorId,
			ConnectorVersion:       targetVersion,
			EncryptedConfiguration: encryptedConfig,
			UserLabels:             candidate.UserLabels,
			Annotations:            candidate.Annotations,
			SetupStep:              candidate.SetupStep,
			SetupError:             candidate.SetupError,
			HealthState:            &health,
		},
	)
	if err != nil {
		return err
	}

	if candidate.RefreshAuth {
		// Based on the transition (e.g. changing scopes, changing oauth
		// credentials), the connection was flagged for needing to refresh the
		// auth credentials to quickly expose any changes that might require
		// the connection to require re-authentication.
		if err := s.refreshAuthAfterConnectionMigration(ctx, updated, candidate); err != nil {
			logger.Info(
				"auth refresh after migration failed; marking connection as needing reauth",
				"error", err,
			)
			candidate.HealthState = database.ConnectionHealthStateUnhealthy
			addAuthRequiredNotification(
				candidate,
				migrationNotificationMetadata(candidate, "oauth_refresh_failed"),
			)
			if markErr := s.db.SetConnectionHealthState(
				ctx,
				connectionID,
				database.ConnectionHealthStateUnhealthy,
			); markErr != nil {
				return markErr
			}
		} else {
			applySuccessfulMigrationAuthRefresh(candidate)
		}
	}

	if shouldRunMigrationProbes(candidate) {
		for _, probeID := range candidate.ProbeIdsToRun {
			if err := s.RunProbe(ctx, connectionID, probeID); err != nil {
				// Probe failures aren't blocking on migration, but they will
				// count against the connection transitioning to unhealthy if
				// they fail.
				logger.Warn(
					"target probe failed after migration",
					"probe_id", probeID, "error", err,
				)
			}
		}
	}

	// Resolve any notifications that are no longer applicable.
	if err := s.resolveNotificationsForResourceKeys(
		ctx,
		"connection", // resource type
		connectionID,
		candidate.NotificationKeysToResolve(),
	); err != nil {
		return err
	}

	// Add newly generated notifications
	for _, notification := range candidate.Notifications {
		notification.Labels = updated.Labels
		if _, err := s.upsertNotification(ctx, notification); err != nil {
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

// shouldRunMigrationProbes returns true if the migration candidate should run
// probes. It checks if there are probes to run and if the connection doesn't
// require configuration and is in a healthy state.
func shouldRunMigrationProbes(candidate *connectionMigrationCandidate) bool {
	return len(candidate.ProbeIdsToRun) > 0 &&
		candidate.SetupStep == nil &&
		candidate.HealthState == database.ConnectionHealthStateHealthy
}

// refreshAuthAfterConnectionMigration refreshes the authentication method
// after a connection version migration. Not all connection types implement
// refresh and for some this may be a noop.
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

	return authenticator.Refresh(ctx)
}

// encryptMigrationConfig encrypts the connection configuration after a
// migration. It's simple wrapper around the data serialization and encryption
// logic.
func (s *service) encryptMigrationConfig(
	ctx context.Context,
	namespace string,
	cfg map[string]any,
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
