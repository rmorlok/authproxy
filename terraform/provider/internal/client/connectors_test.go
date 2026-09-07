package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConnectorRequestsKeepDefinitionInsideSpec(t *testing.T) {
	definition := json.RawMessage(`{"displayName":"Example","logo":{"publicUrl":"https://example.com/logo.png"},"description":"example","auth":{"type":"no-auth"}}`)
	request := CreateConnectorRequest{
		TypeMeta: NewTypeMeta(ConnectorKind),
		Metadata: ObjectMetadata{
			Namespace: "root.acme",
			Labels:    map[string]string{"team": "platform"},
		},
		Spec: ConnectorSpec{
			Release:    ConnectorReleaseSpec{DesiredState: "primary"},
			Definition: definition,
		},
	}
	body := assertCanonicalRequest(t, request, ConnectorKind)
	if _, exists := body["definition"]; exists {
		t.Fatalf("legacy top-level definition present: %+v", body)
	}
	spec := body["spec"].(map[string]any)
	if spec["definition"] == nil {
		t.Fatalf("spec.definition missing: %+v", spec)
	}
	metadata := body["metadata"].(map[string]any)
	definitionBody := spec["definition"].(map[string]any)
	for _, field := range []string{"namespace", "labels", "generation", "state"} {
		if _, exists := definitionBody[field]; exists {
			t.Errorf("resource metadata %q leaked into definition: %+v", field, definitionBody)
		}
	}
	if metadata["namespace"] != "root.acme" {
		t.Fatalf("metadata.namespace: %+v", metadata)
	}
}

func TestGetConnectorTracksRedactedResponseHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/connectors/cxr_test" {
			t.Errorf("path: %s", request.URL.Path)
		}
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("X-AuthProxy-Data-Redacted", "true")
		_, _ = response.Write([]byte(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","metadata":{"id":"cxr_test","generation":1},"spec":{"definition":{}},"status":{"release":{"state":"primary"}}}`))
	}))
	defer server.Close()

	api, err := New(Config{Endpoint: server.URL, BearerToken: "test"})
	if err != nil {
		t.Fatal(err)
	}
	connector, err := api.GetConnector(t.Context(), "cxr_test")
	if err != nil {
		t.Fatal(err)
	}
	if !connector.DataRedacted {
		t.Fatal("redaction response header was not retained")
	}
}

func TestConnectorPatchIncludesRequiredEmptySections(t *testing.T) {
	request := UpdateConnectorRequest{
		TypeMeta: NewTypeMeta(ConnectorKind),
		Metadata: &ObjectMetadataPatch{},
		Spec:     &ConnectorSpecPatch{},
	}
	assertCanonicalRequest(t, request, ConnectorKind)
}

func TestConnectorVersionListUsesCanonicalEnvelope(t *testing.T) {
	data := []byte(`{
		"apiVersion":"authproxy.net/v1alpha1",
		"kind":"ConnectorList",
		"metadata":{"continue":"next"},
		"items":[{"apiVersion":"authproxy.net/v1alpha1","kind":"Connector","metadata":{"id":"cxr_test","generation":2},"spec":{"definition":{}},"status":{"release":{"state":"draft"}}}]
	}`)
	var response ListConnectorVersionsResponse
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	if response.Kind != "ConnectorList" || response.Metadata.Continue != "next" || len(response.Items) != 1 {
		t.Fatalf("list response: %+v", response)
	}
	if response.Items[0].Metadata.Generation != 2 {
		t.Fatalf("generation: %+v", response.Items[0].Metadata)
	}
}

func TestDecodeConnectorDefinitionSummary(t *testing.T) {
	tests := []struct {
		name string
		logo string
		want string
	}{
		{name: "string", logo: `"https://example.com/logo.png"`, want: "https://example.com/logo.png"},
		{name: "object", logo: `{"publicUrl":"https://example.com/logo.png"}`, want: "https://example.com/logo.png"},
		{name: "base64", logo: `{"mimeType":"image/png","base64":"aGVsbG8="}`, want: "data:image/png;base64,aGVsbG8="},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			summary, err := DecodeConnectorDefinitionSummary(json.RawMessage(`{"displayName":"Example","description":"Description","logo":` + test.logo + `}`))
			if err != nil {
				t.Fatal(err)
			}
			if summary.DisplayName != "Example" || summary.Description != "Description" || summary.Logo != test.want {
				t.Fatalf("summary: %+v", summary)
			}
		})
	}
}
