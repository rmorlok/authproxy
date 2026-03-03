package tasks

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apctx"
)

const taskTypePurgeSoftDeleted = "database:purge_soft_deleted"

func (th *taskHandler) purgeSoftDeletedRecords(ctx context.Context, t *asynq.Task) error {
	th.logger.Info("running purge soft-deleted records task")

	retention := th.cfg.GetRoot().Database.GetSoftDeleteRetentionOrDefault()
	olderThan := apctx.GetClock(ctx).Now().Add(-retention)

	deleted, err := th.db.PurgeSoftDeletedRecords(ctx, olderThan)
	if err != nil {
		th.logger.Error("failed to purge soft-deleted records", "error", err)
		return err
	}

	th.logger.Info("purged soft-deleted records", "deleted", deleted, "retention", retention.String())
	return nil
}

func newPurgeSoftDeletedTask() *asynq.Task {
	return asynq.NewTask(taskTypePurgeSoftDeleted, nil)
}
