package core

import (
	"context"
	"errors"
	"testing"

	"github.com/cschleiden/go-workflows/client"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/tasks"
	"github.com/stretchr/testify/require"

	mockAsynq "github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/apid"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	mockEncrypt "github.com/rmorlok/authproxy/internal/encrypt/mock"
	"github.com/stretchr/testify/assert"
)

type fakeDisconnectWorkflowClient struct {
	instance *wflib.Instance
	err      error

	options  client.WorkflowInstanceOptions
	workflow wflib.Workflow
	args     []any
}

func (f *fakeDisconnectWorkflowClient) CreateWorkflowInstance(ctx context.Context, options client.WorkflowInstanceOptions, workflow wflib.Workflow, args ...any) (*wflib.Instance, error) {
	f.options = options
	f.workflow = workflow
	f.args = args
	return f.instance, f.err
}

func TestDisconnectConnection(t *testing.T) {
	ctx := context.Background()
	connectionId := apid.New(apid.PrefixConnection)

	setup := func(t *testing.T) (*service, *mockDb.MockDB, *fakeDisconnectWorkflowClient, *gomock.Controller) {
		ctrl := gomock.NewController(t)
		db := mockDb.NewMockDB(ctrl)
		ac := mockAsynq.NewMockClient(ctrl)
		encrypt := mockEncrypt.NewMockE(ctrl)
		logger, _ := mockLog.NewTestLogger(t)
		wc := &fakeDisconnectWorkflowClient{
			instance: &wflib.Instance{
				InstanceID:  "workflow-instance-id",
				ExecutionID: "workflow-execution-id",
			},
		}

		return &service{
			cfg:     nil,
			db:      db,
			encrypt: encrypt,
			ac:      ac,
			wc:      wc,
			logger:  logger,
		}, db, wc, ctrl
	}

	t.Run("successfully disconnect connection", func(t *testing.T) {
		svc, dbMock, workflowClient, ctrl := setup(t)
		defer ctrl.Finish()

		dbMock.
			EXPECT().
			SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateDisconnecting).
			Return(nil)

		taskInfo, err := svc.DisconnectConnection(ctx, connectionId)

		assert.NoError(t, err)
		require.NotNil(t, taskInfo)
		assert.Equal(t, tasks.TrackedViaWorkflow, taskInfo.TrackedVia)
		assert.Equal(t, "workflow-instance-id", taskInfo.WorkflowInstanceId)
		assert.Equal(t, "workflow-execution-id", taskInfo.WorkflowExecutionId)
		assert.Equal(t, WorkflowNameDisconnectConnectionV1, taskInfo.WorkflowName)
		assert.Equal(t, WorkflowNameDisconnectConnectionV1, workflowClient.workflow)
		assert.Equal(t, []any{connectionId.String()}, workflowClient.args)
		assert.Contains(t, workflowClient.options.InstanceID, connectionId.String())
	})

	t.Run("database not found error", func(t *testing.T) {
		svc, dbMock, _, ctrl := setup(t)
		defer ctrl.Finish()

		dbMock.EXPECT().
			SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateDisconnecting).
			Return(database.ErrNotFound)

		taskInfo, err := svc.DisconnectConnection(ctx, connectionId)

		assert.Error(t, err)
		assert.Nil(t, taskInfo)
	})

	t.Run("database internal error", func(t *testing.T) {
		svc, dbMock, _, ctrl := setup(t)
		defer ctrl.Finish()

		dbMock.EXPECT().
			SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateDisconnecting).
			Return(errors.New("some error"))

		taskInfo, err := svc.DisconnectConnection(ctx, connectionId)

		assert.Error(t, err)
		assert.Nil(t, taskInfo)
	})

	t.Run("workflow start error", func(t *testing.T) {
		svc, dbMock, workflowClient, ctrl := setup(t)
		defer ctrl.Finish()

		dbMock.EXPECT().
			SetConnectionState(gomock.Any(), connectionId, database.ConnectionStateDisconnecting).
			Return(nil)

		workflowClient.err = errors.New("workflow error")

		taskInfo, err := svc.DisconnectConnection(ctx, connectionId)

		assert.Error(t, err)
		assert.Nil(t, taskInfo)
	})
}
