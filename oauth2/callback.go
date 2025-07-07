package oauth2

import (
	"context"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"net/url"
)

func (o *OAuth2) getPublicCallbackUrl() (string, error) {
	if o.cfg == nil {
		return "", errors.New("config is nil")
	}

	if o.cfg.GetRoot() == nil {
		return "", errors.New("config root is nil")
	}

	u, err := url.Parse(o.cfg.GetRoot().Public.GetBaseUrl())
	if err != nil {
		return "", errors.Wrap(err, "failed to parse base url for oauth2 return")
	}

	u.Path += "/oauth2/callback"
	return u.String(), nil
}

func (o *OAuth2) CallbackFrom3rdParty(ctx context.Context, query url.Values) (string, error) {
	errorRedirectPage := o.cfg.GetErrorPageUrl(config.ErrorPageInternalError)

	if o.state == nil {
		return errorRedirectPage, errors.New("state is nil")
	}

	code := query.Get("code")
	if code == "" {
		return errorRedirectPage, errors.New("no code in query")
	}

	callbackUrl, err := o.getPublicCallbackUrl()
	if err != nil {
		return errorRedirectPage, errors.Wrapf(err, "failed to get public callback url")
	}

	c := o.httpf.NewTopLevel().
		UseContext(ctx)

	clientId, err := o.auth.ClientId.GetValue(ctx)
	if err != nil {
		return errorRedirectPage, errors.Wrapf(err, "failed to get client id for connector")
	}

	clientSecret, err := o.auth.ClientSecret.GetValue(ctx)
	if err != nil {
		return errorRedirectPage, errors.Wrapf(err, "failed to get client id for connector")
	}

	req := c.Request().
		Method("POST").
		URL(o.auth.Token.Endpoint).
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
		return errorRedirectPage, errors.Wrapf(err, "failed to post to exchange authorization code for access token")
	}

	if resp.StatusCode != 200 {
		return errorRedirectPage, errors.Errorf("received status code %d from exchange authorization code for access token", resp.StatusCode)
	}

	_, err = o.createDbTokenFromResponse(ctx, resp, nil)
	if err != nil {
		return errorRedirectPage, errors.Wrapf(err, "failed to create db token from response")
	}

	return o.state.ReturnToUrl, nil
}
