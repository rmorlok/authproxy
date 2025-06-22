package oauth2

import (
	"context"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/jwt"
	"github.com/rmorlok/authproxy/util"
	"net/url"
	"strings"
)

func (o *OAuth2) getPublicRedirectUrl(ctx context.Context, stateId uuid.UUID, actor database.Actor) (string, error) {
	if o.cfg == nil {
		return "", errors.New("config is nil")
	}

	if o.cfg.GetRoot() == nil {
		return "", errors.New("config root is nil")
	}

	tb, err := jwt.NewJwtTokenBuilder().
		WithActor(util.ToPtr(actor.ToJwtActor())).
		WithExpiresInCtx(ctx, o.cfg.GetRoot().Oauth.GetInitiateToRedirectTtlOrDefault()).
		WithServiceId(config.ServiceIdPublic).
		WithSelfSigned().
		WithSecretConfigKeyData(ctx, o.cfg.GetRoot().SystemAuth.GlobalAESKey)

	if err != nil {
		return "", errors.Wrap(err, "failed to create token builder to sign redirect jwt")
	}

	tokenString, err := tb.TokenCtx(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to create generate temporary auth token")
	}

	u, err := url.Parse(o.cfg.GetRoot().Public.GetBaseUrl())
	if err != nil {
		return "", errors.Wrap(err, "failed to parse base url for oauth2 return")
	}

	query := u.Query()
	query.Set("state_id", stateId.String())
	auth.SetJwtQueryParm(query, tokenString)

	u.Path += "/oauth2/redirect"
	u.RawQuery = query.Encode()

	return u.String(), nil
}

func (o *OAuth2) GenerateAuthUrl(ctx context.Context, actor database.Actor) (string, error) {
	if !o.auth.ClientId.HasValue(ctx) {
		return "", errors.Errorf("client id does not have value for connector %s", o.cv.ID)
	}

	clientId, err := o.auth.ClientId.GetValue(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get client id for connector %s", o.cv.ID)
	}

	if o.auth.Authorization.Endpoint == "" {
		return "", errors.Errorf("no authorization endpoint for connector %s", o.cv.ID)
	}

	if o.state == nil {
		return "", errors.Errorf("must have existing state stored to redis")
	}

	callbackUrl, err := o.getPublicCallbackUrl()
	if err != nil {
		return "", errors.Wrapf(err, "failed to get public callback url")
	}

	scopes := util.Map(o.auth.Scopes, func(s config.Scope) string {
		return s.Id
	})

	authUrl3p, err := url.Parse(o.auth.Authorization.Endpoint)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse authorization endpoint for connector %s", o.cv.ID)
	}

	query := authUrl3p.Query()

	query.Set("redirect_uri", callbackUrl)
	query.Set("access_type", "offline")
	query.Set("response_type", "code")
	query.Set("client_id", clientId)
	query.Set("scope", strings.Join(scopes, " "))
	query.Set("state", o.state.Id.String())

	for k, v := range o.auth.Authorization.QueryOverrides {
		query.Set(k, v)
	}

	authUrl3p.RawQuery = query.Encode()

	return authUrl3p.String(), nil
}

// SetStateAndGeneratePublicUrl starts the OAuth process. It creates a state record for the connection authorization
// flow, which begins the TTL for when that connection must be completed. Returns a redirect URL to our Public redirect.
// This redirect will read from state, validate everything, then cookie the user and redirect to the 3rd party.
func (o *OAuth2) SetStateAndGeneratePublicUrl(
	ctx context.Context,
	actor database.Actor,
	returnToUrl string,
) (string, error) {
	stateId := uuid.New()

	if err := o.saveStateToRedis(ctx, actor, stateId, returnToUrl); err != nil {
		return "", err
	}

	redirectUrl, err := o.getPublicRedirectUrl(ctx, stateId, actor)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get public redirect url")
	}

	return redirectUrl, nil
}
