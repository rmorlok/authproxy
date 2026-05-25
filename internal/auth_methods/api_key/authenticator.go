package api_key

import (
	"context"

	"github.com/rmorlok/authproxy/internal/auth_methods"
)

// Resolve loads the active api-key credential and returns the
// AuthApplication (header or query, per the credential's placement
// snapshot) to apply to the outgoing request. The credential is read
// fresh from the database each call so a rotation (which inserts a new
// row and soft-deletes the prior) takes effect immediately on the next
// request — no in-memory cache to invalidate.
func (a *apiKeyConnection) Resolve(ctx context.Context) (auth_methods.AuthApplication, error) {
	app, err := a.resolveAuth(ctx)
	if err != nil {
		return auth_methods.AuthApplication{}, err
	}

	out := auth_methods.AuthApplication{}
	if app.HeaderName != "" {
		out.Headers = map[string]string{app.HeaderName: app.HeaderValue}
	}
	if app.QueryName != "" {
		out.QueryParams = map[string]string{app.QueryName: app.QueryValue}
	}
	return out, nil
}

// RecoverFrom401 returns ErrCannotRecover — there is no automated path
// to obtain a replacement api key. The orchestrator surfaces the
// upstream 401 unchanged.
func (a *apiKeyConnection) RecoverFrom401(ctx context.Context) error {
	return auth_methods.ErrCannotRecover
}

// SupportsRevoke returns false — an api key is a static secret with no
// "revoke" call against the 3rd party. Rotation happens by issuing a new
// key in the provider's console and re-running the connection setup.
func (a *apiKeyConnection) SupportsRevoke() bool {
	return false
}

// Revoke is a no-op for api-key connections. See SupportsRevoke.
func (a *apiKeyConnection) Revoke(ctx context.Context) error {
	return nil
}

var _ auth_methods.Authenticator = (*apiKeyConnection)(nil)
