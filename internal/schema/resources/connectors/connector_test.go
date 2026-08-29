package connectors

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func testConnectorResource() Connector {
	refreshInBackground := true
	refreshDuration := common.HumanDuration{Duration: 5 * time.Minute}
	return Connector{
		TypeMeta: meta.NewTypeMeta(ConnectorKind),
		Metadata: meta.ObjectMeta{
			ID:         "cxr_test12345678abcd",
			Name:       "test-connector",
			Namespace:  "root",
			Generation: 2,
			Labels:     map[string]string{"type": "oauth2-type"},
			Annotations: map[string]string{
				"example.com/owner": "integrations",
			},
		},
		Spec: ConnectorSpec{
			Release: ConnectorReleaseSpec{DesiredState: ConnectorReleaseStateDraft},
			Definition: ConnectorDefinition{
				DisplayName: "OAuth2 Connector",
				Logo: common.NewBase64Image(common.ImageBase64{
					MimeType: "image/png",
					Base64:   "dGVzdCBiYXNlNjQgZGF0YQ==",
				}),
				Description: "OAuth2 description",
				Auth: &Auth{InnerVal: &AuthOAuth2{
					Type:   AuthTypeOAuth2,
					Scopes: []Scope{},
					ClientId: &common.StringValue{InnerVal: &common.StringValueDirect{
						Value: "client-id-value",
					}},
					ClientSecret: &common.StringValue{InnerVal: &common.StringValueDirect{
						Value: "client-secret-value",
					}},
					Authorization: AuthOauth2Authorization{Endpoint: "https://example.com/auth"},
					Token: AuthOauth2Token{
						Endpoint:                "https://example.com/token",
						RefreshTimeout:          &refreshDuration,
						RefreshInBackground:     &refreshInBackground,
						RefreshTimeBeforeExpiry: &refreshDuration,
					},
				}},
			},
		},
		Status: &ConnectorStatus{
			Release: ConnectorReleaseStatus{State: ConnectorReleaseStateDraft},
		},
	}
}

func TestConnectorRoundtrip(t *testing.T) {
	expected := testConnectorResource()

	t.Run("YAML", func(t *testing.T) {
		data, err := yaml.Marshal(expected)
		require.NoError(t, err)
		require.Contains(t, string(data), "apiVersion: authproxy.net/v1alpha1\nkind: Connector\nmetadata:")

		var actual Connector
		require.NoError(t, yaml.Unmarshal(data, &actual))
		require.Empty(t, cmp.Diff(expected, actual))
	})

	t.Run("JSON", func(t *testing.T) {
		data, err := json.Marshal(expected)
		require.NoError(t, err)

		var actual Connector
		require.NoError(t, json.Unmarshal(data, &actual))
		require.Empty(t, cmp.Diff(expected, actual))
	})
}

func TestConnectorDefinitionHashOnlyTracksDefinition(t *testing.T) {
	connector := testConnectorResource()
	before := connector.DefinitionHash()

	connector.Metadata.ID = "cxr_other12345678abc"
	connector.Metadata.Name = "renamed"
	connector.Metadata.Generation++
	connector.Metadata.Labels["env"] = "prod"
	connector.Metadata.Annotations["example.com/owner"] = "platform"
	connector.Spec.Release.DesiredState = ConnectorReleaseStatePrimary
	connector.Status.Release.State = ConnectorReleaseStatePrimary
	require.Equal(t, before, connector.DefinitionHash())

	connector.Spec.Definition.Description = "changed provider behavior"
	require.NotEqual(t, before, connector.DefinitionHash())
}

func TestConnectorValidation(t *testing.T) {
	connector := testConnectorResource()
	connector.Status = nil
	require.NoError(t, connector.Validate(nil))

	connector.Spec.Release.DesiredState = ConnectorReleaseStateArchived
	require.ErrorContains(t, connector.Validate(nil), "spec.release.desiredState")

	connector = testConnectorResource()
	require.ErrorContains(t, connector.Validate(nil), "status: is server-owned")
}
