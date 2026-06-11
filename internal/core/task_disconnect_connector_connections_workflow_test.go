package core

import (
	"context"
	"testing"
	"time"

	"github.com/cschleiden/go-workflows/registry"
	"github.com/cschleiden/go-workflows/tester"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDisconnectConnectorConnectionsWorkflowV1ExecutesChildWorkflows(t *testing.T) {
	connectorID := apid.New(apid.PrefixConnectorVersion).String()
	connectionIDs := []string{
		apid.New(apid.PrefixConnection).String(),
		apid.New(apid.PrefixConnection).String(),
	}
	workflowTester := tester.NewWorkflowTester[any](disconnectConnectorConnectionsWorkflowV1)

	listActivity := func(context.Context, string) ([]string, error) {
		return connectionIDs, nil
	}
	forceActivity := func(context.Context, []string) error {
		return nil
	}
	childWorkflow := func(wflib.Context, string) error {
		return nil
	}

	workflowTester.
		OnActivityByName(
			ActivityNameDisconnectConnectorConnectionsListConnectionsV1,
			listActivity,
			testifymock.Anything,
			connectorID,
		).
		Return(connectionIDs, nil).
		Once()
	for _, connectionID := range connectionIDs {
		workflowTester.
			OnSubWorkflowByName(
				WorkflowNameDisconnectConnectionV1,
				childWorkflow,
				testifymock.Anything,
				connectionID,
			).
			Return(nil).
			Once()
	}
	workflowTester.
		OnActivityByName(
			ActivityNameDisconnectConnectorConnectionsForceRemainingV1,
			forceActivity,
			testifymock.Anything,
			testifymock.Anything,
		).
		Return(nil).
		Maybe()

	workflowTester.Execute(context.Background(), disconnectConnectorConnectionsWorkflowInputV1{
		ConnectorID: connectorID,
		Timeout:     time.Minute,
	})

	require.True(t, workflowTester.WorkflowFinished())
	_, err := workflowTester.WorkflowResult()
	require.NoError(t, err)
	workflowTester.AssertExpectations(t)
}

func TestDisconnectConnectorConnectionsWorkflowV1ForcesRemainingOnTimeout(t *testing.T) {
	connectorID := apid.New(apid.PrefixConnectorVersion).String()
	connectionID := apid.New(apid.PrefixConnection).String()
	workflowTester := tester.NewWorkflowTester[any](disconnectConnectorConnectionsWorkflowV1)

	listActivity := func(context.Context, string) ([]string, error) {
		return []string{connectionID}, nil
	}
	forceActivity := func(context.Context, []string) error {
		return nil
	}
	childWorkflow := func(ctx wflib.Context, _ string) error {
		return wflib.Sleep(ctx, time.Hour)
	}

	workflowTester.
		OnActivityByName(
			ActivityNameDisconnectConnectorConnectionsListConnectionsV1,
			listActivity,
			testifymock.Anything,
			connectorID,
		).
		Return([]string{connectionID}, nil).
		Once()
	require.NoError(t, workflowTester.Registry().RegisterWorkflow(childWorkflow, registry.WithName(WorkflowNameDisconnectConnectionV1)))
	workflowTester.
		OnActivityByName(
			ActivityNameDisconnectConnectorConnectionsForceRemainingV1,
			forceActivity,
			testifymock.Anything,
			[]string{connectionID},
		).
		Return(nil).
		Once()

	workflowTester.Execute(context.Background(), disconnectConnectorConnectionsWorkflowInputV1{
		ConnectorID: connectorID,
		Timeout:     time.Millisecond,
	})

	require.True(t, workflowTester.WorkflowFinished())
	_, err := workflowTester.WorkflowResult()
	require.NoError(t, err)
	workflowTester.AssertExpectations(t)
}

func TestDisconnectConnectorConnectionChildWorkflowInstanceID(t *testing.T) {
	connectionID := apid.New(apid.PrefixConnection).String()
	require.Equal(
		t,
		"parent:"+connectionID,
		disconnectConnectorConnectionChildWorkflowInstanceID("parent", connectionID),
	)
}

func TestRegisterDisconnectConnectorConnectionsWorkflowV1DurableNames(t *testing.T) {
	reg := registry.New()
	svc := &service{}

	require.NoError(t, svc.registerDisconnectConnectorConnectionsWorkflow(reg))

	_, err := reg.GetWorkflow(WorkflowNameDisconnectConnectorConnectionsV1)
	require.NoError(t, err)

	_, err = reg.GetActivity(ActivityNameDisconnectConnectorConnectionsListConnectionsV1)
	require.NoError(t, err)

	_, err = reg.GetActivity(ActivityNameDisconnectConnectorConnectionsForceRemainingV1)
	require.NoError(t, err)
}

func TestDisconnectConnectorConnectionsStartsWorkflow(t *testing.T) {
	ctx := context.Background()
	connectorID := apid.New(apid.PrefixConnectorVersion)
	workflowClient := &fakeDisconnectWorkflowClient{
		instance: &wflib.Instance{
			InstanceID:  "workflow-instance",
			ExecutionID: "workflow-execution",
		},
	}
	svc := &service{wc: workflowClient}

	taskInfo, err := svc.DisconnectConnectorConnections(ctx, connectorID, iface.ConnectorLifecycleOptions{
		Timeout: 5 * time.Minute,
	})
	require.NoError(t, err)
	require.NotNil(t, taskInfo)
	require.Equal(t, WorkflowNameDisconnectConnectorConnectionsV1, taskInfo.WorkflowName)
	require.Equal(t, apworkflows.DefaultQueue, workflowClient.options.Queue)
	require.Equal(t, WorkflowNameDisconnectConnectorConnectionsV1, workflowClient.workflow)
	require.Len(t, workflowClient.args, 1)

	input, ok := workflowClient.args[0].(disconnectConnectorConnectionsWorkflowInputV1)
	require.True(t, ok)
	require.Equal(t, connectorID.String(), input.ConnectorID)
	require.Equal(t, 5*time.Minute, input.Timeout)
}
