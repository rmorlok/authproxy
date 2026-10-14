package oauth2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/retry"
	"gopkg.in/h2non/gentleman.v2"
)

// tokenResponse is the OAuth response from the authorization token request and the refresh request
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    *int   `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`

	// Non-standard refresh-token lifetime fields; see refresh_token_expiry.go
	// for which providers and drafts use each.
	RefreshTokenExpiresIn  expiresInSeconds `json:"refresh_token_expires_in"`
	XRefreshTokenExpiresIn expiresInSeconds `json:"x_refresh_token_expires_in"`
	RefreshTokenTimeout    expiresInSeconds `json:"refresh_token_timeout"`
}

type tokenPersistOptions struct {
	PersistRefreshToken bool
}

// Token persistence retry policy. By the time a token response is being
// persisted the provider has already issued it — and, for providers that
// rotate refresh tokens or for the one-time authorization code, has already
// invalidated whatever we redeemed. Losing the response to a transient DB or
// encryption hiccup therefore strands the connection until the user
// re-authenticates, so persistence gets its own small retry budget and a
// deadline that is independent of the caller's context.
const (
	// tokenPersistMaxAttempts is the total number of encrypt+insert
	// attempts, including the first.
	tokenPersistMaxAttempts = 3
	// tokenPersistBackoffStep is the linear backoff between attempts.
	tokenPersistBackoffStep = 100 * time.Millisecond
	// tokenPersistTimeout bounds all persistence attempts together.
	tokenPersistTimeout = 10 * time.Second
)

// errMalformedTokenResponse marks token responses that could not be used at
// all (unparseable JSON, missing access_token). Callers classify these as the
// provider's fault (malformed_response); every other error from
// createDbTokenFromResponseWithOptions is proxy-side (internal_error).
var errMalformedTokenResponse = errors.New("malformed token response")

// createDbTokenFromResponse deserializes an oauth token from a refresh or authorization code response. It deserializes
// the response and then inserts a token into the database. It returns the newly created token.
func (o *oAuth2Connection) createDbTokenFromResponse(
	ctx context.Context,
	resp *gentleman.Response,
	refreshFrom *database.OAuth2Token,
) (*database.OAuth2Token, error) {
	return o.createDbTokenFromResponseWithOptions(
		ctx,
		resp,
		refreshFrom,
		tokenPersistOptions{
			PersistRefreshToken: true,
		},
	)
}

