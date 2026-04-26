package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/core/iface"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// HandleCredentialsEstablished advances the connection to the next setup phase after an auth
// method has stored valid credentials. The decision tree is the same regardless of how
// credentials were acquired:
//   - probes defined → enter verify phase and enqueue the verify task
//   - else configure steps defined → enter configure:0
//   - else → clear setup step (connection is effectively ready)
func (c *connection) HandleCredentialsEstablished(ctx context.Context) (iface.PostAuthOutcome, error) {
	connectorDef := c.cv.GetDefinition()
	if connectorDef == nil {
		return iface.PostAuthOutcome{}, fmt.Errorf("connector definition is missing")
	}

	if len(connectorDef.Probes) > 0 {
		if err := c.SetSetupStep(ctx, &cschema.SetupStepVerify); err != nil {
			return iface.PostAuthOutcome{}, fmt.Errorf("failed to set setup step to verify: %w", err)
		}
		if err := c.s.EnqueueVerifyConnection(ctx, c.GetId()); err != nil {
			return iface.PostAuthOutcome{}, fmt.Errorf("failed to enqueue verify connection task: %w", err)
		}
		return iface.PostAuthOutcome{SetupPending: true}, nil
	}

	if connectorDef.SetupFlow.HasConfigure() {
		first := cschema.MustNewIndexedSetupStep(cschema.SetupPhaseConfigure, 0)
		if err := c.SetSetupStep(ctx, &first); err != nil {
			return iface.PostAuthOutcome{}, fmt.Errorf("failed to set setup step to configure:0: %w", err)
		}
		return iface.PostAuthOutcome{SetupPending: true}, nil
	}

	if c.GetSetupStep() != nil {
		if err := c.SetSetupStep(ctx, nil); err != nil {
			return iface.PostAuthOutcome{}, fmt.Errorf("failed to clear setup step: %w", err)
		}
	}
	return iface.PostAuthOutcome{SetupPending: false}, nil
}

// HandleAuthFailed records an auth-phase failure on the connection so it lands in the same
// retryable terminal state the verify phase uses. setup_error captures the message and
// setup_step becomes auth_failed; the marketplace UI surfaces this via the standard failure
// screen and offers retry/cancel via /connections/{id}/_retry.
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
