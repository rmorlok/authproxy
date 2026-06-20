package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/core/iface"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// HandleCredentialsEstablished advances the connection through the manifest
// after an auth method has stored valid credentials. Called from the OAuth2
// callback handler (which finalizes the token exchange outside the normal
// SubmitForm path). For api-key the dispatcher in SubmitForm handles the
// same transition inline.
//
// The current setup_step is expected to be the auth-method-emitted step
// whose credentials were just established. The manifest computes the next
// step (verify when probes are configured, else the first configure step,
// else flow complete) and the connection state machine transitions.
func (c *connection) HandleCredentialsEstablished(ctx context.Context) (iface.PostAuthOutcome, error) {
	current := c.GetSetupStep()
	if current == nil {
		return iface.PostAuthOutcome{}, fmt.Errorf("connection has no active setup step")
	}

	flow := c.s.buildManifestSetupFlow(c)
	next, hasNext, err := flow.NextStep(ctx, current.Id())
	if err != nil {
		return iface.PostAuthOutcome{}, fmt.Errorf("failed to evaluate setup flow: %w", err)
	}
	if !hasNext {
		if _, err := c.completeFlow(ctx); err != nil {
			return iface.PostAuthOutcome{}, err
		}
		return iface.PostAuthOutcome{SetupPending: false}, nil
	}

	nextStep := cschema.MustNewSetupStep(next.Id())
	if err := c.SetSetupStep(ctx, &nextStep); err != nil {
		return iface.PostAuthOutcome{}, fmt.Errorf("failed to set setup step to %q: %w", next.Id(), err)
	}
	if next.Type() == iface.ManifestStepTypeVerify {
		if err := c.s.EnqueueVerifyConnection(ctx, c.GetId()); err != nil {
			return iface.PostAuthOutcome{}, fmt.Errorf("failed to enqueue verify connection task: %w", err)
		}
	}
	return iface.PostAuthOutcome{SetupPending: true}, nil
}

// HandleAuthFailed records an auth-phase failure on the connection so it
// lands in the auth_failed terminal pseudo-step. setup_error captures the
// message; the marketplace UI surfaces it via the standard failure screen
// and offers retry / cancel.
func (c *connection) HandleAuthFailed(ctx context.Context, authErr error) error {
	msg := authErr.Error()
	if err := c.SetSetupError(ctx, &msg); err != nil {
		return fmt.Errorf("failed to record setup error after auth failure: %w", err)
	}
	if err := c.SetSetupStep(ctx, &cschema.SetupStepAuthFailed); err != nil {
		return fmt.Errorf("failed to set setup step to auth_failed: %w", err)
	}
	return nil
}
