package oauth2

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/database"
	"net/url"
)

func (o *oAuth2Connection) SupportsRevokeTokens() bool {
	return o.auth != nil && o.auth.Revocation != nil && o.auth.Revocation.Endpoint != ""
}

func (o *oAuth2Connection) RevokeTokens(ctx context.Context) error {
	if !o.SupportsRevokeTokens() {
		return nil
	}

	token, err := o.db.GetOAuth2Token(ctx, o.connection.ID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil
		}
		return err
	}

	if o.auth.Revocation.SupportRevokingRefreshToken() {
		err := o.revokeRefreshToken(ctx, token)
		if err != nil {
			return err
		}
	}

	if o.auth.Revocation.SupportRevokingAccessToken() {
		err := o.revokeAccessToken(ctx, token)
		if err != nil {
			return err
		}
	}

	err = o.db.DeleteOAuth2Token(ctx, token.ID)
	return err
}

func (o *oAuth2Connection) revokeRefreshToken(ctx context.Context, token *database.OAuth2Token) error {
	if o.auth.Revocation == nil {
		// Connector does not support token revocation
		return nil
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
		URL(o.auth.Revocation.Endpoint).
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

func (o *oAuth2Connection) revokeAccessToken(ctx context.Context, token *database.OAuth2Token) error {
	if o.auth.Revocation == nil {
		// Connector does not support token revocation
		return nil
	}

	// Get the latest token to make sure we still need to refresh
	token, err := o.db.GetOAuth2Token(ctx, o.connection.ID)
	if err != nil {
		return err
	}

	if token.EncryptedAccessToken == "" {
		return fmt.Errorf("token does not have refresh token")
	}

	accessToken, err := o.encrypt.DecryptStringForConnection(ctx, o.connection, token.EncryptedAccessToken)
	if err != nil {
		return err
	}

	c := o.httpf.NewTopLevel().
		UseContext(ctx)

	req := c.Request().
		Method("POST").
		URL(o.auth.Revocation.Endpoint).
		Type("application/x-www-form-urlencoded").
		AddHeader("accept", "application/json").
		SetHeader("Authorization", "Bearer "+accessToken)

	for k, v := range o.auth.Token.QueryOverrides {
		req = req.SetQuery(k, v)
	}

	values := url.Values{
		"token":           {accessToken},
		"token_type_hint": {"access_token"},
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
