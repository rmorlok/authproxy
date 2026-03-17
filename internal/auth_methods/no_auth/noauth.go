package no_auth

import (
	"log/slog"

	connIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
)

// connectionWithRateLimiting wraps a database.Connection to also implement
// httpf.RateLimitConfigProvider so the httpf factory can access rate limiting config.
type connectionWithRateLimiting struct {
	database.Connection
	rateLimiting *connectors.RateLimiting
}

func (c *connectionWithRateLimiting) GetRateLimitConfig() *connectors.RateLimiting {
	return c.rateLimiting
}

var _ httpf.RateLimitConfigProvider = (*connectionWithRateLimiting)(nil)

type noAuthConnection struct {
	logger *slog.Logger
	auth   *config.AuthNoAuth
	httpf  httpf.F

	c  connectionWithRateLimiting
	cv connIface.ConnectorVersion
}

func NewNoAuth(
	logger *slog.Logger,
	httpf httpf.F,
	auth *config.AuthNoAuth,
	connection database.Connection,
	cv connIface.ConnectorVersion,
	rateLimiting *connectors.RateLimiting,
) NoAuthConnection {
	return &noAuthConnection{
		logger: logger,
		httpf:  httpf,
		auth:   auth,
		c: connectionWithRateLimiting{
			Connection:   connection,
			rateLimiting: rateLimiting,
		},
		cv: cv,
	}
}

var _ connIface.Proxy = (*noAuthConnection)(nil)
var _ NoAuthConnection = (*noAuthConnection)(nil)
