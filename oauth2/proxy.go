package oauth2

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/proxy"
	"github.com/rmorlok/authproxy/request_log"
	"gopkg.in/h2non/gentleman.v2"
	"net/http"
	"net/url"
)

func (o *oAuth2Connection) proxyToplevel() *gentleman.Client {
	return o.httpf.
		ForRequestType(request_log.RequestTypeProxy).
		ForConnection(&o.connection).
		ForConnectorVersion(o.cv).
		New()
}

type refreshMode int

const (
	refreshModeOnlyExpired refreshMode = iota
	refreshModeAlways
)

func (o *oAuth2Connection) refreshAccessToken(ctx context.Context, token *database.OAuth2Token, mode refreshMode) (*database.OAuth2Token, error) {
	m := o.tokenMutex()
	err := m.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer m.Unlock(ctx)

	// Get the latest token to make sure we still need to refresh
	token, err = o.db.GetOAuth2Token(ctx, o.connection.ID)
	if err != nil {
		return nil, err
	}

	if mode == refreshModeOnlyExpired && !token.IsAccessTokenExpired(ctx) {
		return token, nil
	}

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
		URL(o.auth.Token.Endpoint).
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

	newToken, err := o.createDbTokenFromResponse(ctx, refreshResp, token)
	if err != nil {
		return nil, errors.Wrap(err, "failed to refresh token")
	}

	return newToken, nil
}

func (o *oAuth2Connection) getValidToken(ctx context.Context) (*database.OAuth2Token, error) {
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
		token, err = o.refreshAccessToken(ctx, token, refreshModeOnlyExpired)
		if err != nil {
			return nil, err
		}
	}

	return token, nil
}

func (o *oAuth2Connection) ProxyRequest(ctx context.Context, req *proxy.ProxyRequest) (*proxy.ProxyResponse, error) {
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

func (o *oAuth2Connection) ProxyRequestRaw(ctx context.Context, req *proxy.ProxyRequest, w http.ResponseWriter) error {
	return nil
}

var _ proxy.Proxy = (*oAuth2Connection)(nil)
