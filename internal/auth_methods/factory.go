package auth_methods

import (
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
)

// Factory is the uniform construction surface every auth method exposes.
// Core resolves the factory by auth type once at service construction and
// then asks it for whatever it needs (Authenticator today; ManifestSetupSteps
// is added by #366 once the runtime manifest types land in core/iface).
//
// Per-method extras (e.g. oauth2.Factory.NewOAuth2 / GetOAuth2State) live on
// the concrete factory type, not this interface — only the cross-cutting
// surface that core dispatches against generically belongs here.
type Factory interface {
	// NewAuthenticator returns the per-connection Authenticator that
	// internal/proxy applies to outbound requests.
	NewAuthenticator(connection coreIface.Connection) Authenticator
}
