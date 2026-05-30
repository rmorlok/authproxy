// Package setup_token implements signed, one-time-use redirect tokens used
// by the schema-defined redirect-step machinery. A token represents an
// authorization for a 3rd party to bounce a user back through one of the
// /public/connections/{id}/setup/{advance,abort} endpoints to transition a
// connection's setup_step.
//
// Wire format: opaque base64-encoded UUID (the jti). The claim payload is
// stored encrypted in Redis under that jti — verifying = looking up the
// jti, decrypting the payload, and atomically deleting the row so the
// token can only be used once. The redis-as-source-of-truth shape mirrors
// the OAuth2 state machinery and gets us authentication (via encrypt.E's
// AES-GCM) plus one-time-use for free.
package setup_token

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
)

// Intent identifies what consuming the token does. A token minted with
// IntentAdvance is rejected at the /abort endpoint and vice-versa, so a
// leaked token can't be repurposed.
type Intent string

const (
	IntentAdvance Intent = "advance"
	IntentAbort   Intent = "abort"
)

// IsValid reports whether i is a known intent.
func (i Intent) IsValid() bool { return i == IntentAdvance || i == IntentAbort }

// Claims is the verified payload a /setup/advance or /setup/abort handler
// reads after VerifyAndConsume succeeds. ConnectionId + StepId + ActorId
// are all pinned at mint time so a token can only act on the specific
// connection/step pair it was issued for and only when consumed by the
// same actor that initiated the flow; ReturnToUrl carries the
// marketplace's eventual destination through the off-platform bounce.
type Claims struct {
	// ConnectionId is the connection the token authorizes a transition on.
	ConnectionId apid.ID `json:"connection_id"`

	// StepId is the redirect step the token was minted from. The handler
	// rejects if the connection's current setup_step has changed away
	// from this id between mint and consume, defending against a stale
	// token unexpectedly advancing past an unrelated step.
	StepId string `json:"step_id"`

	// ActorId is the actor that initiated the redirect step. The /advance
	// and /abort endpoints require a session and reject when the
	// session's actor doesn't match this claim — defends against a
	// leaked token being used by a different user even before its TTL
	// expires.
	ActorId apid.ID `json:"actor_id"`

	// Intent gates which endpoint can consume the token.
	Intent Intent `json:"intent"`

	// ReturnToUrl is the marketplace URL the consumer should redirect the
	// user back to after the transition completes. Carries the original
	// _initiate return-to URL through the off-platform bounce.
	ReturnToUrl string `json:"return_to_url,omitempty"`

	// Jti is the opaque token id surfaced as the URL-side parameter.
	// Populated by Mint; callers do not set it.
	Jti string `json:"jti"`
}

// ErrNotFound is returned when VerifyAndConsume cannot find the supplied
// token in Redis — either the token is forged, expired, or already
// consumed. Callers map this to 401/403.
var ErrNotFound = errors.New("setup_token: not found")

// ErrTampered is returned when the Redis payload exists but fails AEAD
// authentication on decrypt. Implies the row was modified outside this
// proxy (or a critical encryption-key rotation went wrong).
var ErrTampered = errors.New("setup_token: tampered payload")

// MintInput collects the per-mint parameters. The jti is generated
// inside Mint — the caller supplies just the claim payload.
type MintInput struct {
	ConnectionId apid.ID
	StepId       string
	ActorId      apid.ID
	Intent       Intent
	ReturnToUrl  string
}

// Validate sanity-checks the input. Returns nil on success.
func (m MintInput) Validate() error {
	if m.ConnectionId == apid.Nil {
		return errors.New("setup_token: connection_id is required")
	}
	if m.StepId == "" {
		return errors.New("setup_token: step_id is required")
	}
	if m.ActorId == apid.Nil {
		return errors.New("setup_token: actor_id is required")
	}
	if !m.Intent.IsValid() {
		return fmt.Errorf("setup_token: invalid intent %q", m.Intent)
	}
	return nil
}

// Mint generates a fresh one-time-use token. The returned string is what
// callers embed into the redirect URL's `?token=` parameter; the claim
// payload is encrypted and stored in Redis under the token id with the
// given TTL. The TTL bounds how long a 3rd party has to bounce the user
// back; once it expires the token is gone forever.
func Mint(ctx context.Context, r apredis.Client, e encrypt.E, in MintInput, ttl time.Duration) (string, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}
	if ttl <= 0 {
		return "", errors.New("setup_token: ttl must be positive")
	}

	jti := apid.New(apid.PrefixSetupToken).String()
	claims := Claims{
		ConnectionId: in.ConnectionId,
		StepId:       in.StepId,
		ActorId:      in.ActorId,
		Intent:       in.Intent,
		ReturnToUrl:  in.ReturnToUrl,
		Jti:          jti,
	}

	plaintext, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("setup_token: marshal claims: %w", err)
	}
	ef, err := e.EncryptGlobal(ctx, plaintext)
	if err != nil {
		return "", fmt.Errorf("setup_token: encrypt claims: %w", err)
	}
	if err := r.Set(ctx, redisKey(jti), ef.ToInlineString(), ttl).Err(); err != nil {
		return "", fmt.Errorf("setup_token: persist to redis: %w", err)
	}
	return jti, nil
}

// VerifyAndConsume looks up the token, decrypts the payload, and deletes
// the row atomically — replays return ErrNotFound. Callers MUST check
// intent against the endpoint that received the token before acting on
// the returned claims.
func VerifyAndConsume(ctx context.Context, r apredis.Client, e encrypt.E, token string) (Claims, error) {
	if token == "" {
		return Claims{}, ErrNotFound
	}

	key := redisKey(token)
	raw, err := r.GetDel(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return Claims{}, ErrNotFound
		}
		return Claims{}, fmt.Errorf("setup_token: redis getdel: %w", err)
	}
	ef, err := encfield.ParseInlineString(raw)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: parse envelope: %v", ErrTampered, err)
	}
	plaintext, err := e.Decrypt(ctx, ef)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: decrypt: %v", ErrTampered, err)
	}
	var c Claims
	if err := json.Unmarshal(plaintext, &c); err != nil {
		return Claims{}, fmt.Errorf("%w: unmarshal: %v", ErrTampered, err)
	}
	return c, nil
}

func redisKey(jti string) string {
	return "setup_token:" + jti
}
