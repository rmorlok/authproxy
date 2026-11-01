package tasks

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	dbMock "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// rateLimiterMatcher matches any *rate.Limiter whose limit equals
// reconcileRecordsPerSecond — the handler is expected to construct one
// with the package-level defaults each invocation.
type rateLimiterMatcher struct{}

func (rateLimiterMatcher) Matches(x interface{}) bool {
	l, ok := x.(*rate.Limiter)
	if !ok {
		return false
	}
	return l != nil && l.Limit() == reconcileRecordsPerSecond
}
func (rateLimiterMatcher) String() string { return "is *rate.Limiter at default rps" }

func TestReconcileCarryForwardLabelsTask(t *testing.T) {
	t.Run("delegates to ReconcileCarryForwardLabels with default limiter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		mockDB.EXPECT().
			ReconcileCarryForwardLabels(gomock.Any(), gomock.Any(), reconcileBatchSize, rateLimiterMatcher{}).
			Return(int64(3), nil)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}
		require.NoError(t, th.reconcileCarryForwardLabels(context.Background(), asynq.NewTask(taskTypeReconcileCarryForwardLabels, nil)))
	})

	t.Run("propagates DB errors so asynq retries", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		dbErr := errors.New("transient")
		mockDB.EXPECT().
			ReconcileCarryForwardLabels(gomock.Any(), gomock.Any(), reconcileBatchSize, rateLimiterMatcher{}).
			Return(int64(0), dbErr)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}
		err := th.reconcileCarryForwardLabels(context.Background(), asynq.NewTask(taskTypeReconcileCarryForwardLabels, nil))
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("hands the database a deadline within the run budget", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		start := time.Now()

		// The sweep must be told to stop before asynq would fail the task.
		mockDB.EXPECT().
			ReconcileCarryForwardLabels(gomock.Any(), gomock.Any(), reconcileBatchSize, rateLimiterMatcher{}).
			DoAndReturn(func(ctx context.Context, _ *database.ReconcileCheckpointer, _ int32, _ *rate.Limiter) (int64, error) {
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.False(t, deadline.Before(start.Add(reconcileRunBudget)))
				require.True(t, deadline.Before(start.Add(reconcileTaskTimeout)))
				return 0, nil
			})

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}
		require.NoError(t, th.reconcileCarryForwardLabels(context.Background(), asynq.NewTask(taskTypeReconcileCarryForwardLabels, nil)))
	})

	t.Run("running out of budget succeeds so the next run resumes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)

		// The sweep stops at its deadline; the checkpoint carries the remaining work.
		mockDB.EXPECT().
			ReconcileCarryForwardLabels(gomock.Any(), gomock.Any(), reconcileBatchSize, rateLimiterMatcher{}).
			DoAndReturn(func(ctx context.Context, _ *database.ReconcileCheckpointer, _ int32, _ *rate.Limiter) (int64, error) {
				<-ctx.Done()
				return 7, ctx.Err()
			})

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}
		require.NoError(t, th.runReconcile(context.Background(), time.Millisecond))
	})

	t.Run("a limiter refusing to wait past the deadline counts as out of budget", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)

		// The limiter fails before the context expires, with an unrelated error type.
		mockDB.EXPECT().
			ReconcileCarryForwardLabels(gomock.Any(), gomock.Any(), reconcileBatchSize, rateLimiterMatcher{}).
			DoAndReturn(func(ctx context.Context, _ *database.ReconcileCheckpointer, _ int32, _ *rate.Limiter) (int64, error) {
				require.NoError(t, ctx.Err())
				return 0, errors.New("rate: Wait(n=1) would exceed context deadline")
			})

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}
		require.NoError(t, th.runReconcile(context.Background(), reconcileBudgetSlack/2))
	})

	t.Run("hands the database a checkpointer keyed by the task type", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)

		// A stable key is what lets each scheduled run continue the last one.
		key := database.TaskCheckpointKey{TaskType: taskTypeReconcileCarryForwardLabels}
		mockDB.EXPECT().GetTaskCheckpoint(gomock.Any(), key).Return(nil, database.ErrNotFound)
		mockDB.EXPECT().
			SaveTaskCheckpoint(gomock.Any(), key, "", gomock.Any(), reconcileCheckpointTTL).
			Return("rev-1", nil)
		mockDB.EXPECT().
			ReconcileCarryForwardLabels(gomock.Any(), gomock.Any(), reconcileBatchSize, rateLimiterMatcher{}).
			DoAndReturn(func(ctx context.Context, checkpointer *database.ReconcileCheckpointer, _ int32, _ *rate.Limiter) (int64, error) {
				position, err := checkpointer.Load(ctx)
				require.NoError(t, err)
				require.Nil(t, position)
				require.NoError(t, checkpointer.Save(ctx, database.ReconcilePosition{Step: "keys"}))
				return 0, nil
			})

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}
		require.NoError(t, th.reconcileCarryForwardLabels(context.Background(), asynq.NewTask(taskTypeReconcileCarryForwardLabels, nil)))
	})

	t.Run("losing the checkpoint to an overlapping run succeeds without a retry", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)

		// Retrying would only compete with the run that now owns the sweep.
		mockDB.EXPECT().
			ReconcileCarryForwardLabels(gomock.Any(), gomock.Any(), reconcileBatchSize, rateLimiterMatcher{}).
			Return(int64(2), fmt.Errorf("save reconcile checkpoint: %w", database.ErrTaskCheckpointConflict))

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}
		require.NoError(t, th.reconcileCarryForwardLabels(context.Background(), asynq.NewTask(taskTypeReconcileCarryForwardLabels, nil)))
	})

	t.Run("cancellation by asynq is still reported as an error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)

		// A worker shutting down cancels the parent context, not the budget.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		mockDB.EXPECT().
			ReconcileCarryForwardLabels(gomock.Any(), gomock.Any(), reconcileBatchSize, rateLimiterMatcher{}).
			Return(int64(0), context.Canceled)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}
		require.ErrorIs(t, th.runReconcile(ctx, reconcileBudgetSlack/2), context.Canceled)
	})
}

func TestNewReconcileCarryForwardLabelsTask(t *testing.T) {
	task := newReconcileCarryForwardLabelsTask()
	require.Equal(t, taskTypeReconcileCarryForwardLabels, task.Type())
}

func TestReconcileTaskOptions(t *testing.T) {
	// Collect the options the same way asynq reads them when enqueueing.
	var timeout time.Duration
	var maxRetry int
	for _, opt := range reconcileTaskOptions() {
		switch opt.Type() {
		case asynq.TimeoutOpt:
			timeout = opt.Value().(time.Duration)
		case asynq.MaxRetryOpt:
			maxRetry = opt.Value().(int)
		}
	}

	// The hard timeout must leave room for the run to stop itself, and stay under
	// the daily schedule so that runs do not overlap.
	require.Equal(t, reconcileTaskTimeout, timeout)
	require.Greater(t, timeout, reconcileRunBudget+reconcileBudgetSlack)
	require.Less(t, timeout, 24*time.Hour)
	require.Equal(t, reconcileMaxRetry, maxRetry)

	// Progress must survive the gap between daily runs.
	require.Greater(t, reconcileCheckpointTTL, 24*time.Hour)

	// The budget must cover the documented scale of millions of rows.
	require.GreaterOrEqual(t, reconcileRunBudget.Seconds()*float64(reconcileRecordsPerSecond), 5_000_000.0)
}
