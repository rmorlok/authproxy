package config

import (
	"github.com/stretchr/testify/require"
	"testing"
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
			au, err := UnmarshallYamlAdminUserString(data)
			assert.NoError(err)
			assert.Equal(&AdminUser{
				Username: "bobdole",
				Key: &KeyPublicPrivate{
					PublicKey: &KeyDataValue{
						Value: "some-key-value",
					},
				},
			}, au)
		})
	})
}
