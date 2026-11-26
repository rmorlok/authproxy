package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cschleiden/go-workflows/registry"
	"github.com/cschleiden/go-workflows/tester"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestMigrateConnectionGenerationWorkflowReconcilesToolSets verifies that queue
// retries cannot repeat a committed migration, including apply failures after
// the generation write but before its notification/auth follow-ups finish.
func TestMigrateConnectionGenerationWorkflowReconcilesToolSets(t *testing.T) {
	for _, tc := range []struct {
		name     string
		applyErr error
	}{
		{name: "successful migration"},
		{name: "apply reported a follow-up failure", applyErr: errors.New("notification update failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			connectionID := apid.New(apid.PrefixConnection)
			workflowTester := tester.NewWorkflowTester[any](migrateConnectionGenerationWorkflowV1)
			activity := func(context.Context, apid.ID, uint64) error { return nil }

			// Existing apply retries finish before the independently retried
			// enqueue activity starts. A successful apply runs exactly once.
			applyAttempts := 1
			if tc.applyErr != nil {
				applyAttempts = wflib.DefaultActivityOptions.RetryOptions.MaxAttempts
			}
			applyCall := workflowTester.OnActivityByName(
				ActivityNameMigrateConnectionGenerationApplyV1, activity,
				testifymock.Anything, connectionID, uint64(2),
			).Return(tc.applyErr).Times(applyAttempts)
			failedEnqueue := workflowTester.OnActivityByName(
				ActivityNameMigrateConnectionGenerationReconcileToolSetsV1, activity,
				testifymock.Anything, connectionID, uint64(2),
			).Return(errors.New("queue unavailable")).Once().NotBefore(applyCall)
			workflowTester.OnActivityByName(
				ActivityNameMigrateConnectionGenerationReconcileToolSetsV1, activity,
				testifymock.Anything, connectionID, uint64(2),
			).Return(nil).Once().NotBefore(failedEnqueue)

			// Recovery still runs after an apply error, while the workflow
			// preserves that error rather than reporting a successful migration.
			workflowTester.Execute(context.Background(), migrateConnectionGenerationWorkflowInputV1{
				ConnectionID: connectionID, TargetGeneration: 2, Timeout: time.Minute,
			})
			require.True(t, workflowTester.WorkflowFinished())
			_, err := workflowTester.WorkflowResult()
			if tc.applyErr != nil {
				require.ErrorContains(t, err, tc.applyErr.Error())
			} else {
				require.NoError(t, err)
			}
			workflowTester.AssertExpectations(t)
		})
	}
}

// TestMigrateConnectionGenerationWorkflowToolSetReconciliationTimeout checks that
// the original migration timeout still bounds the added recovery activity.
func TestMigrateConnectionGenerationWorkflowToolSetReconciliationTimeout(t *testing.T) {
	connectionID := apid.New(apid.PrefixConnection)
	workflowTester := tester.NewWorkflowTester[any](migrateConnectionGenerationWorkflowV1, tester.WithTestTimeout(10*time.Second))
	activity := func(context.Context, apid.ID, uint64) error { return nil }
	workflowTester.OnActivityByName(
		ActivityNameMigrateConnectionGenerationApplyV1, activity,
		testifymock.Anything, connectionID, uint64(2),
	).Return(nil).Once()

	// The tester uses wall-clock time while an activity runs and waits for its
	// cancellation to finish. Release it through the tester's ordered timer
	// queue after the workflow deadline, allowing ample time for apply on CI.
	releaseActivity := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseActivity) }) }
	defer release()
	workflowTester.ScheduleCallback(2*time.Second, release)
	workflowTester.OnActivityByName(
		ActivityNameMigrateConnectionGenerationReconcileToolSetsV1, activity,
		testifymock.Anything, connectionID, uint64(2),
	).Run(func(testifymock.Arguments) {
		<-releaseActivity
	}).Return(context.Canceled).Once()
	workflowTester.Execute(context.Background(), migrateConnectionGenerationWorkflowInputV1{
		ConnectionID: connectionID, TargetGeneration: 2, Timeout: time.Second,
	})

	// Enqueue work cannot silently extend a completed migration's total budget.
	require.True(t, workflowTester.WorkflowFinished())
	_, err := workflowTester.WorkflowResult()
	require.ErrorContains(t, err, "connection generation migration timed out")
	workflowTester.AssertExpectations(t)
}

// TestRegisterMigrateConnectionGenerationWorkflowDurableNames verifies both
// activities can be resolved by the names persisted in workflow history.
func TestRegisterMigrateConnectionGenerationWorkflowDurableNames(t *testing.T) {
	reg := registry.New()
	svc := &service{}
	require.NoError(t, svc.registerMigrateConnectionGenerationWorkflow(reg))
	_, err := reg.GetWorkflow(WorkflowNameMigrateConnectionGenerationV1)
	require.NoError(t, err)
	for _, name := range []string{
		ActivityNameMigrateConnectionGenerationApplyV1,
		ActivityNameMigrateConnectionGenerationReconcileToolSetsV1,
	} {
		_, err := reg.GetActivity(name)
		require.NoError(t, err)
	}
}

func TestMigrateConnectionGenerationWorkflowInstanceID(t *testing.T) {
	connectionID := apid.New(apid.PrefixConnection)

	require.Equal(
		t,
		WorkflowNameMigrateConnectionGenerationV1+":"+connectionID.String(),
		migrateConnectionGenerationWorkflowInstanceID(connectionID),
	)
}

func TestStartMigrateConnectionGenerationWorkflowRequiresClient(t *testing.T) {
	s := &service{}

	instance, err := s.startMigrateConnectionGenerationWorkflow(context.Background(), apid.New(apid.PrefixConnection), iface.ConnectionMigrationOptions{
		TargetGeneration: 2,
		Timeout:          time.Minute,
	})
	require.Nil(t, instance)
	require.ErrorContains(t, err, "workflow client is not configured")
}
