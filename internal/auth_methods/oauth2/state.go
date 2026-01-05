package oauth2

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
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

func (o *oAuth2Connection) saveStateToRedis(ctx context.Context, actor IActorData, stateId uuid.UUID, returnToUrl string) error {
	ttl := o.cfg.GetRoot().Oauth.GetRoundTripTtlOrDefault()
	s := &state{
		Id:               stateId,
		ActorId:          actor.GetId(),
		ConnectorId:      o.connection.GetConnectorVersionEntity().GetId(),
		ConnectorVersion: o.connection.GetConnectorVersionEntity().GetVersion(),
		ConnectionId:     o.connection.GetId(),
		ExpiresAt:        time.Now().Add(ttl),
		ReturnToUrl:      returnToUrl,
	}
	result := o.r.Set(ctx, getStateRedisKey(stateId), s, ttl)
	if result.Err() != nil {
		return errors.Wrapf(result.Err(), "failed to set state in redis for connection %s", o.connection.GetId())
	}

	o.state = s

	return nil
}

func getOAuth2State(
	ctx context.Context,
	cfg config.C,
	db database.DB,
	r apredis.Client,
	core coreIface.C,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
	actor IActorData,
	stateId uuid.UUID,
) (OAuth2Connection, error) {
	logger.DebugContext(ctx, "getting oauth state",
		"state_id", stateId,
		"actor_id", actor.GetId(),
	)

	result := r.Get(ctx, getStateRedisKey(stateId))

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

	if s.ActorId != actor.GetId() {
		return nil, errors.Errorf("actor id %s does not match state actor id %s", actor.GetId(), s.ActorId)
	}

	connection, err := core.GetConnection(ctx, s.ConnectionId)
	if err != nil {
		if errors.Is(err, coreIface.ErrNotFound) {
			return nil, errors.Errorf("connection %s not found for state %s", s.ConnectionId.String(), stateId.String())
		}

		return nil, errors.Wrapf(err, "failed to get connection %s for state %s", s.ConnectionId.String(), stateId.String())
	}

	cv := connection.GetConnectorVersionEntity()
	connector := cv.GetDefinition()
	if connector.Auth.GetType() != config.AuthTypeOAuth2 {
		return nil, errors.Errorf("connector %s is not an oauth2 connector", s.ConnectorId)
	}

	// TODO: add actor auth validation once connections get ownership

	o := newOAuth2(cfg, db, r, core, encrypt, logger, httpf, connection)
	o.state = &s

	deleteResult := r.Del(ctx, getStateRedisKey(stateId))
	if deleteResult.Err() != nil {
		return nil, errors.Wrapf(result.Err(), "failed to delete oauth state from redis for id %s", stateId.String())
	}

	return o, nil
}
