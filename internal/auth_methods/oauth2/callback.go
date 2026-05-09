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
		// We have no connection context to record against — fall back to the static error page.
		return errorRedirectPage, errors.New("state is nil")
	}

	redirectUrl, err := o.exchangeCodeAndAdvance(ctx, query)
	if err == nil {
		return redirectUrl, nil
	}

	// Record the failure on the connection so it lands in the auth_failed terminal state. The
	// user is sent back to the original return URL with the setup-pending annotation; the
	// marketplace UI then sees auth_failed via the connection record and offers retry/cancel.
	if recordErr := o.connection.HandleAuthFailed(ctx, err); recordErr != nil {
		return errorRedirectPage, fmt.Errorf("failed to record auth failure (%v) after: %w", recordErr, err)
	}
	return o.appendSetupPendingToReturnUrl(o.state.ReturnToUrl), nil
}

// exchangeCodeAndAdvance performs the OAuth token exchange and post-auth state transition.
// Errors returned from here are recorded by the caller as auth failures on the connection so
// the user can retry via the standard failure flow. Each failure site emits a structured
// "oauth token exchange failed" event with a stable category before returning, so SOC
// dashboards and alerts can key off `category` rather than parsing error strings.
func (o *oAuth2Connection) exchangeCodeAndAdvance(ctx context.Context, query url.Values) (string, error) {
	// Delete the state from Redis now that the callback is consuming it.
	// This is the terminal step of the OAuth flow, so the state is no longer needed.
	if err := deleteStateFromRedis(ctx, o.r, o.state.Id); err != nil {
		err = fmt.Errorf("failed to clean up oauth state: %w", err)
		emitTokenExchangeFailure(ctx, o.logger, tokenExchangeStateCleanupError, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	// RFC 6749 §4.1.2.1 — when the resource owner denies the request (or the provider
	// can't satisfy it), the authorization endpoint redirects with `error=` instead of
	// `code=`. Surface that as a denial-shaped failure rather than a generic missing-code
	// error so the marketplace UI can render an authorization-denied result.
	if errCode := query.Get("error"); errCode != "" {
		msg := errCode
		if desc := query.Get("error_description"); desc != "" {
			msg = errCode + ": " + desc
		}
		err := fmt.Errorf("authorization denied by provider: %s", msg)
		attrs := o.tokenExchangeAttrsFromConn(err)
		attrs.ProviderError = errCode
		emitTokenExchangeFailure(ctx, o.logger, tokenExchangeProviderDenied, attrs)
		return "", err
	}

	code := query.Get("code")
	if code == "" {
		err := errors.New("no code in query")
		emitTokenExchangeFailure(ctx, o.logger, tokenExchangeMissingCode, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	callbackUrl, err := o.getPublicCallbackUrl()
	if err != nil {
		err = fmt.Errorf("failed to get public callback url: %w", err)
		emitTokenExchangeFailure(ctx, o.logger, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	c := o.httpf.
		ForRequestType(httpf.RequestTypeOAuth).
		ForConnection(o.connection).
		New().
		UseContext(ctx)

	clientId, err := o.auth.ClientId.GetValue(ctx)
	if err != nil {
		err = fmt.Errorf("failed to get client id for connector: %w", err)
		emitTokenExchangeFailure(ctx, o.logger, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	clientSecret, err := o.auth.ClientSecret.GetValue(ctx)
	if err != nil {
		err = fmt.Errorf("failed to get client secret for connector: %w", err)
		emitTokenExchangeFailure(ctx, o.logger, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	tokenEndpoint, err := o.renderMustache(ctx, o.auth.Token.Endpoint)
	if err != nil {
		err = fmt.Errorf("failed to render token endpoint template: %w", err)
		emitTokenExchangeFailure(ctx, o.logger, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return "", err
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
		err = fmt.Errorf("failed to post to exchange authorization code for access token: %w", err)
		emitTokenExchangeFailure(ctx, o.logger, tokenExchangeNetworkError, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	if resp.StatusCode != 200 {
		category, providerErr := classifyTokenEndpointStatus(resp.StatusCode, resp.Bytes())
		err := fmt.Errorf("received status code %d from exchange authorization code for access token", resp.StatusCode)
		attrs := o.tokenExchangeAttrsFromConn(err)
		attrs.ProviderStatusCode = resp.StatusCode
		attrs.ProviderError = providerErr
		emitTokenExchangeFailure(ctx, o.logger, category, attrs)
		return "", err
	}

	_, err = o.createDbTokenFromResponse(ctx, resp, nil)
	if err != nil {
		err = fmt.Errorf("failed to create db token from response: %w", err)
		emitTokenExchangeFailure(ctx, o.logger, tokenExchangeMalformedResponse, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	outcome, err := o.connection.HandleCredentialsEstablished(ctx)
	if err != nil {
		err = fmt.Errorf("failed to handle post-auth state transition: %w", err)
		emitTokenExchangeFailure(ctx, o.logger, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return "", err
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
