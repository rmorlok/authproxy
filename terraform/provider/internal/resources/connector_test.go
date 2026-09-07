package resources

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/rmorlok/authproxy/terraform/provider/internal/client"
)

func TestSetConnectorStateMapsCanonicalResourceWithoutDefinitionStripping(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	definition := json.RawMessage(`{
		"displayName":"Example",
		"logo":{"publicUrl":"https://example.com/logo.png"},
		"description":"Example connector",
		"auth":{"type":"no-auth"}
	}`)
	connector := &client.Connector{
		TypeMeta: client.NewTypeMeta(client.ConnectorKind),
		Metadata: client.ObjectMetadata{
			ID:          "cxr_test",
			Namespace:   "root.acme",
			Generation:  3,
			Labels:      map[string]string{"team": "platform"},
			Annotations: map[string]string{"owner": "integrations"},
			CreatedAt:   &now,
			UpdatedAt:   &now,
		},
		Spec: client.ConnectorSpec{
			Release:    client.ConnectorReleaseSpec{DesiredState: "primary"},
			Definition: definition,
		},
		Status: &client.ConnectorStatus{Release: client.ConnectorReleaseStatus{State: "primary"}},
	}

	model := ConnectorResourceModel{Publish: types.BoolValue(true)}
	setConnectorState(&model, connector)

	if model.Id.ValueString() != "cxr_test" || model.Version.ValueInt64() != 3 || !model.Publish.ValueBool() {
		t.Fatalf("identity/release mapping: %+v", model)
	}
	if model.State.ValueString() != "primary" || model.DisplayName.ValueString() != "Example" {
		t.Fatalf("status/definition mapping: %+v", model)
	}
	if !model.Definition.Equal(jsontypes.NewNormalizedValue(string(definition))) {
		t.Fatalf("definition changed while mapping to state: %s", model.Definition.ValueString())
	}
	var stateDefinition map[string]any
	if err := json.Unmarshal([]byte(model.Definition.ValueString()), &stateDefinition); err != nil {
		t.Fatal(err)
	}
	if _, exists := stateDefinition["logo"]; !exists {
		t.Fatalf("provider-owned logo removed from spec.definition: %+v", stateDefinition)
	}
	var labels map[string]string
	model.Labels.ElementsAs(context.Background(), &labels, false)
	if labels["team"] != "platform" {
		t.Fatalf("labels: %+v", labels)
	}
}

func TestConnectorDefinitionAttributeIsSensitive(t *testing.T) {
	var response resource.SchemaResponse
	(&ConnectorResource{}).Schema(t.Context(), resource.SchemaRequest{}, &response)
	definition, ok := response.Schema.Attributes["definition"].(resourceschema.StringAttribute)
	if !ok || !definition.Sensitive {
		t.Fatalf("connector definition must be a sensitive string attribute: %#v", response.Schema.Attributes["definition"])
	}
}

func TestSetConnectorStatePreservesDraftPublishPreference(t *testing.T) {
	connector := &client.Connector{
		Metadata: client.ObjectMetadata{ID: "cxr_test", Namespace: "root", Generation: 1},
		Spec: client.ConnectorSpec{
			Release:    client.ConnectorReleaseSpec{DesiredState: "draft"},
			Definition: json.RawMessage(`{"displayName":"Draft"}`),
		},
		Status: &client.ConnectorStatus{Release: client.ConnectorReleaseStatus{State: "draft"}},
	}
	model := ConnectorResourceModel{Publish: types.BoolValue(false)}
	setConnectorState(&model, connector)
	if model.Publish.ValueBool() || model.State.ValueString() != "draft" {
		t.Fatalf("draft release mapping: %+v", model)
	}
}

func TestSetConnectorStatePreservesProviderPublishPreference(t *testing.T) {
	connector := &client.Connector{
		Metadata: client.ObjectMetadata{ID: "cxr_test", Namespace: "root", Generation: 1},
		Spec: client.ConnectorSpec{
			Release:    client.ConnectorReleaseSpec{DesiredState: "primary"},
			Definition: json.RawMessage(`{"displayName":"Published"}`),
		},
		Status: &client.ConnectorStatus{Release: client.ConnectorReleaseStatus{State: "primary"}},
	}
	model := ConnectorResourceModel{Publish: types.BoolValue(false)}
	setConnectorState(&model, connector)
	if model.Publish.ValueBool() {
		t.Fatal("API release state must not overwrite the provider-local publish preference")
	}
}

func TestConnectorMetadataPatchPreservesClearAndOmission(t *testing.T) {
	empty := map[string]string{}
	patch := connectorMetadataPatch(empty, nil)
	if patch.Labels == nil || len(*patch.Labels) != 0 {
		t.Fatalf("empty labels must remain an explicit clear: %+v", patch)
	}
	if patch.Annotations != nil {
		t.Fatalf("nil annotations must be omitted: %+v", patch)
	}
}

func TestPreserveRedactedJSONRetainsPriorSecrets(t *testing.T) {
	observed := []byte(`{"auth":{"clientSecret":"*************"},"description":"updated","scopes":[{"token":"*****"}]}`)
	prior := []byte(`{"auth":{"clientSecret":"client-secret"},"description":"old","scopes":[{"token":"token"}]}`)
	merged, err := preserveRedactedJSON(observed, prior)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(merged, &value); err != nil {
		t.Fatal(err)
	}
	auth := value["auth"].(map[string]any)
	if auth["clientSecret"] != "client-secret" {
		t.Fatalf("secret was not retained: %s", merged)
	}
	if value["description"] != "updated" {
		t.Fatalf("observable drift was lost: %s", merged)
	}
	scopes := value["scopes"].([]any)
	if scopes[0].(map[string]any)["token"] != "token" {
		t.Fatalf("nested secret was not retained: %s", merged)
	}
}

func TestSetConnectorStateRetainsSecretFromPriorStateOnRedactedRead(t *testing.T) {
	model := ConnectorResourceModel{
		Definition: jsontypes.NewNormalizedValue(`{"displayName":"Example","auth":{"type":"OAuth2","clientSecret":"client-secret"}}`),
	}
	connector := &client.Connector{
		Metadata: client.ObjectMetadata{ID: "cxr_test", Namespace: "root", Generation: 1},
		Spec: client.ConnectorSpec{
			Release:    client.ConnectorReleaseSpec{DesiredState: "primary"},
			Definition: json.RawMessage(`{"displayName":"Example","auth":{"type":"OAuth2","clientSecret":"*************"}}`),
		},
		Status:       &client.ConnectorStatus{Release: client.ConnectorReleaseStatus{State: "primary"}},
		DataRedacted: true,
	}
	setConnectorState(&model, connector)
	var definition struct {
		Auth struct {
			ClientSecret string `json:"clientSecret"`
		} `json:"auth"`
	}
	if err := json.Unmarshal([]byte(model.Definition.ValueString()), &definition); err != nil {
		t.Fatal(err)
	}
	if definition.Auth.ClientSecret != "client-secret" {
		t.Fatalf("redacted response replaced prior secret: %q", definition.Auth.ClientSecret)
	}
}
