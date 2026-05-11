package oauth2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	gentleman "gopkg.in/h2non/gentleman.v2"
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
	m := o.tokenMutex()
	err := m.Lock(ctx)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", err)
	}
	defer m.Unlock(ctx)

	// Get the latest token to make sure we still need to refresh
	token, err = o.db.GetOAuth2Token(ctx, o.connection.GetId())
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", err)
	}

	if mode == refreshModeOnlyExpired && !token.IsAccessTokenExpired(ctx) {
		return token, nil
	}

	if token.EncryptedRefreshToken.IsZero() {
		// Permanent — there is no way to obtain a new access token without
		// user interaction. Flip the connection unhealthy so the unified
		// reauth UX surfaces the prompt.
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshNoRefreshToken, 0, "", errNoRefreshToken)
	}

	clientId, err := o.auth.ClientId.GetValue(ctx)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", err)
	}

	clientSecret, err := o.auth.ClientSecret.GetValue(ctx)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", err)
	}

	refreshToken, err := o.encrypt.DecryptString(ctx, token.EncryptedRefreshToken)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "", err)
	}

	// Prepare a refresh token request

	client := o.httpf.
		ForRequestType(httpf.RequestTypeOAuth).
		ForConnection(o.connection).
		New()
	tokenEndpoint, err := o.renderMustache(ctx, o.auth.Token.Endpoint)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshInternalError, 0, "",
			fmt.Errorf("failed to render token endpoint template: %w", err))
	}

	refreshReq := client.
		UseContext(ctx).
		Request().
		Method("POST").
		URL(tokenEndpoint).
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
		// Transport-layer failure — provider never produced a status code.
		// Transient by classification; does not flip unhealthy.
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshNetworkError, 0, "", err)
	}

	if refreshResp.StatusCode != 200 {
		category, providerErr := classifyTokenRefreshStatus(refreshResp.StatusCode, refreshResp.Bytes())
		err := fmt.Errorf("refresh token request failed with status %d", refreshResp.StatusCode)
		return nil, o.classifyAndRecordRefreshFailure(ctx, category, refreshResp.StatusCode, providerErr, err)
	}

	newToken, err := o.createDbTokenFromResponse(ctx, refreshResp, token)
	if err != nil {
		return nil, o.classifyAndRecordRefreshFailure(ctx, tokenRefreshMalformedResponse, refreshResp.StatusCode, "",
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
func (o *oAuth2Connection) classifyAndRecordRefreshFailure(
	ctx context.Context,
	category tokenRefreshCategory,
	providerStatusCode int,
	providerErr string,
	err error,
) error {
	attrs := o.tokenRefreshAttrsFromConn(err)
	attrs.ProviderStatusCode = providerStatusCode
	attrs.ProviderError = providerErr
	emitTokenRefreshFailure(ctx, o.logger, category, attrs)

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

func (o *oAuth2Connection) ProxyRequestRaw(ctx context.Context, reqType httpf.RequestType, req *iface.ProxyRequest, w http.ResponseWriter) error {
	return nil
}

var _ iface.Proxy = (*oAuth2Connection)(nil)
