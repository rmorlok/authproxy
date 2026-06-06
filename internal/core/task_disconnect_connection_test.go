package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	mockAuthMethods "github.com/rmorlok/authproxy/internal/auth_methods/mock"
	mockOauth2 "github.com/rmorlok/authproxy/internal/auth_methods/oauth2/mock"
	"github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
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

	setupWithMocks := func(t *testing.T) (*service, *mockDb.MockDB, *mockEncrypt.MockE, *gomock.Controller) {
		ctrl := gomock.NewController(t)
		db := mockDb.NewMockDB(ctrl)
		encrypt := mockEncrypt.NewMockE(ctrl)
		logger, _ := mockLog.NewTestLogger(t)

		return &service{
			cfg:                 nil,
			db:                  db,
			encrypt:             encrypt,
			logger:              logger,
			authMethodFactories: map[cschema.AuthType]auth_methods.Factory{},
		}, db, encrypt, ctrl
	}

	t.Run("successfully disconnect connection", func(t *testing.T) {
		svc, dbMock, e, ctrl := setupWithMocks(t)
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

		err := svc.finalizeDisconnectConnectionV1(ctx, connectionId.String())

		assert.NoError(t, err)
	})

	t.Run("is retriable on database state update error", func(t *testing.T) {
		svc, dbMock, e, ctrl := setupWithMocks(t)
		defer ctrl.Finish()

		mock.MockConnectionRetrieval(context.Background(), dbMock, e, connectionId, apiKeyConnector)

		dbMock.
			EXPECT().
			SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateDisconnected).
			Return(errors.New("some error"))

		err := svc.finalizeDisconnectConnectionV1(ctx, connectionId.String())
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

		svc, dbMock, e, ctrl := setupWithMocks(t)
		defer ctrl.Finish()

		// Swap the OAuth2 factory in the auth-method registry with a mock that
		// returns a mock Authenticator. Disconnect drives revocation through
		// the generic Authenticator surface (SupportsRevoke / Revoke), so the
		// test mocks at that level rather than at the OAuth2-specific one.
		o2Factory := mockOauth2.NewMockFactory(ctrl)
		authMock := mockAuthMethods.NewMockAuthenticator(ctrl)
		svc.authMethodFactories[cschema.AuthTypeOAuth2] = o2Factory

		o2Factory.EXPECT().NewAuthenticator(gomock.Any()).Return(authMock).AnyTimes()
		authMock.EXPECT().SupportsRevoke().Return(true).AnyTimes()
		authMock.EXPECT().Revoke(gomock.Any()).Return(errors.New("3rd party 400")).Times(maxRevokeAttempts)

		mock.MockConnectionRetrieval(context.Background(), dbMock, e, connectionId, oauthConnector)

		// Use a fake clock so the inter-attempt backoff doesn't burn real
		// time. The retry helper sleeps via clk.After + select, which is not
		// auto-stepped by FakeClock (unlike clk.Sleep), so a stepper
		// goroutine advances the clock whenever the helper subscribes.
		fakeClk := tclock.NewFakeClock(time.Now())
		retryCtx := apctx.WithClock(ctx, fakeClk)
		stepperCtx, stopStepper := context.WithCancel(context.Background())
		defer stopStepper()
		go func() {
			for {
				select {
				case <-stepperCtx.Done():
					return
				default:
				}
				if fakeClk.HasWaiters() {
					fakeClk.Step(time.Second)
				}
				time.Sleep(time.Millisecond)
			}
		}()

		err := svc.revokeDisconnectConnectionCredentialsV1(retryCtx, connectionId.String())
		assert.NoError(t, err, "disconnect should succeed even when revocation fails")
	})

	t.Run("is retriable on database delete error", func(t *testing.T) {
		svc, dbMock, e, ctrl := setupWithMocks(t)
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

		err := svc.finalizeDisconnectConnectionV1(ctx, connectionId.String())
		assert.Error(t, err)
		assert.True(t, apasynq.IsRetriable(err))
	})
}