// createDbTokenFromResponseWithOptions parses a token-endpoint response and
// persists it as the connection's current token, soft-deleting the previous
// one. refreshFrom is the token being refreshed (nil for an initial
// exchange); its refresh token is carried forward when the provider does not
// return a new one. opts.PersistRefreshToken=false discards any refresh token
// (client_credentials grants).
//
// Errors wrapping errMalformedTokenResponse mean the response was unusable.
// Encryption and database failures are retried (see tokenPersistMaxAttempts)
// under a context detached from ctx's cancellation, because abandoning a
// token the provider has already issued is worse than finishing the write
// after the caller gave up; they are logged at error level if they still fail.
func (o *oAuth2Connection) createDbTokenFromResponseWithOptions(
	ctx context.Context,
	resp *gentleman.Response,
	refreshFrom *database.OAuth2Token,
	opts tokenPersistOptions,
) (*database.OAuth2Token, error) {
	jsonResp := tokenResponse{}
	err := resp.JSON(&jsonResp)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse token response: %w", errMalformedTokenResponse, err)
	}

	if jsonResp.AccessToken == "" {
		return nil, fmt.Errorf("%w: no access token in response", errMalformedTokenResponse)
	}

	effectiveScopes, err := o.effectiveScopes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve oauth2 scopes: %w", err)
	}
	requestedScopes := JoinScopes(effectiveScopes)

	persistRefreshToken := jsonResp.RefreshToken != "" && opts.PersistRefreshToken
	if jsonResp.RefreshToken != "" &&
		!opts.PersistRefreshToken &&
		o.logger != nil {
		o.logger.WarnContext(ctx, "oauth token response included refresh_token for client_credentials grant; discarding")
	}

	scopes := requestedScopes
	if jsonResp.Scope != "" {
		scopes = jsonResp.Scope
	}

	now := apctx.GetClock(ctx).Now()
	var expiresAt *time.Time
	if jsonResp.ExpiresIn != nil {
		expiresAt = util.ToPtr(now.Add(time.Duration(*jsonResp.ExpiresIn) * time.Second))
	}
	refreshTokenExpiry := o.refreshTokenExpiry(now, &jsonResp, opts, persistRefreshToken, refreshFrom)

	var refreshFromId *apid.ID
	if refreshFrom != nil {
		refreshFromId = &refreshFrom.Id
	}

	// On the authorization-code path the state has the initiating actor.
	// On refresh the state is gone, so carry forward the actor recorded on
	// the prior token row.
	var createdByActorId *apid.ID
	if refreshFrom != nil {
		createdByActorId = refreshFrom.CreatedByActorId
	} else if o.state != nil && o.state.ActorId != apid.Nil {
		actorId := o.state.ActorId
		createdByActorId = &actorId
	}

	persistCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		tokenPersistTimeout,
	)
	defer cancel()
	
	res, err := retry.Do(persistCtx, retry.Options[*database.OAuth2Token]{
		MaxAttempts: tokenPersistMaxAttempts,
		Backoff:     &retry.LinearBackOff{Step: tokenPersistBackoffStep},
		OnRetry: func(attempt int, _ *database.OAuth2Token, err error) {
			if o.logger != nil {
				o.logger.WarnContext(ctx, "failed to persist oauth token; retrying",
					"attempt", attempt,
					"max_attempts", tokenPersistMaxAttempts,
					"error", err,
				)
			}
		},
	}, func(ctx context.Context) (*database.OAuth2Token, error) {
		encryptedAccessToken, err := o.encrypt.EncryptStringForEntity(ctx, o.connection, jsonResp.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt access token: %w", err)
		}

		// Not all OAuth has refresh tokens. When the provider omits one on
		// refresh, keep using the one we redeemed (non-rotating providers).
		var encryptedRefreshToken encfield.EncryptedField
		if persistRefreshToken {
			encryptedRefreshToken, err = o.encrypt.EncryptStringForEntity(ctx, o.connection, jsonResp.RefreshToken)
			if err != nil {
				return nil, fmt.Errorf("failed to encrypt refresh token: %w", err)
			}
		} else if opts.PersistRefreshToken && refreshFrom != nil {
			encryptedRefreshToken = refreshFrom.EncryptedRefreshToken
		}

		token, err := o.db.InsertOAuth2Token(
			ctx,
			o.connection.GetId(),
			refreshFromId,
			encryptedRefreshToken,
			encryptedAccessToken,
			expiresAt,
			refreshTokenExpiry,
			scopes,
			requestedScopes,
			createdByActorId,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert oauth2 token: %w", err)
		}
		return token, nil
	})
	if err != nil {
		if o.logger != nil {
			// Loud: the provider issued this token (and may have invalidated
			// the one we redeemed), but we could not store it. The connection
			// will likely need to be re-authenticated.
			o.logger.ErrorContext(ctx, "failed to persist oauth token issued by provider; connection may require re-authentication",
				"attempts", res.Attempts,
				"included_refresh_token", jsonResp.RefreshToken != "",
				"error", err,
			)
		}
		return nil, err
	}
	token := res.Value

	mismatch, err := detectScopeMismatch(effectiveScopes, scopes)
	if err != nil {
		return nil, fmt.Errorf("failed to detect oauth2 scope mismatch: %w", err)
	}
	if err := o.applyScopeMismatch(ctx, mismatch); err != nil {
		return nil, err
	}

	return token, nil
}
