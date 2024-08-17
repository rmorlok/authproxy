package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSecret(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("value", func(t *testing.T) {
			data := `
value: some-client-id
`
			image, err := UnmarshallYamlSecretString(data)
			assert.NoError(err)
			assert.Equal(&SecretValue{
				Value: "some-client-id",
			}, image)
		})
		t.Run("base64", func(t *testing.T) {
			data := `
base64: iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=`
			image, err := UnmarshallYamlSecretString(data)
			assert.NoError(err)
			assert.Equal(&SecretBase64Val{
				Base64: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/wcAAwAB/1J8qkwAAAAASUVORK5CYII=",
			}, image)
		})
		t.Run("env var", func(t *testing.T) {
			data := `
env_var: MY_SECRET_ENV`
			image, err := UnmarshallYamlSecretString(data)
			assert.NoError(err)
			assert.Equal(&SecretEnvVar{
				EnvVar: "MY_SECRET_ENV",
			}, image)
		})
		t.Run("file", func(t *testing.T) {
			data := `
path: /foo/bar/baz`
			image, err := UnmarshallYamlSecretString(data)
			assert.NoError(err)
			assert.Equal(&SecretFile{
				Path: "/foo/bar/baz",
			}, image)
		})
	})
}
