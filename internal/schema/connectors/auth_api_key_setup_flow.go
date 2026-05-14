package connectors

import (
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/schema/common"
)

// SynthesizedApiKeyCredentialsStepId is the id assigned to the auto-generated
// credentials step for api-key connectors that did not declare their own
// setup_flow.credentials.
const SynthesizedApiKeyCredentialsStepId = "_authproxy_api_key_credentials"

// SynthesizeApiKeyCredentialsStep builds a credential-collection SetupFlowStep
// for an api-key placement, lives in setup_flow.credentials. The returned step
// has a JSON Schema requiring "api_key" (plus the placement's UsernameField
// for basic auth) and a JSONForms UI Schema rendering each field — api_key is
// rendered with a password input so the value is masked.
//
// Returns nil if placement is nil.
func SynthesizeApiKeyCredentialsStep(placement *ApiKeyPlacement) *SetupFlowStep {
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

	if placement.Type == ApiKeyPlacementBasic && placement.UsernameField != "" {
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
		panic("connectors: failed to marshal synthesized api-key json_schema: " + err.Error())
	}
	uiBytes, err := json.Marshal(ui)
	if err != nil {
		panic("connectors: failed to marshal synthesized api-key ui_schema: " + err.Error())
	}

	return &SetupFlowStep{
		Id:          SynthesizedApiKeyCredentialsStepId,
		Title:       "Enter your API key",
		Description: "Provide the API key used to authenticate with this service.",
		JsonSchema:  common.RawJSON(jsBytes),
		UiSchema:    common.RawJSON(uiBytes),
	}
}
