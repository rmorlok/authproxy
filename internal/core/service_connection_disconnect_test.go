package core

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	mockAsynq "github.com/rmorlok/authproxy/internal/apasynq/mock"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	"github.com/stretchr/testify/assert"
)

func TestDisconnectConnection(t *testing.T) {
	ctx := context.Background()
	connectionId := apid.New(apid.PrefixActor)

	setup := func(t *testing.T) (*service, *mockDb.MockDB, *mockAsynq.MockClient, *gomock.Controller) {
		ctrl := gomock.NewController(t)
		db := mockDb.NewMockDB(ctrl)
		ac := mockAsynq.NewMockClient(ctrl)
		encrypt := mockEncrypt.NewMockE(ctrl)
		logger, _ := mockLog.NewTestLogger(t)

		return &service{
			cfg:     nil,
			db:      db,
			encrypt: encrypt,
			ac:      ac,
			logger:  logger,
		}, db, ac, ctrl
	}

	t.Run("successfully disconnect connection", func(t *testing.T) {
		svc, dbMock, asynqMock, ctrl := setup(t)
		defer ctrl.Finish()

		dbMock.
			EXPECT().
			SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateDisconnecting).
			Return(nil)

		taskMatcher := gomock.AssignableToTypeOf(&asynq.Task{})
		asynqMock.
			EXPECT().
			EnqueueContext(gomock.Any(), taskMatcher, asynq.Retention(10*time.Minute)).
			DoAndReturn(func(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
				// Verify the task type
				assert.Equal(t, "connectors:disconnect_connection", task.Type())

				// Parse the payload to verify the connection ID
				var payload struct {
					ConnectionId apid.ID `json:"connection_id"`
				}
				err := json.Unmarshal(task.Payload(), &payload)
				require.NoError(t, err)

				// Verify the connection ID matches what we expect
				assert.Equal(t, connectionId, payload.ConnectionId)

				// Return a mock TaskInfo
				return &asynq.TaskInfo{ID: "mock-task-id"}, nil

			})

		taskInfo, err := svc.DisconnectConnection(ctx, connectionId)

		assert.NoError(t, err)
		assert.Equal(t, "mock-task-id", taskInfo.AsynqId)
	})

	t.Run("database not found error", func(t *testing.T) {
		svc, dbMock, _, ctrl := setup(t)
		defer ctrl.Finish()

		dbMock.EXPECT().
			SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateDisconnecting).
			Return(database.ErrNotFound)

		taskInfo, err := svc.DisconnectConnection(ctx, connectionId)

		assert.Error(t, err)
		assert.Nil(t, taskInfo)
	})

	t.Run("database internal error", func(t *testing.T) {
		svc, dbMock, _, ctrl := setup(t)
		defer ctrl.Finish()

		dbMock.EXPECT().
			SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateDisconnecting).
			Return(errors.New("some error"))

		taskInfo, err := svc.DisconnectConnection(ctx, connectionId)

		assert.Error(t, err)
		assert.Nil(t, taskInfo)
	})

	t.Run("task creation error", func(t *testing.T) {
		svc, dbMock, asynqMock, ctrl := setup(t)
		defer ctrl.Finish()

		dbMock.EXPECT().
			SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateDisconnecting).
			Return(nil)

		asynqMock.
			EXPECT().
			EnqueueContext(gomock.Any(), gomock.Any(), asynq.Retention(10*time.Minute)).
			Return((*asynq.TaskInfo)(nil), errors.New("enqueue error"))

		taskInfo, err := svc.DisconnectConnection(ctx, connectionId)

		assert.Error(t, err)
		assert.Nil(t, taskInfo)
	})
}
