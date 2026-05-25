package no_auth

import (
	"github.com/rmorlok/authproxy/internal/auth_methods"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
)

// Factory builds the Authenticator for a no-auth connection. Stateless; one
// factory per core service, shared across all no-auth connections. Mirrors
// the oauth2 and api_key factories so core can resolve auth methods through
// a uniform auth_methods.Factory registry.
type Factory interface {
	NewAuthenticator(connection coreIface.Connection) auth_methods.Authenticator
}

type factory struct{}

// NewFactory constructs a no-auth authenticator factory.
func NewFactory() Factory {
	return &factory{}
}

var _ auth_methods.Factory = (*factory)(nil)

// NewAuthenticator returns the per-connection Authenticator. The connection
// argument is unused — no-auth applies nothing — but kept in the signature
// for symmetry with the other auth methods' factories.
func (f *factory) NewAuthenticator(_ coreIface.Connection) auth_methods.Authenticator {
	return &noAuthAuthenticator{}
}
