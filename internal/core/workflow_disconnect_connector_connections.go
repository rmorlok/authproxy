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
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
)

const (
	WorkflowNameDisconnectConnectorConnectionsV1 = "core.connector.disconnect_all.v1"

	ActivityNameDisconnectConnectorConnectionsListConnectionsV1 = "core.connector.disconnect_all.list_connections.v1"
	ActivityNameDisconnectConnectorConnectionsForceRemainingV1  = "core.connector.disconnect_all.force_remaining.v1"
)

type disconnectConnectorConnectionsWorkflowInputV1 struct {
	ConnectorID apid.ID       `json:"connector_id"`
	Timeout     time.Duration `json:"timeout"`
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

// disconnectConnectorConnectionsWorkflowInstanceID computes the id used for the disconnect connector
// connections workflow. This is specific to the connector id, so multiple invocations will result in
// same id. If the workflow is already running, this will return an error. If the workflow had finished
// previously and is then re-run, that will be allowed and be a new execution id.
func disconnectConnectorConnectionsWorkflowInstanceID(connectorID apid.ID) string {
	return fmt.Sprintf("%s:%s", WorkflowNameDisconnectConnectorConnectionsV1, connectorID)
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

	childCtx, cancelChildren := wflib.WithCancel(ctx)
	defer cancelChildren()

	pending := make(map[apid.ID]wflib.Future[any], len(connectionIDs))
	for _, connectionID := range connectionIDs {
		pending[connectionID] = wflib.CreateSubWorkflowInstance[any](
			childCtx,
			wflib.SubWorkflowOptions{
				InstanceID: disconnectConnectionWorkflowInstanceID(connectionID),
				Queue:      apworkflows.DefaultQueue,
			},
			WorkflowNameDisconnectConnectionV1,
			connectionID.String(),
		)
	}

	timerCtx, cancelTimer := wflib.WithCancel(ctx)
	timer := wflib.ScheduleTimer(
		timerCtx,
		input.Timeout,
		wflib.WithTimerName("disconnect-all-timeout"),
	)
	defer cancelTimer()

	connectionsToForce := make([]apid.ID, 0)
	timedOut := false

	// Loop until all workflows resolve, the timer expires, or a child workflow errors.
	for len(pending) > 0 && !timedOut {
		// Add a select case for the time for this iteration
		cases := make([]wflib.SelectCase, 0, len(pending)+1)
		cases = append(cases, wflib.Await(timer, func(_ wflib.Context, _ wflib.Future[any]) {
			timedOut = true
			cancelChildren()
		}))

		// Add a select case for each pending workflow
		for connectionID, future := range pending {
			id := connectionID
			f := future
			cases = append(cases, wflib.Await(f, func(ctx wflib.Context, future wflib.Future[any]) {
				if _, err := future.Get(ctx); err != nil {
					connectionsToForce = append(connectionsToForce, id)
				}
				delete(pending, id)
			}))
		}
		wflib.Select(ctx, cases...)
	}

	// Did everything finish without needing forced finalization?
	if len(pending) == 0 && len(connectionsToForce) == 0 {
		cancelTimer()
		return nil
	}

	// Not everything finished, force the remaining connections to the disconnected state
	for connectionID, future := range pending {
		_, _ = future.Get(ctx)
		connectionsToForce = append(connectionsToForce, connectionID)
	}
	
	if len(connectionsToForce) == 0 {
		return nil
	}

	_, err = wflib.ExecuteActivity[any](
		ctx,
		wflib.DefaultActivityOptions,
		ActivityNameDisconnectConnectorConnectionsForceRemainingV1,
		connectionsToForce,
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
