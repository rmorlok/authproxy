package connectors

import (
	"github.com/rmorlok/authproxy/config/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAuth(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("oauth2", func(t *testing.T) {
			data := `
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
`
			auth, err := UnmarshallYamlAuthString(data)
			assert.NoError(err)
			assert.Equal(&AuthOAuth2{
				Type: AuthTypeOAuth2,
				ClientId: &common.StringValueDirect{
					Value: "some-client-id",
				},
				ClientSecret: &common.StringValueEnvVar{
					EnvVar: "GOOGLE_DRIVE_CLIENT_SECRET",
				},
				Scopes: []Scope{
					{
						Id:       "https://www.googleapis.com/auth/drive.readonly",
						Required: true,
						Reason:   "We need to be able to view the files\n",
					},
					{
						Id:       "https://www.googleapis.com/auth/drive.activity.readonly",
						Required: false,
						Reason:   "We need to be able to see what's been going on in drive\n",
					},
				},
			}, auth)
		})
		t.Run("api key", func(t *testing.T) {
			data := `
type: api-key
`
			auth, err := UnmarshallYamlAuthString(data)
			assert.NoError(err)
			assert.Equal(&AuthApiKey{
				Type: AuthTypeAPIKey,
			}, auth)
		})
	})

	t.Run("yaml gen", func(t *testing.T) {
		t.Run("oauth2", func(t *testing.T) {
			data := &AuthOAuth2{
				Type: AuthTypeOAuth2,
				ClientId: &common.StringValueDirect{
					Value: "some-client-id",
				},
				ClientSecret: &common.StringValueEnvVar{
					EnvVar: "GOOGLE_DRIVE_CLIENT_SECRET",
				},
				Scopes: []Scope{
					{
						Id:       "https://www.googleapis.com/auth/drive.readonly",
						Required: true,
						Reason:   "We need to be able to view the files\n",
					},
					{
						Id:       "https://www.googleapis.com/auth/drive.activity.readonly",
						Required: false,
						Reason:   "We need to be able to see what's been going on in drive\n",
					},
				},
				Authorization: AuthOauth2Authorization{
					Endpoint: "https://example.com/authorization",
				},

				Token: AuthOauth2Token{
					Endpoint: "https://example.com/token",
				},
			}
			assert.Equal(`type: OAuth2
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
    endpoint: https://example.com/token
`, common.MustMarshalToYamlString(data))
		})
		t.Run("api key", func(t *testing.T) {
			data := &AuthApiKey{
				Type: AuthTypeAPIKey,
			}
			assert.Equal(`type: api-key
`, common.MustMarshalToYamlString(data))
		})
	})
}
