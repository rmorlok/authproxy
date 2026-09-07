package client

import "testing"

func TestKeyRequestsPutDesiredStateAndKeyDataInSpec(t *testing.T) {
	create := CreateKeyRequest{
		TypeMeta: NewTypeMeta(KeyKind),
		Metadata: ObjectMetadata{
			Namespace: "root.acme",
		},
		Spec: KeySpec{
			DesiredState: "active",
			KeyData:      map[string]any{"numBytes": float64(32)},
		},
	}
	body := assertCanonicalRequest(t, create, KeyKind)
	if _, exists := body["keyData"]; exists {
		t.Fatalf("legacy top-level keyData present: %+v", body)
	}
	spec := body["spec"].(map[string]any)
	if spec["desiredState"] != "active" || spec["keyData"] == nil {
		t.Fatalf("key spec: %+v", spec)
	}

	disabled := "disabled"
	update := UpdateKeyRequest{
		TypeMeta: NewTypeMeta(KeyKind),
		Metadata: &ObjectMetadataPatch{},
		Spec:     &KeySpecPatch{DesiredState: &disabled},
	}
	assertCanonicalRequest(t, update, KeyKind)
}
