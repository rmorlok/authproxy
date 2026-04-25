package oauth2

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

func (o *oAuth2Connection) getPublicCallbackUrl() (string, error) {
	if o.cfg == nil {
		return "", errors.New("config is nil")
	}

	if o.cfg.GetRoot() == nil {
		return "", errors.New("config root is nil")
	}

	u, err := url.Parse(o.cfg.GetRoot().Public.GetBaseUrl())
	if err != nil {
		return "", fmt.Errorf("failed to parse base url for oauth2 return: %w", err)
	}

	u.Path += "/oauth2/callback"
	return u.String(), nil
}

func (o *oAuth2Connection) CallbackFrom3rdParty(ctx context.Context, query url.Values) (string, error) {
	errorRedirectPage := o.cfg.GetErrorPageUrl(config.ErrorPageInternalError)

	if o.state == nil {
		return errorRedirectPage, errors.New("state is nil")
	}

	// Delete the state from Redis now that the callback is consuming it.
	// This is the terminal step of the OAuth flow, so the state is no longer needed.
	if err := deleteStateFromRedis(ctx, o.r, o.state.Id); err != nil {
		return errorRedirectPage, fmt.Errorf("failed to clean up oauth state: %w", err)
	}

	code := query.Get("code")
	if code == "" {
		return errorRedirectPage, errors.New("no code in query")
	}

	callbackUrl, err := o.getPublicCallbackUrl()
	if err != nil {
		return errorRedirectPage, fmt.Errorf("failed to get public callback url: %w", err)
	}

	c := o.httpf.
		ForRequestType(httpf.RequestTypeOAuth).
		ForConnection(o.connection).
		New().
		UseContext(ctx)

	clientId, err := o.auth.ClientId.GetValue(ctx)
	if err != nil {
		return errorRedirectPage, fmt.Errorf("failed to get client id for connector: %w", err)
	}

	clientSecret, err := o.auth.ClientSecret.GetValue(ctx)
	if err != nil {
		return errorRedirectPage, fmt.Errorf("failed to get client id for connector: %w", err)
	}

	tokenEndpoint, err := o.renderMustache(ctx, o.auth.Token.Endpoint)
	if err != nil {
		return errorRedirectPage, fmt.Errorf("failed to render token endpoint template: %w", err)
	}

	req := c.Request().
		Method("POST").
		URL(tokenEndpoint).
		Type("application/x-www-form-urlencoded").
		AddHeader("accept", "application/json")

	for k, v := range o.auth.Token.QueryOverrides {
		req = req.SetQuery(k, v)
	}

	values := url.Values{
		"client_id":     {clientId},
		"client_secret": {clientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {callbackUrl},
	}

	for k, v := range o.auth.Token.FormOverrides {
		values.Set(k, v)
	}

	resp, err := req.
		BodyString(values.Encode()).
		Send()

	if err != nil {
		return errorRedirectPage, fmt.Errorf("failed to post to exchange authorization code for access token: %w", err)
	}

	if resp.StatusCode != 200 {
		return errorRedirectPage, fmt.Errorf("received status code %d from exchange authorization code for access token", resp.StatusCode)
	}

	_, err = o.createDbTokenFromResponse(ctx, resp, nil)
	if err != nil {
		return errorRedirectPage, fmt.Errorf("failed to create db token from response: %w", err)
	}

	outcome, err := o.connection.HandleCredentialsEstablished(ctx)
	if err != nil {
		return errorRedirectPage, fmt.Errorf("failed to handle post-auth state transition: %w", err)
	}

	if outcome.SetupPending {
		return o.appendSetupPendingToReturnUrl(o.state.ReturnToUrl), nil
	}
	return o.state.ReturnToUrl, nil
}

// appendSetupPendingToReturnUrl augments the return URL with query params that signal the UI
// to poll for setup-step advancement. Falls back to the raw URL if parsing fails.
func (o *oAuth2Connection) appendSetupPendingToReturnUrl(raw string) string {
	returnUrl, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := returnUrl.Query()
	q.Set("setup", "pending")
	q.Set("connection_id", string(o.connection.GetId()))
	returnUrl.RawQuery = q.Encode()
	return returnUrl.String()
}
