package client

import (
	"encoding/json"
	"testing"
)

func TestNamespaceMetadataForPath(t *testing.T) {
	tests := []struct {
		path      string
		name      string
		namespace string
	}{
		{path: "root", name: "root"},
		{path: "root.acme", name: "acme", namespace: "root"},
		{path: "root.acme.prod", name: "prod", namespace: "root.acme"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			metadata := NamespaceMetadataForPath(test.path)
			if metadata.Name != test.name || metadata.Namespace != test.namespace {
				t.Fatalf("metadata: %+v", metadata)
			}
		})
	}
}

func TestNamespaceRequestsUseCanonicalResourceEnvelopes(t *testing.T) {
	create := CreateNamespaceRequest{
		TypeMeta: NewTypeMeta(NamespaceKind),
		Metadata: ObjectMetadata{
			Name:      "acme",
			Namespace: "root",
		},
		Spec: NamespaceSpec{},
	}
	assertCanonicalRequest(t, create, NamespaceKind)

	labels := map[string]string{"team": "platform"}
	update := UpdateNamespaceRequest{
		TypeMeta: NewTypeMeta(NamespaceKind),
		Metadata: &ObjectMetadataPatch{Labels: &labels},
		Spec:     &NamespaceSpecPatch{},
	}
	assertCanonicalRequest(t, update, NamespaceKind)
}

func assertCanonicalRequest(t *testing.T, value any, kind string) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if body["apiVersion"] != APIVersion || body["kind"] != kind {
		t.Fatalf("type metadata: %s", data)
	}
	if body["metadata"] == nil || body["spec"] == nil {
		t.Fatalf("metadata and spec must be non-null: %s", data)
	}
	return body
}
