package jwt

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			Actor: NewActorClaim(&core.Actor{
				ExternalId: "bobdole",
				Namespace:  "root",
			}),
		}

		assert.NotNil(t, tc.Actor)
		assert.Equal(t, "bobdole", tc.Actor.Spec.ExternalId)
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
	t.Run("Validate top-level permissions", func(t *testing.T) {
		tc := AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "bobdole",
			},
			Permissions: []aschema.Permission{
				{
					Namespace: "root.**",
					Resources: []string{"connections"},
					Verbs:     []string{"list"},
				},
			},
		}
		assert.NoError(t, tc.Validate(jwt.NewValidator()))

		tc.Permissions = []aschema.Permission{{Namespace: "root.**"}}
		err := tc.Validate(jwt.NewValidator())
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidClaims)
	})
}

func TestAuthProxyClaimsValidateActorContract(t *testing.T) {
	basePermissions := []aschema.Permission{{
		Namespace: "root.platform.**",
		Resources: []string{"connections"},
		Verbs:     []string{"get", "list"},
	}}
	valid := AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "deploy-bot"},
		Namespace:        "root.platform",
		Actor: NewActorClaim(&core.Actor{
			ExternalId:  "deploy-bot",
			Namespace:   "root.platform",
			Permissions: basePermissions,
		}),
		Permissions: []aschema.Permission{{
			Namespace: "root.platform.production",
			Resources: []string{"connections"},
			Verbs:     []string{"get"},
		}},
	}
	require.NoError(t, valid.Validate(jwt.NewValidator()))

	tests := map[string]struct {
		mutate    func(*AuthProxyClaims)
		errorText string
	}{
		"subject mismatch": {
			mutate:    func(claims *AuthProxyClaims) { claims.Subject = "other" },
			errorText: "subject and actor spec.externalId",
		},
		"namespace mismatch": {
			mutate:    func(claims *AuthProxyClaims) { claims.Namespace = "root.other" },
			errorText: "namespace and actor metadata.namespace",
		},
		"permission escalation": {
			mutate: func(claims *AuthProxyClaims) {
				claims.Permissions = []aschema.Permission{{
					Namespace: "root.**",
					Resources: []string{"actors"},
					Verbs:     []string{"delete"},
				}}
			},
			errorText: "beyond the actor's permissions",
		},
		"invalid actor resource": {
			mutate:    func(claims *AuthProxyClaims) { claims.Actor.Kind = "Connection" },
			errorText: "kind",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			claims := valid
			actor := *valid.Actor
			claims.Actor = &actor
			tt.mutate(&claims)
			err := claims.Validate(jwt.NewValidator())
			require.ErrorIs(t, err, ErrInvalidClaims)
			require.True(t, strings.Contains(err.Error(), tt.errorText), err.Error())
		})
	}
}
