package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cschleiden/go-workflows/client"
	"github.com/cschleiden/go-workflows/registry"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/tasks"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
)

const (
	WorkflowNameDisconnectConnectorConnectionsV1 = "core.connector.disconnect_all.v1"
	WorkflowNameArchiveConnectorV1               = "core.connector.archive.v1"

	ActivityNameDisconnectConnectorConnectionsListConnectionsV1 = "core.connector.disconnect_all.list_connections.v1"
	ActivityNameDisconnectConnectorConnectionsForceRemainingV1  = "core.connector.disconnect_all.force_remaining.v1"
)

type disconnectConnectorConnectionsWorkflowInputV1 struct {
	ConnectorID apid.ID       `json:"connector_id"`
	Timeout     time.Duration `json:"timeout"`
}

func (s *service) DisconnectConnectorConnections(
	ctx context.Context,
	id apid.ID,
	opts iface.ConnectorLifecycleOptions,
) (*tasks.TaskInfo, error) {
	if id == apid.Nil {
		return nil, httperr.BadRequest("connector id is required")
	}
	if s.wc == nil {
		return nil, fmt.Errorf("workflow client is not configured")
	}

	instance, err := s.startDisconnectConnectorConnectionsWorkflow(ctx, id, opts)
	if err != nil {
		return nil, err
	}

	return tasks.FromWorkflowInstance(instance, WorkflowNameDisconnectConnectorConnectionsV1, string(apworkflows.DefaultQueue)), nil
}

func (s *service) ArchiveConnector(
	_ context.Context,
	_ apid.ID,
	_ iface.ConnectorLifecycleOptions,
) (*tasks.TaskInfo, error) {
	return nil, httperr.New(http.StatusNotImplemented, "connector archive workflow is not implemented")
}

func (s *service) startDisconnectConnectorConnectionsWorkflow(
	ctx context.Context,
	connectorID apid.ID,
	opts iface.ConnectorLifecycleOptions,
) (*wflib.Instance, error) {
	return s.wc.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: disconnectConnectorConnectionsWorkflowInstanceID(connectorID),
		Queue:      apworkflows.DefaultQueue,
	}, WorkflowNameDisconnectConnectorConnectionsV1, disconnectConnectorConnectionsWorkflowInputV1{
		ConnectorID: connectorID,
		Timeout:     opts.Timeout,
	})
}

func disconnectConnectorConnectionsWorkflowInstanceID(connectorID apid.ID) string {
	return fmt.Sprintf("%s:%s", WorkflowNameDisconnectConnectorConnectionsV1, connectorID)
}

func disconnectConnectorConnectionChildWorkflowInstanceID(parentInstanceID string, connectionID apid.ID) string {
	return fmt.Sprintf("%s:%s", parentInstanceID, connectionID)
}

func disconnectConnectorConnectionsWorkflowV1(ctx wflib.Context, input disconnectConnectorConnectionsWorkflowInputV1) error {
	connectionIDs, err := wflib.ExecuteActivity[[]apid.ID](
		ctx,
		wflib.DefaultActivityOptions,
		ActivityNameDisconnectConnectorConnectionsListConnectionsV1,
		input.ConnectorID,
	).Get(ctx)
	if err != nil {
		return err
	}
	if len(connectionIDs) == 0 {
		return nil
	}

	parentInstanceID := wflib.WorkflowInstance(ctx).InstanceID
	childCtx, cancelChildren := wflib.WithCancel(ctx)
	defer cancelChildren()
	pending := make(map[apid.ID]wflib.Future[any], len(connectionIDs))
	for _, connectionID := range connectionIDs {
		pending[connectionID] = wflib.CreateSubWorkflowInstance[any](
			childCtx,
			wflib.SubWorkflowOptions{
				InstanceID: disconnectConnectorConnectionChildWorkflowInstanceID(parentInstanceID, connectionID),
				Queue:      apworkflows.DefaultQueue,
			},
			WorkflowNameDisconnectConnectionV1,
			connectionID.String(),
		)
	}

	timerCtx, cancelTimer := wflib.WithCancel(ctx)
	timer := wflib.ScheduleTimer(timerCtx, input.Timeout, wflib.WithTimerName("disconnect-all-timeout"))
	defer cancelTimer()
	timedOut := false
	for len(pending) > 0 && !timedOut {
		cases := make([]wflib.SelectCase, 0, len(pending)+1)
		cases = append(cases, wflib.Await(timer, func(_ wflib.Context, _ wflib.Future[any]) {
			timedOut = true
			cancelChildren()
		}))
		for connectionID, future := range pending {
			id := connectionID
			f := future
			cases = append(cases, wflib.Await(f, func(ctx wflib.Context, future wflib.Future[any]) {
				if _, err := future.Get(ctx); err != nil {
					timedOut = true
					return
				}
				delete(pending, id)
			}))
		}
		wflib.Select(ctx, cases...)
	}

	if len(pending) == 0 {
		cancelTimer()
		return nil
	}

	remaining := make([]apid.ID, 0, len(pending))
	for connectionID, future := range pending {
		_, _ = future.Get(ctx)
		remaining = append(remaining, connectionID)
	}
	_, err = wflib.ExecuteActivity[any](
		ctx,
		wflib.DefaultActivityOptions,
		ActivityNameDisconnectConnectorConnectionsForceRemainingV1,
		remaining,
	).Get(ctx)
	return err
}

