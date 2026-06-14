package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// advanceToStep transitions setup_step to next and returns the response the
// API surface renders to the caller. Three special cases beyond plain form
// rendering:
//
//   - next is apxy:verify: enqueue the verify task and return Verifying.
//   - next is a redirect step: call RenderRedirect with the supplied return-
//     to-url and return Redirect.
//   - next is nil: flow is complete — clear setup_step, transition state to
//     Configured, return Complete.
//
// This is the single shared transition path used by InitiateConnection,
// SubmitForm, ReauthConnection, RetryConnectionSetup, Reconfigure, and
// HandleCredentialsEstablished.
func (c *connection) advanceToStep(
	ctx context.Context,
	next iface.ManifestSetupStep,
	flow iface.ManifestSetupFlow,
	returnToUrl string,
) (iface.ConnectionSetupResponse, error) {
	if next == nil {
		return c.completeFlow(ctx)
	}

	// For redirect steps, render the URL FIRST. The closure may reject
	// invalid inputs (missing return_to_url) — surfacing those before any
	// DB write keeps the connection's setup_step consistent on rejection.
	var redirectInfo iface.RedirectInfo
	if next.Type() == iface.ManifestStepTypeRedirect {
		var err error
		redirectInfo, err = next.RenderRedirect(ctx, iface.RenderRedirectOptions{ReturnToUrl: returnToUrl})
		if err != nil {
			return nil, httperr.BadRequest(err.Error())
		}
	}

	nextStep := cschema.MustNewSetupStep(next.Id())
	if err := c.SetSetupStep(ctx, &nextStep); err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to update setup step: %w", err))
	}

	if next.Type() == iface.ManifestStepTypeVerify {
		if err := c.s.EnqueueVerifyConnection(ctx, c.GetId()); err != nil {
			return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to enqueue verify connection task: %w", err))
		}
	}

	if next.Type() == iface.ManifestStepTypeRedirect {
		return &iface.ConnectionSetupRedirect{
			Id:          c.GetId(),
			Type:        iface.ConnectionSetupResponseTypeRedirect,
			RedirectUrl: redirectInfo.URL,
		}, nil
	}

	return c.renderStepResponse(ctx, flow, next, iface.RenderRedirectOptions{ReturnToUrl: returnToUrl})
}

// completeFlow clears the connection's setup_step and transitions state to
// Configured. Idempotent — safe to call when the connection is already
// configured or already has no setup step.
func (c *connection) completeFlow(ctx context.Context) (iface.ConnectionSetupResponse, error) {
	if c.GetSetupStep() != nil {
		if err := c.SetSetupStep(ctx, nil); err != nil {
			return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to clear setup step: %w", err))
		}
	}
	if c.GetState() != database.ConnectionStateConfigured {
		if err := c.SetState(ctx, database.ConnectionStateConfigured); err != nil {
			return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to set connection state to configured: %w", err))
		}
	}
	return &iface.ConnectionSetupComplete{
		Id:   c.GetId(),
		Type: iface.ConnectionSetupResponseTypeComplete,
	}, nil
}

// renderStepResponse turns a ManifestSetupStep into the API response shape.
// Resume-mode rendering is keyed by an empty ReturnToUrl: redirect-type
// steps in that mode return an empty-URL Redirect response so the UI knows
// the connection is waiting on a previously-issued external redirect (e.g.
// the OAuth2 callback) rather than synthesizing a fresh URL.
func (c *connection) renderStepResponse(
	ctx context.Context,
	flow iface.ManifestSetupFlow,
	step iface.ManifestSetupStep,
	opts iface.RenderRedirectOptions,
) (iface.ConnectionSetupResponse, error) {
	switch step.Type() {
	case iface.ManifestStepTypeForm:
		return &iface.ConnectionSetupForm{
			Id:              c.GetId(),
			Type:            iface.ConnectionSetupResponseTypeForm,
			StepId:          step.Id(),
			StepTitle:       step.Title(),
			StepDescription: step.Description(),
			JsonSchema:      json.RawMessage(step.JsonSchema()),
			UiSchema:        json.RawMessage(step.UiSchema()),
		}, nil

	case iface.ManifestStepTypeRedirect:
		// Resume mode: no fresh URL, UI is mid-redirect already.
		if opts.ReturnToUrl == "" {
			return &iface.ConnectionSetupRedirect{
				Id:   c.GetId(),
				Type: iface.ConnectionSetupResponseTypeRedirect,
			}, nil
		}
		info, err := step.RenderRedirect(ctx, opts)
		if err != nil {
			return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to render redirect: %w", err))
		}
		return &iface.ConnectionSetupRedirect{
			Id:          c.GetId(),
			Type:        iface.ConnectionSetupResponseTypeRedirect,
			RedirectUrl: info.URL,
		}, nil

	case iface.ManifestStepTypeVerify:
		return &iface.ConnectionSetupVerifying{
			Id:   c.GetId(),
			Type: iface.ConnectionSetupResponseTypeVerifying,
		}, nil

	}
	return nil, httperr.InternalServerError(httperr.WithInternalErrorf("unsupported manifest step type %q", step.Type()))
}

// renderResumeResponse returns the response shape for the connection's
// current setup_step, without transitioning anything. Used by
// GetCurrentSetupStepResponse for the UI's resume / polling path.
//
// Terminal-failure pseudo-steps (apxy:verify_failed, apxy:auth_failed) are
// not in the manifest — the dispatcher recognizes them by IsTerminalFailure
// and renders a ConnectionSetupError directly.
func (c *connection) renderResumeResponse(ctx context.Context, flow iface.ManifestSetupFlow, setupStep cschema.SetupStep) (iface.ConnectionSetupResponse, error) {
	if setupStep.IsTerminalFailure() {
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
	step, ok, err := flow.StepById(ctx, setupStep.Id())
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to evaluate setup flow: %w", err))
	}
	if !ok {
		if flow.ContainsStep(setupStep.Id()) {
			next, hasNext, err := flow.NextStep(ctx, setupStep.Id())
			if err != nil {
				return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to evaluate setup flow: %w", err))
			}
			if !hasNext {
				return c.completeFlow(ctx)
			}
			return c.advanceToStep(ctx, next, flow, "")
		}
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("current setup step %q not in manifest", setupStep.String()))
	}
	return c.renderStepResponse(ctx, flow, step, iface.RenderRedirectOptions{})
}

// ensureStepMatches enforces that the supplied request step_id matches the
// connection's current setup step. Both IDs and missing-step conditions are
// surfaced as BadRequest so the caller sees a 400, not a 500.
func ensureStepMatches(setupStep *cschema.SetupStep, reqStepId string) error {
	if setupStep == nil {
		return httperr.BadRequest("connection has no active setup step")
	}
	if reqStepId == "" {
		return httperr.BadRequest("step_id is required")
	}
	if reqStepId != setupStep.Id() {
		return httperr.BadRequest(fmt.Sprintf("step_id %q does not match current step %q", reqStepId, setupStep.Id()))
	}
	return nil
}
