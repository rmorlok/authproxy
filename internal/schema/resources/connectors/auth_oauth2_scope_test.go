package connectors

import (
	"github.com/rmorlok/authproxy/internal/schema/common"
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
			required, err := scope.IsRequired(nil)
			assert.NoError(err)
			assert.True(required)
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
			required, err := scope.IsRequired(nil)
			assert.NoError(err)
			assert.False(required)
		})
		t.Run("allowed to use dynamic required", func(t *testing.T) {
			data := `id: https://www.googleapis.com/auth/drive.activity.readonly
required:
  javascript: cfg.sync_activity === true
reason: We need to be able to see what's been going on in drive
`
			scope := &Scope{}
			err := yaml.Unmarshal([]byte(data), scope)
			assert.NoError(err)
			require.NotNil(t, scope.Required)
			require.NotNil(t, scope.Required.Predicate)
			assert.Equal("cfg.sync_activity === true", scope.Required.Predicate.Javascript)
			required, err := scope.IsRequired(map[string]any{
				"cfg": map[string]any{"sync_activity": true},
			})
			assert.NoError(err)
			assert.True(required)
			required, err = scope.IsRequired(map[string]any{
				"cfg": map[string]any{"sync_activity": false},
			})
			assert.NoError(err)
			assert.False(required)
		})
		t.Run("allowed to use conditional inclusion", func(t *testing.T) {
			data := `id: https://www.googleapis.com/auth/drive.readwrite
if:
  javascript: cfg.push_files === true
reason: We need to be able to write the files
`
			scope := &Scope{}
			err := yaml.Unmarshal([]byte(data), scope)
			assert.NoError(err)
			require.NotNil(t, scope.If)
			assert.Equal("cfg.push_files === true", scope.If.Javascript)
		})
	})

	t.Run("yaml gen", func(t *testing.T) {
		t.Run("defaults to required", func(t *testing.T) {
			data := &Scope{
				Id:       "https://www.googleapis.com/auth/drive.readonly",
				Required: NewScopeRequiredBool(false),
				Reason:   "We need to be able to view the files",
			}
			assert.Equal(`id: https://www.googleapis.com/auth/drive.readonly
required: false
reason: We need to be able to view the files
`, common.MustMarshalToYamlString(data))
		})
		t.Run("dynamic required", func(t *testing.T) {
			data := &Scope{
				Id: "https://www.googleapis.com/auth/drive.activity.readonly",
				Required: NewScopeRequiredPredicate(&common.Predicate{
					Javascript: "cfg.sync_activity === true",
				}),
				Reason: "We need to be able to see what's been going on in drive",
			}
			assert.Equal(`id: https://www.googleapis.com/auth/drive.activity.readonly
required:
    javascript: cfg.sync_activity === true
reason: We need to be able to see what's been going on in drive
`, common.MustMarshalToYamlString(data))
		})
	})

	t.Run("validate", func(t *testing.T) {
		t.Run("rejects blank if javascript", func(t *testing.T) {
			scope := &Scope{If: &common.Predicate{Javascript: " \n\t "}}
			err := scope.Validate(&common.ValidationContext{Path: "scope"})
			assert.Error(err)
			assert.Contains(err.Error(), "scope.if.javascript")
		})
		t.Run("rejects blank required javascript", func(t *testing.T) {
			scope := &Scope{
				Required: NewScopeRequiredPredicate(&common.Predicate{Javascript: " "}),
			}
			err := scope.Validate(&common.ValidationContext{Path: "scope"})
			assert.Error(err)
			assert.Contains(err.Error(), "scope.required.javascript")
		})
		t.Run("rejects empty required object", func(t *testing.T) {
			scope := &Scope{Required: &ScopeRequired{}}
			err := scope.Validate(&common.ValidationContext{Path: "scope"})
			assert.Error(err)
			assert.Contains(err.Error(), "scope.required")
		})
		t.Run("rejects required object with bool and predicate", func(t *testing.T) {
			required := false
			scope := &Scope{
				Required: &ScopeRequired{
					Bool:      &required,
					Predicate: &common.Predicate{Javascript: "true"},
				},
			}
			err := scope.Validate(&common.ValidationContext{Path: "scope"})
			assert.Error(err)
			assert.Contains(err.Error(), "scope.required")
		})
	})
}
