package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
)

// SubmitForm handles a form submission against the connection's current
// setup step. Dispatch is uniform — the manifest tells the dispatcher which
// step is active, the step's OnSubmit closure handles the data (schema-step
// merges into config; auth-method-step delegates to the auth method's
// persistence), and the dispatcher then transitions to the next step (or
// completes the flow).
//
// Auth-method-emitted redirect steps cannot accept a form submission;
// dispatcher rejects with a 400 before calling OnSubmit.
func (c *connection) SubmitForm(ctx context.Context, req iface.SubmitConnectionRequest) (iface.ConnectionSetupResponse, error) {
	setupStep := c.GetSetupStep()
	if err := ensureStepMatches(setupStep, req.StepId); err != nil {
		return nil, err
	}

	flow := c.s.buildManifestSetupFlow(c)
	current, ok, err := flow.StepById(ctx, setupStep.Id())
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to evaluate setup flow: %w", err))
	}
	if !ok {
		return nil, httperr.InternalServerErrorMsg("current setup step is not addressable in manifest")
	}
	if current.Type() != iface.ManifestStepTypeForm {
		return nil, httperr.BadRequestf("step %q does not accept form submissions", current.Id())
	}

	if err := current.OnSubmit(ctx, req.Data); err != nil {
		return nil, err
	}

	next, hasNext, err := flow.NextStep(ctx, current.Id())
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to evaluate setup flow: %w", err))
	}
	if !hasNext {
		return c.completeFlow(ctx)
	}
	return c.advanceToStep(ctx, next, flow, req.ReturnToUrl)
}

// GetCurrentSetupStepResponse renders the response for the connection's
// current setup_step without transitioning anything. Used by the resume /
// polling path on the UI.
func (c *connection) GetCurrentSetupStepResponse(ctx context.Context) (iface.ConnectionSetupResponse, error) {
	setupStep := c.GetSetupStep()
	if setupStep == nil {
		return &iface.ConnectionSetupComplete{
			Id:   c.GetId(),
			Type: iface.ConnectionSetupResponseTypeComplete,
		}, nil
	}
	flow := c.s.buildManifestSetupFlow(c)
	return c.renderResumeResponse(ctx, flow, *setupStep)
}
