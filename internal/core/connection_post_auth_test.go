package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleCredentialsEstablished(t *testing.T) {
	t.Run("enters verify and enqueues task when connector has probes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db, ac := newTestConnectionWithSetupFlowAndAsynq(t, ctrl, &cschema.SetupFlow{})
		conn.cv.GetDefinition().Probes = []cschema.Probe{{Id: "ping"}}

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStr(cschema.SetupStepVerify.String())).Return(nil)
		ac.EXPECT().
			EnqueueContext(gomock.Any(), gomock.AssignableToTypeOf(&asynq.Task{}), asynq.Retention(10*time.Minute)).
			Return(&asynq.TaskInfo{ID: "mock-task-id"}, nil)

		outcome, err := conn.HandleCredentialsEstablished(context.Background())
		require.NoError(t, err)
		assert.True(t, outcome.SetupPending)
		require.NotNil(t, conn.GetSetupStep())
		assert.Equal(t, cschema.SetupStepVerify.String(), *conn.GetSetupStep())
	})

	t.Run("enters configure:0 when connector has configure but no probes", func(t *testing.T) {
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

		outcome, err := conn.HandleCredentialsEstablished(context.Background())
		require.NoError(t, err)
		assert.True(t, outcome.SetupPending)
		require.NotNil(t, conn.GetSetupStep())
		assert.Equal(t, "configure:0", *conn.GetSetupStep())
	})

	t.Run("clears setup step when connector has neither probes nor configure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})
		authStep := "auth"
		conn.SetupStep = &authStep

		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, (*string)(nil)).Return(nil)

		outcome, err := conn.HandleCredentialsEstablished(context.Background())
		require.NoError(t, err)
		assert.False(t, outcome.SetupPending)
		assert.Nil(t, conn.GetSetupStep())
	})

	t.Run("no-op when no setup step and no probes/configure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})

		outcome, err := conn.HandleCredentialsEstablished(context.Background())
		require.NoError(t, err)
		assert.False(t, outcome.SetupPending)
		assert.Nil(t, conn.GetSetupStep())
	})
}

func TestHandleAuthFailed(t *testing.T) {
	t.Run("records error and moves to auth_failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, db := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{})

		db.EXPECT().SetConnectionSetupError(gomock.Any(), conn.Id, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, msg *string) error {
				require.NotNil(t, msg)
				assert.Contains(t, *msg, "token exchange boom")
				return nil
			})
		db.EXPECT().SetConnectionSetupStep(gomock.Any(), conn.Id, ptrStr(cschema.SetupStepAuthFailed.String())).Return(nil)

		err := conn.HandleAuthFailed(context.Background(), errors.New("token exchange boom"))
		require.NoError(t, err)
		require.NotNil(t, conn.GetSetupStep())
		assert.Equal(t, cschema.SetupStepAuthFailed.String(), *conn.GetSetupStep())
		require.NotNil(t, conn.GetSetupError())
		assert.Contains(t, *conn.GetSetupError(), "token exchange boom")
	})
}
