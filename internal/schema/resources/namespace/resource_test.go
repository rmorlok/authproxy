package namespace

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

func testNamespaceResource() Namespace {
	createdAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	return Namespace{
		TypeMeta: meta.NewTypeMeta(NamespaceKind),
		Metadata: meta.ObjectMeta{
			ID:          "root.acme",
			Name:        "acme",
			Namespace:   "root",
			Labels:      map[string]string{"team": "platform"},
			Annotations: map[string]string{"example.com/owner": "integrations"},
			CreatedAt:   &createdAt,
			UpdatedAt:   &updatedAt,
		},
		Spec: NamespaceSpec{EncryptionKeyRef: &meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       "Key",
			ID:         "key_test550e8400abcd",
		}},
		Status: &NamespaceStatus{State: NamespaceStateActive},
	}
}

func TestNamespaceRoundTrip(t *testing.T) {
	expected := testNamespaceResource()

	t.Run("JSON", func(t *testing.T) {
		data, err := json.Marshal(expected)
		require.NoError(t, err)
		require.JSONEq(t, `{
          "apiVersion":"authproxy.net/v1alpha1",
          "kind":"Namespace",
          "metadata":{
            "id":"root.acme",
            "name":"acme",
            "namespace":"root",
            "labels":{"team":"platform"},
            "annotations":{"example.com/owner":"integrations"},
            "createdAt":"2026-08-30T12:00:00Z",
            "updatedAt":"2026-08-30T12:01:00Z"
          },
          "spec":{
            "encryptionKeyRef":{
              "apiVersion":"authproxy.net/v1alpha1",
              "kind":"Key",
              "id":"key_test550e8400abcd"
            }
          },
          "status":{"state":"active"}
        }`, string(data))

		var actual Namespace
		require.NoError(t, json.Unmarshal(data, &actual))
		require.Empty(t, cmp.Diff(expected, actual))
	})

	t.Run("YAML", func(t *testing.T) {
		data, err := yaml.Marshal(expected)
		require.NoError(t, err)
		require.Contains(t, string(data), "apiVersion: authproxy.net/v1alpha1\nkind: Namespace\nmetadata:")

		var actual Namespace
		require.NoError(t, yaml.Unmarshal(data, &actual))
		require.Empty(t, cmp.Diff(expected, actual))
	})
}

func TestNamespacePathMetadata(t *testing.T) {
	tests := []struct {
		path      string
		name      common.ResourceName
		parent    string
		wantError string
	}{
		{path: Root, name: "root"},
		{path: "root.acme", name: "acme", parent: "root"},
		{path: "root.acme.prod", name: "prod", parent: "root.acme"},
		{path: "invalid", wantError: "child of root"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			metadata, err := NewResourceMetadata(tt.path)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.path, metadata.ID)
			require.Equal(t, tt.name, metadata.Name)
			require.Equal(t, tt.parent, metadata.Namespace)

			path, err := PathFromMetadata(metadata)
			require.NoError(t, err)
			require.Equal(t, tt.path, path)
		})
	}

	_, err := PathFromMetadata(meta.ObjectMeta{Name: "child"})
	require.ErrorContains(t, err, "parent namespace is required")
	_, err = PathFromMetadata(meta.ObjectMeta{Name: "child", Namespace: "invalid"})
	require.ErrorContains(t, err, "invalid parent namespace")
}

func TestNamespaceValidationModes(t *testing.T) {
	response := testNamespaceResource()
	require.NoError(t, response.ValidateFor(meta.ValidationModeResponse, nil))
	require.NoError(t, response.ValidateFor(meta.ValidationModePersistence, nil))

	create := NewNamespace()
	create.Metadata.Name = "acme"
	create.Metadata.Namespace = "root"
	require.NoError(t, create.ValidateFor(meta.ValidationModeCreate, nil))

	create.Metadata.ID = "root.acme"
	require.ErrorContains(t, create.ValidateFor(meta.ValidationModeCreate, nil), "server-owned on create")

	invalid := testNamespaceResource()
	invalid.Metadata.ID = "root.other"
	invalid.Metadata.Generation = 1
	invalid.Spec.EncryptionKeyRef.Kind = "Connector"
	invalid.Status.State = "unknown"
	err := invalid.ValidateFor(meta.ValidationModeResponse, nil)
	require.ErrorContains(t, err, "metadata.id")
	require.ErrorContains(t, err, "metadata.generation")
	require.ErrorContains(t, err, "spec.encryptionKeyRef.kind")
	require.ErrorContains(t, err, "status.state")

	writeWithStatus := NewNamespace()
	writeWithStatus.Metadata.Name = "acme"
	writeWithStatus.Metadata.Namespace = "root"
	writeWithStatus.Status = &NamespaceStatus{State: NamespaceStateActive}
	require.ErrorContains(t, writeWithStatus.ValidateFor(meta.ValidationModeUpdate, nil), "status: is server-owned")

	namedKey := NewNamespace()
	namedKey.Metadata.Name = "acme"
	namedKey.Metadata.Namespace = "root"
	namedKey.Spec.EncryptionKeyRef = &meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       EncryptionKeyKind,
		Namespace:  "root",
		Name:       "key_global",
	}
	require.NoError(t, namedKey.ValidateFor(meta.ValidationModeCreate, nil))

	namedKey.Spec.EncryptionKeyRef.Namespace = ""
	require.ErrorContains(t, namedKey.ValidateFor(meta.ValidationModeCreate, nil), "spec.encryptionKeyRef: must contain id or namespace and name")
}

