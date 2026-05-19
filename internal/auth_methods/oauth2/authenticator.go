package oauth2

import (
	"context"

	"github.com/rmorlok/authproxy/internal/auth_methods"
)

// Resolve loads the active access token (refreshing if expired) and
// returns an AuthApplication that sets the Bearer Authorization header
// on the outgoing request.
//
// Mirrors the credential-loading half of the previous in-package
// ProxyRequest: token + decrypt. The retry-once-after-refresh wrapper
// lives in internal/proxy and calls RecoverFrom401 followed by another
// Resolve when the upstream rejects the request.
func (o *oAuth2Connection) Resolve(ctx context.Context) (auth_methods.AuthApplication, error) {
	token, err := o.getValidToken(ctx)
	if err != nil {
		return auth_methods.AuthApplication{}, err
	}

	accessToken, err := o.encrypt.DecryptString(ctx, token.EncryptedAccessToken)
	if err != nil {
		return auth_methods.AuthApplication{}, err
	}

	return auth_methods.AuthApplication{
		Headers: map[string]string{
			"Authorization": "Bearer " + accessToken,
		},
	}, nil
}

// RecoverFrom401 forces a refresh of the access token. The new token is
// persisted to the database by refreshAccessToken; the orchestrator's
// next Resolve call will read it back. Mirrors the retry-once-after-401
// branch of the previous in-package ProxyRequest.
//
// If the refresh fails (transient or permanent) the error is returned;
// the orchestrator falls back to surfacing the original 401 unchanged,
// so the customer's app sees the same auth failure it would have without
// this retry path. The refresh failure was already classified and (if
// permanent) flipped the connection unhealthy by refreshAccessToken.
func (o *oAuth2Connection) RecoverFrom401(ctx context.Context) error {
	token, err := o.db.GetOAuth2Token(ctx, o.connection.GetId())
	if err != nil {
		return err
	}
	_, err = o.refreshAccessToken(ctx, token, refreshModeAlways)
	return err
}

var _ auth_methods.Authenticator = (*oAuth2Connection)(nil)
