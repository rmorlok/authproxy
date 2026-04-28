package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apasynq"
	mockAsynq "github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	mockOauth2 "github.com/rmorlok/authproxy/internal/auth_methods/oauth2/mock"
	"github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tclock "k8s.io/utils/clock/testing"
)

func TestTaskDisconnectConnection(t *testing.T) {
	ctx := context.Background()
	connectionId := apid.New(apid.PrefixConnection)
	connectorId := apid.New(apid.PrefixConnectorVersion)

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

	t.Run("retries revocation up to 3 times then proceeds with disconnect", func(t *testing.T) {
		oauthConnector := &cschema.Connector{
			Id:          apid.New(apid.PrefixConnectorVersion),
			Version:     1,
			DisplayName: "OAuth Connector",
			Auth: &cschema.Auth{InnerVal: &cschema.AuthOAuth2{
				Type: cschema.AuthTypeOAuth2,
				Revocation: &cschema.AuthOauth2Revocation{
					Endpoint: "https://example.com/revoke",
				},
			}},
		}

		svc, dbMock, _, e, ctrl := setupWithMocks(t)
		defer ctrl.Finish()

		// Skip lazy init of the real OAuth2 factory by pre-populating the mock.
		o2Factory := mockOauth2.NewMockFactory(ctrl)
		o2Conn := mockOauth2.NewMockOAuth2Connection(ctrl)
		svc.o2Factory = o2Factory
		svc.o2FactoryOnce.Do(func() {})

		o2Factory.EXPECT().NewOAuth2(gomock.Any()).Return(o2Conn).AnyTimes()
		o2Conn.EXPECT().SupportsRevokeTokens().Return(true).AnyTimes()
		o2Conn.EXPECT().RevokeTokens(gomock.Any()).Return(errors.New("3rd party 400")).Times(maxRevokeAttempts)

		mock.MockConnectionRetrieval(context.Background(), dbMock, e, connectionId, oauthConnector)

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

		// Use a fake clock so the inter-attempt sleeps return immediately.
		fakeClk := tclock.NewFakeClock(time.Now())
		retryCtx := apctx.WithClock(ctx, fakeClk)

		err = svc.disconnectConnection(retryCtx, task)
		assert.NoError(t, err, "disconnect should succeed even when revocation fails")
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
