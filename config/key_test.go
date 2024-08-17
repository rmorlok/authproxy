package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestKey(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("shared", func(t *testing.T) {
			data := `
shared_key:
  value: some-key-value
`
			key, err := UnmarshallYamlKeyString(data)
			assert.NoError(err)
			assert.Equal(&KeyShared{
				SharedKey: &KeyDataValue{
					Value: "some-key-value",
				},
			}, key)
		})
		t.Run("public", func(t *testing.T) {
			data := `
public_key:
  value: some-key-value
`
			key, err := UnmarshallYamlKeyString(data)
			assert.NoError(err)
			assert.Equal(&KeyPublicPrivate{
				PublicKey: &KeyDataValue{
					Value: "some-key-value",
				},
			}, key)
		})
		t.Run("private", func(t *testing.T) {
			data := `
private_key:
  value: some-key-value
`
			key, err := UnmarshallYamlKeyString(data)
			assert.NoError(err)
			assert.Equal(&KeyPublicPrivate{
				PrivateKey: &KeyDataValue{
					Value: "some-key-value",
				},
			}, key)
		})
		t.Run("public private", func(t *testing.T) {
			data := `
public_key:
  value: some-key-value-1
private_key:
  value: some-key-value-2
`
			key, err := UnmarshallYamlKeyString(data)
			assert.NoError(err)
			assert.Equal(&KeyPublicPrivate{
				PublicKey: &KeyDataValue{
					Value: "some-key-value-1",
				},
				PrivateKey: &KeyDataValue{
					Value: "some-key-value-2",
				},
			}, key)
		})
	})
}
