package core

import (
	"context"
	"net/http"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/tasks"
)

const (
	WorkflowNameDisconnectConnectorConnectionsV1 = "core.connector.disconnect_all.v1"
	WorkflowNameArchiveConnectorV1               = "core.connector.archive.v1"
)

func (s *service) DisconnectConnectorConnections(
	_ context.Context,
	_ apid.ID,
	_ iface.ConnectorLifecycleOptions,
) (*tasks.TaskInfo, error) {
	return nil, httperr.New(http.StatusNotImplemented, "connector disconnect-all workflow is not implemented")
}

func (s *service) ArchiveConnector(
	_ context.Context,
	_ apid.ID,
	_ iface.ConnectorLifecycleOptions,
) (*tasks.TaskInfo, error) {
	return nil, httperr.New(http.StatusNotImplemented, "connector archive workflow is not implemented")
}
