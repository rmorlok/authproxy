package core

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apasynq"
	mockAsynq "github.com/rmorlok/authproxy/internal/apasynq/mock"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	"github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskDisconnectConnection(t *testing.T) {
	ctx := context.Background()
	connectionId := apid.New(apid.PrefixActor)
	connectorId := apid.New(apid.PrefixActor)

	apiKeyConnector := &cschema.Connector{
		Id:          connectorId,
		Version:     1,
		DisplayName: "Test Connector",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
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
