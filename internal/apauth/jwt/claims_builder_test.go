package jwt

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apctx"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/config"
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
			WithActorExternalId("bob-dole").
			WithNamespace("root.child").
			WithActorEmail("bobdole@example.com"). // This does nothing because the actor isn't specified
			WithIssuer("me")

		claims, err := cb.BuildCtx(ctx)
		require.NoError(t, err)

		require.NotEmpty(t, claims.ID)
		require.Nil(t, claims.Actor)
		require.Equal(t, "me", claims.Issuer)
		require.Equal(t, "public", claims.Audience[0])
		require.Equal(t, "admin/bob-dole", claims.Subject)
		require.Equal(t, "root.child", claims.Namespace)
		require.Equal(t, apctx.GetClock(ctx).Now().Add(10*time.Minute), claims.ExpiresAt.Time)
	})
	t.Run("namespace optional no actor", func(t *testing.T) {
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		cb := NewClaimsBuilder()

		cb.WithServiceId(config.ServiceIdPublic).
			WithExpiresIn(10 * time.Minute).
			WithAdmin().
			WithActorExternalId("bob-dole").
			WithActorEmail("bobdole@example.com").
			WithIssuer("me")

		claims, err := cb.BuildCtx(ctx)
		require.NoError(t, err)

		require.NotEmpty(t, claims.ID)
		require.Nil(t, claims.Actor)
		require.Equal(t, "me", claims.Issuer)
		require.Equal(t, "public", claims.Audience[0])
		require.Equal(t, "admin/bob-dole", claims.Subject)
		require.Equal(t, "root", claims.GetNamespace()) // Defaults to root
		require.Equal(t, apctx.GetClock(ctx).Now().Add(10*time.Minute), claims.ExpiresAt.Time)
	})
	t.Run("valid with actor", func(t *testing.T) {
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		cb := NewClaimsBuilder()

		cb.WithServiceId(config.ServiceIdPublic).
			WithActor(&core.Actor{}).
			WithExpiresIn(10 * time.Minute).
			WithAdmin().
			WithActorExternalId("bob-dole").
			WithNamespace("root.child").
			WithActorEmail("bobdole@example.com").
			WithIssuer("me")

		claims, err := cb.BuildCtx(ctx)
		require.NoError(t, err)

		require.NotEmpty(t, claims.ID)
		require.Equal(t, "me", claims.Issuer)
		require.Equal(t, "public", claims.Audience[0])
		require.Equal(t, "admin/bob-dole", claims.Actor.ExternalId)
		require.Equal(t, "root.child", claims.Actor.Namespace)
		require.Equal(t, "admin/bob-dole", claims.Subject)
		require.Equal(t, "root.child", claims.Namespace)
		require.Equal(t, "bobdole@example.com", claims.Actor.Email)
		require.True(t, claims.Actor.IsAdmin())
		require.Equal(t, apctx.GetClock(ctx).Now().Add(10*time.Minute), claims.ExpiresAt.Time)
	})
	t.Run("valid with actor specified data", func(t *testing.T) {
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		cb := NewClaimsBuilder()

		cb.WithServiceId(config.ServiceIdPublic).
			WithActor(&core.Actor{
				ExternalId: "bob-dole",
				Namespace:  "root.child",
				Email:      "bobdole@example.com",
				Permissions: []aschema.Permission{
					{
						Namespace: "root.child",
						Resources: []string{"connections"},
						Verbs:     []string{"get"},
					},
				},
			}).
			WithExpiresIn(10 * time.Minute).
			WithIssuer("me")

		claims, err := cb.BuildCtx(ctx)
		require.NoError(t, err)

		require.NotEmpty(t, claims.ID)
		require.Equal(t, "me", claims.Issuer)
		require.Equal(t, "public", claims.Audience[0])
		require.Equal(t, "bob-dole", claims.Actor.ExternalId)
		require.Equal(t, "root.child", claims.Actor.Namespace)
		require.Equal(t, "bob-dole", claims.Subject)
		require.Equal(t, "root.child", claims.Namespace)
		require.Equal(t, "bobdole@example.com", claims.Actor.Email)
		require.Equal(t, apctx.GetClock(ctx).Now().Add(10*time.Minute), claims.ExpiresAt.Time)
	})
	t.Run("nonce", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			cb := NewClaimsBuilder()

			cb.WithServiceId(config.ServiceIdPublic).
				WithAdmin().
				WithActorExternalId("bob-dole").
				WithNamespace("root.child").
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
				WithActor(&core.Actor{}).
				WithAdmin().
				WithActorExternalId("bob-dole").
				WithNamespace("root.child").
				WithActorEmail("bobdole@example.com").
				WithIssuer("me").
				WithExpiresIn(10 * time.Minute).
				WithNonce()

			claims, err := cb.BuildCtx(ctx)
			require.NoError(t, err)

			require.NotEmpty(t, claims.ID)
			require.Equal(t, "me", claims.Issuer)
			require.Equal(t, "public", claims.Audience[0])
			require.Equal(t, "admin/bob-dole", claims.Actor.ExternalId)
			require.Equal(t, "root.child", claims.Actor.Namespace)
			require.Equal(t, "admin/bob-dole", claims.Subject)
			require.Equal(t, "root.child", claims.Namespace)
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
				WithActorExternalId("bob-dole").
				WithNamespace("root.child").
				WithActorEmail("bobdole@example.com").
				WithIssuer("me").
				WithNonce()

			_, err := cb.BuildCtx(ctx)
			require.Error(t, err)
		})
		t.Run("missing namespace when specifying actor", func(t *testing.T) {
			now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			cb := NewClaimsBuilder()

			cb.WithServiceId(config.ServiceIdPublic).
				WithActor(&core.Actor{}).
				WithAdmin().
				WithActorExternalId("bob-dole").
				WithActorEmail("bobdole@example.com").
				WithIssuer("me").
				WithExpiresIn(10 * time.Minute).
				WithNonce()

			_, err := cb.BuildCtx(ctx)
			require.Error(t, err)
		})
	})
}
