package config

import (
	tu "github.com/rmorlok/authproxy/test_utils"
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
		assert.Equal(3, len(aues.All()))

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
}
