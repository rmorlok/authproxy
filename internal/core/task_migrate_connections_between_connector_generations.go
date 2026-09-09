package core

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/aplog"
)

const taskTypeMigrateConnectionsBetweenConnectorGenerations = "core:migrate_connections_between_connector_generations"

func newMigrateConnectionsBetweenConnectorGenerationsTask() (*asynq.Task, error) {
	return asynq.NewTask(taskTypeMigrateConnectionsBetweenConnectorGenerations, nil), nil
}

func (s *service) migrateConnectionsBetweenConnectorGenerations(ctx context.Context, t *asynq.Task) error {
	logger := aplog.NewBuilder(s.logger).
		WithTask(t).
		WithCtx(ctx).
		Build()
	logger.Info("Migrate connections between connector generations task started")
	defer logger.Info("Migrate connections between connector generations task completed")

	return nil
}
