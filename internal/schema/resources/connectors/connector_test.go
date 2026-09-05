package connectors

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/rmorlok/authproxy/internal/apid"
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

func TestConnectorValidationModesAndCreateDefaults(t *testing.T) {
	create := testConnectorResource()
	create.Metadata.ID = ""
	create.Metadata.Name = ""
	create.Metadata.Generation = 0
	create.Status = nil
	create.Spec.Release.DesiredState = ""
	require.NoError(t, create.ValidateFor(meta.ValidationModeCreate, nil))

	id := apid.MustParse("cxr_testcreate0000001")
	normalized := create.ApplyAPICreateDefaults(id)
	require.Equal(t, id, normalized.GetId())
	require.Equal(t, common.ResourceName(id.String()), normalized.Metadata.Name)
	require.Equal(t, uint64(1), normalized.Metadata.Generation)
	require.Equal(t, ConnectorReleaseStateDraft, normalized.Spec.Release.DesiredState)
	require.Empty(t, create.Metadata.ID, "defaulting must not mutate the request")

	normalized.Status = &ConnectorStatus{Release: ConnectorReleaseStatus{State: ConnectorReleaseStateDraft}}
	require.NoError(t, normalized.ValidateFor(meta.ValidationModeResponse, nil))

	normalized.Status.Release.State = ConnectorReleaseStatePrimary
	require.ErrorContains(t, normalized.ValidateFor(meta.ValidationModeResponse, nil), "must be draft")

	normalized.Spec.Release.DesiredState = ConnectorReleaseStatePrimary
	require.NoError(t, normalized.ValidateFor(meta.ValidationModeResponse, nil))
	normalized.Status.Release.State = ConnectorReleaseStateArchived
	require.NoError(t, normalized.ValidateFor(meta.ValidationModeResponse, nil))
}

func TestConnectorDesiredReleaseStateForObserved(t *testing.T) {
	require.Equal(t, ConnectorReleaseStateDraft, DesiredReleaseStateForObserved(ConnectorReleaseStateDraft))
	for _, state := range []ConnectorReleaseState{
		ConnectorReleaseStatePrimary,
		ConnectorReleaseStateActive,
		ConnectorReleaseStateArchived,
	} {
		require.Equal(t, ConnectorReleaseStatePrimary, DesiredReleaseStateForObserved(state))
	}
}

func TestConnectorPatchRoundtripAndApply(t *testing.T) {
	current := testConnectorResource()
	labels := map[string]string{"environment": "staging"}
	annotations := map[string]string{}
	name := common.ResourceName("renamed-connector")
	primary := ConnectorReleaseStatePrimary
	definition := current.Spec.Definition
	definition.Description = "updated"

	patch := NewConnectorPatch()
	patch.Metadata.Name = &name
	patch.Metadata.Labels = &labels
	patch.Metadata.Annotations = &annotations
	patch.Spec.Release = &ConnectorReleaseSpecPatch{DesiredState: &primary}
	patch.Spec.Definition = &definition

	data, err := json.Marshal(patch)
	require.NoError(t, err)
	require.JSONEq(t, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"Connector",
      "metadata":{
        "name":"renamed-connector",
        "labels":{"environment":"staging"},
        "annotations":{}
      },
      "spec":{
        "release":{"desiredState":"primary"},
        "definition":`+mustJSON(t, definition)+`
      }
    }`, string(data))

	var decoded ConnectorPatch
	require.NoError(t, json.Unmarshal(data, &decoded))
	updated, err := decoded.ApplyTo(&current, nil)
	require.NoError(t, err)
	require.Equal(t, name, updated.Metadata.Name)
	require.Equal(t, labels, updated.Metadata.Labels)
	require.Empty(t, updated.Metadata.Annotations)
	require.NotNil(t, updated.Metadata.Annotations)
	require.Equal(t, primary, updated.Spec.Release.DesiredState)
	require.Equal(t, "updated", updated.Spec.Definition.Description)
	require.Equal(t, current.Metadata.ID, updated.Metadata.ID)
	require.Equal(t, current.Metadata.Generation, updated.Metadata.Generation)
}

func TestConnectorPatchRejectsNullAndImmutableFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
		yaml string
		want string
	}{
		{
			name: "definition",
			json: `{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","metadata":{},"spec":{"definition":null}}`,
			yaml: "apiVersion: authproxy.net/v1alpha1\nkind: Connector\nmetadata: {}\nspec:\n  definition: null\n",
			want: "spec.definition: must not be null",
		},
		{
			name: "release",
			json: `{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","metadata":{},"spec":{"release":null}}`,
			yaml: "apiVersion: authproxy.net/v1alpha1\nkind: Connector\nmetadata: {}\nspec:\n  release: null\n",
			want: "spec.release: must not be null",
		},
		{
			name: "desired state",
			json: `{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","metadata":{},"spec":{"release":{"desiredState":null}}}`,
			yaml: "apiVersion: authproxy.net/v1alpha1\nkind: Connector\nmetadata: {}\nspec:\n  release:\n    desiredState: null\n",
			want: "spec.release.desiredState: must not be null",
		},
	} {
		t.Run(tc.name+" JSON", func(t *testing.T) {
			var patch ConnectorPatch
			require.NoError(t, json.Unmarshal([]byte(tc.json), &patch))
			require.ErrorContains(t, patch.ValidateFor(meta.ValidationModeUpdate, nil), tc.want)
		})
		t.Run(tc.name+" YAML", func(t *testing.T) {
			var patch ConnectorPatch
			require.NoError(t, yaml.Unmarshal([]byte(tc.yaml), &patch))
			require.ErrorContains(t, patch.ValidateFor(meta.ValidationModeUpdate, nil), tc.want)
		})
	}

	current := testConnectorResource()
	patch := NewConnectorPatch()
	generation := current.Metadata.Generation + 1
	patch.Metadata.Generation = &generation
	_, err := patch.ApplyTo(&current, nil)
	require.ErrorContains(t, err, "metadata.generation: is immutable")

	namespace := "root.other"
	patch = NewConnectorPatch()
	patch.Metadata.Namespace = &namespace
	_, err = patch.ApplyTo(&current, nil)
	require.ErrorContains(t, err, "metadata.namespace: is immutable")
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
