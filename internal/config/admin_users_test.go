package config

import (
	"encoding/json"
	"fmt"

	tu "github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"testing"
)

func TestAdminUsers(t *testing.T) {
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
			var adminUsers AdminUsers
			err := yaml.Unmarshal([]byte(data), &adminUsers)
			assert.NoError(err)

			assert.Equal(2, len(adminUsers.All()))
			assert.NotNil(adminUsers.GetByUsername("georgebush"))
			assert.NotNil(adminUsers.GetByUsername("bobdole"))
		})
		t.Run("external source", func(t *testing.T) {
			data := fmt.Sprintf(`
keys_path: %s
`, tu.TestDataPath("admin_user_keys"))
			var adminUsers AdminUsers
			err := yaml.Unmarshal([]byte(data), &adminUsers)
			assert.NoError(err)
			assert.NotNil(adminUsers.GetByUsername("bobdole"))
		})
	})
	t.Run("json parse", func(t *testing.T) {
		t.Run("list of users", func(t *testing.T) {
			data := `[
  {
    "username": "georgebush",
    "key": {
      "public_key": {
        "value": "some-key-value"
      }
    }
  },
  {
    "username": "bobdole",
    "key": {
      "public_key": {
        "value": "some-key-value"
      }
    }
  }
]`
			var adminUsers AdminUsers
			err := json.Unmarshal([]byte(data), &adminUsers)
			assert.NoError(err)

			assert.Equal(2, len(adminUsers.All()))
			assert.NotNil(adminUsers.GetByUsername("georgebush"))
			assert.NotNil(adminUsers.GetByUsername("bobdole"))
		})
		t.Run("external source", func(t *testing.T) {
			data := fmt.Sprintf(`{
"keys_path": %s
}`, tu.TestDataPath("admin_user_keys"))
			var adminUsers AdminUsers
			err := json.Unmarshal([]byte(data), &adminUsers)
			assert.NoError(err)
			assert.NotNil(adminUsers.GetByUsername("bobdole"))
		})
	})
}
