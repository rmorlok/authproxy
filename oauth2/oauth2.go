package oauth2

import (
	"encoding"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/jwt"
	"github.com/rmorlok/authproxy/redis"
	"github.com/rmorlok/authproxy/util"
	"net/url"
	"strings"
	"time"
)

type OAuth2 struct {
	cfg        config.C
	db         database.DB
	redis      *redis.Wrapper
	connection database.Connection
	connector  *config.Connector
	auth       *config.AuthOAuth2
	stateId    uuid.UUID
}

type State struct {
	ActorId      string    `json:"actor_id"`
	ConnectorId  string    `json:"connector_id"`
	ConnectionId uuid.UUID `json:"connection_id"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (s *State) MarshalBinary() ([]byte, error) {
	return json.Marshal(s)
}

func (s *State) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, s)
}

// Make sure we have implemented the interface correctly
var _ encoding.BinaryMarshaler = (*State)(nil)
var _ encoding.BinaryUnmarshaler = (*State)(nil)

func (s *State) IsValid() bool {
	return s.ActorId != "" && s.ConnectorId != "" && s.ConnectionId != uuid.Nil && !s.ExpiresAt.IsZero()
}

func getStateRedisKey(u uuid.UUID) string {
	// Not using the state id directly to avoid cases where we do a direct lookup with a value that can be
	// tainted by a value included in URLs. The fact that this method takes a UUID also forces parsing
	// of state ids to UUIDs to ensure validation.
	return fmt.Sprintf("oauth2:state:%s", u.String())
}

func NewOAuth2(cfg config.C, db database.DB, redis *redis.Wrapper, connection database.Connection, connector config.Connector) *OAuth2 {
	auth, ok := connector.Auth.(*config.AuthOAuth2)
	if !ok {
		panic(fmt.Sprintf("connector id %s is not an oauth2 connector", connector.Id))
	}

	return &OAuth2{
		cfg:        cfg,
		db:         db,
		redis:      redis,
		connection: connection,
		auth:       auth,
		connector:  &connector,
	}
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

func (o *OAuth2) getPublicRedirectUrl(ctx context.Context, stateId uuid.UUID, actor jwt.Actor) (string, error) {
	if o.cfg == nil {
		return "", errors.New("config is nil")
	}

	if o.cfg.GetRoot() == nil {
		return "", errors.New("config root is nil")
	}

	tb, err := jwt.NewJwtTokenBuilder().
		WithActor(&actor).
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

func (o *OAuth2) saveStateToRedis(ctx context.Context, actor jwt.Actor, stateId uuid.UUID) error {
	ttl := o.cfg.GetRoot().Oauth.GetRoundTripTtlOrDefault()
	s := &State{
		ActorId:      actor.ID,
		ConnectorId:  o.connector.Id,
		ConnectionId: o.connection.ID,
		ExpiresAt:    time.Now().Add(ttl),
	}
	result := o.redis.Client.Set(ctx, getStateRedisKey(stateId), s, ttl)
	if result.Err() != nil {
		return errors.Wrapf(result.Err(), "failed to set state in redis for connector %s", o.connector.Id)
	}

	o.stateId = stateId

	return nil
}

// SetStateAndGeneratePublicUrl starts the OAuth process. It creates a state record for the connection authorization
// flow, which begins the TTL for when that connection must be completed. Returns a redirect URL to our Public redirect.
// This redirect will read from state, validate everything, then cookie the user and redirect to the 3rd party.
func (o *OAuth2) SetStateAndGeneratePublicUrl(ctx context.Context, actor jwt.Actor) (string, error) {
	stateId := uuid.New()

	if err := o.saveStateToRedis(ctx, actor, stateId); err != nil {
		return "", err
	}

	redirectUrl, err := o.getPublicRedirectUrl(ctx, stateId, actor)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get public redirect url")
	}

	return redirectUrl, nil
}

func (o *OAuth2) GenerateAuthUrl(ctx context.Context, actor jwt.Actor) (string, error) {
	if !o.auth.ClientId.HasValue(ctx) {
		return "", errors.Errorf("client id does not have value for connector %s", o.connector.Id)
	}

	clientId, err := o.auth.ClientId.GetValue(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get client id for connector %s", o.connector.Id)
	}

	if o.auth.AuthorizationEndpoint == "" {
		return "", errors.Errorf("no authorization endpoint for connector %s", o.connector.Id)
	}

	callbackUrl, err := o.getPublicCallbackUrl()
	if err != nil {
		return "", errors.Wrapf(err, "failed to get public callback url")
	}

	scopes := util.Map(o.auth.Scopes, func(s config.Scope) string {
		return s.Id
	})

	if o.stateId != uuid.Nil {
		if err = o.saveStateToRedis(ctx, actor, uuid.New()); err != nil {
			return "", err
		}
	}

	authUrl3p, err := url.Parse(o.auth.AuthorizationEndpoint)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse authorization endpoint for connector %s", o.connector.Id)
	}

	query := authUrl3p.Query()

	query.Set("redirect_uri", callbackUrl)
	query.Set("response_type", "code")
	query.Set("client_id", clientId)
	query.Set("scope", strings.Join(scopes, " "))
	query.Set("state", o.stateId.String())

	authUrl3p.RawQuery = query.Encode()

	return authUrl3p.String(), nil
}

func GetOAuth2State(
	ctx context.Context,
	cfg config.C,
	db database.DB,
	redis *redis.Wrapper,
	actor jwt.Actor,
	stateId uuid.UUID,
) (*OAuth2, error) {
	result := redis.Client.Get(ctx, getStateRedisKey(stateId))

	if result.Err() != nil {
		return nil, errors.Wrapf(result.Err(), "failed to get oauth state from redis for id %s", stateId.String())
	}

	var state State
	if err := result.Scan(&state); err != nil {
		return nil, errors.Wrap(err, "failed to parse state from redis value")
	}

	if !state.IsValid() {
		return nil, errors.Errorf("state %s is invalid", stateId.String())
	}

	if state.ExpiresAt.Before(time.Now()) {
		return nil, errors.Errorf("state %s has expired", stateId.String())
	}

	if state.ActorId != actor.ID {
		return nil, errors.Errorf("actor id %s does not match state actor id %s", actor.ID, state.ActorId)
	}

	var connector config.Connector
	found := false
	for _, connector = range cfg.GetRoot().Connectors {
		if connector.Id == state.ConnectorId {
			found = true
			break
		}
	}

	if !found {
		return nil, errors.Errorf("connector %s not found from state %s", state.ConnectorId, stateId.String())
	}

	if connector.Auth.GetType() != config.AuthTypeOAuth2 {
		return nil, errors.Errorf("connector %s is not an oauth2 connector", state.ConnectorId)
	}

	connection, err := db.GetConnection(ctx, state.ConnectionId)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get connection %s for state %s", state.ConnectionId.String(), stateId.String())
	}

	if connection == nil {
		return nil, errors.Errorf("connection %s not found for state %s", state.ConnectionId.String(), stateId.String())
	}

	// TODO: add actor auth validation once connections get ownership

	// TODO: add connector validation to make sure the connection is of the specified connector type once connections get mapped to connectors

	o := NewOAuth2(cfg, db, redis, *connection, connector)
	o.stateId = stateId

	return o, nil
}
