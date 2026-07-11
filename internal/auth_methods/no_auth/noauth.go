// Package no_auth provides the Authenticator implementation for connectors
// whose auth type is AuthNoAuth — i.e. proxy calls go straight through
// without any credential application. The orchestrator (internal/proxy)
// owns request building; this package only describes the no-op auth.
package no_auth

import (
	"context"

	"github.com/rmorlok/authproxy/internal/auth_methods"
)

type noAuthAuthenticator struct{}

// NewAuthenticator returns the Authenticator for an AuthNoAuth connector.
// Stateless — the connection argument is unused (kept in the signature for
// symmetry with the other auth methods' factories so callers don't need a
// special-case construction path).
func NewAuthenticator() auth_methods.Authenticator {
	return &noAuthAuthenticator{}
}

func (n *noAuthAuthenticator) Resolve(ctx context.Context) (auth_methods.AuthApplication, error) {
	return auth_methods.AuthApplication{}, nil
}

func (n *noAuthAuthenticator) RecoverFrom401(ctx context.Context) error {
	return auth_methods.ErrCannotRecover
}

// Refresh is a no-op for no-auth connections. There are no credentials to
// refresh, which is successful for migration and maintenance callers.
func (n *noAuthAuthenticator) Refresh(ctx context.Context) error {
	return nil
}

// SupportsRevoke returns false — there is no credential to revoke.
func (n *noAuthAuthenticator) SupportsRevoke() bool {
	return false
}

// Revoke is a no-op for no-auth connections.
func (n *noAuthAuthenticator) Revoke(ctx context.Context) error {
	return nil
}
