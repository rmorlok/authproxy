package meta

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTypeMetaSerializationIsTopLevelWhenEmbedded(t *testing.T) {
	type resource struct {
		TypeMeta `json:",inline" yaml:",inline"`
		Metadata ObjectMeta     `json:"metadata" yaml:"metadata"`
		Spec     map[string]any `json:"spec" yaml:"spec"`
	}
	value := resource{
		TypeMeta: NewTypeMeta("Widget"),
		Metadata: ObjectMeta{Name: "example"},
		Spec:     map[string]any{"enabled": true},
	}

	jsonBytes, err := json.Marshal(value)
	require.NoError(t, err)
	require.JSONEq(t, `{"apiVersion":"authproxy.net/v1alpha1","kind":"Widget","metadata":{"name":"example"},"spec":{"enabled":true}}`, string(jsonBytes))

	yamlBytes, err := yaml.Marshal(value)
	require.NoError(t, err)
	require.Contains(t, string(yamlBytes), "apiVersion: authproxy.net/v1alpha1\nkind: Widget\nmetadata:")
}

func TestCloneObjectMetaDeepCopiesMutableValues(t *testing.T) {
	createdAt := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	original := ObjectMeta{
		Labels:      map[string]string{"environment": "demo"},
		Annotations: map[string]string{"example.com/owner": "integrations"},
		CreatedAt:   &createdAt,
		UpdatedAt:   &updatedAt,
	}

	clone := CloneObjectMeta(original)
	clone.Labels["environment"] = "production"
	clone.Annotations["example.com/owner"] = "platform"
	*clone.CreatedAt = clone.CreatedAt.Add(time.Hour)
	*clone.UpdatedAt = clone.UpdatedAt.Add(time.Hour)

	require.Equal(t, "demo", original.Labels["environment"])
	require.Equal(t, "integrations", original.Annotations["example.com/owner"])
	require.Equal(t, createdAt, *original.CreatedAt)
	require.Equal(t, updatedAt, *original.UpdatedAt)
}

func TestAPIVersionParsing(t *testing.T) {
	parsed, err := ParseAPIVersion("authproxy.net/v1alpha1")
	require.NoError(t, err)
	require.Equal(t, APIGroup, parsed.Group())
	require.Equal(t, APIVersionName, parsed.Version())

	for _, invalid := range []string{"", "v1alpha1", "authproxy.net/alpha1", "AuthProxy.net/v1", "authproxy..net/v1"} {
		t.Run(invalid, func(t *testing.T) {
			_, err := ParseAPIVersion(invalid)
			require.Error(t, err)
		})
	}
}

func TestValidationContexts(t *testing.T) {
	now := time.Date(2026, 8, 28, 14, 30, 0, 0, time.FixedZone("offset", -5*60*60))
	typeMeta := NewTypeMeta("Widget")

	t.Run("create rejects server-owned identity", func(t *testing.T) {
		err := ValidateResource(typeMeta, ObjectMeta{
			ID:         "wid_123",
			Name:       "example",
			Namespace:  "root",
			Generation: 1,
			CreatedAt:  &now,
		}, ValidationOptions{
			Mode:               ValidationModeCreate,
			ExpectedAPIVersion: APIVersionV1Alpha1,
			ExpectedKind:       "Widget",
			RequireName:        true,
			RequireNamespace:   true,
		})
		require.ErrorContains(t, err, "$.metadata.id: is server-owned on create")
		require.ErrorContains(t, err, "$.metadata.generation: is server-owned on create")
		require.ErrorContains(t, err, "$.metadata.createdAt: is server-owned")
	})

	t.Run("update accepts immutable identity for comparison but rejects timestamps", func(t *testing.T) {
		err := ValidateObjectMeta(ObjectMeta{
			ID:         "wid_123",
			Name:       "example",
			Namespace:  "root",
			Generation: 2,
			UpdatedAt:  &now,
		}, ValidationOptions{Mode: ValidationModeUpdate})
		require.ErrorContains(t, err, "$.metadata.updatedAt: is server-owned")
		require.NotContains(t, err.Error(), "generation")
	})

	t.Run("config permits explicit identity and generation", func(t *testing.T) {
		err := ValidateObjectMeta(ObjectMeta{
			ID:         "wid_123",
			Name:       "example",
			Namespace:  "root",
			Generation: 2,
			Labels:     map[string]string{"environment": "demo"},
		}, ValidationOptions{Mode: ValidationModeConfig})
		require.NoError(t, err)
	})

	t.Run("persistence permits system metadata", func(t *testing.T) {
		err := ValidateObjectMeta(ObjectMeta{
			ID:         "wid_123",
			Name:       "example",
			Namespace:  "root",
			Generation: 2,
			Labels:     map[string]string{"apxy/wid/-/ns": "root.child"},
			CreatedAt:  &now,
			UpdatedAt:  &now,
		}, ValidationOptions{Mode: ValidationModePersistence, RequireID: true})
		require.NoError(t, err)
	})

	t.Run("response enforces resource-selected requirements", func(t *testing.T) {
		err := ValidateObjectMeta(ObjectMeta{Name: "example"}, ValidationOptions{
			Mode:             ValidationModeResponse,
			RequireID:        true,
			RequireName:      true,
			RequireNamespace: true,
		})
		require.ErrorContains(t, err, "$.metadata.id: is required")
		require.ErrorContains(t, err, "$.metadata.namespace: is required")
	})
}

