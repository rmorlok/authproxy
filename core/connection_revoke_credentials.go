package core

import (
	"github.com/rmorlok/authproxy/config"
)

// getRevokeCredentialsOperations returns the operations that can be performed to revoke credentials for this
// this may return a nil slice if no operations are supported. Support will depend on the auth type for the
// connection and how that auth type is configured.
func (c *connection) getRevokeCredentialsOperations() []operation {
	def := c.cv.GetDefinition()
	auth := def.Auth

	if _, ok := auth.(*config.AuthOAuth2); ok {
		o2f := c.s.getOAuth2Factory()
		o2 := o2f.NewOAuth2(c.Connection, c.cv)

		if o2.SupportsRevokeTokens() {
			return []operation{o2.RevokeTokens}
		}
	}

	return nil
}
