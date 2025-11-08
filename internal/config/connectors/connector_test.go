package connectors

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/rmorlok/authproxy/internal/config/common"
)

func TestConnectorRoundtrip(t *testing.T) {
	// Create test UUID for consistent testing
	testID := uuid.MustParse("12345678-1234-1234-1234-123456789012")
	refreshInBackground := true
	refreshDuration := common.HumanDuration{Duration: 5 * time.Minute}

	testCases := []struct {
		name      string
		connector Connector
	}{
		{
			name: "Basic Connector with API Key Auth",
			connector: Connector{
				Id:          testID,
				Type:        "test-type",
				Version:     1,
				State:       "primary",
				DisplayName: "Test Connector",
				Logo: common.NewPublicUrlImage(common.ImagePublicUrl{
					PublicUrl: "https://example.com/logo.png",
				}),
				Description: "Test description",
				Auth: &AuthApiKey{
					Type: AuthTypeAPIKey,
				},
			},
		},
		{
			name: "Connector with OAuth2 Auth and Direct String Values",
			connector: Connector{
				Id:          testID,
				Type:        "oauth2-type",
				Version:     2,
				State:       "draft",
				DisplayName: "OAuth2 Connector",
				Logo: common.NewBase64Image(common.ImageBase64{
					MimeType: "image/png",
					Base64:   "dGVzdCBiYXNlNjQgZGF0YQ==", // "test base64 data"
				}),
				Description: "OAuth2 description",
				Auth: &AuthOAuth2{
					Type: AuthTypeOAuth2,
					ClientId: &common.StringValue{InnerVal: &common.StringValueDirect{
						Value: "client-id-value",
					}},
					ClientSecret: &common.StringValue{InnerVal: &common.StringValueDirect{
						Value: "client-secret-value",
					}},
					Scopes: []Scope{
						{
							Id:       "scope1",
							Required: util.ToPtr(true),
							Reason:   "Required for basic functionality",
						},
						{
							Id:       "scope2",
							Required: util.ToPtr(false),
							Reason:   "Optional for advanced features",
						},
					},
					Authorization: AuthOauth2Authorization{
						Endpoint: "https://example.com/auth",
						QueryOverrides: map[string]string{
							"key1": "value1",
							"key2": "value2",
						},
					},
					Token: AuthOauth2Token{
						Endpoint: "https://example.com/token",
						QueryOverrides: map[string]string{
							"token_key": "token_value",
						},
						FormOverrides: map[string]string{
							"form_key": "form_value",
						},
						RefreshTimeout:          &refreshDuration,
						RefreshInBackground:     &refreshInBackground,
						RefreshTimeBeforeExpiry: &refreshDuration,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name+" YAML", func(t *testing.T) {
			// Marshal to YAML
			yamlData, err := yaml.Marshal(tc.connector)
			require.NoError(t, err, "Failed to marshal connector to YAML")

			// Unmarshal from YAML
			var unmarshaledConnector Connector
			err = yaml.Unmarshal(yamlData, &unmarshaledConnector)
			require.NoError(t, err, "Failed to unmarshal connector from YAML")

			// Compare original and unmarshaled connectors
			diff := cmp.Diff(tc.connector, unmarshaledConnector)
			assert.True(t, cmp.Equal(tc.connector, unmarshaledConnector), "Connector diff: %s", diff)
		})

		t.Run(tc.name+" JSON", func(t *testing.T) {
			// Marshal to JSON
			jsonData, err := json.Marshal(tc.connector)
			require.NoError(t, err, "Failed to marshal connector to JSON")

			// Unmarshal from JSON
			var unmarshaledConnector Connector
			err = json.Unmarshal(jsonData, &unmarshaledConnector)
			require.NoError(t, err, "Failed to unmarshal connector from JSON")

			// Compare original and unmarshaled connectors
			diff := cmp.Diff(tc.connector, unmarshaledConnector)
			assert.True(t, cmp.Equal(tc.connector, unmarshaledConnector), "Connector diff: %s", diff)
		})
	}
}
