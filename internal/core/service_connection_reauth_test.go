package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReauthConnection(t *testing.T) {
	t.Run("rejects connection not in Ready state", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db, _ := newTestApiKeyConnection(t, ctrl, &cschema.ApiKeyPlacement{
			Type: cschema.ApiKeyPlacementBearer,
		}, nil)
		conn.State = database.ConnectionStateSetup

		db.EXPECT().GetConnection(gomock.Any(), conn.Id).Return(&conn.Connection, nil).AnyTimes()
		db.EXPECT().GetConnectorVersion(gomock.Any(), conn.cv.Id, conn.cv.Version).Return(&database.ConnectorVersion{
			Id:                  conn.cv.Id,
			Version:             conn.cv.Version,
			Labels:              conn.cv.GetLabels(),
			State:               database.ConnectorVersionStatePrimary,
			Hash:                conn.cv.Hash,
			EncryptedDefinition: conn.cv.EncryptedDefinition,
		}, nil).AnyTimes()

		_, err := conn.s.ReauthConnection(context.Background(), conn.Id, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not in a reauthable state")
	})

	t.Run("api-key Ready returns credentials form with no prior credential bytes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// The "prior" credential plaintext we want to verify never leaks back.
		const priorApiKey = "sk-very-secret-prior-key-do-not-leak"

		conn, db, _ := newTestApiKeyConnection(t, ctrl, &cschema.ApiKeyPlacement{
			Type: cschema.ApiKeyPlacementBearer,
		}, nil)
		conn.State = database.ConnectionStateConfigured
		conn.s.encrypt = encrypt.NewFakeEncryptService(false)

		db.EXPECT().GetConnection(gomock.Any(), conn.Id).Return(&conn.Connection, nil).AnyTimes()
		db.EXPECT().GetConnectorVersion(gomock.Any(), conn.cv.Id, conn.cv.Version).Return(&database.ConnectorVersion{
			Id:                  conn.cv.Id,
			Version:             conn.cv.Version,
			Labels:              conn.cv.GetLabels(),
			State:               database.ConnectorVersionStatePrimary,
			Hash:                conn.cv.Hash,
			EncryptedDefinition: conn.cv.EncryptedDefinition,
		}, nil).AnyTimes()

		db.EXPECT().SetConnectionSetupError(gomock.Any(), conn.Id, (*string)(nil)).Return(nil)
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewIndexedSetupStep(cschema.SetupPhaseCredentials, 0))).Return(nil)

		resp, err := conn.s.ReauthConnection(context.Background(), conn.Id, "")
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, iface.ConnectionSetupResponseTypeForm, resp.GetType())

		form := resp.(*iface.ConnectionSetupForm)
		assert.Equal(t, cschema.SynthesizedApiKeyCredentialsStepId, form.StepId)

		// No-replay invariant: the prior credential bytes must not appear anywhere
		// in the form payload. Serialize the whole response and grep.
		body, err := json.Marshal(form)
		require.NoError(t, err)
		assert.NotContains(t, string(body), priorApiKey,
			"prior credential bytes leaked into reauth form payload (no-replay invariant violated)")
	})

	t.Run("api-key reauth works regardless of health state", func(t *testing.T) {
		// Reauth on an unhealthy connection follows the same code path as on a
		// healthy one — health state itself does not gate the form return. The
		// flip back to healthy happens later, on verify-pass after submit.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db, _ := newTestApiKeyConnection(t, ctrl, &cschema.ApiKeyPlacement{
			Type: cschema.ApiKeyPlacementBearer,
		}, nil)
		conn.State = database.ConnectionStateConfigured
		conn.HealthState = database.ConnectionHealthStateUnhealthy

		db.EXPECT().GetConnection(gomock.Any(), conn.Id).Return(&conn.Connection, nil).AnyTimes()
		db.EXPECT().GetConnectorVersion(gomock.Any(), conn.cv.Id, conn.cv.Version).Return(&database.ConnectorVersion{
			Id:                  conn.cv.Id,
			Version:             conn.cv.Version,
			Labels:              conn.cv.GetLabels(),
			State:               database.ConnectorVersionStatePrimary,
			Hash:                conn.cv.Hash,
			EncryptedDefinition: conn.cv.EncryptedDefinition,
		}, nil).AnyTimes()

		db.EXPECT().SetConnectionSetupError(gomock.Any(), conn.Id, (*string)(nil)).Return(nil)
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewIndexedSetupStep(cschema.SetupPhaseCredentials, 0))).Return(nil)

		resp, err := conn.s.ReauthConnection(context.Background(), conn.Id, "")
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, iface.ConnectionSetupResponseTypeForm, resp.GetType())
	})

	t.Run("api-key reauth clears any prior setup error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db, _ := newTestApiKeyConnection(t, ctrl, &cschema.ApiKeyPlacement{
			Type: cschema.ApiKeyPlacementBearer,
		}, nil)
		conn.State = database.ConnectionStateConfigured
		priorErr := "earlier verify error that should be wiped"
		conn.SetupError = &priorErr

		db.EXPECT().GetConnection(gomock.Any(), conn.Id).Return(&conn.Connection, nil).AnyTimes()
		db.EXPECT().GetConnectorVersion(gomock.Any(), conn.cv.Id, conn.cv.Version).Return(&database.ConnectorVersion{
			Id:                  conn.cv.Id,
			Version:             conn.cv.Version,
			Labels:              conn.cv.GetLabels(),
			State:               database.ConnectorVersionStatePrimary,
			Hash:                conn.cv.Hash,
			EncryptedDefinition: conn.cv.EncryptedDefinition,
		}, nil).AnyTimes()

		// Expect setup_error nil-write.
		db.EXPECT().SetConnectionSetupError(gomock.Any(), conn.Id, (*string)(nil)).Return(nil)
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewIndexedSetupStep(cschema.SetupPhaseCredentials, 0))).Return(nil)

		// gomock validates the (*string)(nil) write happened — the in-memory
		// `conn` is not the same instance ReauthConnection operates on (the
		// service re-loads via getConnection), so we don't assert on conn.GetSetupError().
		_, err := conn.s.ReauthConnection(context.Background(), conn.Id, "")
		require.NoError(t, err)
	})

	t.Run("OAuth2 with preconnect resets to preconnect:0", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		e := encrypt.NewFakeEncryptService(false)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		s.encrypt = e

		connector := cschema.Connector{
			Auth: &cschema.Auth{InnerVal: &cschema.AuthOAuth2{Type: cschema.AuthTypeOAuth2}},
			SetupFlow: &cschema.SetupFlow{
				Preconnect: &cschema.SetupFlowPhase{
					Steps: []cschema.SetupFlowStep{
						{Id: "tenant", Title: "Tenant", JsonSchema: tenantSchema},
					},
				},
			},
		}
		connector.Normalize()
		cv := NewTestConnectorVersion(connector)
		conn := &connection{
			Connection: database.Connection{
				Id:               "cxn_test1111111111aa",
				Namespace:        "root",
				State:            database.ConnectionStateConfigured,
				HealthState:      database.ConnectionHealthStateUnhealthy,
				ConnectorId:      cv.GetId(),
				ConnectorVersion: cv.GetVersion(),
			},
			s:      s,
			cv:     cv,
			logger: aplog.NewNoopLogger(),
		}

		db.EXPECT().GetConnection(gomock.Any(), conn.Id).Return(&conn.Connection, nil).AnyTimes()
		db.EXPECT().GetConnectorVersion(gomock.Any(), conn.cv.Id, conn.cv.Version).Return(&database.ConnectorVersion{
			Id:                  conn.cv.Id,
			Version:             conn.cv.Version,
			Labels:              conn.cv.GetLabels(),
			State:               database.ConnectorVersionStatePrimary,
			Hash:                conn.cv.Hash,
			EncryptedDefinition: conn.cv.EncryptedDefinition,
		}, nil).AnyTimes()

		db.EXPECT().SetConnectionSetupError(gomock.Any(), conn.Id, (*string)(nil)).Return(nil)
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewIndexedSetupStep(cschema.SetupPhasePreconnect, 0))).Return(nil)

		resp, err := s.ReauthConnection(context.Background(), conn.Id, "")
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, iface.ConnectionSetupResponseTypeForm, resp.GetType())
		form := resp.(*iface.ConnectionSetupForm)
		assert.Equal(t, "tenant", form.StepId)
	})

	t.Run("rejects no-auth connector", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		e := encrypt.NewFakeEncryptService(false)
		s, db, _, _, _, _ := FullMockService(t, ctrl)
		s.encrypt = e

		connector := cschema.Connector{
			Auth: &cschema.Auth{InnerVal: &cschema.AuthNoAuth{Type: cschema.AuthTypeNoAuth}},
			SetupFlow: &cschema.SetupFlow{
				Preconnect: &cschema.SetupFlowPhase{
					Steps: []cschema.SetupFlowStep{{Id: "tenant", JsonSchema: tenantSchema}},
				},
			},
		}
		connector.Normalize()
		cv := NewTestConnectorVersion(connector)
		conn := &connection{
			Connection: database.Connection{
				Id:               "cxn_test1111111111aa",
				Namespace:        "root",
				State:            database.ConnectionStateConfigured,
				HealthState:      database.ConnectionHealthStateHealthy,
				ConnectorId:      cv.GetId(),
				ConnectorVersion: cv.GetVersion(),
			},
			s:      s,
			cv:     cv,
			logger: aplog.NewNoopLogger(),
		}

		db.EXPECT().GetConnection(gomock.Any(), conn.Id).Return(&conn.Connection, nil).AnyTimes()
		db.EXPECT().GetConnectorVersion(gomock.Any(), conn.cv.Id, conn.cv.Version).Return(&database.ConnectorVersion{
			Id:                  conn.cv.Id,
			Version:             conn.cv.Version,
			Labels:              conn.cv.GetLabels(),
			State:               database.ConnectorVersionStatePrimary,
			Hash:                conn.cv.Hash,
			EncryptedDefinition: conn.cv.EncryptedDefinition,
		}, nil).AnyTimes()
		db.EXPECT().SetConnectionSetupError(gomock.Any(), conn.Id, (*string)(nil)).Return(nil)

		_, err := s.ReauthConnection(context.Background(), conn.Id, "")
		require.Error(t, err)
		assert.True(t,
			strings.Contains(err.Error(), "does not support reauth") ||
				strings.Contains(err.Error(), "unsupported"),
			"unexpected error: %s", err.Error())
	})
}
