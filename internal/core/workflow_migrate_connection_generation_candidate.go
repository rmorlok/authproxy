package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
)

func (s *service) buildConnectionMigrationCandidate(
	ctx context.Context,
	connectionID apid.ID,
	targetGeneration uint64,
) (*connectionMigrationCandidate, error) {
	// Get the connection to be migrated
	conn, err := s.getConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	// We track some details of what we are changing in the application log
	// even if they don't notify the user
	log := s.logger.With(
		"operation", "migrate_connection_generation",
		"connection_id", connectionID,
		"source_generation", conn.ConnectorGeneration,
		"target_generation", targetGeneration,
	)

	// We don't allow migrating to the same generation
	if conn.ConnectorGeneration == targetGeneration {
		return nil, fmt.Errorf("connection is already on connector generation %d", targetGeneration)
	}

	// Get the target generation for the connector
	target, err := s.getConnectorGeneration(ctx, conn.ConnectorId, targetGeneration)
	if err != nil {
		return nil, err
	}

	// Must be primary or active. If the old generation had previously been
	// archived, the connector needs to have its state manually set prior
	// to a rollback.
	if target.State != database.ConnectorGenerationStatePrimary &&
		target.State != database.ConnectorGenerationStateActive {
		return nil, fmt.Errorf("target connector generation must be primary or active")
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

	// Get the generations that will be included in the migration.
	generations, err := s.migrationGenerationPath(ctx, conn.ConnectorId, conn.ConnectorGeneration, targetGeneration)
	if err != nil {
		return nil, err
	}

	// Step through each generation and apply the migration. This will allow
	// registered javascript migration hooks to run to apply defaults/changes.
	//
	// This doesn't actually make changes to the connection but rather
	// aggregates the changes that will be applied in the candidate object.
	for _, generation := range generations {
		if err := s.applyMigrationHookForGeneration(
			ctx,
			candidate,
			generation,
			conn.ConnectorGeneration,
			targetGeneration,
		); err != nil {
			return nil, err
		}
	}

	// Update the candidate based on if auth has changed to flag that auth
	// be refreshed after upgrade
	if err := applyAuthMigrationAnalysis(log, candidate); err != nil {
		return nil, err
	}

	// Run the target probe set after migration. Probes that existed on the
	// source generation can still change outcome after config/auth/label updates.
	candidate.ProbeIdsToRun = targetProbeIDs(target.GetDefinition())

	if err := applySetupFlowMigrationAnalysis(log, candidate); err != nil {
		return nil, err
	}
	applyRequiredActionNotification(candidate)

	return candidate, nil
}

// migrationGenerationPath retrieves the ordered list of connector generations that
// will be included in the migration.
func (s *service) migrationGenerationPath(
	ctx context.Context,
	connectorID apid.ID,
	sourceGeneration,
	targetGeneration uint64,
) ([]*Connector, error) {
	var generations []*Connector
	if targetGeneration > sourceGeneration {
		for v := sourceGeneration + 1; v <= targetGeneration; v++ {
			c, err := s.getConnectorGeneration(ctx, connectorID, v)
			if err != nil {
				return nil, err
			}
			generations = append(generations, c)
		}
		return generations, nil
	}

	for v := sourceGeneration; v > targetGeneration; v-- {
		c, err := s.getConnectorGeneration(ctx, connectorID, v)
		if err != nil {
			return nil, err
		}
		generations = append(generations, c)
	}

	return generations, nil
}
