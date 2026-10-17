package oauth2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/util/retry"
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

// refreshMode decides, once the token mutex is held and the current token
// has been re-read, whether a refresh is still needed.
type refreshMode int

const (
	// refreshModeOnlyExpired refreshes only if the current access token
	// has expired (lazy refresh on use).
	refreshModeOnlyExpired refreshMode = iota
	// refreshModeAlways refreshes unconditionally (background refresh,
	// explicit Refresh calls).
	refreshModeAlways
	// refreshModeUnlessReplaced refreshes only if the current token row is
	// still the one passed to refreshAccessToken — i.e. nobody else has
	// refreshed since the caller observed that token being rejected.
	refreshModeUnlessReplaced
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
// when the connection / connector generation isn't populated (returns the
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

// refreshAccessTokenInner runs the refresh while holding the per-connection
// token mutex. The mutex lease is renewed for the full duration of the
// refresh (apredis.RunWithMutex), so a slow token endpoint or a refresh that
// spends its retry budget cannot let the lease lapse and admit a second
// refresher. That matters most for providers that rotate refresh tokens: a
// second refresher would read the same, already-consumed refresh token and
// get invalid_grant, flipping a perfectly good connection unhealthy.
//
// If the lease is lost anyway (Redis unavailable), the context passed to the
// refresh is cancelled so the in-flight work stops rather than outliving the
// lock. Failures inside the locked section classify themselves; failures to
// acquire the lock are classified here as internal errors (transient).
func (o *oAuth2Connection) refreshAccessTokenInner(
	ctx context.Context,
	token *database.OAuth2Token,
	mode refreshMode,
) (*database.OAuth2Token, error) {
	var (
		result  *database.OAuth2Token
		lockErr error
		ran     bool
	)
	lockErr = apredis.RunWithMutex(
		ctx,
		o.tokenMutex(),
		o.auth.Token.GetRefreshTimeout(),
		func(lockedCtx context.Context) error {
			ran = true
			var err error
			result, err = o.refreshAccessTokenLocked(lockedCtx, token, mode)
			return err
		},
	)

	switch {
	case !ran:
		// Never acquired the lock; nothing was sent to the provider.
		return nil, o.classifyAndRecordRefreshFailure(
			ctx,
			tokenRefreshInternalError,
			0,  // providerStatusCode
			"", // providerErr
			0,  // attempts
			lockErr,
		)
	case result != nil:
		// The refresh succeeded and the new token is persisted. A lease or
		// unlock error after that point does not invalidate the result.
		if lockErr != nil {
			o.logger.WarnContext(
				ctx,
				"oauth token refresh succeeded but releasing the token mutex failed",
				"error", lockErr,
			)
		}
		return result, nil
	default:
		return nil, lockErr
	}
}

// refreshAccessTokenLocked performs the refresh assuming the caller holds the
// token mutex. It re-reads the current token (another holder may already
// have refreshed it), posts the grant, and persists the response. Every
// failure path is classified and recorded via classifyAndRecordRefreshFailure.
//
// observed is the token the caller based its decision to refresh on; only
// its Id is consulted, and only in refreshModeUnlessReplaced.
func (o *oAuth2Connection) refreshAccessTokenLocked(
	ctx context.Context,
	observed *database.OAuth2Token,
	mode refreshMode,
) (*database.OAuth2Token, error) {
	// Get the latest token to make sure we still need to refresh
	token, err := o.db.GetOAuth2Token(ctx, o.connection.GetId())
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(
			ctx,
			tokenRefreshInternalError,
			0,  // providerStatus
			"", // providerErr
			0,  // attempts
			err,
		)
	}

	if mode == refreshModeOnlyExpired && !token.IsAccessTokenExpired(ctx) {
		return token, nil
	}

	if mode == refreshModeUnlessReplaced &&
		observed != nil &&
		token.Id != observed.Id {
		// A concurrent refresher already replaced the rejected token. With
		// rotating refresh tokens, refreshing again would burn another
		// rotation for no benefit.
		o.logger.DebugContext(
			ctx,
			"skipping oauth token refresh; rejected token was already replaced",
			"rejected_token_id", observed.Id,
			"current_token_id", token.Id,
		)
		return token, nil
	}

	if token.EncryptedRefreshToken.IsZero() && o.auth.SupportsRefreshToken() {
		// Permanent — there is no way to obtain a new access token without
		// user interaction. Flip the connection unhealthy so the unified
		// reauth UX surfaces the prompt.
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshNoRefreshToken, 0, "", 0, errNoRefreshToken)
	}

	clientId, clientSecret, err := o.resolveClientCredentials(ctx)
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

	values := url.Values{}
	persistOptions := tokenPersistOptions{PersistRefreshToken: true}
	if o.auth.SupportsRefreshToken() {
		refreshToken, err := o.encrypt.DecryptString(ctx, token.EncryptedRefreshToken)
		if err != nil {
			return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", 0, err)
		}
		values.Set("grant_type", "refresh_token")
		values.Set("refresh_token", refreshToken)
	} else {
		values.Set("grant_type", "client_credentials")
		effectiveScopes, err := o.effectiveScopes(ctx)
		if err != nil {
			return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", 0,
				fmt.Errorf("failed to resolve oauth2 scopes: %w", err))
		}
		if scopeString := JoinScopes(effectiveScopes); scopeString != "" {
			values.Set("scope", scopeString)
		}
		persistOptions.PersistRefreshToken = false
	}

	values, authHeader, err := applyTokenEndpointClientAuth(
		o.auth.GetTokenEndpointAuthMethodOrDefault(), clientId, clientSecret, values,
	)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", 0, err)
	}

	refreshResp, attempts, sawAmbiguous, err := o.postRefreshWithRetry(
		ctx,
		client,
		tokenEndpoint,
		values,
		authHeader,
		o.auth.Token.GetRefreshTimeout(),
	)
	if err != nil {
		// Transport-layer failure — provider never produced a status code.
		// Transient by classification; does not flip unhealthy.
		if sawAmbiguous {
			o.recordAmbiguousRefresh(ctx, token.Id)
		}

		return nil, o.classifyAndRecordRefreshFailure(
			ctx,
			tokenRefreshNetworkError,
			0,  // providerStatusCode
			"", // providerErr
			attempts,
			err)
	}

	if refreshResp.StatusCode != 200 {
		category, providerErr := classifyTokenRefreshStatus(refreshResp.StatusCode, refreshResp.Bytes())
		if category == tokenRefreshInvalidGrant &&
			(sawAmbiguous || o.hadAmbiguousRefresh(ctx, token.Id)) {
			// Most likely our own earlier attempt consumed this refresh token
			// and its response was lost, rather than the user revoking access.
			category = tokenRefreshInvalidGrantAfterAmbiguousAttempt
		} else if sawAmbiguous {
			o.recordAmbiguousRefresh(ctx, token.Id)
		}

		err := fmt.Errorf("refresh token request failed with status %d", refreshResp.StatusCode)
		return nil, o.classifyAndRecordRefreshFailure(ctx, category, refreshResp.StatusCode, providerErr, attempts, err)
	}

	// A definitive success supersedes any earlier ambiguous attempt.
	defer o.clearAmbiguousRefresh(ctx)

	newToken, err := o.createDbTokenFromResponseWithOptions(ctx, refreshResp, token, persistOptions)
	if err != nil {
		// Only an unusable response is the provider's fault (permanent);
		// a failure to store a good response is ours and transient.
		category := tokenRefreshInternalError
		if errors.Is(err, errMalformedTokenResponse) {
			category = tokenRefreshMalformedResponse
		}
		return nil, o.classifyAndRecordRefreshFailure(ctx, category, refreshResp.StatusCode, "", attempts,
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

// bufferResponseBody reads the whole response body into memory while the
// caller's per-attempt deadline is still live, closes the network body, and
// swaps in an in-memory body that later Bytes()/JSON() calls read from. A body
// that fails to arrive in time is reported as a transport error.
//
// It deliberately does not use gentleman's Response.Bytes() for this:
// Bytes() closes the body and a later JSON() closes it again, and response
// bodies wrapped by the request-metrics layer are not safe to close twice.
// The in-memory replacement tolerates any number of Close calls.
func bufferResponseBody(resp *gentleman.Response) error {
	if resp.RawResponse == nil || resp.RawResponse.Body == nil {
		return nil
	}
	body, readErr := io.ReadAll(resp.RawResponse.Body)
	closeErr := resp.RawResponse.Body.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	resp.RawResponse.Body = io.NopCloser(bytes.NewReader(body))
	return nil
}

// postRefreshWithRetry POSTs a refresh-token grant to the provider's token
// endpoint with a small bounded retry budget for transient failures
// (transport errors and 5xx responses). Returns the final response (or
// last network error) along with the number of attempts actually made;
// callers attach that to the failure event so "exhausted budget" is
// visibly distinct from a single non-retryable failure. The bool result
// reports whether any attempt was ambiguous — it may have been processed by
// the provider without us seeing the answer — so the caller can recognize
// an invalid_grant caused by our own retry of a consumed refresh token.
//
// 4xx responses are never retried — the provider has classified the
// refresh token itself as invalid/expired/revoked, and resubmitting it
// will not change the outcome. With some providers (notably ones that
// enforce refresh-token rotation), a retried 4xx counts against the
// token's lifetime, so retrying is observably worse than not.
//
// gentleman requests are single-use (Send panics on the second call), so
// each iteration rebuilds the request from scratch.
//
// attemptTimeout bounds each individual POST, including reading the response
// body (the connector's token.refreshTimeout). Zero or negative disables the
// per-attempt deadline and leaves only ctx in charge. The body is buffered
// inside the attempt so the deadline can be released before the caller
// parses it.
func (o *oAuth2Connection) postRefreshWithRetry(
	ctx context.Context,
	client *gentleman.Client,
	tokenEndpoint string,
	values url.Values,
	authHeader string,
	attemptTimeout time.Duration,
) (*gentleman.Response, int, bool, error) {
	// sawAmbiguous is set if any attempt may have reached the provider
	// without a definitive answer (see isAmbiguousRefreshOutcome). Attempts
	// run sequentially, so no synchronization is needed.
	sawAmbiguous := false
	res, err := retry.Do(ctx, retry.Options[*gentleman.Response]{
		MaxAttempts: tokenRefreshMaxAttempts,
		Backoff:     &retry.LinearBackOff{Step: tokenRefreshBackoffStep},
		Classify: func(resp *gentleman.Response, err error) bool {
			return err != nil || (resp != nil && resp.StatusCode >= 500)
		},
		OnRetry: func(attempt int, resp *gentleman.Response, err error) {
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
		},
	}, func(ctx context.Context) (*gentleman.Response, error) {
		if attemptTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, attemptTimeout)
			defer cancel()
		}

		req := client.
			UseContext(ctx).
			Request().
			Method("POST").
			URL(tokenEndpoint).
			SetHeader("Content-Type", "application/x-www-form-urlencoded").
			AddHeader("accept", "application/json").
			BodyString(values.Encode())

		if authHeader != "" {
			req = req.AddHeader("Authorization", authHeader)
		}

		resp, err := req.Send()
		if err != nil {
			sawAmbiguous = sawAmbiguous || isAmbiguousTransportError(err)
			return resp, err
		}
		if err := bufferResponseBody(resp); err != nil {
			// The status line arrived but the body did not: the provider
			// may well have processed the request.
			sawAmbiguous = true
			return nil, err
		}
		sawAmbiguous = sawAmbiguous || isAmbiguousRefreshOutcome(resp.StatusCode, nil)
		return resp, nil
	})

	// Mirror the original contract: a returned error always implies a nil
	// response. Callers (and the unit tests) key on err first; a 5xx
	// response left dangling on a ctx-cancelled return is just noise.
	if err != nil {
		return nil, res.Attempts, sawAmbiguous, err
	}
	return res.Value, res.Attempts, sawAmbiguous, nil
}
