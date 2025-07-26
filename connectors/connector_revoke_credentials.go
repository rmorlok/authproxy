package connectors

import (
	"context"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/oauth2"
)

func (cv *ConnectorVersion) supportsRevokeCredentials() bool {
	def := cv.GetDefinition()
	auth := def.Auth

	if o2, ok := auth.(*config.AuthOAuth2); ok {
		return o2.Revocation != nil && o2.Revocation.Endpoint != ""
	}

	return false
}

func (cv *ConnectorVersion) revokeCredentials(ctx context.Context, c *service, conn database.Connection) error {
	def := cv.GetDefinition()
	auth := def.Auth

	if _, ok := auth.(*config.AuthOAuth2); ok {
		o2f := oauth2.NewFactory(c.cfg, c.db, c.redis, c, c.httpf, c.encrypt, c.logger)
		o2 := o2f.NewOAuth2(conn, cv)
		return o2.RevokeRefreshToken(ctx)
	}

	return errors.Errorf("connection %s does not support crecential revocation", conn.ID.String())
}
