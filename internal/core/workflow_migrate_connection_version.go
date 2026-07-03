package core

import (
	"context"
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
	WorkflowNameMigrateConnectionVersionV1 = "core.connection.migrate_version.v1"

	ActivityNameMigrateConnectionVersionApplyV1 = "core.connection.migrate_version.apply.v1"

	connectionMigrationNotificationSource = "connector_migration"
)

type migrateConnectionVersionWorkflowInputV1 struct {
	ConnectionID  apid.ID       `json:"connection_id"`
	TargetVersion uint64        `json:"target_version"`
	Timeout       time.Duration `json:"timeout"`
}

func migrateConnectionVersionWorkflowV1(ctx wflib.Context, input migrateConnectionVersionWorkflowInputV1) error {
	activityCtx, cancelActivities := wflib.WithCancel(ctx)
	defer cancelActivities()

	timerCtx, cancelTimer := wflib.WithCancel(ctx)
	timer := wflib.ScheduleTimer(timerCtx, input.Timeout, wflib.WithTimerName("migration-timeout"))
	defer cancelTimer()

	applyFuture := wflib.ExecuteActivity[any](
		activityCtx,
		wflib.DefaultActivityOptions,
		ActivityNameMigrateConnectionVersionApplyV1,
		input.ConnectionID,
		input.TargetVersion,
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
		return fmt.Errorf("connection version migration timed out")
	}
	if applyErr != nil {
		return applyErr
	}

	cancelTimer()
	return nil
}

func (s *service) registerMigrateConnectionVersionWorkflow(worker workflowRegistrar) error {
	if err := worker.RegisterWorkflow(
		migrateConnectionVersionWorkflowV1,
		registry.WithName(WorkflowNameMigrateConnectionVersionV1),
	); err != nil {
		return err
	}
	return worker.RegisterActivity(
		s.applyMigrateConnectionVersionV1,
		registry.WithName(ActivityNameMigrateConnectionVersionApplyV1),
	)
}

func migrateConnectionVersionWorkflowInstanceID(connectionID apid.ID) string {
	return fmt.Sprintf("%s:%s", WorkflowNameMigrateConnectionVersionV1, connectionID)
}

func (s *service) startMigrateConnectionVersionWorkflow(
	ctx context.Context,
	connectionID apid.ID,
	opts iface.ConnectionMigrationOptions,
) (*wflib.Instance, error) {
	if s.wc == nil {
		return nil, fmt.Errorf("workflow client is not configured")
	}
	return s.wc.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: migrateConnectionVersionWorkflowInstanceID(connectionID),
		Queue:      apworkflows.DefaultQueue,
	}, WorkflowNameMigrateConnectionVersionV1, migrateConnectionVersionWorkflowInputV1{
		ConnectionID:  connectionID,
		TargetVersion: opts.TargetVersion,
		Timeout:       opts.Timeout,
	})
}
