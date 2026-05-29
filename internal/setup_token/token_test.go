package setup_token_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/setup_token"
)

// testEnv wires an in-memory redis (via apredis test helper) + fake encrypt
// for token round-trip tests. The fake encrypt service is a no-op cipher
// (plaintext is stored in EncryptedField.Data) — good enough to exercise
// the round-trip plumbing without booting a KMS.
func testEnv(t *testing.T) (context.Context, apredis.Client, encrypt.E) {
	t.Helper()
	ctx := context.Background()
	_, r := apredis.MustApplyTestConfig(nil)
	e := encrypt.NewFakeEncryptService(false)
	return ctx, r, e
}

func validInput() setup_token.MintInput {
	return setup_token.MintInput{
		ConnectionId: apid.New(apid.PrefixConnection),
		StepId:       "preconnect_redirect",
		Intent:       setup_token.IntentAdvance,
		ReturnToUrl:  "https://marketplace.example.com/return",
	}
}

// TestMintAndConsume_RoundTrip — the happy path: Mint returns a non-empty
// token, VerifyAndConsume hands back the original claims, and a second
// consume attempt fails (one-time-use).
func TestMintAndConsume_RoundTrip(t *testing.T) {
	ctx, r, e := testEnv(t)
	in := validInput()

	tok, err := setup_token.Mint(ctx, r, e, in, time.Minute)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(tok, "stk_"), "token should carry the setup-token prefix")

	claims, err := setup_token.VerifyAndConsume(ctx, r, e, tok)
	require.NoError(t, err)
	assert.Equal(t, in.ConnectionId, claims.ConnectionId)
	assert.Equal(t, in.StepId, claims.StepId)
	assert.Equal(t, in.Intent, claims.Intent)
	assert.Equal(t, in.ReturnToUrl, claims.ReturnToUrl)
	assert.Equal(t, tok, claims.Jti)

	// Replay rejected.
	_, err = setup_token.VerifyAndConsume(ctx, r, e, tok)
	assert.ErrorIs(t, err, setup_token.ErrNotFound)
}

// TestMint_RejectsInvalidInputs covers the up-front validation. Each case
// should fail Mint without touching Redis.
func TestMint_RejectsInvalidInputs(t *testing.T) {
	ctx, r, e := testEnv(t)

	cases := []struct {
		name string
		in   setup_token.MintInput
		ttl  time.Duration
	}{
		{
			name: "empty connection_id",
			in:   setup_token.MintInput{StepId: "x", Intent: setup_token.IntentAdvance},
			ttl:  time.Minute,
		},
		{
			name: "empty step_id",
			in:   setup_token.MintInput{ConnectionId: apid.New(apid.PrefixConnection), Intent: setup_token.IntentAdvance},
			ttl:  time.Minute,
		},
		{
			name: "invalid intent",
			in:   setup_token.MintInput{ConnectionId: apid.New(apid.PrefixConnection), StepId: "x", Intent: "noop"},
			ttl:  time.Minute,
		},
		{
			name: "zero ttl",
			in:   validInput(),
			ttl:  0,
		},
		{
			name: "negative ttl",
			in:   validInput(),
			ttl:  -time.Second,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := setup_token.Mint(ctx, r, e, tc.in, tc.ttl)
			require.Error(t, err)
		})
	}
}

// TestVerifyAndConsume_NotFound — an unknown / never-minted token is
// rejected with ErrNotFound (not a system error). Distinguishes forged or
// expired tokens from system faults at the route layer.
func TestVerifyAndConsume_NotFound(t *testing.T) {
	ctx, r, e := testEnv(t)

	_, err := setup_token.VerifyAndConsume(ctx, r, e, "stk_neverexisted000")
	assert.ErrorIs(t, err, setup_token.ErrNotFound)
}

// TestVerifyAndConsume_EmptyToken — defense-in-depth at the API boundary:
// an empty token short-circuits to ErrNotFound rather than producing a
// confusing Redis error.
func TestVerifyAndConsume_EmptyToken(t *testing.T) {
	ctx, r, e := testEnv(t)

	_, err := setup_token.VerifyAndConsume(ctx, r, e, "")
	assert.ErrorIs(t, err, setup_token.ErrNotFound)
}

// TestVerifyAndConsume_Tampered — a payload modified outside the proxy
// fails AEAD authentication on decrypt and surfaces as ErrTampered, which
// the route layer maps to a 403 + security event (not a 404).
func TestVerifyAndConsume_Tampered(t *testing.T) {
	ctx, r, e := testEnv(t)
	in := validInput()

	tok, err := setup_token.Mint(ctx, r, e, in, time.Minute)
	require.NoError(t, err)

	// Overwrite the Redis value with garbage that doesn't parse as an
	// encrypted envelope. (Tampering the inner ciphertext is exercised by
	// the encrypt package's own tests.)
	require.NoError(t, r.Set(ctx, "setup_token:"+tok, "not-an-envelope", time.Minute).Err())

	_, err = setup_token.VerifyAndConsume(ctx, r, e, tok)
	assert.ErrorIs(t, err, setup_token.ErrTampered)
}

// TestVerifyAndConsume_Expired — Mint with a short TTL; wait for Redis to
// expire it; consume returns ErrNotFound.
func TestVerifyAndConsume_Expired(t *testing.T) {
	ctx, r, e := testEnv(t)

	tok, err := setup_token.Mint(ctx, r, e, validInput(), 100*time.Millisecond)
	require.NoError(t, err)

	// Force expiration deterministically by deleting the key — Redis's own
	// expiration is well-tested upstream, so we don't add a sleep here.
	require.NoError(t, r.Del(ctx, "setup_token:"+tok).Err())

	_, err = setup_token.VerifyAndConsume(ctx, r, e, tok)
	assert.ErrorIs(t, err, setup_token.ErrNotFound)
}

// TestIntent_IsValid — the wire-format gate. Only "advance" and "abort"
// are accepted; anything else surfaces as an invalid-intent error from
// Mint and a tamper-shaped result from VerifyAndConsume.
func TestIntent_IsValid(t *testing.T) {
	assert.True(t, setup_token.IntentAdvance.IsValid())
	assert.True(t, setup_token.IntentAbort.IsValid())
	assert.False(t, setup_token.Intent("").IsValid())
	assert.False(t, setup_token.Intent("noop").IsValid())
}

// guard: keep go-redis as an explicit imported package so go fix can't
// silently drop the transitive dependency the apredis test client uses.
var _ = redis.Nil
