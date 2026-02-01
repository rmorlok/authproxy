package jwt

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/stretchr/testify/assert"
)

func TestJwtTokenClaims(t *testing.T) {
	t.Parallel()
	t.Run("String", func(t *testing.T) {
		assert.NotPanics(t, func() {
			var tc *AuthProxyClaims
			tc.String()
		}, "it doesn't panic on a nil value")
	})
	t.Run("Actor", func(t *testing.T) {
		var tc *AuthProxyClaims
		assert.Nil(t, tc, "nil claims")

		tc = &AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "bobdole",
			},
			Actor: &core.Actor{
				ExternalId: "bobdole",
			},
		}

		assert.NotNil(t, tc.Actor)
		assert.Equal(t, "bobdole", tc.Actor.ExternalId)
	})
	t.Run("IsExpired", func(t *testing.T) {
		t.Run("nil", func(t *testing.T) {
			var tc *AuthProxyClaims
			assert.True(t, tc.IsExpired(context.Background()), "nil values default to expired")
		})
		t.Run("does not have expiration", func(t *testing.T) {
			var tc AuthProxyClaims
			assert.False(t, tc.IsExpired(context.Background()), "no expiration specified should never be expired")
		})
		t.Run("expired", func(t *testing.T) {
			tc := AuthProxyClaims{
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: &jwt.NumericDate{
						Time: time.Date(1985, time.October, 26, 1, 22, 0, 0, time.UTC),
					},
				},
			}
			assert.True(t, tc.IsExpired(context.Background()), "expired token should be expired")
		})
	})
}
