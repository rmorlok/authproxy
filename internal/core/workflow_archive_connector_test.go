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
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestArchiveConnectorWorkflowV1ExecutesRegisteredSteps(t *testing.T) {
	connectorID := apid.New(apid.PrefixConnectorVersion)
	timeout := 5 * time.Minute
	workflowTester := tester.NewWorkflowTester[any](archiveConnectorWorkflowV1)

	prepareActivity := func(context.Context, apid.ID) error {
		return nil
	}
	finalizeActivity := func(context.Context, apid.ID) error {
		return nil
	}
	disconnectAllWorkflow := func(wflib.Context, disconnectConnectorConnectionsWorkflowInputV1) error {
		return nil
	}

	prepareCall := workflowTester.
		OnActivityByName(
			ActivityNameArchiveConnectorPrepareVersionsV1,
			prepareActivity,
			testifymock.Anything,
			connectorID,
		).
		Return(nil).
		Once()
	disconnectCall := workflowTester.
		OnSubWorkflowByName(
			WorkflowNameDisconnectConnectorConnectionsV1,
			disconnectAllWorkflow,
			testifymock.Anything,
			disconnectConnectorConnectionsWorkflowInputV1{
				ConnectorID: connectorID,
				Timeout:     timeout,
			},
		).
		Return(nil).
		Once().
		NotBefore(prepareCall)
	workflowTester.
		OnActivityByName(
			ActivityNameArchiveConnectorFinalizeVersionsV1,
			finalizeActivity,
			testifymock.Anything,
			connectorID,
		).
		Return(nil).
		Once().
		NotBefore(disconnectCall)

	workflowTester.Execute(context.Background(), archiveConnectorWorkflowInputV1{
		ConnectorID: connectorID,
		Timeout:     timeout,
	})

	require.True(t, workflowTester.WorkflowFinished())
	_, err := workflowTester.WorkflowResult()
	require.NoError(t, err)
	workflowTester.AssertExpectations(t)
}

func TestArchiveConnectorWorkflowInstanceID(t *testing.T) {
	connectorID := apid.New(apid.PrefixConnectorVersion)
	require.Equal(
		t,
		WorkflowNameArchiveConnectorV1+":"+connectorID.String(),
		archiveConnectorWorkflowInstanceID(connectorID),
	)
}

func TestRegisterArchiveConnectorWorkflowV1DurableNames(t *testing.T) {
	reg := registry.New()
	svc := &service{}

	require.NoError(t, svc.registerArchiveConnectorWorkflow(reg))

	_, err := reg.GetWorkflow(WorkflowNameArchiveConnectorV1)
	require.NoError(t, err)

	_, err = reg.GetActivity(ActivityNameArchiveConnectorPrepareVersionsV1)
	require.NoError(t, err)

	_, err = reg.GetActivity(ActivityNameArchiveConnectorFinalizeVersionsV1)
	require.NoError(t, err)
}

func TestArchiveConnectorStartsWorkflow(t *testing.T) {
	ctx := context.Background()
	connectorID := apid.New(apid.PrefixConnectorVersion)
	workflowClient := &fakeDisconnectWorkflowClient{
		instance: &wflib.Instance{
			InstanceID:  "workflow-instance",
			ExecutionID: "workflow-execution",
		},
	}
	svc := &service{wc: workflowClient}

	taskInfo, err := svc.ArchiveConnector(ctx, connectorID, iface.ConnectorLifecycleOptions{
		Timeout: 5 * time.Minute,
	})
	require.NoError(t, err)
	require.NotNil(t, taskInfo)
	require.Equal(t, WorkflowNameArchiveConnectorV1, taskInfo.WorkflowName)
	require.Equal(t, apworkflows.DefaultQueue, workflowClient.options.Queue)
	require.Equal(t, WorkflowNameArchiveConnectorV1, workflowClient.workflow)
	require.Len(t, workflowClient.args, 1)
	require.Equal(t, archiveConnectorWorkflowInstanceID(connectorID), workflowClient.options.InstanceID)

	input, ok := workflowClient.args[0].(archiveConnectorWorkflowInputV1)
	require.True(t, ok)
	require.Equal(t, connectorID, input.ConnectorID)
	require.Equal(t, 5*time.Minute, input.Timeout)
}

func TestArchiveConnectorVersionActivitiesTransitionStates(t *testing.T) {
	ctx := context.Background()
	_, db := database.MustApplyBlankTestDbConfig(t, nil)
	svc := &service{db: db}
	connectorID := apid.New(apid.PrefixConnectorVersion)

	upsertConnectorVersion(t, db, connectorID, 1, database.ConnectorVersionStatePrimary)
	upsertConnectorVersion(t, db, connectorID, 2, database.ConnectorVersionStatePrimary)
	upsertConnectorVersion(t, db, connectorID, 3, database.ConnectorVersionStateDraft)

	require.NoError(t, svc.prepareArchiveConnectorVersionsV1(ctx, connectorID))
	requireConnectorVersionState(t, db, connectorID, 1, database.ConnectorVersionStateActive)
	requireConnectorVersionState(t, db, connectorID, 2, database.ConnectorVersionStateActive)
	requireConnectorVersionState(t, db, connectorID, 3, database.ConnectorVersionStateArchived)

	require.NoError(t, svc.prepareArchiveConnectorVersionsV1(ctx, connectorID))
	require.NoError(t, svc.finalizeArchiveConnectorVersionsV1(ctx, connectorID))
	requireConnectorVersionState(t, db, connectorID, 1, database.ConnectorVersionStateArchived)
	requireConnectorVersionState(t, db, connectorID, 2, database.ConnectorVersionStateArchived)
	requireConnectorVersionState(t, db, connectorID, 3, database.ConnectorVersionStateArchived)

	require.NoError(t, svc.finalizeArchiveConnectorVersionsV1(ctx, connectorID))
}

func TestArchiveConnectorVersionActivitiesReturnNotFound(t *testing.T) {
	ctx := context.Background()
	_, db := database.MustApplyBlankTestDbConfig(t, nil)
	svc := &service{db: db}
	connectorID := apid.New(apid.PrefixConnectorVersion)

	require.ErrorIs(t, svc.prepareArchiveConnectorVersionsV1(ctx, connectorID), database.ErrNotFound)
	require.ErrorIs(t, svc.finalizeArchiveConnectorVersionsV1(ctx, connectorID), database.ErrNotFound)
}

func upsertConnectorVersion(
	t *testing.T,
	db database.DB,
	connectorID apid.ID,
	version uint64,
	state database.ConnectorVersionState,
) {
	t.Helper()
	require.NoError(t, db.UpsertConnectorVersion(context.Background(), &database.ConnectorVersion{
		Id:                  connectorID,
		Version:             version,
		Namespace:           sconfig.RootNamespace,
		State:               state,
		Hash:                "hash",
		EncryptedDefinition: encfield.EncryptedField{ID: apid.MustParse("ekv_test000000000001"), Data: "encrypted-definition"},
		Labels:              database.Labels{"type": "test"},
	}))
}

func requireConnectorVersionState(
	t *testing.T,
	db database.DB,
	connectorID apid.ID,
	version uint64,
	expected database.ConnectorVersionState,
) {
	t.Helper()
	connectorVersion, err := db.GetConnectorVersion(context.Background(), connectorID, version)
	require.NoError(t, err)
	require.Equal(t, expected, connectorVersion.State)
}
