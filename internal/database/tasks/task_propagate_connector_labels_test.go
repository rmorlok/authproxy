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

func TestPropagateConnectorLabelsTask(t *testing.T) {
	id := apid.MustParse("cxr_test1234567890ab")

	t.Run("delegates to RefreshConnectionsForConnector", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		mockDB.EXPECT().
			RefreshConnectionsForConnector(gomock.Any(), id).
			Return(nil)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}

		task, err := NewPropagateConnectorLabelsTask(id)
		require.NoError(t, err)
		require.NoError(t, th.propagateConnectorLabels(context.Background(), task))
	})

	t.Run("propagates DB errors so the task can be retried", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		dbErr := errors.New("transient")
		mockDB.EXPECT().
			RefreshConnectionsForConnector(gomock.Any(), id).
			Return(dbErr)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}

		task, err := NewPropagateConnectorLabelsTask(id)
		require.NoError(t, err)
		err = th.propagateConnectorLabels(context.Background(), task)
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("rejects nil id without retry", func(t *testing.T) {
		th := &taskHandler{logger: aplog.NewNoopLogger()}
		task := asynq.NewTask(taskTypePropagateConnectorLabels, []byte(`{"connector_id":""}`))
		err := th.propagateConnectorLabels(context.Background(), task)
		require.Error(t, err)
		require.ErrorIs(t, err, asynq.SkipRetry)
	})
}
