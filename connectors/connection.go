package connectors

import (
	"log/slog"
	"sync"

	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/connectors/iface"
	"github.com/rmorlok/authproxy/database"
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

func newConnection(c *database.Connection, s *service, cv *ConnectorVersion) *connection {
	return &connection{
		Connection: *c,
		s:          s,
		cv:         cv,
		logger: aplog.NewBuilder(s.logger).
			WithConnectionId(c.ID).
			WithConnectorId(cv.ID).
			Build(),
	}
}
