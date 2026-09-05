package connection

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apserde"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
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
			ActorRef: &meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       actorschema.ActorKind,
				ID:         apid.New(apid.PrefixActor).String(),
			},
			Configuration: map[string]any{
				"tenant": "acme",
				"nested": map[string]any{"apiKey": "secret"},
			},
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
	resource.Spec.ActorRef.Kind = connectorschema.ConnectorKind
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeResponse, nil), "Actor")

	resource = storedConnectionResource()
	resource.Status.Health.State = "unknown"
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeResponse, nil), "health")

	resource = storedConnectionResource()
	resource.Status.Setup = &ConnectionSetupStatus{}
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeResponse, nil), "stepId or error")
}

func TestConnectionCloneAndReferences(t *testing.T) {
	resource := storedConnectionResource()
	setupError := "failed"
	resource.Status.Setup = &ConnectionSetupStatus{StepID: "apxy:verify_failed", Error: &setupError}
	clone := resource.Clone()
	clone.Metadata.Labels["team"] = "changed"
	clone.Spec.ActorRef.ID = apid.New(apid.PrefixActor).String()
	clone.Spec.Configuration["nested"].(map[string]any)["apiKey"] = "changed"
	*clone.Status.Setup.Error = "changed"
	require.Equal(t, "platform", resource.Metadata.Labels["team"])
	require.NotEqual(t, clone.Spec.ActorRef.ID, resource.Spec.ActorRef.ID)
	require.Equal(t, "secret", resource.Spec.Configuration["nested"].(map[string]any)["apiKey"])
	require.Equal(t, "failed", *resource.Status.Setup.Error)

	connectionID := apid.New(apid.PrefixConnection)
	require.Equal(t, connectionID.String(), NewConnectionReference(connectionID).ID)
	require.Nil(t, NewActorReference(apid.Nil))
	require.Equal(t, actorschema.ActorKind, NewActorReference(apid.New(apid.PrefixActor)).Kind)
	require.NoError(t, ValidateID(connectionID.String()))
	require.Error(t, ValidateID(apid.New(apid.PrefixConnector).String()))
	require.False(t, IsValidConnectionState("unknown"))
	require.False(t, IsValidConnectionHealthState("unknown"))
}

func TestConnectionConfigurationIsRedactedForAPI(t *testing.T) {
	resource := storedConnectionResource()
	encoded, report, err := apserde.MarshalJSONForAPI(context.Background(), resource)
	require.NoError(t, err)
	require.True(t, report.Redacted)
	require.NotContains(t, string(encoded), "secret")

	var value map[string]any
	require.NoError(t, json.Unmarshal(encoded, &value))
	configuration := value["spec"].(map[string]any)["configuration"].(map[string]any)
	require.Equal(t, "****", configuration["tenant"])
	require.Equal(t, "******", configuration["nested"].(map[string]any)["apiKey"])
}

func TestConnectionConfigurationCannotBeReplayed(t *testing.T) {
	resource := storedConnectionResource()
	redacted, err := RedactConfiguration(resource.Spec.Configuration)
	require.NoError(t, err)
	resource.Spec.Configuration = redacted

	ctx := apserde.WithSecretReplay(context.Background(), true)
	encoded, report, err := apserde.MarshalJSONForAPI(ctx, resource)
	require.NoError(t, err)
	require.False(t, report.Redacted, "the configuration is already irreversibly redacted")
	require.NotContains(t, string(encoded), "secret")
	require.Contains(t, string(encoded), "******")
}
