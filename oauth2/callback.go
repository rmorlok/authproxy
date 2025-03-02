package oauth2

import (
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/util"
	"net/url"
	"strings"
	"time"
)

type authorizationCodeResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    *int   `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

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
	errorRedirectPage := o.cfg.GetRoot().ErrorPages.Fallback

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
	req := c.Request()

	clientId, err := o.auth.ClientId.GetValue(ctx)
	if err != nil {
		return errorRedirectPage, errors.Wrapf(err, "failed to get client id for connector")
	}

	clientSecret, err := o.auth.ClientSecret.GetValue(ctx)
	if err != nil {
		return errorRedirectPage, errors.Wrapf(err, "failed to get client id for connector")
	}

	resp, err := req.Method("POST").
		URL(o.auth.TokenEndpoint).
		Type("application/x-www-form-urlencoded").
		AddHeader("accept", "application/json").
		BodyString(url.Values{
			"client_id":     {clientId},
			"client_secret": {clientSecret},
			"grant_type":    {"authorization_code"},
			"code":          {code},
			"redirect_uri":  {callbackUrl},
		}.Encode()).
		Send()

	if err != nil {
		return errorRedirectPage, errors.Wrapf(err, "failed to post to exchange authorization code for access token")
	}

	if resp.StatusCode != 200 {
		print(resp.String())
		return errorRedirectPage, errors.Errorf("received status code %d from exchange authorization code for access token", resp.StatusCode)
	}

	jsonResp := authorizationCodeResponse{}
	err = resp.JSON(&jsonResp)
	if err != nil {
		return errorRedirectPage, errors.Wrapf(err, "failed to parse response from exchange authorization code for access token")
	}

	if jsonResp.AccessToken == "" {
		return errorRedirectPage, errors.New("no access token in response")
	}

	encryptedAccessToken, err := o.encrypt.EncryptStringForConnection(ctx, o.connection, jsonResp.AccessToken)
	if err != nil {
		return errorRedirectPage, errors.Wrapf(err, "failed to encrypt access token")
	}

	encryptedRefreshToken := ""

	// Not all OAuth has refresh tokens
	if jsonResp.RefreshToken != "" {
		encryptedRefreshToken, err = o.encrypt.EncryptStringForConnection(ctx, o.connection, jsonResp.RefreshToken)
		if err != nil {
			return errorRedirectPage, errors.Wrapf(err, "failed to encrypt refresh token")
		}
	}

	scopes := strings.Join(util.Map(o.auth.Scopes, func(s config.Scope) string { return s.Id }), " ")
	if jsonResp.Scope != "" {
		scopes = jsonResp.Scope
	}

	var expiresAt *time.Time
	if jsonResp.ExpiresIn != nil {
		expiresAt = util.ToPtr(ctx.Clock().Now().Add(time.Duration(*jsonResp.ExpiresIn) * time.Second))
	}

	_, err = o.db.InsertOAuth2Token(
		ctx,
		o.connection.ID,
		nil, // refreshedFrom
		encryptedRefreshToken,
		encryptedAccessToken,
		expiresAt,
		scopes,
	)
	if err != nil {
		return errorRedirectPage, errors.Wrapf(err, "failed to insert oauth2 token")
	}

	return o.state.ReturnToUrl, nil
}
