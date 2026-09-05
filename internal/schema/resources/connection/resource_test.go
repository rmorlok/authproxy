package connection

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/schema/common"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func storedConnectionResource() *Connection {
	now := time.Now().UTC()
	return &Connection{
		TypeMeta: meta.NewTypeMeta(ConnectionKind),
		Metadata: meta.ObjectMeta{
			ID:          apid.New(apid.PrefixConnection).String(),
			Name:        "production",
			Namespace:   "root.acme",
			Labels:      map[string]string{"team": "platform"},
			Annotations: map[string]string{"owner": "integrations"},
			CreatedAt:   &now,
			UpdatedAt:   &now,
		},
		Spec: ConnectionSpec{
			ConnectorRef: meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       connectorschema.ConnectorKind,
				ID:         apid.New(apid.PrefixConnector).String(),
				Generation: 2,
			},
			Configuration: map[string]any{
				"tenant":  "acme",
				"options": map[string]any{"region": "us"},
			},
			ConfigurationSchema: common.RawJSON(`{
				"$schema":"https://json-schema.org/draft/2020-12/schema",
				"type":"object",
				"properties":{
					"tenant":{"type":"string"},
					"options":{"type":"object"}
				},
				"additionalProperties":true
			}`),
		},
		Status: &ConnectionStatus{
			Lifecycle:               ConnectionLifecycleStatus{State: ConnectionStateConfigured},
			Health:                  ConnectionHealthStatus{State: ConnectionHealthStateHealthy},
			ConfigurationConfigured: true,
		},
	}
}

func TestConnectionResourceValidationAndPatch(t *testing.T) {
	resource := storedConnectionResource()
	require.NoError(t, resource.ValidateFor(meta.ValidationModePersistence, nil))
	require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))

	patch := NewConnectionPatch()
	name := resource.Metadata.Name
	name = "renamed"
	labels := map[string]string{"team": "security"}
	patch.Metadata.Name = &name
	patch.Metadata.Labels = &labels

	updated, err := resource.ApplyUpdate(patch)
	require.NoError(t, err)
	require.Equal(t, "renamed", string(updated.Metadata.Name))
	require.Equal(t, "security", updated.Metadata.Labels["team"])
	require.Equal(t, resource.Spec, updated.Spec)

	namespace := "root.other"
	patch.Metadata.Namespace = &namespace
	_, err = resource.ApplyUpdate(patch)
	require.ErrorContains(t, err, "immutable")
}

func TestConnectionResourceRejectsInvalidReferencesAndStatus(t *testing.T) {
	resource := storedConnectionResource()
	resource.Spec.ConnectorRef.Generation = 0
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeResponse, nil), "generation")

	resource = storedConnectionResource()
	resource.Status.Health.State = "unknown"
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeResponse, nil), "health")

	resource = storedConnectionResource()
	resource.Status.Setup = &ConnectionSetupStatus{}
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeResponse, nil), "stepId or error")

	resource = storedConnectionResource()
	resource.Spec.ConfigurationSchema = nil
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeResponse, nil), "configurationSchema")

	resource = storedConnectionResource()
	resource.Spec.ConfigurationSchema = common.RawJSON(`[]`)
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeResponse, nil), "JSON object")
}

func TestConnectionCloneAndReferences(t *testing.T) {
	resource := storedConnectionResource()
	setupError := "failed"
	resource.Status.Setup = &ConnectionSetupStatus{StepID: "apxy:verify_failed", Error: &setupError}
	clone := resource.Clone()
	clone.Metadata.Labels["team"] = "changed"
	clone.Spec.Configuration["options"].(map[string]any)["region"] = "changed"
	clone.Spec.ConfigurationSchema[0] = '['
	*clone.Status.Setup.Error = "changed"
	require.Equal(t, "platform", resource.Metadata.Labels["team"])
	require.Equal(t, "us", resource.Spec.Configuration["options"].(map[string]any)["region"])
	require.Equal(t, byte('{'), resource.Spec.ConfigurationSchema[0])
	require.Equal(t, "failed", *resource.Status.Setup.Error)

	connectionID := apid.New(apid.PrefixConnection)
	require.Equal(t, connectionID.String(), NewConnectionReference(connectionID).ID)
	require.NoError(t, ValidateID(connectionID.String()))
	require.Error(t, ValidateID(apid.New(apid.PrefixConnector).String()))
	require.False(t, IsValidConnectionState("unknown"))
	require.False(t, IsValidConnectionHealthState("unknown"))
}

func TestConnectionConfigurationIsReturnedForAPI(t *testing.T) {
	resource := storedConnectionResource()
	encoded, report, err := apserde.MarshalJSONForAPI(context.Background(), resource)
	require.NoError(t, err)
	require.False(t, report.Redacted)
	require.Contains(t, string(encoded), "acme")

	var value map[string]any
	require.NoError(t, json.Unmarshal(encoded, &value))
	spec := value["spec"].(map[string]any)
	configuration := spec["configuration"].(map[string]any)
	require.Equal(t, "acme", configuration["tenant"])
	require.Equal(t, "us", configuration["options"].(map[string]any)["region"])
	configurationSchema := spec["configurationSchema"].(map[string]any)
	require.Equal(t, "object", configurationSchema["type"])
}

func TestConnectionConfigurationSchemaIsStructuredYAML(t *testing.T) {
	resource := storedConnectionResource()
	encoded, err := yaml.Marshal(resource)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "configurationSchema:\n        $schema:")
	require.Contains(t, string(encoded), "properties:\n            options:")

	var decoded Connection
	require.NoError(t, yaml.Unmarshal(encoded, &decoded))
	require.JSONEq(t, string(resource.Spec.ConfigurationSchema), string(decoded.Spec.ConfigurationSchema))
	require.Equal(t, resource.Spec.Configuration, decoded.Spec.Configuration)
}
