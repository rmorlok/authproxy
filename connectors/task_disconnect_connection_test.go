package connectors

import (
	"context"
	"errors"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/apasynq"
	mockAsynq "github.com/rmorlok/authproxy/apasynq/mock"
	mockLog "github.com/rmorlok/authproxy/aplog/mock"
	cfg "github.com/rmorlok/authproxy/config/connectors"
	"github.com/rmorlok/authproxy/connectors/mock"
	"github.com/rmorlok/authproxy/database"
	mockDb "github.com/rmorlok/authproxy/database/mock"
	mockEncrypt "github.com/rmorlok/authproxy/encrypt/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTaskDisconnectConnection(t *testing.T) {
	ctx := context.Background()
	connectionId := uuid.New()
	connectorId := uuid.New()

	apiKeyConnector := &cfg.Connector{
		Id:          connectorId,
		Version:     1,
		DisplayName: "Test Connector",
		Auth:        &cfg.AuthApiKey{},
	}

	setupWithMocks := func(t *testing.T) (*service, *mockDb.MockDB, *mockAsynq.MockClient, *mockEncrypt.MockE, *gomock.Controller) {
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
		}, db, ac, encrypt, ctrl
	}

	t.Run("successfully disconnect connection", func(t *testing.T) {
		svc, dbMock, _, e, ctrl := setupWithMocks(t)
		defer ctrl.Finish()

		mock.MockConnectionRetrieval(context.Background(), dbMock, e, connectionId, apiKeyConnector)

		dbMock.
			EXPECT().
			SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateDisconnected).
			Return(nil)

		dbMock.
			EXPECT().
			DeleteConnection(gomock.Any(), connectionId).
			Return(nil)

		task, err := newDisconnectConnectionTask(connectionId)
		require.NoError(t, err)

		err = svc.disconnectConnection(ctx, task)

		assert.NoError(t, err)
	})

	t.Run("is retriable on database state update error", func(t *testing.T) {
		svc, dbMock, _, e, ctrl := setupWithMocks(t)
		defer ctrl.Finish()

		mock.MockConnectionRetrieval(context.Background(), dbMock, e, connectionId, apiKeyConnector)

		dbMock.
			EXPECT().
			SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateDisconnected).
			Return(errors.New("some error"))

		task, err := newDisconnectConnectionTask(connectionId)
		require.NoError(t, err)

		err = svc.disconnectConnection(ctx, task)
		assert.Error(t, err)
		assert.True(t, apasynq.IsRetriable(err))
	})

	t.Run("is retriable on database delete error", func(t *testing.T) {
		svc, dbMock, _, e, ctrl := setupWithMocks(t)
		defer ctrl.Finish()

		mock.MockConnectionRetrieval(context.Background(), dbMock, e, connectionId, apiKeyConnector)

		dbMock.
			EXPECT().
			SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateDisconnected).
			Return(nil)

		dbMock.
			EXPECT().
			DeleteConnection(gomock.Any(), connectionId).
			Return(errors.New("some error"))

		task, err := newDisconnectConnectionTask(connectionId)
		require.NoError(t, err)

		err = svc.disconnectConnection(ctx, task)
		assert.Error(t, err)
		assert.True(t, apasynq.IsRetriable(err))
	})
}
