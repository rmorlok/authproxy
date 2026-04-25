package core

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnVerifyPassed(t *testing.T) {
	t.Run("advances to configure:0 when configure exists", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "workspace", JsonSchema: workspaceSchema},
				},
			},
		})

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStr("configure:0")).Return(nil)

		err := conn.onVerifyPassed(context.Background())
		require.NoError(t, err)
		require.NotNil(t, conn.GetSetupStep())
		assert.Equal(t, "configure:0", *conn.GetSetupStep())
	})

	t.Run("clears setup step and marks ready when no further steps", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*string)(nil)).Return(nil)
		db.EXPECT().SetConnectionState(gomock.Any(), conn.Id, database.ConnectionStateReady).Return(nil)

		err := conn.onVerifyPassed(context.Background())
		require.NoError(t, err)
		assert.Nil(t, conn.GetSetupStep())
		assert.Equal(t, database.ConnectionStateReady, conn.GetState())
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
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStr(cschema.SetupStepVerifyFailed.String())).Return(nil)

		err := conn.onVerifyFailed(context.Background(), "ping", errors.New("boom"))
		require.NoError(t, err)
		require.NotNil(t, conn.GetSetupStep())
		assert.Equal(t, cschema.SetupStepVerifyFailed.String(), *conn.GetSetupStep())
		require.NotNil(t, conn.GetSetupError())
		assert.Contains(t, *conn.GetSetupError(), "boom")
	})
}
