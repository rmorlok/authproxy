package oauth2

import (
	"encoding"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	"time"
)

type state struct {
	Id           uuid.UUID `json:"id"`
	ActorId      uuid.UUID `json:"actor_id"`
	ConnectorId  string    `json:"connector_id"`
	ConnectionId uuid.UUID `json:"connection_id"`
	ReturnToUrl  string    `json:"return_to"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (s *state) MarshalBinary() ([]byte, error) {
	return json.Marshal(s)
}

func (s *state) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, s)
}

// Make sure we have implemented the interface correctly
var _ encoding.BinaryMarshaler = (*state)(nil)
var _ encoding.BinaryUnmarshaler = (*state)(nil)

func (s *state) IsValid() bool {
	return s.ActorId != uuid.Nil && s.ConnectorId != "" && s.ConnectionId != uuid.Nil && !s.ExpiresAt.IsZero()
}

func getStateRedisKey(u uuid.UUID) string {
	// Not using the state id directly to avoid cases where we do a direct lookup with a value that can be
	// tainted by a value included in URLs. The fact that this method takes a UUID also forces parsing
	// of state ids to UUIDs to ensure validation.
	return fmt.Sprintf("oauth2:state:%s", u.String())
}

func (o *OAuth2) saveStateToRedis(ctx context.Context, actor database.Actor, stateId uuid.UUID, returnToUrl string) error {
	ttl := o.cfg.GetRoot().Oauth.GetRoundTripTtlOrDefault()
	s := &state{
		Id:           stateId,
		ActorId:      actor.ID,
		ConnectorId:  o.connector.Id,
		ConnectionId: o.connection.ID,
		ExpiresAt:    time.Now().Add(ttl),
		ReturnToUrl:  returnToUrl,
	}
	result := o.redis.Client().Set(ctx, getStateRedisKey(stateId), s, ttl)
	if result.Err() != nil {
		return errors.Wrapf(result.Err(), "failed to set state in redis for connector %s", o.connector.Id)
	}

	o.state = s

	return nil
}

func getOAuth2State(
	ctx context.Context,
	cfg config.C,
	db database.DB,
	redis redis.R,
	httpf httpf.F,
	encrypt encrypt.E,
	actor database.Actor,
	stateId uuid.UUID,
) (*OAuth2, error) {
	result := redis.Client().Get(ctx, getStateRedisKey(stateId))

	if result.Err() != nil {
		return nil, errors.Wrapf(result.Err(), "failed to get oauth state from redis for id %s", stateId.String())
	}

	var s state
	if err := result.Scan(&s); err != nil {
		return nil, errors.Wrap(err, "failed to parse state from redis value")
	}

	if !s.IsValid() {
		return nil, errors.Errorf("state %s is invalid", stateId.String())
	}

	if s.ExpiresAt.Before(ctx.Clock().Now()) {
		return nil, errors.Errorf("state %s has expired", stateId.String())
	}

	if s.ActorId != actor.ID {
		return nil, errors.Errorf("actor id %s does not match state actor id %s", actor.ID, s.ActorId)
	}

	var connector config.Connector
	found := false
	for _, connector = range cfg.GetRoot().Connectors {
		if connector.Id == s.ConnectorId {
			found = true
			break
		}
	}

	if !found {
		return nil, errors.Errorf("connector %s not found from state %s", s.ConnectorId, stateId.String())
	}

	if connector.Auth.GetType() != config.AuthTypeOAuth2 {
		return nil, errors.Errorf("connector %s is not an oauth2 connector", s.ConnectorId)
	}

	connection, err := db.GetConnection(ctx, s.ConnectionId)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get connection %s for state %s", s.ConnectionId.String(), stateId.String())
	}

	if connection == nil {
		return nil, errors.Errorf("connection %s not found for state %s", s.ConnectionId.String(), stateId.String())
	}

	// TODO: add actor auth validation once connections get ownership

	// TODO: add connector validation to make sure the connection is of the specified connector type once connections get mapped to connectors

	o := newOAuth2(cfg, db, redis, httpf, encrypt, *connection, connector)
	o.state = &s

	deleteResult := redis.Client().Del(ctx, getStateRedisKey(stateId))
	if deleteResult.Err() != nil {
		return nil, errors.Wrapf(result.Err(), "failed to delete oauth state from redis for id %s", stateId.String())
	}

	return o, nil
}
