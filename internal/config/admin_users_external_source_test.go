package config

import (
	tu "github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAdminUsersExternalSource(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("standard path", func(t *testing.T) {
			data := `
keys_path: some/path/to/keys
`
			adminUsersExternalSource, err := UnmarshallYamlAdminUsersExternalSourceString(data)
			assert.NoError(err)
			assert.Equal("some/path/to/keys", adminUsersExternalSource.KeysPath)
		})
	})
	t.Run("loads users from path", func(t *testing.T) {
		aues := AdminUsersExternalSource{
			KeysPath: tu.TestDataPath("admin_user_keys"),
		}

		// Check the test_data/admin_user_keys folder to see what this count should be
		assert.Equal(8, len(aues.All()))

		u, found := aues.GetByUsername("bobdole")
		assert.True(found)
		assert.NotNil(u)
		assert.True(u.Key.CanVerifySignature())

		u, found = aues.GetByUsername("bobdole")
		assert.True(found)
		assert.NotNil(u)
		assert.True(u.Key.CanVerifySignature())

		u, found = aues.GetByUsername("bobdole")
		assert.True(found)
		assert.NotNil(u)
		assert.True(u.Key.CanVerifySignature())
	})
	t.Run("get by jwt subject", func(t *testing.T) {
		aues := AdminUsersExternalSource{
			KeysPath: tu.TestDataPath("admin_user_keys"),
		}

		// Check the test_data/admin_user_keys folder to see what this count should be
		assert.Equal(8, len(aues.All()))

		u, found := aues.GetByJwtSubject("admin/bobdole")
		assert.True(found)
		assert.NotNil(u)

		u, found = aues.GetByJwtSubject("bobdole")
		assert.False(found)
		assert.Nil(u)

		u, found = aues.GetByJwtSubject("andrewjackson")
		assert.False(found)
		assert.Nil(u)
	})
}
