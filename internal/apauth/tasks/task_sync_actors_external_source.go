package tasks

import (
	"context"

	"github.com/hibiken/asynq"
)

const (
	taskTypeSyncActorsExternalSource = "auth_tasks:sync_external_source"
)

func NewSyncActorsExternalSourceTask() *asynq.Task {
	return asynq.NewTask(
		taskTypeSyncActorsExternalSource,
		nil,
		asynq.MaxRetry(3),
	)
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
