package tasks

import (
	"context"

	"github.com/hibiken/asynq"
	"golang.org/x/time/rate"
)

const taskTypeReconcileCarryForwardLabels = "database:reconcile_carry_forward_labels"

// Defaults for the reconciliation pass. Drift is rare under normal
// operation, so the rate limit is set conservatively to keep DB load
// negligible — even on a fleet of millions of rows the daily sweep
// finishes within a handful of hours.
const (
	reconcileBatchSize        int32      = 100
	reconcileRecordsPerSecond rate.Limit = 100
	reconcileBurst            int        = 100
)

func newReconcileCarryForwardLabelsTask() *asynq.Task {
	return asynq.NewTask(taskTypeReconcileCarryForwardLabels, nil)
}

func (th *taskHandler) reconcileCarryForwardLabels(ctx context.Context, _ *asynq.Task) error {
	th.logger.Info("running carry-forward labels reconciliation task")
	limiter := rate.NewLimiter(reconcileRecordsPerSecond, reconcileBurst)
	corrected, err := th.db.ReconcileCarryForwardLabels(ctx, reconcileBatchSize, limiter)
	if err != nil {
		th.logger.Error("carry-forward labels reconciliation failed", "corrected_so_far", corrected, "error", err)
		return err
	}
	th.logger.Info("carry-forward labels reconciliation complete", "corrected", corrected)
	return nil
}
