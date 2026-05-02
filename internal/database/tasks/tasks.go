package tasks

import (
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
)

// TaskRegistrar interface for registering tasks and providing cron configs.
type TaskRegistrar interface {
	RegisterTasks(mux *asynq.ServeMux)
	GetCronTasks() []*asynq.PeriodicTaskConfig
}

type taskHandler struct {
	cfg    config.C
	db     database.DB
	logger *slog.Logger
}

// NewTaskHandler creates a new task handler for database maintenance operations.
func NewTaskHandler(
	cfg config.C,
	db database.DB,
	logger *slog.Logger,
) TaskRegistrar {
	return &taskHandler{
		cfg:    cfg,
		db:     db,
		logger: logger,
	}
}

// RegisterTasks registers the database task handlers with the asynq mux.
func (th *taskHandler) RegisterTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskTypePurgeSoftDeleted, th.purgeSoftDeletedRecords)
	mux.HandleFunc(taskTypeCleanupStaleConnections, th.cleanupStaleConnections)
	mux.HandleFunc(taskTypePropagateNamespaceLabels, th.propagateNamespaceLabels)
	mux.HandleFunc(taskTypePropagateConnectorVersionLabels, th.propagateConnectorVersionLabels)
	mux.HandleFunc(taskTypeReconcileCarryForwardLabels, th.reconcileCarryForwardLabels)
}

// GetCronTasks returns the cron task configurations for database maintenance.
func (th *taskHandler) GetCronTasks() []*asynq.PeriodicTaskConfig {
	retention := th.cfg.GetRoot().Database.GetSoftDeleteRetentionOrDefault()

	cronspec := "@daily"
	if retention <= 24*time.Hour {
		cronspec = "@hourly"
	}

	setupTtl := th.cfg.GetRoot().Connections.GetSetupTtlOrDefault()
	cleanupCronspec := "@hourly"
	if setupTtl > 24*time.Hour {
		cleanupCronspec = "@daily"
	}

	return []*asynq.PeriodicTaskConfig{
		{
			Cronspec: cronspec,
			Task:     newPurgeSoftDeletedTask(),
		},
		{
			Cronspec: cleanupCronspec,
			Task:     newCleanupStaleConnectionsTask(),
		},
		{
			Cronspec: "@daily",
			Task:     newReconcileCarryForwardLabelsTask(),
		},
	}
}
