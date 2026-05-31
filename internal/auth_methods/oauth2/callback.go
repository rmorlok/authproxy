package oauth2

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/retry"
	gentleman "gopkg.in/h2non/gentleman.v2"
)

// Token-exchange retry policy. The token endpoint must be reachable for
// the OAuth flow to complete; rather than fail the whole flow on a
// momentary upstream blip, we make a small bounded number of attempts
// before giving up. Permanent failures (4xx) are not retried — retrying
// would re-submit the same authorization code, which a sane provider
// rejects as invalid_grant on the second try.
//
// The policy is intentionally hardcoded. If different connectors need
// different budgets, this becomes a connector-level config knob — but
// only when there's evidence that they do.
const (
	// tokenExchangeMaxAttempts is the total number of token-endpoint POST
	// attempts (including the first). 3 = 1 try + 2 retries — enough to
	// ride through a single bad pod or a quick rolling restart, not so
	// many that a hard outage stretches the user's wait beyond a couple
	// of seconds.
	tokenExchangeMaxAttempts = 3
	// tokenExchangeBackoffStep is the linear backoff between attempts:
	// 200ms before retry 1, 400ms before retry 2. Short enough that a
	// user staring at the marketplace post-consent does not notice;
	// long enough that a node-local failover has time to settle.
	tokenExchangeBackoffStep = 200 * time.Millisecond
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
	var returnURL string
	err := o.tel.withSpan(
		ctx,
		"token_exchange",
		o.connectorIDForTelemetry(),
		func(ctx context.Context) error {
			var err error
			returnURL, err = o.exchangeCodeAndAdvanceInner(ctx, query)
			return err
		})
	return returnURL, err
}

// emitAndRecordExchangeFailure centralizes the structured-event emit + OTel
// failure-counter bump for token-exchange failures. Mirrors the refresh-side
// classifyAndRecordRefreshFailure: every callsite uses the same shape so the
// "category" string lands consistently on logs, metrics, and span errors.
func (o *oAuth2Connection) emitAndRecordExchangeFailure(
	ctx context.Context,
	category tokenExchangeCategory,
	attrs tokenExchangeAttrs,
) {
	emitTokenExchangeFailure(ctx, o.logger, category, attrs)
	o.tel.recordTokenExchangeFailure(ctx, string(category), o.connectionLabelsForTelemetry())
}

