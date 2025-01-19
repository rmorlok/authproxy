package database

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/context"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
	"testing"
	"time"
)

func TestOAuth2Tokens(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("nonce_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := context.Background().WithClock(clock.NewFakeClock(now))

		connectionId := uuid.New()

		tok, err := db.InsertOAuth2Token(
			ctx,
			connectionId,
			nil,
			"encryptedRefreshToken",
			"encryptedAccessToken",
			nil,
			"scope1 scope2",
		)
		require.NoError(t, err)
		require.NotNil(t, tok)
		require.Equal(t, connectionId, tok.ConnectionID)
		require.Nil(t, tok.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken", tok.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken", tok.EncryptedAccessToken)
		require.Equal(t, now, tok.CreatedAt)

		tok2, err := db.GetOAuth2Token(ctx, connectionId)
		require.NoError(t, err)
		require.NotNil(t, tok2)
		require.Equal(t, connectionId, tok2.ConnectionID)
		require.Nil(t, tok2.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken", tok2.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken", tok2.EncryptedAccessToken)
	})
	t.Run("no tokens", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("nonce_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := context.Background().WithClock(clock.NewFakeClock(now))

		connectionId1 := uuid.New()
		connectionId2 := uuid.New()

		_, err := db.InsertOAuth2Token(
			ctx,
			connectionId1,
			nil,
			"encryptedRefreshToken",
			"encryptedAccessToken",
			nil,
			"scope1 scope2",
		)
		require.NoError(t, err)

		tok, err := db.GetOAuth2Token(ctx, connectionId2)
		require.NoError(t, err)
		require.Nil(t, tok)
	})
	t.Run("replaces previous when tagging previous", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("nonce_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := context.Background().WithClock(clock.NewFakeClock(now))

		connectionId := uuid.New()

		tok1, err := db.InsertOAuth2Token(
			ctx,
			connectionId,
			nil,
			"encryptedRefreshToken",
			"encryptedAccessToken",
			nil,
			"scope1 scope2",
		)
		require.NoError(t, err)
		require.NotNil(t, tok1)
		require.Equal(t, connectionId, tok1.ConnectionID)
		require.Nil(t, tok1.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken", tok1.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken", tok1.EncryptedAccessToken)
		require.Equal(t, now, tok1.CreatedAt)

		tok2, err := db.InsertOAuth2Token(
			ctx,
			connectionId,
			&tok1.ID,
			"encryptedRefreshToken2",
			"encryptedAccessToken2",
			nil,
			"scope1 scope2",
		)
		require.NoError(t, err)
		require.NotNil(t, tok2)
		require.Equal(t, connectionId, tok2.ConnectionID)
		require.Equal(t, &tok1.ID, tok2.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken2", tok2.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken2", tok2.EncryptedAccessToken)
		require.Equal(t, now, tok2.CreatedAt)

		tok3, err := db.GetOAuth2Token(ctx, connectionId)
		require.NoError(t, err)
		require.NotNil(t, tok2)
		require.Equal(t, connectionId, tok3.ConnectionID)
		require.Equal(t, &tok1.ID, tok2.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken2", tok2.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken2", tok2.EncryptedAccessToken)
	})
	t.Run("replaces previous when not tagging previous", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("nonce_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := context.Background().WithClock(clock.NewFakeClock(now))

		connectionId := uuid.New()

		tok1, err := db.InsertOAuth2Token(
			ctx,
			connectionId,
			nil,
			"encryptedRefreshToken",
			"encryptedAccessToken",
			nil,
			"scope1 scope2",
		)
		require.NoError(t, err)
		require.NotNil(t, tok1)
		require.Equal(t, connectionId, tok1.ConnectionID)
		require.Nil(t, tok1.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken", tok1.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken", tok1.EncryptedAccessToken)
		require.Equal(t, now, tok1.CreatedAt)

		tok2, err := db.InsertOAuth2Token(
			ctx,
			connectionId,
			nil, // not tagging previous
			"encryptedRefreshToken2",
			"encryptedAccessToken2",
			nil,
			"scope1 scope2",
		)
		require.NoError(t, err)
		require.NotNil(t, tok2)
		require.Equal(t, connectionId, tok2.ConnectionID)
		require.Nil(t, tok2.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken2", tok2.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken2", tok2.EncryptedAccessToken)
		require.Equal(t, now, tok2.CreatedAt)

		tok3, err := db.GetOAuth2Token(ctx, connectionId)
		require.NoError(t, err)
		require.NotNil(t, tok2)
		require.Equal(t, connectionId, tok3.ConnectionID)
		require.Nil(t, tok2.RefreshedFromID)
		require.Equal(t, "encryptedRefreshToken2", tok2.EncryptedRefreshToken)
		require.Equal(t, "encryptedAccessToken2", tok2.EncryptedAccessToken)
	})
}
