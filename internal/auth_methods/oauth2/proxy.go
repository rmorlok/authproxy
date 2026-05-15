package oauth2

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	gentleman "gopkg.in/h2non/gentleman.v2"
)

// Refresh-token retry policy. Mirrors the token-exchange policy in
// callback.go: a small bounded number of attempts with linear backoff,
// retrying transport errors and 5xx responses only. 4xx responses
// (invalid_grant, invalid_client, etc.) are permanent — resubmitting the
// same refresh token won't change the provider's decision and may, with
// some providers, count toward a per-refresh-token attempt budget.
const (
	// tokenRefreshMaxAttempts is the total number of refresh-endpoint POST
	// attempts (including the first). 3 = 1 try + 2 retries.
	tokenRefreshMaxAttempts = 3
	// tokenRefreshBackoffStep is the linear backoff between attempts:
	// 200ms before retry 1, 400ms before retry 2. Same shape as
	// tokenExchangeBackoffStep — short enough not to stall a proxied
	// request perceptibly, long enough to ride out a node-local hiccup.
	tokenRefreshBackoffStep = 200 * time.Millisecond
)

// errNoRefreshToken is returned by refreshAccessToken when the persisted
// token has no refresh token and the access token is expired. Distinct
// sentinel so callers can recognize this case (it is permanent — the user
// must re-authenticate) without parsing error strings.
var errNoRefreshToken = errors.New("token does not have refresh token")

type refreshMode int

const (
	refreshModeOnlyExpired refreshMode = iota
	refreshModeAlways
)

func (o *oAuth2Connection) refreshAccessToken(ctx context.Context, token *database.OAuth2Token, mode refreshMode) (*database.OAuth2Token, error) {
	var result *database.OAuth2Token
	err := o.tel.withSpan(ctx, "refresh", o.connectorIDForTelemetry(), func(ctx context.Context) error {
		var err error
		result, err = o.refreshAccessTokenInner(ctx, token, mode)
		return err
	})
	return result, err
}

// connectorIDForTelemetry returns the connector id used as the
// authproxy.connector_id SPAN attribute on lifecycle spans. Safe to call
// when the connection / connector version isn't populated (returns the
// zero apid.ID, which connectorAttr treats as "absent"). Not used as a
// metric dimension — see connectorAttr.
func (o *oAuth2Connection) connectorIDForTelemetry() apid.ID {
	if o.connection == nil {
		return apid.Nil
	}
	return o.connection.GetConnectorId()
}

// connectionLabelsForTelemetry returns the connection-level labels used as
// the input to the metric-dimension projector. The projector applies the
// configured allowlist + value cap, so the raw set can include
// high-cardinality keys without leaking them onto metrics. Safe to call
// when the connection isn't populated.
func (o *oAuth2Connection) connectionLabelsForTelemetry() map[string]string {
	if o.connection == nil {
		return nil
	}
	return o.connection.GetLabels()
}

