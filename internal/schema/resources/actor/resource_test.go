package actor

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	authschema "github.com/rmorlok/authproxy/internal/schema/auth"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func testSigningKey() *keyschema.SigningKey {
	return &keyschema.SigningKey{InnerVal: &keyschema.KeyPublicPrivate{
		PublicKey: &keyschema.KeyData{InnerVal: &keyschema.KeyDataValue{Value: "public-key"}},
	}}
}

func testPermission() authschema.Permission {
	return authschema.Permission{
		Namespace: "root.acme.**",
		Resources: []string{"connections"},
		Verbs:     []string{"read"},
	}
}

func TestActorValidateForLifecycle(t *testing.T) {
	resource := &Actor{
		TypeMeta: meta.NewTypeMeta(ActorKind),
		Metadata: meta.ObjectMeta{Namespace: "root.acme"},
		Spec: ActorSpec{
			ExternalId:  "user-123",
			Permissions: []authschema.Permission{testPermission()},
		},
	}
	require.NoError(t, resource.ValidateFor(meta.ValidationModeCreate, nil))
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeConfig, nil), "signingKey")

	resource.Spec.SigningKey = testSigningKey()
	require.NoError(t, resource.ValidateFor(meta.ValidationModeConfig, nil))

	resource.Metadata.ID = apid.New(apid.PrefixActor).String()
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeCreate, nil), "server-owned on create")

	resource.Metadata.Name = "billing-service"
	resource.Spec.SigningKey = nil
	now := time.Now().UTC()
	resource.Metadata.CreatedAt = &now
	resource.Metadata.UpdatedAt = &now
	resource.Status = &ActorStatus{SigningKeyConfigured: true}
	require.NoError(t, resource.ValidateFor(meta.ValidationModePersistence, nil))
	require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))

	resource.Spec.SigningKey = testSigningKey()
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeResponse, nil), "must be redacted")
}