func TestValidateObjectMetaPatch(t *testing.T) {
	id := "wid_123"
	name := common.ResourceName("example")
	namespace := "root"
	labels := map[string]string{}
	annotations := map[string]string{"example.com/owner": "platform"}
	require.NoError(t, ValidateObjectMetaPatch(ObjectMetaPatch{
		ID:          &id,
		Name:        &name,
		Namespace:   &namespace,
		Labels:      &labels,
		Annotations: &annotations,
	}, ValidationOptions{
		Mode:               ValidationModeUpdate,
		IDValidator:        func(value string) error { return nil },
		NamespaceValidator: func(value string) error { return nil },
	}))

	empty := ""
	emptyName := common.ResourceName("")
	zero := uint64(0)
	reservedLabels := map[string]string{"apxy/wid/-/id": "wid_123"}
	now := time.Now()
	err := ValidateObjectMetaPatch(ObjectMetaPatch{
		ID:         &empty,
		Name:       &emptyName,
		Namespace:  &empty,
		Generation: &zero,
		Labels:     &reservedLabels,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}, ValidationOptions{Mode: ValidationModeUpdate})
	for _, path := range []string{
		"$.metadata.id",
		"$.metadata.name",
		"$.metadata.namespace",
		"$.metadata.generation",
		"$.metadata.labels",
		"$.metadata.createdAt",
		"$.metadata.updatedAt",
	} {
		require.ErrorContains(t, err, path)
	}

	err = ValidateObjectMetaPatch(ObjectMetaPatch{}, ValidationOptions{Mode: ValidationModeCreate})
	require.ErrorContains(t, err, "metadata patches require update validation mode")
}

func TestTypeMetaAndImmutableValidationUseFieldPaths(t *testing.T) {
	err := ValidateTypeMeta(
		TypeMeta{APIVersion: "authproxy.net/v1alpha2", Kind: "Gadget"},
		APIVersionV1Alpha1,
		"Widget",
		&common.ValidationContext{Path: "$"},
	)
	require.ErrorContains(t, err, `$.apiVersion: must be "authproxy.net/v1alpha1"`)
	require.ErrorContains(t, err, `$.kind: must be "Widget"`)

	created := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	later := created.Add(time.Minute)
	err = ValidateMetadataUpdate(
		ObjectMeta{ID: "wid_1", Name: "old", Namespace: "root", Generation: 1, CreatedAt: &created},
		ObjectMeta{ID: "wid_2", Name: "new", Namespace: "root.child", Generation: 2, CreatedAt: &later},
		UpdateOptions{ImmutableName: true, ImmutableNamespace: true},
		nil,
	)
	for _, path := range []string{"$.metadata.id", "$.metadata.name", "$.metadata.namespace", "$.metadata.generation", "$.metadata.createdAt"} {
		require.ErrorContains(t, err, path)
	}
}

func TestStatusReferenceAndConditionValidation(t *testing.T) {
	status := struct {
		State string
	}{State: "ready"}
	require.ErrorContains(t, ValidateStatus(status, ValidationModeCreate, nil), "$.status: is server-owned")
	require.ErrorContains(t, ValidateStatus(status, ValidationModeUpdate, nil), "$.status: is server-owned")
	require.ErrorContains(t, ValidateStatus(status, ValidationModeConfig, nil), "$.status: is server-owned")
	require.NoError(t, ValidateStatus(status, ValidationModePersistence, nil))
	require.NoError(t, ValidateStatus(status, ValidationModeResponse, nil))
	require.NoError(t, ValidateStatus(nil, ValidationModeCreate, nil))

	reference := ObjectReference{APIVersion: APIVersionV1Alpha1, Kind: "Connector", ID: "cxr_123"}
	require.NoError(t, ValidateObjectReference(reference, &common.ValidationContext{Path: "$.metadata.target"}))
	reference.ID = ""
	require.ErrorContains(t, ValidateObjectReference(reference, &common.ValidationContext{Path: "$.metadata.target"}), "$.metadata.target: must contain id or name")

	condition := NewCondition("Ready", ConditionTrue, 1, "Configured", "ready", time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC))
	require.NoError(t, ValidateCondition(condition, &common.ValidationContext{Path: "$.status.conditions[0]"}))
	condition.Status = "yes"
	require.ErrorContains(t, ValidateCondition(condition, nil), "$.status")
}

