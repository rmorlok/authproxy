package client

import (
	"encoding/json"
	"testing"
)

func TestCreateActorRequestUsesCanonicalResourceEnvelope(t *testing.T) {
	req := CreateActorRequest{
		APIVersion: ActorAPIVersion,
		Kind:       ActorKind,
		Metadata: ActorMetadata{
			Namespace: "root.acme",
			Labels:    map[string]string{"team": "platform"},
		},
		Spec: ActorSpec{ExternalID: "user-123"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if body["apiVersion"] != ActorAPIVersion || body["kind"] != ActorKind {
		t.Fatalf("type metadata: %s", data)
	}
	if _, exists := body["externalId"]; exists {
		t.Fatalf("legacy externalId field present at top level: %s", data)
	}
	if _, exists := body["metadata"]; !exists {
		t.Fatalf("metadata missing: %s", data)
	}
	if _, exists := body["spec"]; !exists {
		t.Fatalf("spec missing: %s", data)
	}
}

func TestUpdateActorRequestIncludesRequiredPatchSections(t *testing.T) {
	labels := map[string]string{"team": "security"}
	req := UpdateActorRequest{
		APIVersion: ActorAPIVersion,
		Kind:       ActorKind,
		Metadata:   &ActorMetadataPatch{Labels: &labels},
		Spec:       &ActorSpecPatch{},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if _, exists := body["metadata"]; !exists {
		t.Fatalf("metadata missing: %s", data)
	}
	if spec, exists := body["spec"]; !exists || spec == nil {
		t.Fatalf("non-null spec missing: %s", data)
	}
}
