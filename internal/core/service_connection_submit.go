package core

import (
	"context"
	"encoding/json"

	apauthcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// SubmitForm handles form data submission for a connection setup flow step.
//
// Dispatch is by phase:
//   - preconnect / configure: validate, merge submitted fields into the
//     connection's EncryptedConfiguration, advance.
//   - credentials: validate, route to the auth method's credential-submit
//     handler (which encrypts + persists into api_key_credentials), advance via
//     HandleCredentialsEstablished. Field data never enters EncryptedConfiguration.
func (c *connection) SubmitForm(ctx context.Context, req iface.SubmitConnectionRequest) (iface.ConnectionSetupResponse, error) {
	setupStep := c.GetSetupStep()
	if setupStep == nil {
		return nil, httperr.BadRequest("connection has no active setup step")
	}

	connector := c.cv.GetDefinition()
	if connector.SetupFlow == nil {
		return nil, httperr.BadRequest("connector has no setup flow")
	}

	currentSetupStep := *setupStep

	// Only the indexed phases (preconnect, credentials, configure) accept form submissions.
	if !currentSetupStep.Phase().IsIndexed() {
		return nil, httperr.BadRequestf("cannot submit form for phase %q", currentSetupStep.Phase())
	}

	// Look up the current step definition
	currentStep, _, err := connector.SetupFlow.GetStepBySetupStep(currentSetupStep)
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to get current step: %w", err))
	}

	// Credential submissions are dispatched to the auth method — no merge into
	// the general config blob, no risk of field-name collisions with preconnect.
	if currentSetupStep.Phase() == cschema.SetupPhaseCredentials {
		return c.submitCredentialsStep(ctx, req, currentStep, connector)
	}

	// Preconnect / configure submissions follow the generic merge path.
	existingConfig, err := c.GetConfiguration(ctx)
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to get existing configuration: %w", err))
	}

	mergedConfig, err := currentStep.ValidateAndMergeData(req.StepId, req.Data, existingConfig)
	if err != nil {
		return nil, httperr.BadRequest(err.Error())
	}

	if err := c.SetConfiguration(ctx, mergedConfig); err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to save configuration: %w", err))
	}

	// Determine the next step
	nextStep, err := connector.SetupFlow.NextSetupStep(currentSetupStep, connector.HasProbes())
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to determine next step: %w", err))
	}

	if nextStep.IsZero() {
		// Flow complete
		if err := c.SetSetupStep(ctx, nil); err != nil {
			return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to clear setup step: %w", err))
		}
		if err := c.SetState(ctx, database.ConnectionStateReady); err != nil {
			return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to set connection state to ready: %w", err))
		}
		return &iface.ConnectionSetupComplete{
			Id:   c.GetId(),
			Type: iface.ConnectionSetupResponseTypeComplete,
		}, nil
	}

	if nextStep.Equals(cschema.SetupStepAuth) {
		// OAuth2-only redirect step. (api-key never reaches here — its
		// preconnect transitions into the credentials phase instead.)
		return c.initiateAuthStep(ctx, req.ReturnToUrl, connector)
	}

	// Next step is a form step (could be another preconnect, the credentials
	// phase, or a configure step).
	if nextStep.Phase() == cschema.SetupPhaseCredentials && req.ReturnToUrl == "" {
		// Credential steps don't need a returnTo, but log nothing — just build the form.
	}
	return c.buildFormResponse(ctx, nextStep, connector.SetupFlow)
}

// submitCredentialsStep handles a submit against a step in the credentials
// phase. The data is validated against the step's JSON Schema and then handed
// to the auth method, which is responsible for materializing the credential
// (encrypting and persisting). Plaintext field data never lands in the
// connection's general EncryptedConfiguration.
//
// After the auth method persists the credential, HandleCredentialsEstablished
// advances the connection to verify / configure / ready — the same code path
// OAuth2 takes after its callback.
func (c *connection) submitCredentialsStep(
	ctx context.Context,
	req iface.SubmitConnectionRequest,
	step *cschema.SetupFlowStep,
	connector *cschema.Connector,
) (iface.ConnectionSetupResponse, error) {
	// Step-id and schema validation only — we explicitly DO NOT merge into the
	// connection config. Calling ValidateAndMergeData with a nil config map
	// returns a fresh map containing the validated submitted fields, which we
	// hand to the auth method and then discard.
	credData, err := step.ValidateAndMergeData(req.StepId, req.Data, nil)
	if err != nil {
		return nil, httperr.BadRequest(err.Error())
	}

	if connector.Auth == nil {
		return nil, httperr.InternalServerErrorMsg("connector has no auth configuration")
	}

	switch auth := connector.Auth.Inner().(type) {
	case *config.AuthApiKey:
		if err := c.persistApiKeyCredentials(ctx, credData, auth.Placement); err != nil {
			return nil, err
		}
	default:
		return nil, httperr.InternalServerErrorMsg("connector auth type does not accept credentials submissions")
	}

	if _, err := c.HandleCredentialsEstablished(ctx); err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to advance after credentials submission: %w", err))
	}
	return c.GetCurrentSetupStepResponse(ctx)
}

