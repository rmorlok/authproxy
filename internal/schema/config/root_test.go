package config

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRootFullConfig(t *testing.T) {
	data := `
connectors:
  loadFromList:
    - apiVersion: authproxy.net/v1alpha1
      kind: Connector
      metadata:
        name: google-drive
        namespace: root
        generation: 1
        labels:
          type: google-drive
      spec:
        release:
          desiredState: primary
        definition:
          displayName: Google Drive
          logo:
            publicUrl: https://example.com/google-drive.png
          description: Connect Google Drive
          auth:
            type: no-auth
`

	var root Root
	require.NoError(t, yaml.Unmarshal([]byte(data), &root))
	require.Equal(t, &Root{
		Connectors: connectors.FromList([]Connector{{
			TypeMeta: meta.NewTypeMeta(connectors.ConnectorKind),
			Metadata: meta.ObjectMeta{
				Name:       "google-drive",
				Namespace:  "root",
				Generation: 1,
				Labels:     map[string]string{"type": "google-drive"},
			},
			Spec: connectors.ConnectorSpec{
				Release: connectors.ConnectorReleaseSpec{DesiredState: connectors.ConnectorReleaseStatePrimary},
				Definition: connectors.ConnectorDefinition{
					DisplayName: "Google Drive",
					Logo:        &Image{InnerVal: &ImagePublicUrl{PublicUrl: "https://example.com/google-drive.png"}},
					Description: "Connect Google Drive",
					Auth:        &Auth{InnerVal: &AuthNoAuth{Type: connectors.AuthTypeNoAuth}},
				},
			},
		}}),
	}, &root)
}

func TestRootRejectsLegacySnakeCaseKeys(t *testing.T) {
	var root Root
	require.Error(t, util.DecodeYAMLStrict([]byte("system_auth: {}\n"), &root))
}