func TestActorPatchPresenceAndApply(t *testing.T) {
	current := &Actor{
		TypeMeta: meta.NewTypeMeta(ActorKind),
		Metadata: meta.ObjectMeta{
			ID:          apid.New(apid.PrefixActor).String(),
			Name:        "old-name",
			Namespace:   "root.acme",
			Labels:      map[string]string{"team": "platform"},
			Annotations: map[string]string{"owner": "alice"},
		},
		Spec: ActorSpec{
			ExternalId:  "user-123",
			Permissions: []authschema.Permission{testPermission()},
		},
		Status: &ActorStatus{SigningKeyConfigured: true},
	}

	var patch ActorPatch
	require.NoError(t, json.Unmarshal([]byte(`{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"Actor",
      "metadata":{"name":"new-name","labels":{}},
      "spec":{"permissions":[],"signingKey":null}
    }`), &patch))
	require.True(t, patch.Spec.HasPermissions())
	require.True(t, patch.Spec.HasSigningKey())
	require.Nil(t, patch.Spec.SigningKey)

	updated, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	require.Equal(t, "new-name", string(updated.Metadata.Name))
	require.Empty(t, updated.Metadata.Labels)
	require.Empty(t, updated.Spec.Permissions)
	require.Nil(t, updated.Spec.SigningKey)
	require.Equal(t, map[string]string{"owner": "alice"}, updated.Metadata.Annotations)

	encoded, err := json.Marshal(patch)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"signingKey":null`)

	var immutable ActorPatch
	require.NoError(t, json.Unmarshal([]byte(`{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"Actor",
      "metadata":{},
      "spec":{"externalId":"different"}
    }`), &immutable))
	_, err = immutable.ApplyTo(current, nil)
	require.ErrorContains(t, err, "spec.externalId")
	require.ErrorContains(t, err, "immutable")
}

func TestActorPatchYAMLPreservesNull(t *testing.T) {
	var patch ActorPatch
	require.NoError(t, yaml.Unmarshal([]byte(`
apiVersion: authproxy.net/v1alpha1
kind: Actor
metadata: {}
spec:
  permissions: null
  signingKey: null
`), &patch))
	require.True(t, patch.Spec.HasPermissions())
	require.True(t, patch.Spec.HasSigningKey())
	require.ErrorContains(t, patch.ValidateFor(meta.ValidationModeUpdate, nil), "permissions")

	encoded, err := yaml.Marshal(patch)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "signingKey: null")
}

func TestActorApplyCreateDefaultsDoesNotMutateInput(t *testing.T) {
	resource := &Actor{
		TypeMeta: meta.NewTypeMeta(ActorKind),
		Metadata: meta.ObjectMeta{
			Namespace: "root",
			Labels:    map[string]string{"team": "platform"},
		},
		Spec: ActorSpec{ExternalId: "user-123"},
	}
	id := apid.New(apid.PrefixActor)

	created := resource.ApplyCreateDefaults(id)
	require.Equal(t, id.String(), string(created.Metadata.Name))
	created.Metadata.Labels["team"] = "security"
	require.Empty(t, resource.Metadata.Name)
	require.Equal(t, "platform", resource.Metadata.Labels["team"])
}

func TestActorConstructorsAccessorsAndClone(t *testing.T) {
	id := apid.New(apid.PrefixActor)
	resource := NewActor()
	resource.Metadata = meta.ObjectMeta{
		ID:          id.String(),
		Namespace:   "root.acme",
		Labels:      map[string]string{"team": "platform"},
		Annotations: map[string]string{"owner": "alice"},
	}
	resource.Spec = ActorSpec{
		ExternalId:  "user-123",
		Permissions: []authschema.Permission{testPermission()},
	}
	resource.Status = &ActorStatus{SigningKeyConfigured: true}

	require.Equal(t, meta.APIVersionV1Alpha1, resource.APIVersion)
	require.Equal(t, ActorKind, resource.Kind)
	require.Equal(t, id, resource.GetId())
	require.Equal(t, id.String(), string(resource.GetName()))
	require.Equal(t, "user-123", resource.GetExternalId())
	require.Equal(t, resource.Spec.Permissions, resource.GetPermissions())
	require.Equal(t, "root.acme", resource.GetNamespace())
	require.Equal(t, resource.Metadata.Labels, resource.GetLabels())
	require.Equal(t, resource.Metadata.Annotations, resource.GetAnnotations())
	require.Equal(t, slog.KindGroup, resource.LogValue().Kind())

	clone := resource.Clone()
	clone.Metadata.Labels["team"] = "security"
	clone.Metadata.Annotations["owner"] = "bob"
	clone.Spec.Permissions[0].Resources[0] = "actors"
	clone.Status.SigningKeyConfigured = false
	require.Equal(t, "platform", resource.Metadata.Labels["team"])
	require.Equal(t, "alice", resource.Metadata.Annotations["owner"])
	require.Equal(t, "connections", resource.Spec.Permissions[0].Resources[0])
	require.True(t, resource.Status.SigningKeyConfigured)

	patch := NewActorPatch()
	require.Equal(t, meta.APIVersionV1Alpha1, patch.APIVersion)
	require.Equal(t, ActorKind, patch.Kind)
	require.NotNil(t, patch.Metadata)
	require.NotNil(t, patch.Spec)
	patch.Spec.SetSigningKey(testSigningKey())
	require.True(t, patch.Spec.HasSigningKey())

	var nilActor *Actor
	require.Nil(t, nilActor.Clone())
	require.Equal(t, apid.Nil, nilActor.GetId())
	require.Empty(t, nilActor.GetName())
	require.Empty(t, nilActor.GetExternalId())
	require.Nil(t, nilActor.GetPermissions())
	require.Empty(t, nilActor.GetNamespace())
	require.Nil(t, nilActor.GetLabels())
	require.Nil(t, nilActor.GetAnnotations())
	var nilSpec *ActorSpecPatch
	require.False(t, nilSpec.HasExternalId())
	require.False(t, nilSpec.HasPermissions())
	require.False(t, nilSpec.HasSigningKey())
	nilSpec.SetSigningKey(nil)
}

func TestActorValidationRejectsInvalidLifecycleState(t *testing.T) {
	require.ErrorContains(t, (*Actor)(nil).ValidateFor(meta.ValidationModeCreate, nil), "actor is required")
	require.ErrorContains(t, (*ActorPatch)(nil).ValidateFor(meta.ValidationModeUpdate, nil), "actor patch is required")
	_, err := NewActorPatch().ApplyTo(nil, nil)
	require.ErrorContains(t, err, "current actor is required")
	require.ErrorContains(t, ValidateUpdate(nil, NewActor(), nil), "before and after actors are required")

	resource := NewActor()
	resource.Metadata.Namespace = "root"
	resource.Metadata.Generation = 1
	resource.Spec.Permissions = []authschema.Permission{{}}
	resource.Spec.SigningKey = &keyschema.SigningKey{}
	err = resource.ValidateFor(meta.ValidationModeCreate, nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "metadata.generation")
	require.ErrorContains(t, err, "spec.externalId")
	require.ErrorContains(t, err, "spec.permissions")
	require.ErrorContains(t, err, "spec.signingKey")

	patch := NewActorPatch()
	patch.Metadata = nil
	patch.Spec = nil
	patch.Status = &ActorStatus{}
	err = patch.ValidateFor(meta.ValidationModeUpdate, nil)
	require.ErrorContains(t, err, "metadata")
	require.ErrorContains(t, err, "spec")
	require.ErrorContains(t, err, "status")

	configured := NewActor()
	configured.Metadata.Namespace = "root"
	configured.Spec.ExternalId = "service"
	configured.Spec.SigningKey = testSigningKey()
	require.NoError(t, configured.Validate(nil))

	before := configured.Clone()
	before.Metadata.ID = apid.New(apid.PrefixActor).String()
	before.Metadata.Name = "service"
	after := before.Clone()
	after.Metadata.Namespace = "root.other"
	require.ErrorContains(t, ValidateUpdate(before, after, nil), "immutable")

	require.Error(t, ValidateID("not-an-id"))
	require.ErrorContains(t, ValidateID(apid.New(apid.PrefixKey).String()), "actor id")
	require.NoError(t, ValidateID(apid.New(apid.PrefixActor).String()))
}

func TestActorPatchStrictSerialization(t *testing.T) {
	var patch ActorPatch
	require.Error(t, json.Unmarshal([]byte(`{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"Actor",
      "metadata":{},
      "spec":{"unknown":true}
    }`), &patch))

	require.Error(t, yaml.Unmarshal([]byte(`
apiVersion: authproxy.net/v1alpha1
kind: Actor
metadata: {}
spec:
  unknown: true
`), &patch))
}