// persistApiKeyCredentials extracts the api-key credential field values from
// credData, encrypts the resulting plaintext as a single JSON blob, and inserts
// it into api_key_credentials.
func (c *connection) persistApiKeyCredentials(
	ctx context.Context,
	credData map[string]any,
	placement *cschema.ApiKeyPlacement,
) error {
	if placement == nil {
		return httperr.InternalServerErrorMsg("api-key connector missing placement at credential submission time")
	}

	plaintext := database.ApiKeyCredentialPlaintext{}
	if v, ok := credData["api_key"].(string); ok {
		plaintext.ApiKey = v
	}
	if plaintext.ApiKey == "" {
		return httperr.BadRequest("api_key is required")
	}
	if placement.Type == cschema.ApiKeyPlacementBasic {
		if placement.UsernameField == "" {
			return httperr.InternalServerErrorMsg("basic placement missing username_field at credential submission time")
		}
		v, _ := credData[placement.UsernameField].(string)
		if v == "" {
			return httperr.BadRequestf("%q is required for basic placement", placement.UsernameField)
		}
		plaintext.Username = v
	}

	blobJSON, err := json.Marshal(plaintext)
	if err != nil {
		return httperr.InternalServerError(httperr.WithInternalErrorf("failed to marshal api-key plaintext: %w", err))
	}
	encrypted, err := c.s.encrypt.EncryptStringForNamespace(ctx, c.Namespace, string(blobJSON))
	if err != nil {
		return httperr.InternalServerError(httperr.WithInternalErrorf("failed to encrypt api-key credentials: %w", err))
	}

	actorId := apauthcore.GetAuthFromContext(ctx).MustGetActor().GetId()
	if _, err := c.s.db.InsertApiKeyCredential(ctx, c.Id, encrypted, placement, &actorId); err != nil {
		return httperr.InternalServerError(httperr.WithInternalErrorf("failed to persist api-key credentials: %w", err))
	}
	return nil
}

// initiateAuthStep starts the OAuth flow after preconnect steps are complete.
// Only OAuth2 connectors reach this path — api-key uses the credentials phase
// instead, and other auth types are rejected.
func (c *connection) initiateAuthStep(ctx context.Context, returnToUrl string, connector *cschema.Connector) (iface.ConnectionSetupResponse, error) {
	if returnToUrl == "" {
		return nil, httperr.BadRequest("return_to_url is required for auth step")
	}

	if connector.Auth == nil {
		return nil, httperr.InternalServerErrorMsg("connector has no auth configuration")
	}

	if _, ok := connector.Auth.Inner().(*config.AuthOAuth2); !ok {
		return nil, httperr.InternalServerErrorMsg("unsupported connector auth type for setup flow")
	}

	if err := c.SetSetupStep(ctx, &cschema.SetupStepAuth); err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to set setup step to auth: %w", err))
	}

	ra := apauthcore.GetAuthFromContext(ctx)
	o2 := c.s.getAuthMethodFactory(connector).(oauth2.Factory).NewOAuth2(c)
	url, err := o2.SetStateAndGeneratePublicUrl(ctx, ra.MustGetActor(), returnToUrl)
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to generate OAuth redirect URL: %w", err))
	}

	return &iface.ConnectionSetupRedirect{
		Id:          c.GetId(),
		Type:        iface.ConnectionSetupResponseTypeRedirect,
		RedirectUrl: url,
	}, nil
}

// buildFormResponse creates a form response for the given setup step.
func (c *connection) buildFormResponse(ctx context.Context, setupStep cschema.SetupStep, sf *cschema.SetupFlow) (iface.ConnectionSetupResponse, error) {
	step, globalIndex, err := sf.GetStepBySetupStep(setupStep)
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to get step definition: %w", err))
	}

	if err := c.SetSetupStep(ctx, &setupStep); err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to update setup step: %w", err))
	}

	return &iface.ConnectionSetupForm{
		Id:              c.GetId(),
		Type:            iface.ConnectionSetupResponseTypeForm,
		StepId:          step.Id,
		StepTitle:       step.Title,
		StepDescription: step.Description,
		CurrentStep:     globalIndex,
		TotalSteps:      sf.TotalSteps(),
		JsonSchema:      json.RawMessage(step.JsonSchema),
		UiSchema:        json.RawMessage(step.UiSchema),
	}, nil
}

// GetCurrentSetupStepResponse returns the response for the current setup step,
// allowing the UI to resume an interrupted flow.
func (c *connection) GetCurrentSetupStepResponse(ctx context.Context) (iface.ConnectionSetupResponse, error) {
	setupStep := c.GetSetupStep()
	if setupStep == nil {
		return &iface.ConnectionSetupComplete{
			Id:   c.GetId(),
			Type: iface.ConnectionSetupResponseTypeComplete,
		}, nil
	}

	connector := c.cv.GetDefinition()
	if connector.SetupFlow == nil {
		return &iface.ConnectionSetupComplete{
			Id:   c.GetId(),
			Type: iface.ConnectionSetupResponseTypeComplete,
		}, nil
	}

	parsed := *setupStep

	switch parsed.Phase() {
	case cschema.SetupPhaseAuth:
		// The connection is waiting for the OAuth callback — tell the UI
		return &iface.ConnectionSetupRedirect{
			Id:   c.GetId(),
			Type: iface.ConnectionSetupResponseTypeRedirect,
		}, nil
	case cschema.SetupPhaseVerify:
		return &iface.ConnectionSetupVerifying{
			Id:   c.GetId(),
			Type: iface.ConnectionSetupResponseTypeVerifying,
		}, nil
	case cschema.SetupPhaseVerifyFailed, cschema.SetupPhaseAuthFailed:
		msg := ""
		if e := c.GetSetupError(); e != nil {
			msg = *e
		}
		return &iface.ConnectionSetupError{
			Id:       c.GetId(),
			Type:     iface.ConnectionSetupResponseTypeError,
			Error:    msg,
			CanRetry: true,
		}, nil
	}

	return c.buildFormResponse(ctx, parsed, connector.SetupFlow)
}
