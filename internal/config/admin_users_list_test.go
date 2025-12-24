package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminUsersList(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("list of users", func(t *testing.T) {
			data := `
- username: georgebush
  key:
    public_key:
      value: some-key-value
- username: bobdole
  key:
    public_key:
      value: some-key-value
`
			adminUsers, err := UnmarshallYamlAdminUsersListString(data)
			assert.NoError(err)
			assert.Equal(2, len(adminUsers))
			assert.Equal("georgebush", adminUsers[0].Username)
			assert.Equal("bobdole", adminUsers[1].Username)
		})
	})
	t.Run("get by username", func(t *testing.T) {
		adminUsers := AdminUsersList{
			&AdminUser{
				Username: "bobdole",
				Key: &KeyPublicPrivate{
					PublicKey: &KeyData{
						InnerVal: &KeyDataFile{
							Path: "../../test_data/admin_user_keys/bobdole.pub",
						},
					},
				},
			},
		}

		u, found := adminUsers.GetByUsername("bobdole")
		assert.True(found)
		assert.NotNil(u)
		assert.True(u.Key.CanVerifySignature())

		u, found = adminUsers.GetByUsername("billclinton")
		assert.False(found)
		assert.Nil(u)
	})
	t.Run("get by jwt subject", func(t *testing.T) {
		adminUsers := AdminUsersList{
			&AdminUser{
				Username: "bobdole",
				Key: &KeyPublicPrivate{
					PublicKey: &KeyData{
						InnerVal: &KeyDataFile{
							Path: "../../test_data/admin_user_keys/bobdole.pub",
						},
					},
				},
			},
		}

		u, found := adminUsers.GetByJwtSubject("admin/bobdole")
		assert.True(found)
		assert.NotNil(u)
		assert.True(u.Key.CanVerifySignature())

		u, found = adminUsers.GetByJwtSubject("bobdole")
		assert.False(found)
		assert.Nil(u)

		u, found = adminUsers.GetByJwtSubject("admin/billclinton")
		assert.False(found)
		assert.Nil(u)
	})
}
