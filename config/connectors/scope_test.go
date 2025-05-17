package connectors

import (
	"github.com/rmorlok/authproxy/config/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestScope(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("defaults to required", func(t *testing.T) {
			data := `
id: https://www.googleapis.com/auth/drive.readonly
reason: |
  We need to be able to view the files
`
			scope, err := UnmarshallYamlScopeString(data)
			assert.NoError(err)
			assert.Equal("https://www.googleapis.com/auth/drive.readonly", scope.Id)
			assert.Equal("We need to be able to view the files\n", scope.Reason)
			assert.True(scope.Required)
		})
		t.Run("allowed to be not required", func(t *testing.T) {
			data := `id: https://www.googleapis.com/auth/drive.readonly
required: false
reason: We need to be able to view the files
`
			scope, err := UnmarshallYamlScopeString(data)
			assert.NoError(err)
			assert.Equal("https://www.googleapis.com/auth/drive.readonly", scope.Id)
			assert.Equal("We need to be able to view the files", scope.Reason)
			assert.False(scope.Required)
		})
	})

	t.Run("yaml gen", func(t *testing.T) {
		t.Run("defaults to required", func(t *testing.T) {
			data := &Scope{
				Id:       "https://www.googleapis.com/auth/drive.readonly",
				Required: false,
				Reason:   "We need to be able to view the files",
			}
			assert.Equal(`id: https://www.googleapis.com/auth/drive.readonly
required: false
reason: We need to be able to view the files
`, common.MustMarshalToYamlString(data))
		})
	})
}
