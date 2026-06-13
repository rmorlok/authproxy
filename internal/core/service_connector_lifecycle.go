package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/tasks"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
)

// DisconnectConnectorConnections disconnects all connections for a connector but leaves
// the state of the connector intact.
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

// ArchiveConnector archives a connector. First it disconnects all connections for the
// connector and then it archives the connector.
func (s *service) ArchiveConnector(
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

	instance, err := s.startArchiveConnectorWorkflow(ctx, id, opts)
	if err != nil {
		return nil, err
	}

	return tasks.FromWorkflowInstance(instance, WorkflowNameArchiveConnectorV1, string(apworkflows.DefaultQueue)), nil
}
