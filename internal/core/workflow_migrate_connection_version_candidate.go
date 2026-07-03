package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
)

func (s *service) buildConnectionMigrationCandidate(ctx context.Context, connectionID apid.ID, targetVersion uint64) (*connectionMigrationCandidate, error) {
	conn, err := s.getConnection(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	if conn.ConnectorVersion == targetVersion {
		return nil, fmt.Errorf("connection is already on connector version %d", targetVersion)
	}

	target, err := s.getConnectorVersion(ctx, conn.ConnectorId, targetVersion)
	if err != nil {
		return nil, err
	}
	if target.State != database.ConnectorVersionStatePrimary && target.State != database.ConnectorVersionStateActive {
		return nil, fmt.Errorf("target connector version must be primary or active")
	}

	cfg, err := conn.GetConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	userLabels, _ := database.SplitUserAndApxyLabels(conn.Labels)
	annotations := map[string]string{}
	for k, v := range conn.Annotations {
		annotations[k] = v
	}

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

	versions, err := s.migrationVersionPath(ctx, conn.ConnectorId, conn.ConnectorVersion, targetVersion)
	if err != nil {
		return nil, err
	}
	for _, version := range versions {
		if err := s.applyMigrationHookForVersion(ctx, candidate, version, conn.ConnectorVersion, targetVersion); err != nil {
			return nil, err
		}
	}
	if err := s.applyAuthMigrationAnalysis(candidate); err != nil {
		return nil, err
	}
	s.applyProbeMigrationAnalysis(candidate)
	if err := s.applySetupFlowMigrationAnalysis(candidate); err != nil {
		return nil, err
	}

	return candidate, nil
}

func (s *service) migrationVersionPath(ctx context.Context, connectorID apid.ID, sourceVersion, targetVersion uint64) ([]*ConnectorVersion, error) {
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
