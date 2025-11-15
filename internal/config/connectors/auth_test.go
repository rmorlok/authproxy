package connectors

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/config/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAuth(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		tests := []struct {
			Name         string
			Test         AuthImpl
			ExpectedYaml string
			ExpectedJson string
		}{
			{
				Name: "oauth2",
				Test: &AuthOAuth2{
					Type: AuthTypeOAuth2,
					ClientId: &common.StringValue{InnerVal: &common.StringValueDirect{
						Value: "some-client-id",
					}},
					ClientSecret: &common.StringValue{InnerVal: &common.StringValueEnvVar{
						EnvVar: "GOOGLE_DRIVE_CLIENT_SECRET",
					}},
					Authorization: AuthOauth2Authorization{
						Endpoint: "https://example.com/authorization",
					},
					Token: AuthOauth2Token{
						Endpoint: "https://example.com/token",
					},
					Scopes: []Scope{
						{
							Id:       "https://www.googleapis.com/auth/drive.readonly",
							Required: nil,
							Reason:   "We need to be able to view the files\n",
						},
						{
							Id:       "https://www.googleapis.com/auth/drive.activity.readonly",
							Required: util.ToPtr(false),
							Reason:   "We need to be able to see what's been going on in drive\n",
						},
					},
				},
				ExpectedYaml: `
					type: OAuth2
					client_id:
						value: some-client-id
					client_secret:
						env_var: GOOGLE_DRIVE_CLIENT_SECRET
					scopes:
						- id: https://www.googleapis.com/auth/drive.readonly
						  reason: |
							We need to be able to view the files
						- id: https://www.googleapis.com/auth/drive.activity.readonly
						  required: false
						  reason: |
							We need to be able to see what's been going on in drive
					authorization:
						endpoint: https://example.com/authorization
					token:
						endpoint: https://example.com/token
`,
			},
			{
				Name: "api key",
				Test: &AuthOAuth2{
					Type: AuthTypeOAuth2,
					ClientId: &common.StringValue{InnerVal: &common.StringValueDirect{
						Value: "some-client-id",
					}},
					ClientSecret: &common.StringValue{InnerVal: &common.StringValueEnvVar{
						EnvVar: "GOOGLE_DRIVE_CLIENT_SECRET",
					}},
					Scopes: []Scope{
						{
							Id:       "https://www.googleapis.com/auth/drive.readonly",
							Required: util.ToPtr(true),
							Reason:   "We need to be able to view the files\n",
						},
						{
							Id:       "https://www.googleapis.com/auth/drive.activity.readonly",
							Required: util.ToPtr(false),
							Reason:   "We need to be able to see what's been going on in drive\n",
						},
					},
					Authorization: AuthOauth2Authorization{
						Endpoint: "https://example.com/authorization",
					},

					Token: AuthOauth2Token{
						Endpoint: "https://example.com/token",
					},
				},
				ExpectedYaml: `
					type: OAuth2
					client_id:
						value: some-client-id
					client_secret:
						env_var: GOOGLE_DRIVE_CLIENT_SECRET
					scopes:
						- id: https://www.googleapis.com/auth/drive.readonly
						  required: true
						  reason: |
							We need to be able to view the files
						- id: https://www.googleapis.com/auth/drive.activity.readonly
						  required: false
						  reason: |
							We need to be able to see what's been going on in drive
					authorization:
						endpoint: https://example.com/authorization
					token:
						endpoint: https://example.com/token`,
			},
			{
				Name: "no auth",
				Test: &AuthNoAuth{
					Type: AuthTypeNoAuth,
				},
			},
		}

		for _, test := range tests {
			t.Run(test.Name, func(t *testing.T) {
				t.Run("json", func(t *testing.T) {
					wrapper := &Auth{InnerVal: test.Test}
					data, err := json.Marshal(wrapper)
					require.NoError(t, err)
					if test.ExpectedJson != "" {
						require.Equal(
							t,
							util.TabsToSpaces(util.Deindent(test.ExpectedJson), 4),
							util.TabsToSpaces(util.Deindent(string(data)), 4),
						)
					}
					back := &Auth{}
					err = json.Unmarshal(data, back)
					require.NoError(t, err)
					require.Equal(t, test.Test, back.Inner())
				})
				t.Run("yaml", func(t *testing.T) {
					wrapper := &Auth{InnerVal: test.Test}
					data, err := yaml.Marshal(wrapper)
					require.NoError(t, err)
					if test.ExpectedYaml != "" {
						assert.Equal(
							t,
							util.TabsToSpaces(util.Deindent(test.ExpectedYaml), 4),
							util.TabsToSpaces(util.Deindent(string(data)), 4),
						)
					}
					back := &Auth{}
					err = yaml.Unmarshal(data, back)
					require.NoError(t, err)
					require.Equal(t, test.Test, back.Inner())
				})
			})
		}
	})
}
