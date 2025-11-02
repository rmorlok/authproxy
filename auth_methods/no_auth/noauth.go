package no_auth

import (
	"log/slog"

	"github.com/rmorlok/authproxy/config"
	connIface "github.com/rmorlok/authproxy/connectors/iface"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/httpf"
)

type noAuthConnection struct {
	logger *slog.Logger
	auth   *config.AuthNoAuth
	httpf  httpf.F

	c  database.Connection
	cv connIface.ConnectorVersion
}

func NewNoAuth(
	logger *slog.Logger,
	httpf httpf.F,
	auth *config.AuthNoAuth,
	connection database.Connection,
	cv connIface.ConnectorVersion,
) NoAuthConnection {
	return &noAuthConnection{
		logger: logger,
		httpf:  httpf,
		auth:   auth,
		c:      connection,
		cv:     cv,
	}
}

var _ connIface.Proxy = (*noAuthConnection)(nil)
var _ NoAuthConnection = (*noAuthConnection)(nil)
