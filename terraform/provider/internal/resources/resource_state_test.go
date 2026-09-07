package resources

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/terraform/provider/internal/client"
)

func TestSetNamespaceStateMapsMetadataSpecAndStatus(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	namespace := &client.Namespace{
		TypeMeta: client.NewTypeMeta(client.NamespaceKind),
		Metadata: client.ObjectMetadata{
			ID:        "root.acme",
			Name:      "acme",
			Namespace: "root",
			CreatedAt: &now,
			UpdatedAt: &now,
		},
		Spec:   client.NamespaceSpec{EncryptionKeyRef: client.NewIDReference(client.KeyKind, "key_test")},
		Status: &client.NamespaceStatus{State: "active"},
	}
	var model NamespaceResourceModel
	setNamespaceState(&model, namespace)
	if model.Path.ValueString() != "root.acme" || model.State.ValueString() != "active" || model.KeyId.ValueString() != "key_test" {
		t.Fatalf("namespace state: %+v", model)
	}
}

func TestSetKeyStateUsesObservedStateWithoutExposingKeyData(t *testing.T) {
	key := &client.Key{
		TypeMeta: client.NewTypeMeta(client.KeyKind),
		Metadata: client.ObjectMetadata{
			ID:        "key_test",
			Namespace: "root.acme",
		},
		Spec: client.KeySpec{
			DesiredState: "disabled",
			KeyData:      map[string]any{"value": "********"},
		},
		Status: &client.KeyStatus{State: "active", KeyDataConfigured: true},
	}
	var model KeyResourceModel
	setKeyState(&model, key)
	if model.Id.ValueString() != "key_test" || model.Namespace.ValueString() != "root.acme" || model.State.ValueString() != "active" {
		t.Fatalf("key state: %+v", model)
	}
}

func TestSetActorStateMapsMetadataAndSpec(t *testing.T) {
	actor := &client.Actor{
		TypeMeta: client.NewTypeMeta(client.ActorKind),
		Metadata: client.ObjectMetadata{
			ID:        "act_test",
			Namespace: "root.acme",
		},
		Spec: client.ActorSpec{ExternalID: "user-123"},
	}
	var model ActorResourceModel
	setActorState(&model, actor)
	if model.Id.ValueString() != "act_test" || model.Namespace.ValueString() != "root.acme" || model.ExternalId.ValueString() != "user-123" {
		t.Fatalf("actor state: %+v", model)
	}
}
