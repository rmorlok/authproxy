package tasks

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/aplog"
	dbMock "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/stretchr/testify/require"
)

func TestReconcileCarryForwardLabelsTask(t *testing.T) {
	t.Run("delegates to ReconcileCarryForwardLabels with defaults", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		mockDB.EXPECT().
			ReconcileCarryForwardLabels(gomock.Any(), reconcileBatchSize, reconcileInterBatchDelay).
			Return(int64(3), nil)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}
		require.NoError(t, th.reconcileCarryForwardLabels(context.Background(), asynq.NewTask(taskTypeReconcileCarryForwardLabels, nil)))
	})

	t.Run("propagates DB errors so asynq retries", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		dbErr := errors.New("transient")
		mockDB.EXPECT().
			ReconcileCarryForwardLabels(gomock.Any(), reconcileBatchSize, reconcileInterBatchDelay).
			Return(int64(0), dbErr)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}
		err := th.reconcileCarryForwardLabels(context.Background(), asynq.NewTask(taskTypeReconcileCarryForwardLabels, nil))
		require.ErrorIs(t, err, dbErr)
	})
}

func TestNewReconcileCarryForwardLabelsTask(t *testing.T) {
	task := newReconcileCarryForwardLabelsTask()
	require.Equal(t, taskTypeReconcileCarryForwardLabels, task.Type())
}
