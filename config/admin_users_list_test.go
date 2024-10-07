package config

import (
	"github.com/stretchr/testify/require"
	"testing"
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
}
