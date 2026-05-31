package api_key

import (
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/common/json_schema"
	"github.com/rmorlok/authproxy/internal/schema/common/ui_schema"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// SynthesizedApiKeyCredentialsStepId is the canonical id assigned to the
// auto-generated credentials step for api-key connectors. Today every api-key
// connection's credential step uses this id; once #367 introduces
// user-authored credential steps as a first-class kind, this id remains the
// default fallback.
const SynthesizedApiKeyCredentialsStepId = "_authproxy_api_key_credentials"

// synthesizeCredentialsStep builds a credential-collection form spec for an
// api-key placement. The returned SetupFlowStep is not stored in any
// connector definition — it is materialized at runtime by the api-key
// factory's ManifestSetupSteps and discarded after the credential is
// submitted.
//
// The JSON Schema requires "api_key" plus, for basic placements, the
// configured username field. The JSONForms UI Schema renders each field;
// "api_key" uses a password input so the value is masked.
//
// Returns nil if placement is nil.
func synthesizeCredentialsStep(placement *cschema.ApiKeyPlacement) *cschema.SetupFlowStep {
	if placement == nil {
		return nil
	}

	js := json_schema.Schema{
		Type:                 "object",
		Required:             []string{},
		Properties:           map[string]json_schema.Property{},
		AdditionalProperties: false,
	}
	ui := ui_schema.Schema{Type: "VerticalLayout", Elements: []ui_schema.Control{}}

	if placement.Type == cschema.ApiKeyPlacementBasic && placement.UsernameField != "" {
		js.Required = append(js.Required, placement.UsernameField)
		js.Properties[placement.UsernameField] = json_schema.Property{
			Type:      "string",
			Title:     "Username",
			MinLength: 1,
		}
		ui.Elements = append(ui.Elements, ui_schema.Control{
			Type:  "Control",
			Scope: "#/properties/" + placement.UsernameField,
		})
	}

	js.Required = append(js.Required, "api_key")
	js.Properties["api_key"] = json_schema.Property{
		Type:      "string",
		Title:     "API Key",
		MinLength: 1,
	}
	ui.Elements = append(ui.Elements, ui_schema.Control{
		Type:    "Control",
		Scope:   "#/properties/api_key",
		Options: map[string]string{"format": "password"},
	})

	jsBytes, err := json.Marshal(js)
	if err != nil {
		// json.Marshal on these inline types cannot fail; if it ever does, panic
		// is appropriate since this is config-load-time code with no recovery.
		panic("api_key: failed to marshal synthesized json_schema: " + err.Error())
	}
	uiBytes, err := json.Marshal(ui)
	if err != nil {
		panic("api_key: failed to marshal synthesized ui_schema: " + err.Error())
	}

	return &cschema.SetupFlowStep{
		Id:          SynthesizedApiKeyCredentialsStepId,
		Title:       "Enter your API key",
		Description: "Provide the API key used to authenticate with this service.",
		JsonSchema:  common.RawJSON(jsBytes),
		UiSchema:    common.RawJSON(uiBytes),
	}
}