func (o *oAuth2Connection) refreshAccessTokenInner(ctx context.Context, token *database.OAuth2Token, mode refreshMode) (*database.OAuth2Token, error) {
	m := o.tokenMutex()
	err := m.Lock(ctx)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", 0, err)
	}
	defer m.Unlock(ctx)

	// Get the latest token to make sure we still need to refresh
	token, err = o.db.GetOAuth2Token(ctx, o.connection.GetId())
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", 0, err)
	}

	if mode == refreshModeOnlyExpired && !token.IsAccessTokenExpired(ctx) {
		return token, nil
	}

	if token.EncryptedRefreshToken.IsZero() {
		// Permanent — there is no way to obtain a new access token without
		// user interaction. Flip the connection unhealthy so the unified
		// reauth UX surfaces the prompt.
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshNoRefreshToken, 0, "", 0, errNoRefreshToken)
	}

	clientId, err := o.auth.ClientId.GetValue(ctx)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", 0, err)
	}

	clientSecret, err := o.auth.ClientSecret.GetValue(ctx)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", 0, err)
	}

	refreshToken, err := o.encrypt.DecryptString(ctx, token.EncryptedRefreshToken)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", 0, err)
	}

	client := o.httpf.
		ForRequestType(httpf.RequestTypeOAuth).
		ForConnection(o.connection).
		New()
	tokenEndpoint, err := o.renderMustache(ctx, o.auth.Token.Endpoint)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", 0,
			fmt.Errorf("failed to render token endpoint template: %w", err))
	}

	values := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientId},
		"client_secret": {clientSecret},
	}

	refreshResp, attempts, err := o.postRefreshWithRetry(ctx, client, tokenEndpoint, values)
	if err != nil {
		// Transport-layer failure — provider never produced a status code.
		// Transient by classification; does not flip unhealthy.
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshNetworkError, 0, "", attempts, err)
	}

	if refreshResp.StatusCode != 200 {
		category, providerErr := classifyTokenRefreshStatus(refreshResp.StatusCode, refreshResp.Bytes())
		err := fmt.Errorf("refresh token request failed with status %d", refreshResp.StatusCode)
		return nil, o.classifyAndRecordRefreshFailure(ctx, category, refreshResp.StatusCode, providerErr, attempts, err)
	}

	newToken, err := o.createDbTokenFromResponse(ctx, refreshResp, token)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshMalformedResponse, refreshResp.StatusCode, "", attempts,
			fmt.Errorf("failed to refresh token: %w", err))
	}

	// Success: clear any prior unhealthy state. MarkHealthState is
	// idempotent — a no-op if already healthy — so we don't gate this on
	// "was previously unhealthy".
	if err := o.connection.MarkHealthState(ctx, database.ConnectionHealthStateHealthy, "refresh_succeeded"); err != nil {
		// Don't fail the refresh on a bookkeeping error; the token is
		// already persisted. Log and move on.
		o.logger.WarnContext(ctx, "failed to mark connection healthy after successful refresh",
			"error", err,
		)
	}
	emitTokenRefreshSucceeded(ctx, o.logger, o.tokenRefreshAttrsFromConn(nil))
	o.tel.recordRefreshSuccess(ctx, o.connectionLabelsForTelemetry())

	return newToken, nil
}

// classifyAndRecordRefreshFailure emits the structured failure event and, if
// the category is permanent, flips the connection's health_state to
// unhealthy. Returns the underlying error so callers can return it directly
// from refreshAccessToken.
//
// Centralizing the emit + mark dance here ensures every refresh failure
// path produces the same observable shape, and that the "permanent →
// unhealthy" mapping lives in one place.
//
// attempts is the number of refresh-endpoint POSTs actually made (0 for
// failures that occur before any HTTP call — internal_error,
// no_refresh_token). When > 0 it's emitted on the failure event so the
// "retry exhausted" case is visibly distinct from a single non-retryable
// failure.
func (o *oAuth2Connection) classifyAndRecordRefreshFailure(
	ctx context.Context,
	category tokenRefreshCategory,
	providerStatusCode int,
	providerErr string,
	attempts int,
	err error,
) error {
	attrs := o.tokenRefreshAttrsFromConn(err)
	attrs.ProviderStatusCode = providerStatusCode
	attrs.ProviderError = providerErr
	attrs.Attempts = attempts
	emitTokenRefreshFailure(ctx, o.logger, category, attrs)
	o.tel.recordRefreshFailure(ctx, string(category), o.connectionLabelsForTelemetry())

	if category.IsPermanent() && o.connection != nil {
		if markErr := o.connection.MarkHealthState(ctx, database.ConnectionHealthStateUnhealthy, "refresh_"+string(category)); markErr != nil {
			o.logger.WarnContext(ctx, "failed to mark connection unhealthy after permanent refresh failure",
				"error", markErr,
				"category", string(category),
			)
		}
	}

	return err
}

