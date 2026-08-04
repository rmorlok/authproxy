package key

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
sharedKey:
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
publicKey:
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
privateKey:
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
publicKey:
  value: some-key-value-1
privateKey:
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
