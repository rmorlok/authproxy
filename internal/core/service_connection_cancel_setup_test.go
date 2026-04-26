package core

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCancelSetup(t *testing.T) {
	t.Run("clears setup_step and setup_error when state is ready", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{{Id: "x", JsonSchema: workspaceSchema}},
			},
		})
		conn.State = database.ConnectionStateReady
		step := cschema.MustNewIndexedSetupStep(cschema.SetupPhaseConfigure, 0)
		conn.SetupStep = &step
		errMsg := "stale"
		conn.SetupError = &errMsg

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*cschema.SetupStep)(nil)).Return(nil)
		db.EXPECT().SetConnectionSetupError(gomock.Any(), conn.Id, (*string)(nil)).Return(nil)

		require.NoError(t, conn.CancelSetup(context.Background()))
		assert.Nil(t, conn.GetSetupStep())
		assert.Nil(t, conn.GetSetupError())
		assert.Equal(t, database.ConnectionStateReady, conn.GetState())
	})

	t.Run("no-op when ready and nothing to clear", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})
		conn.State = database.ConnectionStateReady

		require.NoError(t, conn.CancelSetup(context.Background()))
	})

	t.Run("rejects when not ready", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})
		conn.State = database.ConnectionStateCreated

		err := conn.CancelSetup(context.Background())
		require.Error(t, err)
	})
}
