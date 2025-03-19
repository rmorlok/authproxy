package oauth2

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/proxy"
	"gopkg.in/h2non/gentleman.v2"
	"net/http"
	"net/url"
)

func (o *OAuth2) proxyToplevel() *gentleman.Client {
	// TODO: add middlewares
	return gentleman.New()
}

func (o *OAuth2) refreshAccessToken(ctx context.Context, token *database.OAuth2Token) (*database.OAuth2Token, error) {
	// TODO: redis locking so only one process can refresh a token at once

	if token.EncryptedRefreshToken == "" {
		return nil, fmt.Errorf("token does not have refresh token")
	}

	clientId, err := o.auth.ClientId.GetValue(ctx)
	if err != nil {
		return nil, err
	}

	clientSecret, err := o.auth.ClientSecret.GetValue(ctx)
	if err != nil {
		return nil, err
	}

	refreshToken, err := o.encrypt.DecryptStringForConnection(ctx, o.connection, token.EncryptedRefreshToken)
	if err != nil {
		return nil, err
	}

	// Prepare a refresh token request
	client := o.proxyToplevel()
	refreshReq := client.
		UseContext(ctx).
		Request().
		Method("POST").
		URL(o.auth.TokenEndpoint).
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		AddHeader("accept", "application/json").
		BodyString(
			url.Values{
				"grant_type":    {"refresh_token"},
				"refresh_token": {refreshToken},
				"client_id":     {clientId},
				"client_secret": {clientSecret},
			}.Encode(),
		)

	// Submit the refresh request
	refreshResp, err := refreshReq.Send()
	if err != nil {
		return nil, err
	}

	newToken, err := o.createDbTokenFromResponse(ctx, refreshResp, &token.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to refresh token")
	}

	return newToken, nil
}

func (o *OAuth2) getValidToken(ctx context.Context) (*database.OAuth2Token, error) {
	token, err := o.db.GetOAuth2Token(ctx, o.connection.ID)
	if err != nil {
		return nil, err
	}

	if token == nil {
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			WithStatus(422).
			WithResponseMsg("no valid oauth token found").
			WithInternalErr(err).
			Build()
	}

	// Check if the token has expired
	if token.IsAccessTokenExpired(ctx) {
		token, err = o.refreshAccessToken(ctx, token)
		if err != nil {
			return nil, err
		}
	}

	return token, nil
}

func (o *OAuth2) ProxyRequest(ctx context.Context, req *proxy.ProxyRequest) (*proxy.ProxyResponse, error) {
	token, err := o.getValidToken(ctx)
	if err != nil {
		return nil, err
	}

	accessToken, err := o.encrypt.DecryptStringForConnection(ctx, o.connection, token.EncryptedAccessToken)
	if err != nil {
		return nil, err
	}

	r := o.proxyToplevel().
		UseContext(ctx).
		Request().
		SetHeader("Authorization", "Bearer "+accessToken)

	req.Apply(r)

	resp, err := r.Do()
	if err != nil {
		return nil, err
	}

	return proxy.ProxyResponseFromGentlemen(resp)
}

func (o *OAuth2) ProxyRequestRaw(ctx context.Context, req *proxy.ProxyRequest, w http.ResponseWriter) error {
	return nil
}

var _ proxy.Proxy = (*OAuth2)(nil)
