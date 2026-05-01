package tasks

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	dbMock "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/stretchr/testify/require"
)

func TestPropagateConnectorVersionLabelsTask(t *testing.T) {
	id := apid.MustParse("cxr_test1234567890ab")

	t.Run("delegates to RefreshConnectionsForConnectorVersion", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		mockDB.EXPECT().
			RefreshConnectionsForConnectorVersion(gomock.Any(), id, uint64(2)).
			Return(nil)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}

		task, err := NewPropagateConnectorVersionLabelsTask(id, 2)
		require.NoError(t, err)
		require.NoError(t, th.propagateConnectorVersionLabels(context.Background(), task))
	})

	t.Run("propagates DB errors so the task can be retried", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		dbErr := errors.New("transient")
		mockDB.EXPECT().
			RefreshConnectionsForConnectorVersion(gomock.Any(), id, uint64(2)).
			Return(dbErr)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}

		task, err := NewPropagateConnectorVersionLabelsTask(id, 2)
		require.NoError(t, err)
		err = th.propagateConnectorVersionLabels(context.Background(), task)
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("rejects nil id without retry", func(t *testing.T) {
		th := &taskHandler{logger: aplog.NewNoopLogger()}
		task := asynq.NewTask(taskTypePropagateConnectorVersionLabels, []byte(`{"connector_version_id":"","version":2}`))
		err := th.propagateConnectorVersionLabels(context.Background(), task)
		require.Error(t, err)
		require.ErrorIs(t, err, asynq.SkipRetry)
	})

	t.Run("rejects zero version without retry", func(t *testing.T) {
		th := &taskHandler{logger: aplog.NewNoopLogger()}
		payload := []byte(`{"connector_version_id":"cxr_test1234567890ab","version":0}`)
		task := asynq.NewTask(taskTypePropagateConnectorVersionLabels, payload)
		err := th.propagateConnectorVersionLabels(context.Background(), task)
		require.Error(t, err)
		require.ErrorIs(t, err, asynq.SkipRetry)
	})
}
