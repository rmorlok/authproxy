package no_auth

import (
	"log/slog"

	connIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

type noAuthConnection struct {
	logger *slog.Logger
	auth   *config.AuthNoAuth
	httpf  httpf.F

	c connIface.Connection
}

func NewNoAuth(
	logger *slog.Logger,
	httpf httpf.F,
	auth *config.AuthNoAuth,
	connection connIface.Connection,
) NoAuthConnection {
	return &noAuthConnection{
		logger: logger,
		httpf:  httpf,
		auth:   auth,
		c:      connection,
	}
}

var _ connIface.Proxy = (*noAuthConnection)(nil)
var _ NoAuthConnection = (*noAuthConnection)(nil)