func (o *oAuth2Connection) exchangeCodeAndAdvanceInner(ctx context.Context, query url.Values) (string, error) {
	// Delete the state from Redis now that the callback is consuming it.
	// This is the terminal step of the OAuth flow, so the state is no longer needed.
	if err := deleteStateFromRedis(ctx, o.r, o.state.Id); err != nil {
		err = fmt.Errorf("failed to clean up oauth state: %w", err)
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeStateCleanupError, o.tokenExchangeAttrsFromConn(err))
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
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeProviderDenied, attrs)
		return "", err
	}

	code := query.Get("code")
	if code == "" {
		err := errors.New("no code in query")
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeMissingCode, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	callbackUrl, err := o.getPublicCallbackUrl()
	if err != nil {
		err = fmt.Errorf("failed to get public callback url: %w", err)
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	c := o.httpf.
		ForRequestType(httpf.RequestTypeOAuth).
		ForConnection(o.connection).
		New().
		UseContext(ctx)

	clientId, clientSecret, err := o.resolveClientCredentials(ctx)
	if err != nil {
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	tokenEndpoint, err := o.renderMustache(ctx, o.auth.Token.Endpoint)
	if err != nil {
		err = fmt.Errorf("failed to render token endpoint template: %w", err)
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	values := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {callbackUrl},
	}

	// RFC 7636 §4.5 — the verifier is only sent on the initial code-for-token
	// exchange. Refresh-token grants must not carry it (§6), which is why
	// task_refresh_oauth_token.go does not have a mirror of this block.
	if o.state.PKCECodeVerifier != "" {
		values.Set("code_verifier", o.state.PKCECodeVerifier)
	}

	values, authHeader, err := applyTokenEndpointClientAuth(
		o.auth.GetTokenEndpointAuthMethodOrDefault(), clientId, clientSecret, values,
	)
	if err != nil {
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	for k, v := range o.auth.Token.FormOverrides {
		values.Set(k, v)
	}

	resp, attempts, err := o.postTokenExchangeWithRetry(ctx, c, tokenEndpoint, values, authHeader)
	if err != nil {
		err = fmt.Errorf("failed to post to exchange authorization code for access token: %w", err)
		attrs := o.tokenExchangeAttrsFromConn(err)
		attrs.Attempts = attempts
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeNetworkError, attrs)
		return "", err
	}

	if resp.StatusCode != 200 {
		category, providerErr := classifyTokenEndpointStatus(resp.StatusCode, resp.Bytes())
		err := fmt.Errorf("received status code %d from exchange authorization code for access token", resp.StatusCode)
		attrs := o.tokenExchangeAttrsFromConn(err)
		attrs.ProviderStatusCode = resp.StatusCode
		attrs.ProviderError = providerErr
		attrs.Attempts = attempts
		o.emitAndRecordExchangeFailure(ctx, category, attrs)
		return "", err
	}

	_, err = o.createDbTokenFromResponse(ctx, resp, nil)
	if err != nil {
		err = fmt.Errorf("failed to create db token from response: %w", err)
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeMalformedResponse, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	outcome, err := o.connection.HandleCredentialsEstablished(ctx)
	if err != nil {
		err = fmt.Errorf("failed to handle post-auth state transition: %w", err)
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return "", err
	}

	o.tel.recordTokenExchangeSuccess(ctx, o.connectionLabelsForTelemetry())

	if outcome.SetupPending {
		return o.appendSetupPendingToReturnUrl(o.state.ReturnToUrl), nil
	}
	return o.state.ReturnToUrl, nil
}

// ExchangeClientCredentials performs RFC 6749 §4.4's synchronous token
// endpoint exchange for service-to-service connectors. There is no authorize
// redirect or callback; the client credentials are the grant.
func (o *oAuth2Connection) ExchangeClientCredentials(ctx context.Context) error {
	return o.tel.withSpan(
		ctx,
		"token_exchange",
		o.connectorIDForTelemetry(),
		func(ctx context.Context) error {
			return o.exchangeClientCredentialsInner(ctx)
		})
}

func (o *oAuth2Connection) exchangeClientCredentialsInner(ctx context.Context) error {
	c := o.httpf.
		ForRequestType(httpf.RequestTypeOAuth).
		ForConnection(o.connection).
		New().
		UseContext(ctx)

	clientId, clientSecret, err := o.resolveClientCredentials(ctx)
	if err != nil {
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return err
	}

	tokenEndpoint, err := o.renderMustache(ctx, o.auth.Token.Endpoint)
	if err != nil {
		err = fmt.Errorf("failed to render token endpoint template: %w", err)
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return err
	}

	values := url.Values{
		"grant_type": {"client_credentials"},
	}
	if scopes := JoinScopes(o.auth.Scopes); scopes != "" {
		values.Set("scope", scopes)
	}

	values, authHeader, err := applyTokenEndpointClientAuth(
		o.auth.GetTokenEndpointAuthMethodOrDefault(), clientId, clientSecret, values,
	)
	if err != nil {
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeInternalError, o.tokenExchangeAttrsFromConn(err))
		return err
	}

	for k, v := range o.auth.Token.FormOverrides {
		values.Set(k, v)
	}

	resp, attempts, err := o.postTokenExchangeWithRetry(ctx, c, tokenEndpoint, values, authHeader)
	if err != nil {
		err = fmt.Errorf("failed to post client credentials token exchange: %w", err)
		attrs := o.tokenExchangeAttrsFromConn(err)
		attrs.Attempts = attempts
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeNetworkError, attrs)
		return err
	}

	if resp.StatusCode != 200 {
		category, providerErr := classifyTokenEndpointStatus(resp.StatusCode, resp.Bytes())
		err := fmt.Errorf("received status code %d from client credentials token exchange", resp.StatusCode)
		attrs := o.tokenExchangeAttrsFromConn(err)
		attrs.ProviderStatusCode = resp.StatusCode
		attrs.ProviderError = providerErr
		attrs.Attempts = attempts
		o.emitAndRecordExchangeFailure(ctx, category, attrs)
		return err
	}

	_, err = o.createDbTokenFromResponseWithOptions(ctx, resp, nil, tokenPersistOptions{PersistRefreshToken: false})
	if err != nil {
		err = fmt.Errorf("failed to create db token from response: %w", err)
		o.emitAndRecordExchangeFailure(ctx, tokenExchangeMalformedResponse, o.tokenExchangeAttrsFromConn(err))
		return err
	}

	o.tel.recordTokenExchangeSuccess(ctx, o.connectionLabelsForTelemetry())
	return nil
}

// postTokenExchangeWithRetry POSTs the token-exchange form to the provider's
// token endpoint, retrying transient failures (transport errors and 5xx
// responses) up to tokenExchangeMaxAttempts times. Returns the final
// response (or last network error), along with the number of attempts
// actually made — callers attach that to the failure event so the
// "exhausted" case is observably distinct from a single non-retryable
// 4xx.
//
// Permanent failures (4xx) are returned immediately without retry; the
// provider has classified the request as malformed or unauthorized,
// and resubmitting the same authorization code would only burn the
// code and produce an `invalid_grant` on the second call.
//
// Each retry rebuilds the gentleman.Request from scratch — gentleman
// requests are single-use (they panic with "Request was already
// dispatched" on a second Send).
func (o *oAuth2Connection) postTokenExchangeWithRetry(
	ctx context.Context,
	client *gentleman.Client,
	tokenEndpoint string,
	values url.Values,
	authHeader string,
) (*gentleman.Response, int, error) {
	res, err := retry.Do(ctx, retry.Options[*gentleman.Response]{
		MaxAttempts: tokenExchangeMaxAttempts,
		Backoff:     &retry.LinearBackOff{Step: tokenExchangeBackoffStep},
		Classify: func(resp *gentleman.Response, err error) bool {
			return err != nil || (resp != nil && resp.StatusCode >= 500)
		},
		OnRetry: func(attempt int, resp *gentleman.Response, err error) {
			args := []any{
				slog.Int("attempt", attempt),
				slog.Int("max_attempts", tokenExchangeMaxAttempts),
			}
			if err != nil {
				args = append(args, slog.String("error", err.Error()))
			} else {
				args = append(args, slog.Int("provider_status_code", resp.StatusCode))
			}
			o.logger.WarnContext(ctx, "oauth token exchange transient failure; retrying", args...)
		},
	}, func(ctx context.Context) (*gentleman.Response, error) {
		req := client.Request().
			Method("POST").
			URL(tokenEndpoint).
			Type("application/x-www-form-urlencoded").
			AddHeader("accept", "application/json")

		if authHeader != "" {
			req = req.AddHeader("Authorization", authHeader)
		}

		for k, v := range o.auth.Token.QueryOverrides {
			req = req.SetQuery(k, v)
		}

		return req.BodyString(values.Encode()).Send()
	})

	// See postRefreshWithRetry for the rationale on dropping the response
	// when err is non-nil — callers and tests treat any error as "no
	// response to inspect".
	if err != nil {
		return nil, res.Attempts, err
	}
	return res.Value, res.Attempts, nil
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
