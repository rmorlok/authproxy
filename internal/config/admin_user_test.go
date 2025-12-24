package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAdminUser(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("with a public key", func(t *testing.T) {
			data := `
username: bobdole
key:
  public_key:
    value: some-key-value
`
			var au AdminUser
			err := yaml.Unmarshal([]byte(data), &au)
			assert.NoError(err)
			assert.Equal(AdminUser{
				Username: "bobdole",
				Key: &Key{
					InnerVal: &KeyPublicPrivate{
						PublicKey: &KeyData{
							InnerVal: &KeyDataValue{
								Value: "some-key-value",
							},
						},
					},
				},
			}, au)
		})

		t.Run("with permissions", func(t *testing.T) {
			data := `
username: bobdole
email: bob@example.com
permissions:
  - admin:read
  - admin:write
  - connectors:manage
key:
  public_key:
    value: some-key-value
`
			var au AdminUser
			err := yaml.Unmarshal([]byte(data), &au)
			assert.NoError(err)
			assert.Equal(AdminUser{
				Username: "bobdole",
				Email:    "bob@example.com",
				Permissions: []string{
					"admin:read",
					"admin:write",
					"connectors:manage",
				},
				Key: &Key{
					InnerVal: &KeyPublicPrivate{
						PublicKey: &KeyData{
							InnerVal: &KeyDataValue{
								Value: "some-key-value",
							},
						},
					},
				},
			}, au)
		})
	})
}
