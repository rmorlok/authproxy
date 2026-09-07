package client

import (
	"encoding/json"
	"testing"
)

func TestNewTypeMetaUsesV1Alpha1(t *testing.T) {
	meta := NewTypeMeta(ConnectorKind)
	if meta.APIVersion != "authproxy.net/v1alpha1" || meta.Kind != ConnectorKind {
		t.Fatalf("type metadata: %+v", meta)
	}
}

func TestCreateMetadataOmitsServerOwnedFields(t *testing.T) {
	data, err := json.Marshal(ObjectMetadata{Namespace: "root.acme"})
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "generation", "createdAt", "updatedAt"} {
		if _, exists := metadata[field]; exists {
			t.Errorf("server-owned field %q present in create metadata: %s", field, data)
		}
	}
}

func TestNewIDReferenceUsesCanonicalIdentity(t *testing.T) {
	reference := NewIDReference(KeyKind, "key_test")
	if reference.APIVersion != APIVersion || reference.Kind != KeyKind || reference.ID != "key_test" {
		t.Fatalf("object reference: %+v", reference)
	}
}
