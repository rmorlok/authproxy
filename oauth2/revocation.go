package oauth2

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"net/url"
)

func (o *OAuth2) SupportsRevokeRefreshToken() bool {
	return o.auth.Revocation != nil && o.auth.Revocation.Endpoint != ""
}

func (o *OAuth2) RevokeRefreshToken(ctx context.Context) error {
	if o.auth.Revocation == nil {
		// Connector does not support token revocation
		return nil
	}

	// Get the latest token to make sure we still need to refresh
	token, err := o.db.GetOAuth2Token(ctx, o.connection.ID)
	if err != nil {
		return err
	}

	if token.EncryptedRefreshToken == "" {
		return fmt.Errorf("token does not have refresh token")
	}

	refreshToken, err := o.encrypt.DecryptStringForConnection(ctx, o.connection, token.EncryptedRefreshToken)
	if err != nil {
		return err
	}

	accessToken, err := o.encrypt.DecryptStringForConnection(ctx, o.connection, token.EncryptedAccessToken)
	if err != nil {
		return err
	}

	c := o.httpf.NewTopLevel().
		UseContext(ctx)

	req := c.Request().
		Method("POST").
		URL(o.auth.Token.Endpoint).
		Type("application/x-www-form-urlencoded").
		AddHeader("accept", "application/json").
		SetHeader("Authorization", "Bearer "+accessToken)

	for k, v := range o.auth.Token.QueryOverrides {
		req = req.SetQuery(k, v)
	}

	values := url.Values{
		"token":           {refreshToken},
		"token_type_hint": {"refresh_token"},
	}

	for k, v := range o.auth.Revocation.FormOverrides {
		values.Set(k, v)
	}

	resp, err := req.
		BodyString(values.Encode()).
		Send()

	if err != nil {
		return errors.Wrap(err, "failed to post to revoke refresh token")
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("received status code %d from revoke refresh token", resp.StatusCode)
	}

	return nil
}
