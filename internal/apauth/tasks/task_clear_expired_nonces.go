package tasks

import (
	"context"

	"github.com/hibiken/asynq"
)

const taskTypeClearExpiredNonces = "auth_tasks:clear_expired_nonces"

func (th *taskHandler) clearExpiredNonces(ctx context.Context, t *asynq.Task) error {
	th.logger.Info("running clear expired nonces task")
	return th.db.DeleteExpiredNonces(ctx)
}

func newClearExpiredNoncesTask() *asynq.Task {
	return asynq.NewTask(taskTypeClearExpiredNonces, nil)
}
