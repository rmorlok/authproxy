package tasks

import (
	"context"

	"github.com/hibiken/asynq"
)

const taskTypeCleanupExpiredTaskCheckpoints = "database:cleanup_expired_task_checkpoints"

// cleanupExpiredTaskCheckpoints removes checkpoints left behind by work that never
// finished. Expired checkpoints are already ignored, so a failed or delayed run
// only leaves storage unreclaimed until the next one.
func (th *taskHandler) cleanupExpiredTaskCheckpoints(ctx context.Context, _ *asynq.Task) error {
	th.logger.Info("running cleanup expired task checkpoints task")

	deleted, err := th.db.DeleteExpiredTaskCheckpoints(ctx)
	if err != nil {
		th.logger.Error("failed to clean up expired task checkpoints", "error", err)
		return err
	}

	th.logger.Info("cleaned up expired task checkpoints", "deleted", deleted)
	return nil
}

func newCleanupExpiredTaskCheckpointsTask() *asynq.Task {
	return asynq.NewTask(taskTypeCleanupExpiredTaskCheckpoints, nil)
}
