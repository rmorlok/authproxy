package oauth2

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/config"
	connIface "github.com/rmorlok/authproxy/connectors/interface"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	"log/slog"
	"time"
)

type state struct {
	Id                     uuid.UUID `json:"id"`
	ActorId                uuid.UUID `json:"actor_id"`
	ConnectorId            uuid.UUID `json:"connector_id"`
	ConnectorVersion       uint64    `json:"connector_version"`
	ConnectionId           uuid.UUID `json:"connection_id"`
	ReturnToUrl            string    `json:"return_to"`
	CancelSessionAfterAuth bool      `json:"cancel_session_after_auth"`
	ExpiresAt              time.Time `json:"expires_at"`
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
	return s.ActorId != uuid.Nil && s.ConnectorId != uuid.Nil && s.ConnectionId != uuid.Nil && !s.ExpiresAt.IsZero()
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
		Id:               stateId,
		ActorId:          actor.ID,
		ConnectorId:      o.cv.GetID(),
		ConnectorVersion: o.cv.GetVersion(),
		ConnectionId:     o.connection.ID,
		ExpiresAt:        time.Now().Add(ttl),
		ReturnToUrl:      returnToUrl,
	}
	result := o.redis.Client().Set(ctx, getStateRedisKey(stateId), s, ttl)
	if result.Err() != nil {
		return errors.Wrapf(result.Err(), "failed to set state in redis for connector %s", o.cv.GetID())
	}

	o.state = s

	return nil
}

func getOAuth2State(
	ctx context.Context,
	cfg config.C,
	db database.DB,
	redis redis.R,
	c connIface.C,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
	actor database.Actor,
	stateId uuid.UUID,
) (*OAuth2, error) {
	logger.DebugContext(ctx, "getting oauth state",
		"state_id", stateId,
		"actor_id", actor.ID,
	)

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

	if s.ExpiresAt.Before(apctx.GetClock(ctx).Now()) {
		return nil, errors.Errorf("state %s has expired", stateId.String())
	}

	if s.ActorId != actor.ID {
		return nil, errors.Errorf("actor id %s does not match state actor id %s", actor.ID, s.ActorId)
	}

	cv, err := c.GetConnectorVersion(ctx, s.ConnectorId, s.ConnectorVersion)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load connector version")
	}

	if cv == nil {
		return nil, errors.Errorf("connector %s version %d not found from state %s", s.ConnectorId, s.ConnectorVersion, stateId.String())
	}

	connector := cv.GetDefinition()
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

	o := newOAuth2(cfg, db, redis, c, encrypt, logger, httpf, *connection, cv)
	o.state = &s

	deleteResult := redis.Client().Del(ctx, getStateRedisKey(stateId))
	if deleteResult.Err() != nil {
		return nil, errors.Wrapf(result.Err(), "failed to delete oauth state from redis for id %s", stateId.String())
	}

	return o, nil
}
