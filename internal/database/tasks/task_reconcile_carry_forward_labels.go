package tasks

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

const taskTypeReconcileCarryForwardLabels = "database:reconcile_carry_forward_labels"

// Defaults for the reconciliation pass. Drift is rare under normal
// operation, so a small per-resource page and a short pause between
// resource types is enough throttling to keep DB load negligible.
const (
	reconcileBatchSize       int32         = 100
	reconcileInterBatchDelay time.Duration = 0
)

func newReconcileCarryForwardLabelsTask() *asynq.Task {
	return asynq.NewTask(taskTypeReconcileCarryForwardLabels, nil)
}

func (th *taskHandler) reconcileCarryForwardLabels(ctx context.Context, _ *asynq.Task) error {
	th.logger.Info("running carry-forward labels reconciliation task")
	corrected, err := th.db.ReconcileCarryForwardLabels(ctx, reconcileBatchSize, reconcileInterBatchDelay)
	if err != nil {
		th.logger.Error("carry-forward labels reconciliation failed", "corrected_so_far", corrected, "error", err)
		return err
	}
	th.logger.Info("carry-forward labels reconciliation complete", "corrected", corrected)
	return nil
}
