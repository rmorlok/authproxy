package key

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func testManagedKeyResource() Key {
	createdAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	return Key{
		TypeMeta: meta.NewTypeMeta(KeyKind),
		Metadata: meta.ObjectMeta{
			ID:          "key_test550e8400abcd",
			Name:        "primary-key",
			Namespace:   "root.acme",
			Labels:      map[string]string{"team": "security"},
			Annotations: map[string]string{"example.com/purpose": "database"},
			CreatedAt:   &createdAt,
			UpdatedAt:   &updatedAt,
		},
		Spec: KeySpec{
			Usage:        KeyUsageDataEncryption,
			MaterialType: KeyMaterialTypeSymmetric,
			DesiredState: KeyStateActive,
			KeyData: &KeyData{InnerVal: &KeyDataValue{
				Value: "super-secret-key",
			}},
		},
		Status: &KeyStatus{
			State:             KeyStateActive,
			KeyDataConfigured: true,
		},
	}
}

func TestManagedKeyRoundTrip(t *testing.T) {
	expected := testManagedKeyResource()

	t.Run("JSON", func(t *testing.T) {
		data, err := json.Marshal(expected)
		require.NoError(t, err)
		require.JSONEq(t, `{
          "apiVersion":"authproxy.net/v1alpha1",
          "kind":"Key",
          "metadata":{
            "id":"key_test550e8400abcd",
            "name":"primary-key",
            "namespace":"root.acme",
            "labels":{"team":"security"},
            "annotations":{"example.com/purpose":"database"},
            "createdAt":"2026-09-01T12:00:00Z",
            "updatedAt":"2026-09-01T12:01:00Z"
          },
          "spec":{
            "usage":"data_encryption",
            "materialType":"symmetric",
            "desiredState":"active",
            "keyData":{"value":"super-secret-key"}
          },
          "status":{"state":"active","keyDataConfigured":true}
        }`, string(data))

		var actual Key
		require.NoError(t, json.Unmarshal(data, &actual))
		require.Empty(t, cmp.Diff(expected, actual))
	})

	t.Run("YAML", func(t *testing.T) {
		data, err := yaml.Marshal(expected)
		require.NoError(t, err)
		require.Contains(t, string(data), "apiVersion: authproxy.net/v1alpha1\nkind: Key\nmetadata:")

		var actual Key
		require.NoError(t, yaml.Unmarshal(data, &actual))
		require.Empty(t, cmp.Diff(expected, actual))
	})
}

func TestManagedKeyValidationModes(t *testing.T) {
	response := testManagedKeyResource()
	require.NoError(t, response.ValidateFor(meta.ValidationModeResponse, nil))
	require.NoError(t, response.ValidateFor(meta.ValidationModePersistence, nil))

	create := NewKey()
	create.Metadata.Namespace = "root.acme"
	create.Spec.KeyData = &KeyData{InnerVal: &KeyDataRandomBytes{NumBytes: 32}}
	require.NoError(t, create.ValidateFor(meta.ValidationModeCreate, nil))

	create.Metadata.ID = "key_test550e8400abcd"
	create.Metadata.Generation = 1
	create.Status = &KeyStatus{State: KeyStateActive}
	err := create.ValidateFor(meta.ValidationModeCreate, nil)
	require.ErrorContains(t, err, "metadata.id: is server-owned on create")
	require.ErrorContains(t, err, "metadata.generation")
	require.ErrorContains(t, err, "status: is server-owned")

	invalid := testManagedKeyResource()
	invalid.Metadata.ID = "cxr_test550e8400abcd"
	invalid.Metadata.Namespace = "invalid"
	invalid.Spec.Usage = "signing"
	invalid.Spec.MaterialType = "magic"
	invalid.Spec.DesiredState = "destroyed"
	invalid.Status.State = "destroyed"
	err = invalid.ValidateFor(meta.ValidationModeResponse, nil)
	require.ErrorContains(t, err, "metadata.id")
	require.ErrorContains(t, err, "metadata.namespace")
	require.ErrorContains(t, err, "spec.usage")
	require.ErrorContains(t, err, "spec.materialType")
	require.ErrorContains(t, err, "spec.desiredState")
	require.ErrorContains(t, err, "status.state")
}

func TestManagedKeyCreateDefaults(t *testing.T) {
	resource := NewKey()
	resource.Metadata.Namespace = "root"
	resource.Spec.KeyData = &KeyData{InnerVal: &KeyDataRandomBytes{NumBytes: 32}}
	id := apid.MustParse("key_test550e8400abcd")

	defaulted, err := resource.ApplyCreateDefaults(id)
	require.NoError(t, err)
	require.Equal(t, id.String(), string(defaulted.Metadata.Name))
	require.Equal(t, KeyUsageDataEncryption, defaulted.Spec.Usage)
	require.Equal(t, KeyMaterialTypeSymmetric, defaulted.Spec.MaterialType)
	require.Equal(t, KeyStateActive, defaulted.Spec.DesiredState)
	require.Empty(t, resource.Metadata.Name)
}

