package core

import (
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

// Connection is a wrapper for the lower level database equivalent that handles wiring up logic specified in this
// connection's connector version.
type connection struct {
	database.Connection

	s      *service
	cv     *ConnectorVersion
	logger *slog.Logger

	proxyImplOnce sync.Once
	proxyImpl     iface.Proxy
	proxyImplErr  error
}

func wrapConnection(c *database.Connection, cv *ConnectorVersion, s *service) *connection {
	return &connection{
		Connection: *c,
		s:          s,
		cv:         cv,
		logger: aplog.NewBuilder(s.logger).
			WithNamespace(c.Namespace).
			WithConnectionId(c.Id).
			WithConnectorId(cv.Id).
			WithConnectorVersion(cv.Version).
			Build(),
	}
}

func (c *connection) GetId() uuid.UUID {
	return c.Id
}

func (c *connection) GetNamespace() string {
	return c.Namespace
}

func (c *connection) GetState() database.ConnectionState {
	return c.State
}

func (c *connection) GetConnectorId() uuid.UUID {
	return c.ConnectorId
}

func (c *connection) GetConnectorVersion() uint64 {
	return c.ConnectorVersion
}

func (c *connection) GetCreatedAt() time.Time {
	return c.CreatedAt
}

func (c *connection) GetUpdatedAt() time.Time {
	return c.UpdatedAt
}

func (c *connection) GetDeletedAt() *time.Time {
	return c.DeletedAt
}

func (c *connection) GetLabels() map[string]string {
	return c.Labels
}

func (c *connection) GetConnectorVersionEntity() iface.ConnectorVersion {
	return c.cv
}

func (c *connection) Logger() *slog.Logger {
	return c.logger
}

var _ iface.Connection = (*connection)(nil)
var _ aplog.HasLogger = (*connection)(nil)
