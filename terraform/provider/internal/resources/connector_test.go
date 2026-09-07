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
	equal, diagnostics := model.Definition.StringSemanticEquals(
		context.Background(),
		jsontypes.NewNormalizedValue(string(definition)),
	)
	if diagnostics.HasError() || !equal {
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

func TestSetConnectorStatePreservesOmittedAPINullFields(t *testing.T) {
	plannedDefinition := `{"displayName":"Example","description":"Example connector","auth":{"type":"no-auth"}}`
	model := ConnectorResourceModel{
		Definition: jsontypes.NewNormalizedValue(plannedDefinition),
	}
	connector := &client.Connector{
		Metadata: client.ObjectMetadata{ID: "cxr_test", Namespace: "root", Generation: 1},
		Spec: client.ConnectorSpec{
			Definition: json.RawMessage(`{"displayName":"Example","logo":null,"description":"Example connector","auth":{"type":"no-auth"}}`),
		},
		Status: &client.ConnectorStatus{Release: client.ConnectorReleaseStatus{State: "primary"}},
	}

	setConnectorState(&model, connector)

	equal, diagnostics := model.Definition.StringSemanticEquals(
		context.Background(),
		jsontypes.NewNormalizedValue(plannedDefinition),
	)
	if diagnostics.HasError() || !equal {
		t.Fatalf("API-added null field changed Terraform state: %s", model.Definition.ValueString())
	}
}

func TestSetConnectorStateRemovesAPINullFieldsDuringImport(t *testing.T) {
	connector := &client.Connector{
		Metadata: client.ObjectMetadata{ID: "cxr_test", Namespace: "root", Generation: 1},
		Spec: client.ConnectorSpec{
			Definition: json.RawMessage(`{"displayName":"Example","logo":null,"description":"Example connector","auth":{"type":"no-auth"}}`),
		},
		Status: &client.ConnectorStatus{Release: client.ConnectorReleaseStatus{State: "primary"}},
	}
	var model ConnectorResourceModel

	setConnectorState(&model, connector)

	want := jsontypes.NewNormalizedValue(`{"displayName":"Example","description":"Example connector","auth":{"type":"no-auth"}}`)
	equal, diagnostics := model.Definition.StringSemanticEquals(context.Background(), want)
	if diagnostics.HasError() || !equal {
		t.Fatalf("API-added null field remained in imported state: %s", model.Definition.ValueString())
	}
}

func TestReconcileConnectorDefinitionRetainsObservableDrift(t *testing.T) {
	observed := []byte(`{"displayName":"Example","logo":null,"description":"changed","auth":{"type":"no-auth"}}`)
	prior := []byte(`{"displayName":"Example","description":"original","auth":{"type":"no-auth"}}`)
	merged, err := reconcileConnectorDefinitionJSON(observed, prior, false)
	if err != nil {
		t.Fatal(err)
	}

	want := jsontypes.NewNormalizedValue(`{"displayName":"Example","description":"changed","auth":{"type":"no-auth"}}`)
	equal, diagnostics := jsontypes.NewNormalizedValue(string(merged)).StringSemanticEquals(context.Background(), want)
	if diagnostics.HasError() || !equal {
		t.Fatalf("observable drift was lost: %s", merged)
	}
}

func TestReconcileConnectorDefinitionPreservesJSONSchemaNulls(t *testing.T) {
	observed := []byte(`{
		"displayName":"Example",
		"logo":null,
		"auth":{"type":"no-auth"},
		"setupFlow":{"configure":{"steps":[{
			"type":"user-input",
			"jsonSchema":{"properties":{"optional":{"default":null}}}
		}]}}
	}`)
	merged, err := reconcileConnectorDefinitionJSON(observed, []byte(`{}`), false)
	if err != nil {
		t.Fatal(err)
	}

	var definition map[string]any
	if err := json.Unmarshal(merged, &definition); err != nil {
		t.Fatal(err)
	}
	if _, exists := definition["logo"]; exists {
		t.Fatalf("API-added logo null remained in state: %s", merged)
	}
	setupFlow := definition["setupFlow"].(map[string]any)
	configure := setupFlow["configure"].(map[string]any)
	steps := configure["steps"].([]any)
	jsonSchema := steps[0].(map[string]any)["jsonSchema"].(map[string]any)
	properties := jsonSchema["properties"].(map[string]any)
	optional := properties["optional"].(map[string]any)
	defaultValue, exists := optional["default"]
	if !exists || defaultValue != nil {
		t.Fatalf("meaningful JSON Schema null was not preserved: %s", merged)
	}
}
