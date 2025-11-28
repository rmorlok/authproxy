package jwt

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
	t.Run("IsAdmin", func(t *testing.T) {
		var tc *AuthProxyClaims
		assert.False(t, tc.IsAdmin(), "nil values aren't admins")

		tc = &AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "admin/bobdole",
			},
			Actor: &Actor{
				ID:    "admin/bobdole",
				Admin: true,
			},
		}

		assert.True(t, tc.IsAdmin(), "admins with actor are admins")

		tc = &AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "admin/bobdole",
			},
		}

		assert.True(t, tc.IsAdmin(), "admins without actor are admins")

		tc = &AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "admin/bobdole",
			},
			Actor: &Actor{
				ID:    "admin/bobdole",
				Admin: false,
			},
		}

		assert.False(t, tc.IsAdmin(), "conflicting values result in false")
	})

	t.Run("IsSuperAdmin", func(t *testing.T) {
		var tc *AuthProxyClaims
		assert.False(t, tc.IsAdmin(), "nil values aren't super admins")
	})
	t.Run("IsNormalActor", func(t *testing.T) {
		var tc *AuthProxyClaims
		assert.True(t, tc.IsNormalActor(), "nil values are normal actors")
	})
	t.Run("AdminUsername", func(t *testing.T) {
		assert := require.New(t)

		// valid
		j := AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "admin/bobdole",
			},
			Actor: &Actor{
				ID:    "admin/bobdole",
				Admin: true,
			},
		}
		username, err := j.AdminUsername()
		assert.NoError(err)
		assert.Equal("bobdole", username)

		// No actor id
		j = AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "admin/bobdole",
			},
			Actor: &Actor{
				Admin: true,
			},
		}
		_, err = j.AdminUsername()
		assert.Error(err)

		// No actor
		j = AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "admin/bobdole",
			},
		}
		username, err = j.AdminUsername()
		assert.NoError(err)
		assert.Equal("bobdole", username)

		// No subject
		j = AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{},
			Actor: &Actor{
				ID:    "admin/bobdole",
				Admin: true,
			},
		}
		_, err = j.AdminUsername()
		assert.Error(err)

		// usernames don't match
		j = AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "admin/bobsmith",
			},
			Actor: &Actor{
				ID:    "admin/bobdole",
				Admin: true,
			},
		}
		_, err = j.AdminUsername()
		assert.Error(err)

		// not formatted as admin username
		j = AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "bobdole",
			},
			Actor: &Actor{
				ID:    "bobdole",
				Admin: true,
			},
		}
		_, err = j.AdminUsername()
		assert.Error(err)

		// not admin
		j = AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "admin/bobdole",
			},
			Actor: &Actor{
				ID:    "admin/bobdole",
				Admin: false,
			},
		}
		_, err = j.AdminUsername()
		assert.Error(err)

		// Blank object
		j = AuthProxyClaims{}
		_, err = j.AdminUsername()
		assert.Error(err)
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
						time.Date(1985, time.October, 26, 1, 22, 0, 0, time.UTC),
					},
				},
			}
			assert.True(t, tc.IsExpired(context.Background()), "no expiration specified should never be expired")
		})
	})
}
