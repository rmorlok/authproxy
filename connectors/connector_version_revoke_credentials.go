package connectors

import (
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/oauth2"
)

func (cv *ConnectorVersion) getRevokeCredentialsOperations(c *service, conn database.Connection) []operation {
	def := cv.GetDefinition()
	auth := def.Auth

	if _, ok := auth.(*config.AuthOAuth2); ok {
		o2f := oauth2.NewFactory(c.cfg, c.db, c.r, c, c.httpf, c.encrypt, c.logger)
		o2 := o2f.NewOAuth2(conn, cv)

		if o2.SupportsRevokeTokens() {
			return []operation{o2.RevokeTokens}
		}
	}

	return nil
}
