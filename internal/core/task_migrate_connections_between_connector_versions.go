package core

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/aplog"
)

const taskTypeMigrateConnectionsBetweenConnectorVersions = "connectors:migrate_connections_between_connector_versions"

func newMigrateConnectionsBetweenConnectorVersionsTask() (*asynq.Task, error) {
	return asynq.NewTask(taskTypeMigrateConnectionsBetweenConnectorVersions, nil), nil
}

func (s *service) migrateConnectionsBetweenConnectorVersions(ctx context.Context, t *asynq.Task) error {
	logger := aplog.NewBuilder(s.logger).
		WithTask(t).
		WithCtx(ctx).
		Build()
	logger.Info("Migrate connections between connector versions task started")
	defer logger.Info("Migrate connections between connector versions task completed")

	return nil
}
