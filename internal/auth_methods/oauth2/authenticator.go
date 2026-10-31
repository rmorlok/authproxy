package oauth2

import (
	"context"
	"errors"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/database"
)

// Resolve loads the active access token (refreshing if expired) and
// returns an AuthApplication that sets the Bearer Authorization header
// on the outgoing request.
//
// Mirrors the credential-loading half of the previous in-package
// ProxyRequest: token + decrypt. The retry-once-after-refresh wrapper
// lives in internal/proxy and calls RecoverFrom401 followed by another
// Resolve when the upstream rejects the request.
func (o *oAuth2Connection) Resolve(
	ctx context.Context,
) (auth_methods.AuthApplication, error) {
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
		CredentialVersion: token.Id.String(),
	}, nil
}

// Refresh forces a refresh of the access token. The new token is persisted to
// the database by refreshAccessToken; the next Resolve call will read it back.
func (o *oAuth2Connection) Refresh(ctx context.Context) error {
	token, err := o.db.GetOAuth2Token(ctx, o.connection.GetId())
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return auth_methods.MarkCredentialsUnavailable(err)
		}
		return err
	}
	_, err = o.refreshAccessToken(ctx, token, refreshModeAlways)
	return err
}

// RecoverFrom401 refreshes the access token after the upstream rejected a
// request with 401. Mirrors the retry-once-after-401 branch of the previous
// in-package ProxyRequest.
//
// rejected.CredentialVersion is the id of the token row the rejected request
// was built from. When several requests fail on the same token at once, the
// first to take the token mutex refreshes; the rest find that the current
// token row is no longer the rejected one and return without contacting the
// provider, so the caller simply retries with the replacement token. Without
// this each 401 forced its own refresh, rotating refresh tokens N times for
// one expiry. An empty CredentialVersion falls back to an unconditional
// refresh.
//
// If the refresh fails (transient or permanent) the error is returned;
// the orchestrator falls back to surfacing the original 401 unchanged,
// so the customer's app sees the same auth failure it would have without
// this retry path. The refresh failure was already classified and (if
// permanent) flipped the connection unhealthy by refreshAccessToken.
func (o *oAuth2Connection) RecoverFrom401(
	ctx context.Context,
	rejected auth_methods.AuthApplication,
) error {
	if rejected.CredentialVersion == "" {
		return o.Refresh(ctx)
	}

	rejectedToken := &database.OAuth2Token{
		Id: apid.ID(rejected.CredentialVersion),
	}

	_, err := o.refreshAccessToken(
		ctx,
		rejectedToken,
		refreshModeUnlessReplaced,
	)

	return err
}

var _ auth_methods.Authenticator = (*oAuth2Connection)(nil)
