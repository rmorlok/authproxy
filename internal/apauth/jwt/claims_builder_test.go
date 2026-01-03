package jwt

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestClaimsBuilder(t *testing.T) {
	t.Parallel()
	t.Run("valid no actor", func(t *testing.T) {
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		cb := NewClaimsBuilder()

		cb.WithServiceId(config.ServiceIdPublic).
			WithExpiresIn(10 * time.Minute).
			WithAdmin().
			WithActorId("bob-dole").
			WithActorEmail("bobdole@example.com"). // This does nothing because the actor isn't specified
			WithIssuer("me")

		claims, err := cb.BuildCtx(ctx)
		require.NoError(t, err)

		require.NotEmpty(t, claims.ID)
		require.Nil(t, claims.Actor)
		require.Equal(t, "me", claims.Issuer)
		require.Equal(t, "public", claims.Audience[0])
		require.Equal(t, "admin/bob-dole", claims.Subject)
		require.Equal(t, apctx.GetClock(ctx).Now().Add(10*time.Minute), claims.ExpiresAt.Time)
	})
	t.Run("valid with actor", func(t *testing.T) {
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		cb := NewClaimsBuilder()

		cb.WithServiceId(config.ServiceIdPublic).
			WithActor(&Actor{}).
			WithExpiresIn(10 * time.Minute).
			WithAdmin().
			WithActorId("bob-dole").
			WithActorEmail("bobdole@example.com").
			WithIssuer("me")

		claims, err := cb.BuildCtx(ctx)
		require.NoError(t, err)

		require.NotEmpty(t, claims.ID)
		require.Equal(t, "me", claims.Issuer)
		require.Equal(t, "public", claims.Audience[0])
		require.Equal(t, "admin/bob-dole", claims.Actor.Id)
		require.Equal(t, "admin/bob-dole", claims.Subject)
		require.Equal(t, "bobdole@example.com", claims.Actor.Email)
		require.True(t, claims.Actor.IsAdmin())
		require.Equal(t, apctx.GetClock(ctx).Now().Add(10*time.Minute), claims.ExpiresAt.Time)
	})
	t.Run("nonce", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			cb := NewClaimsBuilder()

			cb.WithServiceId(config.ServiceIdPublic).
				WithAdmin().
				WithActorId("bob-dole").
				WithIssuer("me").
				WithExpiresIn(10 * time.Minute).
				WithNonce()

			claims, err := cb.BuildCtx(ctx)
			require.NoError(t, err)

			require.NotEmpty(t, claims.ID)
			require.Equal(t, "me", claims.Issuer)
			require.Equal(t, "public", claims.Audience[0])
			require.Equal(t, "admin/bob-dole", claims.Subject)
			require.Equal(t, apctx.GetClock(ctx).Now().Add(10*time.Minute), claims.ExpiresAt.Time)
			require.NotNil(t, claims.Nonce)
		})
		t.Run("valid with actor", func(t *testing.T) {
			now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			cb := NewClaimsBuilder()

			cb.WithServiceId(config.ServiceIdPublic).
				WithActor(&Actor{}).
				WithAdmin().
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
			require.Equal(t, "admin/bob-dole", claims.Actor.Id)
			require.Equal(t, "admin/bob-dole", claims.Subject)
			require.Equal(t, "bobdole@example.com", claims.Actor.Email)
			require.True(t, claims.Actor.IsAdmin())
			require.Equal(t, apctx.GetClock(ctx).Now().Add(10*time.Minute), claims.ExpiresAt.Time)
			require.NotNil(t, claims.Nonce)
		})
		t.Run("missing expiration", func(t *testing.T) {
			now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			cb := NewClaimsBuilder()

			cb.WithServiceId(config.ServiceIdPublic).
				WithAdmin().
				WithActorId("bob-dole").
				WithActorEmail("bobdole@example.com").
				WithIssuer("me").
				WithNonce()

			_, err := cb.BuildCtx(ctx)
			require.Error(t, err)
		})
	})
}
