package tasks

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"golang.org/x/time/rate"
)

const taskTypeReconcileCarryForwardLabels = "database:reconcile_carry_forward_labels"

// Defaults for the reconciliation pass. Drift is rare under normal
// operation, so the rate limit is set conservatively to keep DB load
// negligible. At 100 records/sec a million rows take about 2.8 hours and one
// run covers up to about 7.9 million rows. A larger fleet takes more than one
// run: the sweep checkpoints its progress, so the next run continues from where
// this one stopped rather than starting over.
const (
	reconcileBatchSize        int32      = 100
	reconcileRecordsPerSecond rate.Limit = 100
	reconcileBurst            int        = 100

	// reconcileRunBudget is how long one run walks rows before stopping cleanly.
	// Stopping short of the daily schedule keeps runs from overlapping.
	reconcileRunBudget = 22 * time.Hour

	// reconcileBudgetSlack treats an error this close to the budget's end as the
	// budget ending. rate.Limiter.Wait fails as soon as a wait would pass the
	// deadline, slightly before the context itself expires, with an error that
	// does not wrap context.DeadlineExceeded.
	reconcileBudgetSlack = time.Second

	// reconcileTaskTimeout must exceed reconcileRunBudget. asynq fails a task when
	// its timeout passes whatever the handler returns, so this is only a backstop
	// for a run stuck past its budget.
	reconcileTaskTimeout = 23 * time.Hour

	// reconcileMaxRetry stays small because retries only bridge transient errors:
	// once they run out, the next daily run continues from the checkpoint.
	reconcileMaxRetry = 3
)

// reconcileTaskOptions overrides asynq's defaults of a 30 minute timeout, which
// would end a run after about 180,000 records, and 25 retries.
func reconcileTaskOptions() []asynq.Option {
	return []asynq.Option{
		asynq.Timeout(reconcileTaskTimeout),
		asynq.MaxRetry(reconcileMaxRetry),
	}
}

func newReconcileCarryForwardLabelsTask() *asynq.Task {
	return asynq.NewTask(taskTypeReconcileCarryForwardLabels, nil, reconcileTaskOptions()...)
}

func (th *taskHandler) reconcileCarryForwardLabels(ctx context.Context, _ *asynq.Task) error {
	return th.runReconcile(ctx, reconcileRunBudget)
}

// runReconcile advances the sweep for at most budget. Running out of budget is a
// success because the sweep's checkpoint lets the next run continue; any other
// error is returned so asynq retries, and cancellation of ctx is left to asynq.
func (th *taskHandler) runReconcile(ctx context.Context, budget time.Duration) error {
	th.logger.Info("running carry-forward labels reconciliation task")

	// The deadline is compared with real time because contexts expire in real time.
	deadline := time.Now().Add(budget)
	runCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	limiter := rate.NewLimiter(reconcileRecordsPerSecond, reconcileBurst)
	corrected, err := th.db.ReconcileCarryForwardLabels(runCtx, reconcileBatchSize, limiter)
	if err != nil {
		if ctx.Err() == nil && time.Until(deadline) <= reconcileBudgetSlack {
			th.logger.Info("carry-forward labels reconciliation paused until next run", "corrected", corrected)
			return nil
		}
		th.logger.Error("carry-forward labels reconciliation failed", "corrected_so_far", corrected, "error", err)
		return err
	}
	th.logger.Info("carry-forward labels reconciliation complete", "corrected", corrected)
	return nil
}
