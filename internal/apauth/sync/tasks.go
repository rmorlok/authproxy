package sync

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

const (
	taskTypeSyncActorsExternalSource = "actor_sync:sync_external_source"
)

func GetTaskTypeSyncActorsExternalSourceTask() *asynq.Task {
	return asynq.NewTask(
		taskTypeSyncActorsExternalSource,
		nil,
		asynq.MaxRetry(3),
	)
}

// TaskRegistrar interface for registering tasks and providing cron configs.
type TaskRegistrar interface {
	RegisterTasks(mux *asynq.ServeMux)
	GetCronTasks() []*asynq.PeriodicTaskConfig
}

type taskHandler struct {
	cfg     config.C
	db      database.DB
	encrypt encrypt.E
	logger  *slog.Logger
}

// NewTaskHandler creates a new task handler for actor sync operations.
func NewTaskHandler(
	cfg config.C,
	db database.DB,
	encrypt encrypt.E,
	logger *slog.Logger,
) TaskRegistrar {
	return &taskHandler{
		cfg:     cfg,
		db:      db,
		encrypt: encrypt,
		logger:  logger,
	}
}

// RegisterTasks registers the actor sync task handlers with the asynq mux.
func (th *taskHandler) RegisterTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskTypeSyncActorsExternalSource, th.syncConfiguredActorsExternalSource)
}

// GetCronTasks returns the cron task configurations for actor sync.
// Only returns tasks if ConfiguredActorsExternalSource is configured.
func (th *taskHandler) GetCronTasks() []*asynq.PeriodicTaskConfig {
	actors := th.cfg.GetRoot().SystemAuth.Actors
	if actors == nil {
		return nil
	}

	// Only create cron task for external source configuration
	caes, ok := actors.InnerVal.(*sconfig.ConfiguredActorsExternalSource)
	if !ok {
		return nil
	}

	return []*asynq.PeriodicTaskConfig{
		{
			Cronspec: caes.GetSyncCronScheduleOrDefault(),
			Task:     GetTaskTypeSyncActorsExternalSourceTask(),
		},
	}
}

// syncConfiguredActorsExternalSource is the task handler for syncing actors from external source.
func (th *taskHandler) syncConfiguredActorsExternalSource(ctx context.Context, task *asynq.Task) error {
	th.logger.Info("starting configured actors external source sync task")

	svc := NewService(th.cfg, th.db, th.encrypt, th.logger)
	if err := svc.SyncConfiguredActorsExternalSource(ctx); err != nil {
		th.logger.Error("configured actors external source sync failed", "error", err)
		return err
	}

	th.logger.Info("configured actors external source sync task completed")
	return nil
}
