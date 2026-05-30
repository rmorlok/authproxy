package oauth2

import (
	"context"
	"encoding/json"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

func synthesizeClientCredentialsStep(method cschema.TokenEndpointAuthMethod) *cschema.SetupFlowStep {
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
		Required:             []string{"client_id"},
		Properties:           map[string]schemaProp{},
		AdditionalProperties: false,
	}
	ui := uiSchema{Type: "VerticalLayout", Elements: []uiControl{}}

	js.Properties["client_id"] = schemaProp{
		Type:      "string",
		Title:     "Client ID",
		MinLength: 1,
	}
	ui.Elements = append(ui.Elements, uiControl{
		Type:  "Control",
		Scope: "#/properties/client_id",
	})

	if method != cschema.TokenEndpointAuthNone {
		js.Required = append(js.Required, "client_secret")
		js.Properties["client_secret"] = schemaProp{
			Type:      "string",
			Title:     "Client Secret",
			MinLength: 1,
		}
		ui.Elements = append(ui.Elements, uiControl{
			Type:    "Control",
			Scope:   "#/properties/client_secret",
			Options: map[string]string{"format": "password"},
		})
	}

	jsBytes, err := json.Marshal(js)
	if err != nil {
		panic("oauth2: failed to marshal synthesized client credentials json_schema: " + err.Error())
	}
	uiBytes, err := json.Marshal(ui)
	if err != nil {
		panic("oauth2: failed to marshal synthesized client credentials ui_schema: " + err.Error())
	}

	return &cschema.SetupFlowStep{
		Id:          OAuth2ClientCredentialsStepId,
		Title:       "Enter OAuth client credentials",
		Description: "Provide the client credentials used to authenticate with this service.",
		JsonSchema:  common.RawJSON(jsBytes),
		UiSchema:    common.RawJSON(uiBytes),
	}
}

func (f *factory) PersistClientCredentials(
	ctx context.Context,
	connection coreIface.Connection,
	auth *cschema.AuthOAuth2,
	credData map[string]any,
) error {
	plaintext := database.OAuth2ClientCredentialsPlaintext{}
	if v, ok := credData["client_id"].(string); ok {
		plaintext.ClientId = v
	}
	if plaintext.ClientId == "" {
		return httperr.BadRequest("client_id is required")
	}

	if v, ok := credData["client_secret"].(string); ok {
		plaintext.ClientSecret = v
	}
	if auth.GetTokenEndpointAuthMethodOrDefault() != cschema.TokenEndpointAuthNone && plaintext.ClientSecret == "" {
		return httperr.BadRequest("client_secret is required")
	}

	blobJSON, err := json.Marshal(plaintext)
	if err != nil {
		return httperr.InternalServerError(httperr.WithInternalErrorf("failed to marshal OAuth2 client credentials: %w", err))
	}
	encrypted, err := f.encrypt.EncryptStringForNamespace(ctx, connection.GetNamespace(), string(blobJSON))
	if err != nil {
		return httperr.InternalServerError(httperr.WithInternalErrorf("failed to encrypt OAuth2 client credentials: %w", err))
	}

	actorId := apauthcore.GetAuthFromContext(ctx).MustGetActor().GetId()
	if _, err := f.db.InsertApiKeyCredential(ctx, connection.GetId(), encrypted, nil, &actorId); err != nil {
		return httperr.InternalServerError(httperr.WithInternalErrorf("failed to persist OAuth2 client credentials: %w", err))
	}
	return nil
}
