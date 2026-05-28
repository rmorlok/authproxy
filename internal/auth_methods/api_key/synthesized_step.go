package api_key

import (
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/schema/common"
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

	type schemaProp struct {
		Type      string `json:"type"`
		Title     string `json:"title,omitempty"`
		MinLength int    `json:"minLength,omitempty"`
	}
	type schema struct {
		Type                 string                `json:"type"`
		Required             []string              `json:"required"`
		Properties           map[string]schemaProp `json:"properties"`
		AdditionalProperties bool                  `json:"additionalProperties"`
	}
	type uiControl struct {
		Type    string            `json:"type"`
		Scope   string            `json:"scope"`
		Options map[string]string `json:"options,omitempty"`
	}
	type uiSchema struct {
		Type     string      `json:"type"`
		Elements []uiControl `json:"elements"`
	}

	js := schema{
		Type:                 "object",
		Required:             []string{},
		Properties:           map[string]schemaProp{},
		AdditionalProperties: false,
	}
	ui := uiSchema{Type: "VerticalLayout", Elements: []uiControl{}}

	if placement.Type == cschema.ApiKeyPlacementBasic && placement.UsernameField != "" {
		js.Required = append(js.Required, placement.UsernameField)
		js.Properties[placement.UsernameField] = schemaProp{
			Type:      "string",
			Title:     "Username",
			MinLength: 1,
		}
		ui.Elements = append(ui.Elements, uiControl{
			Type:  "Control",
			Scope: "#/properties/" + placement.UsernameField,
		})
	}

	js.Required = append(js.Required, "api_key")
	js.Properties["api_key"] = schemaProp{
		Type:      "string",
		Title:     "API Key",
		MinLength: 1,
	}
	ui.Elements = append(ui.Elements, uiControl{
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
