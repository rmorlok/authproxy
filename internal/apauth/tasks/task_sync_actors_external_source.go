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

// syncAdminUsersExternalSource is the task handler for syncing admin users from external source.
func (th *taskHandler) syncAdminUsersExternalSource(ctx context.Context, task *asynq.Task) error {
	th.logger.Info("starting admin users external source sync task")

	svc := NewService(th.cfg, th.db, th.encrypt, th.logger)
	if err := svc.SyncAdminUsersExternalSource(ctx); err != nil {
		th.logger.Error("admin users external source sync failed", "error", err)
		return err
	}

	th.logger.Info("admin users external source sync task completed")
	return nil
}