func TestObjectMetaDefaultingNormalizationAndPatchSemantics(t *testing.T) {
	local := time.Date(2026, 8, 28, 9, 0, 0, 0, time.FixedZone("offset", 2*60*60))
	defaults := ObjectMeta{
		ID:          "wid_1",
		Name:        "default-name",
		Namespace:   "root",
		Generation:  1,
		Labels:      map[string]string{"default": "true"},
		Annotations: map[string]string{"note": "default"},
		CreatedAt:   &local,
		UpdatedAt:   &local,
	}

	value, err := ApplyObjectMetaDefaults(ObjectMeta{
		Name:        "explicit-name",
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}, defaults, ValidationModePersistence, nil)
	require.NoError(t, err)
	require.Equal(t, "wid_1", value.ID)
	require.Equal(t, common.ResourceName("explicit-name"), value.Name)
	require.Empty(t, value.Labels)
	require.NotNil(t, value.Labels)
	require.Empty(t, value.Annotations)
	require.NotNil(t, value.Annotations)
	require.Equal(t, time.UTC, value.CreatedAt.Location())

	_, err = ApplyObjectMetaDefaults(ObjectMeta{}, ObjectMeta{ID: "wid_1"}, ValidationModeCreate, nil)
	require.ErrorContains(t, err, "$.metadata.id")

	original := ObjectMeta{
		Name:        "before",
		Labels:      map[string]string{"keep": "me"},
		Annotations: map[string]string{"keep": "me"},
	}
	unchanged := ApplyObjectMetaPatch(original, ObjectMetaPatch{})
	require.Equal(t, original, unchanged)
	unchanged.Labels["mutated"] = "copy"
	require.NotContains(t, original.Labels, "mutated")

	emptyLabels := map[string]string{}
	replaced := ApplyObjectMetaPatch(original, ObjectMetaPatch{Labels: &emptyLabels})
	require.Empty(t, replaced.Labels)
	require.NotNil(t, replaced.Labels)
	require.Equal(t, original.Annotations, replaced.Annotations)

	var omittedPatch ObjectMetaPatch
	require.NoError(t, json.Unmarshal([]byte(`{}`), &omittedPatch))
	require.Nil(t, omittedPatch.Labels)
	var clearPatch ObjectMetaPatch
	require.NoError(t, json.Unmarshal([]byte(`{"labels":{},"annotations":{}}`), &clearPatch))
	require.NotNil(t, clearPatch.Labels)
	require.Empty(t, *clearPatch.Labels)
	require.NotNil(t, clearPatch.Annotations)
	require.Empty(t, *clearPatch.Annotations)
}

func TestCanonicalSpecJSONAndSemanticHash(t *testing.T) {
	left := map[string]any{
		"nested": json.RawMessage(`{ "z": 2, "a": 1 }`),
		"name":   "example",
	}
	right := map[string]any{
		"name":   "example",
		"nested": map[string]any{"a": json.Number("1"), "z": json.Number("2")},
	}

	leftJSON, err := CanonicalSpecJSON(left)
	require.NoError(t, err)
	rightJSON, err := CanonicalSpecJSON(right)
	require.NoError(t, err)
	require.Equal(t, `{"name":"example","nested":{"a":1,"z":2}}`, string(leftJSON))
	require.Equal(t, leftJSON, rightJSON)

	leftHash, err := SemanticSpecHash(left)
	require.NoError(t, err)
	rightHash, err := SemanticSpecHash(right)
	require.NoError(t, err)
	require.Equal(t, leftHash, rightHash)
	require.Len(t, leftHash, 64)

	differentHash, err := SemanticSpecHash(map[string]any{"name": "different"})
	require.NoError(t, err)
	require.NotEqual(t, leftHash, differentHash)
}

func TestReferenceAndConditionConstructors(t *testing.T) {
	reference := NewObjectReference(NewTypeMeta("Connector"), ObjectMeta{
		ID: "cxr_123", Name: "greenhouse", Namespace: "root", Generation: 3,
	})
	require.Equal(t, ObjectReference{
		APIVersion: APIVersionV1Alpha1,
		Kind:       "Connector",
		ID:         "cxr_123",
		Name:       "greenhouse",
		Namespace:  "root",
		Generation: 3,
	}, reference)

	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.FixedZone("offset", 60*60))
	condition := NewCondition("Ready", ConditionTrue, 3, "Configured", "ready", now)
	require.Equal(t, time.UTC, condition.LastTransitionTime.Location())

	later := now.Add(time.Hour)
	updated := UpsertCondition([]Condition{condition}, NewCondition("Ready", ConditionTrue, 4, "StillConfigured", "still ready", later))
	require.Len(t, updated, 1)
	require.Equal(t, condition.LastTransitionTime, updated[0].LastTransitionTime)
	require.Equal(t, uint64(4), updated[0].ObservedGeneration)
}
