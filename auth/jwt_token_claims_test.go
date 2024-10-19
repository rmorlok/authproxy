package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestJwtTokenClaims(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		assert.NotPanics(t, func() {
			var tc *JwtTokenClaims
			tc.String()
		}, "it doesn't panic on a nil value")
	})
	t.Run("IsAdmin", func(t *testing.T) {
		var tc *JwtTokenClaims
		assert.False(t, tc.IsAdmin(), "nil values aren't admins")
	})
	t.Run("IsSuperAdmin", func(t *testing.T) {
		var tc *JwtTokenClaims
		assert.False(t, tc.IsAdmin(), "nil values aren't super admins")
	})
	t.Run("IsNormalActor", func(t *testing.T) {
		var tc *JwtTokenClaims
		assert.True(t, tc.IsNormalActor(), "nil values are normal actors")
	})
	t.Run("AdminUsername", func(t *testing.T) {
		assert := require.New(t)

		// valid
		j := JwtTokenClaims{
			jwt.RegisteredClaims{
				Subject: "admin/bobdole",
			},
			&Actor{
				ID:    "admin/bobdole",
				Admin: true,
			},
			false,
		}
		username, err := j.AdminUsername()
		assert.NoError(err)
		assert.Equal("bobdole", username)

		// No actor id
		j = JwtTokenClaims{
			jwt.RegisteredClaims{
				Subject: "admin/bobdole",
			},
			&Actor{
				Admin: true,
			},
			false,
		}
		_, err = j.AdminUsername()
		assert.Error(err)

		// No subject
		j = JwtTokenClaims{
			jwt.RegisteredClaims{},
			&Actor{
				ID:    "admin/bobdole",
				Admin: true,
			},
			false,
		}
		_, err = j.AdminUsername()
		assert.Error(err)

		// usernames don't match
		j = JwtTokenClaims{
			jwt.RegisteredClaims{
				Subject: "admin/bobsmith",
			},
			&Actor{
				ID:    "admin/bobdole",
				Admin: true,
			},
			false,
		}
		_, err = j.AdminUsername()
		assert.Error(err)

		// not formatted as admin username
		j = JwtTokenClaims{
			jwt.RegisteredClaims{
				Subject: "bobdole",
			},
			&Actor{
				ID:    "bobdole",
				Admin: true,
			},
			false,
		}
		_, err = j.AdminUsername()
		assert.Error(err)

		// not admin
		j = JwtTokenClaims{
			jwt.RegisteredClaims{
				Subject: "admin/bobdole",
			},
			&Actor{
				ID:    "admin/bobdole",
				Admin: false,
			},
			false,
		}
		_, err = j.AdminUsername()
		assert.Error(err)

		// Blank object
		j = JwtTokenClaims{}
		_, err = j.AdminUsername()
		assert.Error(err)
	})
}
