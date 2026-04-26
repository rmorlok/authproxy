package core

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryConnectionSetup(t *testing.T) {
	t.Run("returns error when setup step is not a failed terminal state", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", JsonSchema: tenantSchema},
				},
			},
		})
		step := cschema.MustNewIndexedSetupStep(cschema.SetupPhasePreconnect, 0)
		conn.SetupStep = &step
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

		_, err := conn.s.RetryConnectionSetup(context.Background(), conn.Id, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not in a retryable state")
	})

	t.Run("resets to preconnect:0 when connector has preconnect", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", Title: "Tenant", JsonSchema: tenantSchema},
				},
			},
		})
		step := cschema.SetupStepVerifyFailed
		conn.SetupStep = &step
		errMsg := "probe failed"
		conn.SetupError = &errMsg
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
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewIndexedSetupStep(cschema.SetupPhasePreconnect, 0))).Return(nil)

		resp, err := conn.s.RetryConnectionSetup(context.Background(), conn.Id, "")
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, iface.PreconnectionResponseTypeForm, resp.GetType())
		form := resp.(*iface.InitiateConnectionForm)
		assert.Equal(t, "tenant", form.StepId)
	})

	t.Run("resets to preconnect:0 from auth_failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", Title: "Tenant", JsonSchema: tenantSchema},
				},
			},
		})
		step := cschema.SetupStepAuthFailed
		conn.SetupStep = &step
		errMsg := "token exchange failed"
		conn.SetupError = &errMsg
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
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewIndexedSetupStep(cschema.SetupPhasePreconnect, 0))).Return(nil)

		resp, err := conn.s.RetryConnectionSetup(context.Background(), conn.Id, "")
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, iface.PreconnectionResponseTypeForm, resp.GetType())
		form := resp.(*iface.InitiateConnectionForm)
		assert.Equal(t, "tenant", form.StepId)
	})
}
