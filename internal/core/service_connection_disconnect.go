package core

import (
	"context"
	"errors"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/tasks"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
)

func (s *service) DisconnectConnection(
	ctx context.Context,
	id apid.ID,
	opts iface.ConnectionDisconnectOptions,
) (taskInfo *tasks.TaskInfo, err error) {
	s.logger.Info("disconnecting connection", "id", id)
	err = s.db.SetConnectionState(ctx, id, database.ConnectionStateDisconnecting)
	if err != nil {
		if errors.Is(database.ErrNotFound, err) {
			// Default the error type to a 404 error
			return nil, httperr.NotFound("", httperr.WithInternalErr(err))
		}

		return nil, err
	}

	s.logger.Info("starting disconnect connection workflow", "id", id)
	instance, err := s.startDisconnectConnectionWorkflow(ctx, id, opts)
	if err != nil {
		return nil, err
	}

	s.logger.Info(
		"disconnect connection workflow started",
		"id", id,
		"workflow_instance_id", instance.InstanceID,
		"workflow_execution_id", instance.ExecutionID,
	)
	return tasks.FromWorkflowInstance(instance, WorkflowNameDisconnectConnectionV1, string(apworkflows.DefaultQueue)), nil
}