func (o *oAuth2Connection) getValidToken(ctx context.Context) (*database.OAuth2Token, error) {
	token, err := o.db.GetOAuth2Token(ctx, o.connection.GetId())
	if err != nil {
		if errors.Is(database.ErrNotFound, err) {
			return nil, httperr.New(422, "no valid oauth token found", httperr.WithInternalErr(err))
		}

		return nil, err
	}

	if token == nil {
		return nil, httperr.InternalServerErrorMsg("token unexpectedly nil", httperr.WithInternalErr(err))
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

func (o *oAuth2Connection) ProxyRequest(ctx context.Context, reqType httpf.RequestType, req *iface.ProxyRequest) (*iface.ProxyResponse, error) {
	token, err := o.getValidToken(ctx)
	if err != nil {
		return nil, err
	}

	accessToken, err := o.encrypt.DecryptString(ctx, token.EncryptedAccessToken)
	if err != nil {
		return nil, err
	}

	resp, err := o.sendProxyRequest(ctx, reqType, req, accessToken)
	if err != nil {
		return nil, err
	}

	// Retry-once-after-refresh: a 401 from the upstream means the access
	// token is unauthorized at the provider even though our local
	// expiry-clock said it was still valid. Force a refresh and replay the
	// request exactly once with the new token.
	//
	// If the refresh itself fails (transient or permanent), we return the
	// original 401 unchanged — the customer's app sees the same auth
	// failure it would have without this retry path, and the refresh
	// failure was already classified and (if permanent) flipped the
	// connection unhealthy by refreshAccessToken.
	if resp.StatusCode == http.StatusUnauthorized {
		newToken, refreshErr := o.refreshAccessToken(ctx, token, refreshModeAlways)
		if refreshErr == nil {
			newAccessToken, decryptErr := o.encrypt.DecryptString(ctx, newToken.EncryptedAccessToken)
			if decryptErr == nil {
				retried, retryErr := o.sendProxyRequest(ctx, reqType, req, newAccessToken)
				if retryErr == nil {
					resp = retried
				}
			}
		}
	}

	return iface.ProxyResponseFromGentlemen(resp)
}

// sendProxyRequest builds a fresh gentleman request with the supplied
// access token applied as the bearer header and sends it. Split out because
// gentleman requests are single-use (Send panics on the second call), so
// the retry-once-after-refresh path needs to construct a new request rather
// than mutate the existing one.
func (o *oAuth2Connection) sendProxyRequest(
	ctx context.Context,
	reqType httpf.RequestType,
	req *iface.ProxyRequest,
	accessToken string,
) (*gentleman.Response, error) {
	r := o.httpf.
		ForRequestType(reqType).
		ForConnection(o.connection).
		ForActor(apauthcore.ActorFromContext(ctx)).
		ForLabels(req.Labels).
		New().
		UseContext(ctx).
		Request().
		SetHeader("Authorization", "Bearer "+accessToken)

	req.Apply(r)
	return r.Do()
}

// postRefreshWithRetry POSTs a refresh-token grant to the provider's token
// endpoint with a small bounded retry budget for transient failures
// (transport errors and 5xx responses). Returns the final response (or
// last network error) along with the number of attempts actually made;
// callers attach that to the failure event so "exhausted budget" is
// visibly distinct from a single non-retryable failure.
//
// 4xx responses are never retried — the provider has classified the
// refresh token itself as invalid/expired/revoked, and resubmitting it
// will not change the outcome. With some providers (notably ones that
// enforce refresh-token rotation), a retried 4xx counts against the
// token's lifetime, so retrying is observably worse than not.
//
// gentleman requests are single-use (Send panics on the second call), so
// each iteration rebuilds the request from scratch.
func (o *oAuth2Connection) postRefreshWithRetry(
	ctx context.Context,
	client *gentleman.Client,
	tokenEndpoint string,
	values url.Values,
) (*gentleman.Response, int, error) {
	var lastResp *gentleman.Response
	var lastErr error

	for attempt := 1; attempt <= tokenRefreshMaxAttempts; attempt++ {
		if attempt > 1 {
			backoff := tokenRefreshBackoffStep * time.Duration(attempt-1)
			select {
			case <-ctx.Done():
				return nil, attempt - 1, ctx.Err()
			case <-time.After(backoff):
			}
		}

		req := client.
			UseContext(ctx).
			Request().
			Method("POST").
			URL(tokenEndpoint).
			SetHeader("Content-Type", "application/x-www-form-urlencoded").
			AddHeader("accept", "application/json").
			BodyString(values.Encode())

		resp, err := req.Send()

		if err == nil && resp.StatusCode < 500 {
			return resp, attempt, nil
		}

		lastResp, lastErr = resp, err

		if attempt < tokenRefreshMaxAttempts {
			args := []any{
				slog.Int("attempt", attempt),
				slog.Int("max_attempts", tokenRefreshMaxAttempts),
			}
			if err != nil {
				args = append(args, slog.String("error", err.Error()))
			} else {
				args = append(args, slog.Int("provider_status_code", resp.StatusCode))
			}
			o.logger.WarnContext(ctx, "oauth token refresh transient failure; retrying", args...)
		}
	}

	return lastResp, tokenRefreshMaxAttempts, lastErr
}

var _ iface.Proxy = (*oAuth2Connection)(nil)
