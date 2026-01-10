package config

import (
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	tu "github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"testing"
)

func TestAdminUsersExternalSource(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("standard path", func(t *testing.T) {
			data := `
keys_path: some/path/to/keys
`
			var adminUsersExternalSource AdminUsersExternalSource
			err := yaml.Unmarshal([]byte(data), &adminUsersExternalSource)
			assert.NoError(err)
			assert.Equal("some/path/to/keys", adminUsersExternalSource.KeysPath)
		})
	})
	t.Run("loads users from path", func(t *testing.T) {
		aues := AdminUsersExternalSource{
			KeysPath: tu.TestDataPath("admin_user_keys"),
			Permissions: []aschema.Permission{
				{
					Namespace: "root",
					Resources: []string{"connectors"},
					Verbs:     []string{"list"},
				},
				{
					Namespace: "root",
					Resources: []string{"connections"},
					Verbs:     []string{"list", "get", "disconnect"},
				},
			},
		}

		// Check the test_data/admin_user_keys folder to see what this count should be
		assert.Equal(8, len(aues.All()))

		u, found := aues.GetByUsername("bobdole")
		assert.True(found)
		assert.NotNil(u)
		assert.True(u.Key.CanVerifySignature())
		assert.Equal([]aschema.Permission{
			{
				Namespace: "root",
				Resources: []string{"connectors"},
				Verbs:     []string{"list"},
			},
			{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"list", "get", "disconnect"},
			},
		}, u.Permissions)

		u, found = aues.GetByUsername("bobdole")
		assert.True(found)
		assert.NotNil(u)
		assert.True(u.Key.CanVerifySignature())
		assert.Equal([]aschema.Permission{
			{
				Namespace: "root",
				Resources: []string{"connectors"},
				Verbs:     []string{"list"},
			},
			{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"list", "get", "disconnect"},
			},
		}, u.Permissions)

		u, found = aues.GetByUsername("bobdole")
		assert.True(found)
		assert.NotNil(u)
		assert.True(u.Key.CanVerifySignature())
		assert.Equal([]aschema.Permission{
			{
				Namespace: "root",
				Resources: []string{"connectors"},
				Verbs:     []string{"list"},
			},
			{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"list", "get", "disconnect"},
			},
		}, u.Permissions)
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
