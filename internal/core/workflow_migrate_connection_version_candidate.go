package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
)

func (s *service) buildConnectionMigrationCandidate(ctx context.Context, connectionID apid.ID, targetVersion uint64) (*connectionMigrationCandidate, error) {
	// Get the connection to be migrated
	conn, err := s.getConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	// We track some details of what we are changing in the application log
	// even if they don't notify the user
	log := s.logger.With(
		"operation", "migrate_connection_version",
		"connection_id", connectionID,
		"source_version", conn.ConnectorVersion,
		"target_version", targetVersion,
	)

	// We don't allow migrating to the same version
	if conn.ConnectorVersion == targetVersion {
		return nil, fmt.Errorf("connection is already on connector version %d", targetVersion)
	}

	// Get the target version for the connector
	target, err := s.getConnectorVersion(ctx, conn.ConnectorId, targetVersion)
	if err != nil {
		return nil, err
	}

	// Must be primary or active. If the old version had previously been
	// archived, the connector needs to have its state manually set prior
	// to a rollback.
	if target.State != database.ConnectorVersionStatePrimary &&
		target.State != database.ConnectorVersionStateActive {
		return nil, fmt.Errorf("target connector version must be primary or active")
	}

	// Get the configuration settings (pre and post connect) for the connection
	cfg, err := conn.GetConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = map[string]any{}
	}

	// Filter out the system labels that cannot be changed
	userLabels, _ := database.SplitUserAndApxyLabels(conn.Labels)
	annotations := map[string]string{}
	for k, v := range conn.Annotations {
		annotations[k] = v
	}

	// Create the candidate object. This represents the set of transformations
	// that will be applied to the connection as part of upgrading.
	candidate := &connectionMigrationCandidate{
		Connection:  conn,
		Target:      target,
		Config:      cfg,
		UserLabels:  map[string]string(userLabels),
		Annotations: annotations,
		SetupStep:   conn.SetupStep,
		SetupError:  conn.SetupError,
		HealthState: conn.GetHealthState(),
	}

	// Get the versions that will be included in the migration.
	versions, err := s.migrationVersionPath(ctx, conn.ConnectorId, conn.ConnectorVersion, targetVersion)
	if err != nil {
		return nil, err
	}

	// Step through each version and apply the migration. This will allow
	// registered javascript migration hooks to run to apply defaults/changes.
	//
	// This doesn't actually make changes to the connection but rather
	// aggregates the changes that will be applied in the candidate object.
	for _, version := range versions {
		if err := s.applyMigrationHookForVersion(
			ctx,
			candidate,
			version,
			conn.ConnectorVersion,
			targetVersion,
		); err != nil {
			return nil, err
		}
	}

	// Update the candidate based on if auth has changed to flag that auth
	// be refreshed after upgrade
	if err := applyAuthMigrationAnalysis(candidate); err != nil {
		return nil, err
	}

	// Analyze the probes on the start versus end of the migration to see
	// what delta set of probes should be run.
	applyProbeMigrationAnalysis(candidate)

	if err := applySetupFlowMigrationAnalysis(log, candidate); err != nil {
		return nil, err
	}

	return candidate, nil
}

// migrationVersionPath retrieves the ordered list of connector versions that
// will be included in the migration.
func (s *service) migrationVersionPath(
	ctx context.Context,
	connectorID apid.ID,
	sourceVersion,
	targetVersion uint64,
) ([]*ConnectorVersion, error) {
	var versions []*ConnectorVersion
	if targetVersion > sourceVersion {
		for v := sourceVersion + 1; v <= targetVersion; v++ {
			cv, err := s.getConnectorVersion(ctx, connectorID, v)
			if err != nil {
				return nil, err
			}
			versions = append(versions, cv)
		}
		return versions, nil
	}

	for v := sourceVersion; v > targetVersion; v-- {
		cv, err := s.getConnectorVersion(ctx, connectorID, v)
		if err != nil {
			return nil, err
		}
		versions = append(versions, cv)
	}

	return versions, nil
}