func TestManagedKeyPatchApplyTo(t *testing.T) {
	current := testManagedKeyResource()
	name := current.Metadata.Name
	name = "renamed-key"
	labels := map[string]string{}
	disabled := KeyStateDisabled
	patch := NewKeyPatch()
	patch.Metadata.Name = &name
	patch.Metadata.Labels = &labels
	patch.Spec.DesiredState = &disabled
	patch.Spec.SetKeyData(&KeyData{InnerVal: &KeyDataValue{Value: "replacement"}})

	updated, err := patch.ApplyTo(&current, nil)
	require.NoError(t, err)
	require.Equal(t, name, updated.Metadata.Name)
	require.NotNil(t, updated.Metadata.Labels)
	require.Empty(t, updated.Metadata.Labels)
	require.Equal(t, KeyStateDisabled, updated.Spec.DesiredState)
	require.Equal(t, "replacement", updated.Spec.KeyData.InnerVal.(*KeyDataValue).Value)
	require.Equal(t, "super-secret-key", current.Spec.KeyData.InnerVal.(*KeyDataValue).Value)

	otherNamespace := "root.other"
	patch.Metadata.Namespace = &otherNamespace
	_, err = patch.ApplyTo(&current, nil)
	require.ErrorContains(t, err, "metadata.namespace: is immutable")

	otherUsage := KeyUsage("signing")
	patch.Metadata.Namespace = nil
	patch.Spec.Usage = &otherUsage
	_, err = patch.ApplyTo(&current, nil)
	require.ErrorContains(t, err, "spec.usage")
}

func TestManagedKeyPatchPresenceAndValidation(t *testing.T) {
	t.Run("requires metadata and spec", func(t *testing.T) {
		var patch KeyPatch
		require.NoError(t, json.Unmarshal([]byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Key"}`), &patch))
		err := patch.ValidateFor(meta.ValidationModeUpdate, nil)
		require.ErrorContains(t, err, "metadata: is required and must not be null")
		require.ErrorContains(t, err, "spec: is required and must not be null")
	})

	t.Run("omitted key data", func(t *testing.T) {
		patch := NewKeyPatch()
		require.False(t, patch.Spec.HasKeyData())
		data, err := json.Marshal(patch)
		require.NoError(t, err)
		require.JSONEq(t, `{"apiVersion":"authproxy.net/v1alpha1","kind":"Key","metadata":{},"spec":{}}`, string(data))
	})

	t.Run("explicit JSON null is rejected", func(t *testing.T) {
		var patch KeyPatch
		err := json.Unmarshal([]byte(`{
          "apiVersion":"authproxy.net/v1alpha1",
          "kind":"Key",
          "metadata":{},
          "spec":{"keyData":null}
        }`), &patch)
		require.NoError(t, err)
		require.True(t, patch.Spec.HasKeyData())
		require.ErrorContains(t, patch.ValidateFor(meta.ValidationModeUpdate, nil), "spec.keyData: must not be null or empty")
	})

	t.Run("explicit YAML null is rejected", func(t *testing.T) {
		var patch KeyPatch
		err := yaml.Unmarshal([]byte(`
apiVersion: authproxy.net/v1alpha1
kind: Key
metadata: {}
spec:
  keyData: null
`), &patch)
		require.NoError(t, err)
		require.True(t, patch.Spec.HasKeyData())
		require.ErrorContains(t, patch.ValidateFor(meta.ValidationModeUpdate, nil), "spec.keyData: must not be null or empty")
	})
}

func TestManagedKeyRedactionCannotBeReplayed(t *testing.T) {
	resource := testManagedKeyResource()
	redacted, err := RedactKeyData(resource.Spec.KeyData)
	require.NoError(t, err)
	resource.Spec.KeyData = redacted

	ctx := apserde.WithSecretReplay(context.Background(), true)
	data, report, err := apserde.MarshalJSONForAPI(ctx, resource)
	require.NoError(t, err)
	require.False(t, report.Redacted, "the resource is already irreversibly redacted")
	require.NotContains(t, string(data), "super-secret-key")
	require.Contains(t, string(data), strings.Repeat("*", len("super-secret-key")))

	raw, err := RedactKeyData(&KeyData{InnerVal: &KeyDataRawVal{Raw: []byte("raw-secret")}})
	require.NoError(t, err)
	require.Nil(t, raw, "raw in-memory bytes have no safe API representation")
}

func TestManagedKeyStructuredLoggingOmitsKeyMaterial(t *testing.T) {
	resource := testManagedKeyResource()
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	logger.Info("managed key", "key", resource, "keyData", resource.Spec.KeyData)

	require.NotContains(t, output.String(), "super-secret-key")
	require.NotContains(t, output.String(), "keyData.value")
	require.Contains(t, output.String(), `"provider":"value"`)
}
