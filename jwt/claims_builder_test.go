package jwt

import (
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
	"testing"
	"time"
)

func TestClaimsBuilder(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := context.Background().WithClock(clock.NewFakeClock(now))

		cb := NewClaimsBuilder()

		cb.WithServiceId(config.ServiceIdPublic).
			WithExpiresIn(10 * time.Minute).
			WithAdmin().
			WithSessionOnly().
			WithActorId("bob-dole").
			WithActorEmail("bobdole@example.com").
			WithIssuer("me")

		claims, err := cb.BuildCtx(ctx)
		require.NoError(t, err)

		require.NotEmpty(t, claims.ID)
		require.Equal(t, "me", claims.Issuer)
		require.Equal(t, "public", claims.Audience[0])
		require.Equal(t, "admin/bob-dole", claims.Actor.ID)
		require.Equal(t, "admin/bob-dole", claims.Subject)
		require.Equal(t, "bobdole@example.com", claims.Actor.Email)
		require.True(t, claims.Actor.IsAdmin())
		require.True(t, claims.SessionOnly)
		require.Equal(t, ctx.Clock().Now().Add(10*time.Minute), claims.ExpiresAt.Time)
	})
	t.Run("nonce", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
			ctx := context.Background().WithClock(clock.NewFakeClock(now))

			cb := NewClaimsBuilder()

			cb.WithServiceId(config.ServiceIdPublic).
				WithAdmin().
				WithSessionOnly().
				WithActorId("bob-dole").
				WithActorEmail("bobdole@example.com").
				WithIssuer("me").
				WithExpiresIn(10 * time.Minute).
				WithNonce()

			claims, err := cb.BuildCtx(ctx)
			require.NoError(t, err)

			require.NotEmpty(t, claims.ID)
			require.Equal(t, "me", claims.Issuer)
			require.Equal(t, "public", claims.Audience[0])
			require.Equal(t, "admin/bob-dole", claims.Actor.ID)
			require.Equal(t, "admin/bob-dole", claims.Subject)
			require.Equal(t, "bobdole@example.com", claims.Actor.Email)
			require.True(t, claims.Actor.IsAdmin())
			require.True(t, claims.SessionOnly)
			require.Equal(t, ctx.Clock().Now().Add(10*time.Minute), claims.ExpiresAt.Time)
			require.NotNil(t, claims.Nonce)
		})
		t.Run("missing expiration", func(t *testing.T) {
			now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
			ctx := context.Background().WithClock(clock.NewFakeClock(now))

			cb := NewClaimsBuilder()

			cb.WithServiceId(config.ServiceIdPublic).
				WithAdmin().
				WithSessionOnly().
				WithActorId("bob-dole").
				WithActorEmail("bobdole@example.com").
				WithIssuer("me").
				WithNonce()

			_, err := cb.BuildCtx(ctx)
			require.Error(t, err)
		})
	})
}
