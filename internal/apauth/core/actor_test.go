package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActor(t *testing.T) {
	t.Parallel()
	t.Run("IsAdmin", func(t *testing.T) {
		u := Actor{}
		assert.False(t, u.IsAdmin())
		u.Admin = true
		assert.True(t, u.IsAdmin())
		u.Admin = false
		assert.False(t, u.IsAdmin())

		var nila *Actor
		assert.False(t, nila.IsAdmin())
	})
	t.Run("IsSuperAdmin", func(t *testing.T) {
		u := Actor{}
		assert.False(t, u.IsSuperAdmin())
		u.SuperAdmin = true
		assert.True(t, u.IsSuperAdmin())
		u.SuperAdmin = false
		assert.False(t, u.IsSuperAdmin())

		var nila *Actor
		assert.False(t, nila.IsSuperAdmin())
	})
	t.Run("IsNormalActor", func(t *testing.T) {
		u := Actor{}
		assert.True(t, u.IsNormalActor())
		u.SuperAdmin = true
		assert.False(t, u.IsNormalActor())
		u.SuperAdmin = false
		u.Admin = true
		assert.False(t, u.IsNormalActor())
		u.SuperAdmin = false
		u.Admin = false
		assert.True(t, u.IsNormalActor())

		var nila *Actor
		assert.True(t, nila.IsNormalActor())
	})
}
