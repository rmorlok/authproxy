package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cschleiden/go-workflows/client"
	"github.com/cschleiden/go-workflows/registry"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
)

const (
	WorkflowNameMigrateConnectionGenerationV1 = "core.connection.migrate_generation.v1"

	ActivityNameMigrateConnectionGenerationApplyV1             = "core.connection.migrate_generation.apply.v1"
	ActivityNameMigrateConnectionGenerationReconcileToolSetsV1 = "core.connection.migrate_generation.reconcile_toolsets.v1"
)

type migrateConnectionGenerationWorkflowInputV1 struct {
	ConnectionID     apid.ID       `json:"connectionId"`
	TargetGeneration uint64        `json:"targetGeneration"`
	Timeout          time.Duration `json:"timeout"`
}

// migrateConnectionGenerationWorkflowV1 is the workflow definition to migration a
// given connection from its current connector generation to the target generation.
func migrateConnectionGenerationWorkflowV1(
	ctx wflib.Context,
	input migrateConnectionGenerationWorkflowInputV1,
) error {
	activityCtx, cancelActivities := wflib.WithCancel(ctx)
	defer cancelActivities()

	timerCtx, cancelTimer := wflib.WithCancel(ctx)
	timer := wflib.ScheduleTimer(
		timerCtx,
		input.Timeout,
		wflib.WithTimerName("migration-timeout"),
	)
	defer cancelTimer()

	applyFuture := wflib.ExecuteActivity[any](
		activityCtx,
		wflib.DefaultActivityOptions,
		ActivityNameMigrateConnectionGenerationApplyV1,
		input.ConnectionID,
		input.TargetGeneration,
	)

	var applyErr error
	timedOut := false
	wflib.Select(ctx,
		wflib.Await(timer, func(_ wflib.Context, _ wflib.Future[any]) {
			timedOut = true
			cancelActivities()
		}),
		wflib.Await(applyFuture, func(ctx wflib.Context, future wflib.Future[any]) {
			_, applyErr = future.Get(ctx)
		}),
	)
	if timedOut {
		return fmt.Errorf("connection generation migration timed out")
	}

	// Reconciliation has its own activity so a queue/database failure retries
	// only this follow-up, never the already-committed migration. Run it even
	// when apply reports an error: health/notification updates can fail after
	// the generation was stored. The activity checks whether that write landed.
	reconcileFuture := wflib.ExecuteActivity[any](
		activityCtx,
		wflib.DefaultActivityOptions,
		ActivityNameMigrateConnectionGenerationReconcileToolSetsV1,
		input.ConnectionID,
		input.TargetGeneration,
	)
	var reconcileErr error
	wflib.Select(ctx,
		wflib.Await(timer, func(_ wflib.Context, _ wflib.Future[any]) {
			timedOut = true
			cancelActivities()
		}),
		wflib.Await(reconcileFuture, func(ctx wflib.Context, future wflib.Future[any]) {
			_, reconcileErr = future.Get(ctx)
		}),
	)
	if timedOut {
		return fmt.Errorf("connection generation migration timed out")
	}

	cancelTimer()
	return errors.Join(applyErr, reconcileErr)
}

// registerMigrateConnectionGenerationWorkflow registers the durable workflow
// and its independently retried migration and ToolSet reconciliation activities.
func (s *service) registerMigrateConnectionGenerationWorkflow(
	worker workflowRegistrar,
) error {
	if err := worker.RegisterWorkflow(
		migrateConnectionGenerationWorkflowV1,
		registry.WithName(WorkflowNameMigrateConnectionGenerationV1),
	); err != nil {
		return err
	}
	if err := worker.RegisterActivity(
		s.applyMigrateConnectionGenerationV1,
		registry.WithName(ActivityNameMigrateConnectionGenerationApplyV1),
	); err != nil {
		return err
	}
	return worker.RegisterActivity(
		s.reconcileToolSetsAfterConnectionMigrationV1,
		registry.WithName(ActivityNameMigrateConnectionGenerationReconcileToolSetsV1),
	)
}

func migrateConnectionGenerationWorkflowInstanceID(
	connectionID apid.ID,
) string {
	return fmt.Sprintf("%s:%s", WorkflowNameMigrateConnectionGenerationV1, connectionID)
}

func (s *service) startMigrateConnectionGenerationWorkflow(
	ctx context.Context,
	connectionID apid.ID,
	opts iface.ConnectionMigrationOptions,
) (*wflib.Instance, error) {
	if s.wc == nil {
		return nil, fmt.Errorf("workflow client is not configured")
	}
	return s.wc.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: migrateConnectionGenerationWorkflowInstanceID(connectionID),
		Queue:      apworkflows.DefaultQueue,
	}, WorkflowNameMigrateConnectionGenerationV1, migrateConnectionGenerationWorkflowInputV1{
		ConnectionID:     connectionID,
		TargetGeneration: opts.TargetGeneration,
		Timeout:          opts.Timeout,
	})
}
