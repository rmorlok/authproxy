package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestKey(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("shared", func(t *testing.T) {
			data := `
shared_key:
  value: some-key-value
`
			var key Key
			err := yaml.Unmarshal([]byte(data), &key)
			assert.NoError(err)
			assert.Equal(Key{
				InnerVal: &KeyShared{
					SharedKey: &KeyData{
						InnerVal: &KeyDataValue{
							Value: "some-key-value",
						},
					},
				},
			}, key)
		})
		t.Run("public", func(t *testing.T) {
			data := `
public_key:
  value: some-key-value
`
			var key Key
			err := yaml.Unmarshal([]byte(data), &key)
			assert.NoError(err)
			assert.Equal(Key{
				InnerVal: &KeyPublicPrivate{
					PublicKey: &KeyData{
						InnerVal: &KeyDataValue{
							Value: "some-key-value",
						},
					},
				},
			}, key)
		})
		t.Run("private", func(t *testing.T) {
			data := `
private_key:
  value: some-key-value
`
			var key Key
			err := yaml.Unmarshal([]byte(data), &key)
			assert.NoError(err)
			assert.Equal(Key{
				InnerVal: &KeyPublicPrivate{
					PrivateKey: &KeyData{
						InnerVal: &KeyDataValue{
							Value: "some-key-value",
						},
					},
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
			var key Key
			err := yaml.Unmarshal([]byte(data), &key)
			assert.NoError(err)
			assert.Equal(Key{
				InnerVal: &KeyPublicPrivate{
					PublicKey: &KeyData{
						InnerVal: &KeyDataValue{
							Value: "some-key-value-1",
						},
					},
					PrivateKey: &KeyData{
						InnerVal: &KeyDataValue{
							Value: "some-key-value-2",
						},
					},
				},
			}, key)
		})
	})
}