func TestNamespaceValidateUpdate(t *testing.T) {
	before := testNamespaceResource()
	after := before.Clone()
	after.Metadata.Labels = map[string]string{"team": "security"}
	after.Metadata.Annotations = nil
	require.NoError(t, ValidateUpdate(&before, after, nil))

	after.Metadata.Name = "renamed"
	after.Metadata.Namespace = "root.other"
	after.Metadata.ID = "root.other.renamed"
	err := ValidateUpdate(&before, after, nil)
	require.ErrorContains(t, err, "metadata.id: is immutable")
	require.ErrorContains(t, err, "metadata.name: is immutable")
	require.ErrorContains(t, err, "metadata.namespace: is immutable")
}

func TestNamespacePatchApplyTo(t *testing.T) {
	current := testNamespaceResource()
	emptyLabels := map[string]string{}
	annotations := map[string]string{"example.com/owner": "security"}
	patch := NewNamespacePatch()
	require.NotNil(t, patch.Metadata)
	require.NotNil(t, patch.Spec)
	patch.Metadata.Labels = &emptyLabels
	patch.Metadata.Annotations = &annotations
	data, err := json.Marshal(patch)
	require.NoError(t, err)
	require.JSONEq(t, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"Namespace",
      "metadata":{
        "labels":{},
        "annotations":{"example.com/owner":"security"}
      },
      "spec":{}
    }`, string(data))

	updated, err := patch.ApplyTo(&current, nil)
	require.NoError(t, err)
	require.Empty(t, updated.Metadata.Labels)
	require.NotNil(t, updated.Metadata.Labels)
	require.Equal(t, annotations, updated.Metadata.Annotations)
	require.Equal(t, current.Metadata.ID, updated.Metadata.ID)
	require.NotSame(t, current.Spec.EncryptionKeyRef, updated.Spec.EncryptionKeyRef)

	otherID := "root.other"
	patch.Metadata.ID = &otherID
	_, err = patch.ApplyTo(&current, nil)
	require.ErrorContains(t, err, "metadata.id: is immutable")
}

func TestNamespacePatchValidation(t *testing.T) {
	patch := NewNamespacePatch()
	require.NoError(t, patch.ValidateFor(meta.ValidationModeUpdate, nil))

	invalidParent := "not-rooted"
	patch.Metadata.Namespace = &invalidParent
	patch.Spec.EncryptionKeyRef = &meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       "Connector",
		ID:         "cxr_test550e8400abcd",
	}
	patch.Status = &NamespaceStatus{State: NamespaceStateActive}
	err := patch.ValidateFor(meta.ValidationModeUpdate, nil)
	require.ErrorContains(t, err, "metadata.namespace")
	require.ErrorContains(t, err, "spec.encryptionKeyRef.kind")
	require.ErrorContains(t, err, "spec.encryptionKeyRef.id")
	require.ErrorContains(t, err, "status: is server-owned")
}

func TestNamespacePatchRequiresMetadataAndSpec(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "missing",
			data: `{"apiVersion":"authproxy.net/v1alpha1","kind":"Namespace"}`,
		},
		{
			name: "null",
			data: `{"apiVersion":"authproxy.net/v1alpha1","kind":"Namespace","metadata":null,"spec":null}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var patch NamespacePatch
			require.NoError(t, json.Unmarshal([]byte(test.data), &patch))
			err := patch.ValidateFor(meta.ValidationModeUpdate, nil)
			require.ErrorContains(t, err, "$.metadata: is required and must not be null")
			require.ErrorContains(t, err, "$.spec: is required and must not be null")
		})
	}
}

func TestNamespaceSpecPatchRejectsExplicitNullKeyReference(t *testing.T) {
	var jsonPatch NamespacePatch
	err := json.Unmarshal([]byte(`{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"Namespace",
      "metadata":{},
      "spec":{"encryptionKeyRef":null}
    }`), &jsonPatch)
	require.ErrorContains(t, err, "encryptionKeyRef must not be null")

	var yamlPatch NamespacePatch
	err = yaml.Unmarshal([]byte(`
apiVersion: authproxy.net/v1alpha1
kind: Namespace
metadata: {}
spec:
  encryptionKeyRef: null
`), &yamlPatch)
	require.ErrorContains(t, err, "encryptionKeyRef must not be null")
}

func TestNamespaceCloneDoesNotAliasMutableFields(t *testing.T) {
	original := testNamespaceResource()
	clone := original.Clone()
	clone.Metadata.Labels["team"] = "changed"
	clone.Spec.EncryptionKeyRef.ID = "key_other550e8400abcd"
	clone.Status.State = NamespaceStateDestroyed

	require.Equal(t, "platform", original.Metadata.Labels["team"])
	require.Equal(t, "key_test550e8400abcd", original.Spec.EncryptionKeyRef.ID)
	require.Equal(t, NamespaceStateActive, original.Status.State)
}
