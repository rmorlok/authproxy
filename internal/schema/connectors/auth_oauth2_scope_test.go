package connectors

import (
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

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
			scope := &Scope{}
			err := yaml.Unmarshal([]byte(data), scope)
			assert.NoError(err)
			assert.Equal("https://www.googleapis.com/auth/drive.readonly", scope.Id)
			assert.Equal("We need to be able to view the files\n", scope.Reason)
			assert.True(scope.IsRequired())
		})
		t.Run("allowed to be not required", func(t *testing.T) {
			data := `id: https://www.googleapis.com/auth/drive.readonly
required: false
reason: We need to be able to view the files
`
			scope := &Scope{}
			err := yaml.Unmarshal([]byte(data), scope)
			assert.NoError(err)
			assert.Equal("https://www.googleapis.com/auth/drive.readonly", scope.Id)
			assert.Equal("We need to be able to view the files", scope.Reason)
			assert.False(scope.IsRequired())
		})
	})

	t.Run("yaml gen", func(t *testing.T) {
		t.Run("defaults to required", func(t *testing.T) {
			data := &Scope{
				Id:       "https://www.googleapis.com/auth/drive.readonly",
				Required: util.ToPtr(false),
				Reason:   "We need to be able to view the files",
			}
			assert.Equal(`id: https://www.googleapis.com/auth/drive.readonly
required: false
reason: We need to be able to view the files
`, common.MustMarshalToYamlString(data))
		})
	})
}
