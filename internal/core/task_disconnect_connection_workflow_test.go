package core

import (
	"context"
	"testing"

	"github.com/cschleiden/go-workflows/registry"
	"github.com/cschleiden/go-workflows/tester"
	"github.com/rmorlok/authproxy/internal/apid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDisconnectConnectionWorkflowV1ExecutesRegisteredActivities(t *testing.T) {
	connectionId := apid.New(apid.PrefixConnection).String()
	workflowTester := tester.NewWorkflowTester[any](disconnectConnectionWorkflowV1)

	revokeActivity := func(context.Context, string) error {
		return nil
	}
	finalizeActivity := func(context.Context, string) error {
		return nil
	}

	revokeCall := workflowTester.
		OnActivityByName(
			ActivityNameDisconnectConnectionRevokeCredentialsV1,
			revokeActivity,
			testifymock.Anything,
			connectionId,
		).
		Return(nil).
		Once()
	workflowTester.
		OnActivityByName(
			ActivityNameDisconnectConnectionFinalizeV1,
			finalizeActivity,
			testifymock.Anything,
			connectionId,
		).
		Return(nil).
		Once().
		NotBefore(revokeCall)

	workflowTester.Execute(context.Background(), connectionId)

	require.True(t, workflowTester.WorkflowFinished())
	_, err := workflowTester.WorkflowResult()
	require.NoError(t, err)
	workflowTester.AssertExpectations(t)
}

func TestRegisterDisconnectConnectionWorkflowV1DurableNames(t *testing.T) {
	reg := registry.New()
	svc := &service{}

	require.NoError(t, svc.registerDisconnectConnectionWorkflow(reg))

	_, err := reg.GetWorkflow(WorkflowNameDisconnectConnectionV1)
	require.NoError(t, err)

	_, err = reg.GetActivity(ActivityNameDisconnectConnectionRevokeCredentialsV1)
	require.NoError(t, err)

	_, err = reg.GetActivity(ActivityNameDisconnectConnectionFinalizeV1)
	require.NoError(t, err)
}