func (s *service) listDisconnectConnectorConnectionsV1(ctx context.Context, connectorID apid.ID) ([]apid.ID, error) {
	if connectorID == apid.Nil {
		return nil, fmt.Errorf("connector id not specified")
	}
	if err := connectorID.ValidatePrefix(apid.PrefixConnectorVersion); err != nil {
		return nil, err
	}

	var connectionIDs []apid.ID
	err := s.db.ListConnectionsBuilder().
		WithDeletedHandling(database.DeletedHandlingExclude).
		ForConnectorId(connectorID).
		ForStates(disconnectConnectorConnectionsRelevantStates()).
		Enumerate(ctx, func(page pagination.PageResult[database.Connection]) (pagination.KeepGoing, error) {
			for _, conn := range page.Results {
				if conn.State != database.ConnectionStateDisconnecting {
					if err := s.db.SetConnectionState(ctx, conn.Id, database.ConnectionStateDisconnecting); err != nil {
						return pagination.Stop, err
					}
				}
				connectionIDs = append(connectionIDs, conn.Id)
			}
			return pagination.Continue, nil
		})
	if err != nil {
		return nil, err
	}
	return connectionIDs, nil
}

func (s *service) forceRemainingDisconnectConnectorConnectionsV1(ctx context.Context, connectionIDs []apid.ID) error {
	for _, id := range connectionIDs {
		if id == apid.Nil {
			return fmt.Errorf("connection id not specified")
		}
		if err := id.ValidatePrefix(apid.PrefixConnection); err != nil {
			return err
		}

		if err := s.db.SetConnectionState(ctx, id, database.ConnectionStateDisconnected); err != nil && !errors.Is(err, database.ErrNotFound) {
			return err
		}
		if err := s.db.DeleteConnection(ctx, id); err != nil && !errors.Is(err, database.ErrNotFound) {
			return err
		}
	}
	return nil
}

func disconnectConnectorConnectionsRelevantStates() []database.ConnectionState {
	return []database.ConnectionState{
		database.ConnectionStateSetup,
		database.ConnectionStateConfigured,
		database.ConnectionStateDisabled,
		database.ConnectionStateDisconnecting,
	}
}

func (s *service) registerDisconnectConnectorConnectionsWorkflow(worker workflowRegistrar) error {
	if err := worker.RegisterWorkflow(
		disconnectConnectorConnectionsWorkflowV1,
		registry.WithName(WorkflowNameDisconnectConnectorConnectionsV1),
	); err != nil {
		return err
	}
	if err := worker.RegisterActivity(
		s.listDisconnectConnectorConnectionsV1,
		registry.WithName(ActivityNameDisconnectConnectorConnectionsListConnectionsV1),
	); err != nil {
		return err
	}
	return worker.RegisterActivity(
		s.forceRemainingDisconnectConnectorConnectionsV1,
		registry.WithName(ActivityNameDisconnectConnectorConnectionsForceRemainingV1),
	)
}
