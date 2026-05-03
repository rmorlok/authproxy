package oauth2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

type state struct {
	Id                     apid.ID   `json:"id"`
	Namespace              string    `json:"namespace"`
	ActorId                apid.ID   `json:"actor_id"`
	ConnectorId            apid.ID   `json:"connector_id"`
	ConnectorVersion       uint64    `json:"connector_version"`
	ConnectionId           apid.ID   `json:"connection_id"`
	ReturnToUrl            string    `json:"return_to"`
	CancelSessionAfterAuth bool      `json:"cancel_session_after_auth"`
	ExpiresAt              time.Time `json:"expires_at"`
}

func (s *state) IsValid() bool {
	return s.Namespace != "" && s.ActorId != apid.Nil && s.ConnectorId != apid.Nil && s.ConnectionId != apid.Nil && !s.ExpiresAt.IsZero()
}

// writeStateToRedis encrypts the state with the global key (AES-GCM AEAD)
// and stores it under the state's Redis key. AEAD gives us both
// confidentiality (the payload is unreadable in Redis) and integrity (a
// payload mutated outside this proxy fails authentication on decrypt
// rather than producing a forged state), closing the
// tamper-via-redis-state attack vector.
func writeStateToRedis(ctx context.Context, r apredis.Client, e encrypt.E, s *state, ttl time.Duration) error {
	plaintext, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to marshal oauth2 state: %w", err)
	}
	ef, err := e.EncryptGlobal(ctx, plaintext)
	if err != nil {
		return fmt.Errorf("failed to encrypt oauth2 state: %w", err)
	}
	result := r.Set(ctx, getStateRedisKey(s.Id), ef.ToInlineString(), ttl)
	if result.Err() != nil {
		return fmt.Errorf("failed to set state in redis for state %s: %w", s.Id, result.Err())
	}
	return nil
}

// readStateFromRedis loads, decrypts, and unmarshals the state. A decrypt
// failure (tampered ciphertext, wrong key, etc.) is reported as a generic
// state error so callers don't leak the failure mode to clients.
func readStateFromRedis(ctx context.Context, r apredis.Client, e encrypt.E, stateId apid.ID) (*state, error) {
	raw, err := r.Get(ctx, getStateRedisKey(stateId)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth state from redis for id %s: %w", stateId.String(), err)
	}
	ef, err := encfield.ParseInlineString(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse oauth state envelope for id %s: %w", stateId.String(), err)
	}
	plaintext, err := e.Decrypt(ctx, ef)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt oauth state for id %s: %w", stateId.String(), err)
	}
	var s state
	if err := json.Unmarshal(plaintext, &s); err != nil {
		return nil, fmt.Errorf("failed to unmarshal oauth state for id %s: %w", stateId.String(), err)
	}
	return &s, nil
}

func getStateRedisKey(u apid.ID) string {
	// Not using the state id directly to avoid cases where we do a direct lookup with a value that can be
	// tainted by a value included in URLs. The fact that this method takes a UUID also forces parsing
	// of state ids to UUIDs to ensure validation.
	return fmt.Sprintf("oauth2:state:%s", u.String())
}

func (o *oAuth2Connection) saveStateToRedis(ctx context.Context, actor IActorData, stateId apid.ID, returnToUrl string) error {
	ttl := o.cfg.GetRoot().Oauth.GetRoundTripTtlOrDefault()
	s := &state{
		Id:               stateId,
		Namespace:        actor.GetNamespace(),
		ActorId:          actor.GetId(),
		ConnectorId:      o.connection.GetConnectorVersionEntity().GetId(),
		ConnectorVersion: o.connection.GetConnectorVersionEntity().GetVersion(),
		ConnectionId:     o.connection.GetId(),
		ExpiresAt:        time.Now().Add(ttl),
		ReturnToUrl:      returnToUrl,
	}
	if err := writeStateToRedis(ctx, o.r, o.encrypt, s, ttl); err != nil {
		return fmt.Errorf("failed to set state in redis for connection %s: %w", o.connection.GetId(), err)
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
	stateId apid.ID,
) (OAuth2Connection, error) {
	logger.DebugContext(ctx, "getting oauth state",
		"state_id", stateId,
		"actor_id", actor.GetId(),
	)

	s, err := readStateFromRedis(ctx, r, encrypt, stateId)
	if err != nil {
		return nil, err
	}

	if !s.IsValid() {
		return nil, fmt.Errorf("state %s is invalid", stateId.String())
	}

	if s.ExpiresAt.Before(apctx.GetClock(ctx).Now()) {
		return nil, fmt.Errorf("state %s has expired", stateId.String())
	}

	if s.ActorId != actor.GetId() {
		return nil, fmt.Errorf("actor id %s does not match state actor id %s", actor.GetId(), s.ActorId)
	}

	if s.Namespace != actor.GetNamespace() {
		return nil, fmt.Errorf("actor namespace %q does not match state namespace %q", actor.GetNamespace(), s.Namespace)
	}

	connection, err := core.GetConnection(ctx, s.ConnectionId)
	if err != nil {
		if errors.Is(err, coreIface.ErrNotFound) {
			return nil, fmt.Errorf("connection %s not found for state %s", s.ConnectionId.String(), stateId.String())
		}

		return nil, fmt.Errorf("failed to get connection %s for state %s: %w", s.ConnectionId.String(), stateId.String(), err)
	}

	if s.Namespace != connection.GetNamespace() {
		return nil, fmt.Errorf("connection namespace %q does not match state namespace %q", connection.GetNamespace(), s.Namespace)
	}

	cv := connection.GetConnectorVersionEntity()
	connector := cv.GetDefinition()
	if connector.Auth.GetType() != sconfig.AuthTypeOAuth2 {
		return nil, fmt.Errorf("connector %s is not an oauth2 connector", s.ConnectorId)
	}

	// TODO: add actor auth validation once connections get ownership

	o := newOAuth2(cfg, db, r, core, encrypt, logger, httpf, connection)
	o.state = s

	return o, nil
}

// deleteStateFromRedis removes the OAuth state from Redis. This should be called
// after the state has been fully consumed (i.e., after the callback processes it).
func deleteStateFromRedis(ctx context.Context, r apredis.Client, stateId apid.ID) error {
	result := r.Del(ctx, getStateRedisKey(stateId))
	if result.Err() != nil {
		return fmt.Errorf("failed to delete oauth state from redis for id %s: %w", stateId.String(), result.Err())
	}
	return nil
}
