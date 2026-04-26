package core

import (
	"context"
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// SubmitForm handles form data submission for a connection setup flow step.
// It merges the submitted data into the connection's configuration, advances to the next step,
// and returns the appropriate response (next form, auth redirect, or complete).
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

	// Only preconnect and configure phases accept form submissions
	if !currentSetupStep.Phase().IsIndexed() {
		return nil, httperr.BadRequestf("cannot submit form for phase %q", currentSetupStep.Phase())
	}

	// Look up the current step definition
	currentStep, _, err := connector.SetupFlow.GetStepBySetupStep(currentSetupStep)
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to get current step: %w", err))
	}

	// Get existing configuration to merge into
	existingConfig, err := c.GetConfiguration(ctx)
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to get existing configuration: %w", err))
	}

	// Validate step id, validate data against schema, and merge only allowed fields
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

	// Handle each possible next step
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
		// Transition to auth phase — initiate OAuth flow
		return c.initiateAuthStep(ctx, req.ReturnToUrl, connector)
	}

	// Next step is a form step
	return c.buildFormResponse(ctx, nextStep, connector.SetupFlow)
}

// initiateAuthStep starts the OAuth flow after preconnect steps are complete.
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

	ra := core.GetAuthFromContext(ctx)
	o2 := c.s.getOAuth2Factory().NewOAuth2(c)
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
