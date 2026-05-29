package auth_methods

import (
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// Factory is the uniform construction surface every auth method exposes.
// Core resolves the factory by auth type once at service construction and
// then asks it for whatever it needs.
//
// Per-method extras (e.g. oauth2.Factory.NewOAuth2 / GetOAuth2State) live on
// the concrete factory type, not this interface — only the cross-cutting
// surface that core dispatches against generically belongs here.
type Factory interface {
	// NewAuthenticator returns the per-connection Authenticator that
	// internal/proxy applies to outbound requests.
	NewAuthenticator(connection coreIface.Connection) Authenticator

	// ManifestSetupSteps returns the ordered auth-phase steps this method
	// contributes to a connection's setup flow. May be empty (no_auth
	// returns nil). Called by core's flow builder when materializing the
	// ManifestSetupFlow.
	//
	// Implementations build steps with NewFormStep / NewRedirectStep from
	// internal/core/iface; the closures capture the factory's per-service
	// deps (db, encrypt, etc.) so the step's OnSubmit / RenderRedirect can
	// act without re-resolving them at the call site.
	//
	// The auth method receives the connector's auth configuration so it
	// can shape its steps to the specific connector (e.g. api-key reads
	// the Placement; OAuth2 may inspect grant type once #352 lands). The
	// connector is guaranteed by the caller to have an Auth whose Inner
	// matches the method registering this factory.
	ManifestSetupSteps(connection coreIface.Connection, connector *cschema.Connector) []coreIface.ManifestSetupStep
}
