package oauth2

import (
	"context"
	"encoding/json"
	"fmt"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/common/json_schema"
	"github.com/rmorlok/authproxy/internal/schema/common/ui_schema"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

const attrClientId = "client_id"
const attrClientSecret = "client_secret"

func synthesizeClientCredentialsStep(method cschema.TokenEndpointAuthMethod) *cschema.SetupFlowStep {

	js := json_schema.Schema{
		Type:                 "object",
		Required:             []string{attrClientId},
		Properties:           map[string]json_schema.Property{},
		AdditionalProperties: false,
	}
	ui := ui_schema.Schema{Type: "VerticalLayout", Elements: []ui_schema.Control{}}

	js.Properties[attrClientId] = json_schema.Property{
		Type:      "string",
		Title:     "Client ID",
		MinLength: 1,
	}
	ui.Elements = append(ui.Elements, ui_schema.Control{
		Type:  "Control",
		Scope: fmt.Sprintf("#/properties/%s", attrClientId),
	})

	if method != cschema.TokenEndpointAuthNone {
		js.Required = append(js.Required, attrClientSecret)
		js.Properties[attrClientSecret] = json_schema.Property{
			Type:      "string",
			Title:     "Client Secret",
			MinLength: 1,
		}
		ui.Elements = append(ui.Elements, ui_schema.Control{
			Type:    "Control",
			Scope:   fmt.Sprintf("#/properties/%s", attrClientSecret),
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
	if v, ok := credData[attrClientId].(string); ok {
		plaintext.ClientId = v
	}
	if plaintext.ClientId == "" {
		return httperr.BadRequest(fmt.Sprintf("%s is required", attrClientId))
	}

	if v, ok := credData[attrClientSecret].(string); ok {
		plaintext.ClientSecret = v
	}
	if auth.GetTokenEndpointAuthMethodOrDefault() != cschema.TokenEndpointAuthNone && plaintext.ClientSecret == "" {
		return httperr.BadRequest(fmt.Sprintf("%s is required", attrClientSecret))
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
