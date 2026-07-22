package core

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnVerifyPassed(t *testing.T) {
	t.Run("advances to first configure when configure exists", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "workspace", JsonSchema: workspaceSchema},
				},
			},
		})
		verify := cschema.SetupStepVerify
		conn.SetupStep = &verify

		expectResolveRequiredActionNotification(db, conn.Id, database.NotificationKeyAuthRequired)
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.MustNewSetupStep("workspace"))).Return(nil)

		err := conn.onVerifyPassed(context.Background())
		require.NoError(t, err)
		require.NotNil(t, conn.GetSetupStep())
		assert.Equal(t, "workspace", conn.GetSetupStep().String())
	})

	t.Run("clears setup step and marks ready when no further steps", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})
		// onVerifyPassed is invoked by the verify task handler only when the
		// connection is on apxy:verify. Reflect that in the fixture.
		verify := cschema.SetupStepVerify
		conn.SetupStep = &verify

		expectResolveRequiredActionNotification(db, conn.Id, database.NotificationKeyAuthRequired)
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*cschema.SetupStep)(nil)).Return(nil)
		db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateConfigured).Return(nil)
		expectResolveRequiredActionNotification(db, conn.Id, database.NotificationKeySetupRequired)

		err := conn.onVerifyPassed(context.Background())
		require.NoError(t, err)
		assert.Nil(t, conn.GetSetupStep())
		assert.Equal(t, database.ConnectionStateConfigured, conn.GetState())
	})

	t.Run("flips health back to healthy when previously unhealthy", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})
		conn.HealthState = database.ConnectionHealthStateUnhealthy
		verify := cschema.SetupStepVerify
		conn.SetupStep = &verify

		db.EXPECT().SetConnectionHealthState(gomock.Any(), conn.Id, database.ConnectionHealthStateHealthy).Return(nil)
		expectResolveRequiredActionNotification(db, conn.Id, database.NotificationKeyAuthRequired)
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*cschema.SetupStep)(nil)).Return(nil)
		db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateConfigured).Return(nil)
		expectResolveRequiredActionNotification(db, conn.Id, database.NotificationKeySetupRequired)

		err := conn.onVerifyPassed(context.Background())
		require.NoError(t, err)
		assert.Equal(t, database.ConnectionHealthStateHealthy, conn.GetHealthState())
	})
}

func TestOnVerifyFailed(t *testing.T) {
	t.Run("records error and moves to verify_failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})

		db.EXPECT().SetConnectionSetupError(gomock.Any(), conn.Id, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, msg *string) error {
				require.NotNil(t, msg)
				assert.Contains(t, *msg, `probe "ping" failed`)
				assert.Contains(t, *msg, "boom")
				return nil
			})
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStep(cschema.SetupStepVerifyFailed)).Return(nil)

		err := conn.onVerifyFailed(context.Background(), "ping", errors.New("boom"))
		require.NoError(t, err)
		require.NotNil(t, conn.GetSetupStep())
		assert.Equal(t, cschema.SetupStepVerifyFailed, *conn.GetSetupStep())
		require.NotNil(t, conn.GetSetupError())
		assert.Contains(t, *conn.GetSetupError(), "boom")
	})
}
