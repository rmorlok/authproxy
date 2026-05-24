package core

// getRevokeCredentialsOperations returns the operations that can be performed
// to revoke credentials for this connection. May return a nil slice if the
// connection's auth method does not support revocation. Dispatch is generic
// across auth types — the Authenticator interface carries SupportsRevoke /
// Revoke so this function makes no assumptions about which method is in use.
func (c *connection) getRevokeCredentialsOperations() []operation {
	def := c.cv.GetDefinition()
	if def == nil {
		return nil
	}
	factory := c.s.getAuthMethodFactory(def)
	if factory == nil {
		return nil
	}
	auth := factory.NewAuthenticator(c)
	if !auth.SupportsRevoke() {
		return nil
	}
	return []operation{auth.Revoke}
}
