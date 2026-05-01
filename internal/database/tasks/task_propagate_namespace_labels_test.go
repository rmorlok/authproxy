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

func TestPropagateNamespaceLabelsTask(t *testing.T) {
	t.Run("delegates to RefreshNamespaceLabelsCarryForward", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		mockDB.EXPECT().
			RefreshNamespaceLabelsCarryForward(gomock.Any(), "root.foo").
			Return(nil)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}

		task, err := NewPropagateNamespaceLabelsTask("root.foo")
		require.NoError(t, err)
		require.NoError(t, th.propagateNamespaceLabels(context.Background(), task))
	})

	t.Run("propagates DB errors so the task can be retried", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		dbErr := errors.New("transient")
		mockDB.EXPECT().
			RefreshNamespaceLabelsCarryForward(gomock.Any(), "root.foo").
			Return(dbErr)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}

		task, err := NewPropagateNamespaceLabelsTask("root.foo")
		require.NoError(t, err)
		err = th.propagateNamespaceLabels(context.Background(), task)
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("rejects empty payload without retry", func(t *testing.T) {
		th := &taskHandler{logger: aplog.NewNoopLogger()}
		task := asynq.NewTask(taskTypePropagateNamespaceLabels, []byte(`{"namespace_path":""}`))
		err := th.propagateNamespaceLabels(context.Background(), task)
		require.Error(t, err)
		require.ErrorIs(t, err, asynq.SkipRetry)
	})

	t.Run("rejects malformed payload without retry", func(t *testing.T) {
		th := &taskHandler{logger: aplog.NewNoopLogger()}
		task := asynq.NewTask(taskTypePropagateNamespaceLabels, []byte("not json"))
		err := th.propagateNamespaceLabels(context.Background(), task)
		require.Error(t, err)
		require.ErrorIs(t, err, asynq.SkipRetry)
	})
}
