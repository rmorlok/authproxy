package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// onVerifyPassed advances the connection to the next setup step after all probes have run
// successfully. If there are no remaining steps the connection is marked ready; otherwise
// the next step (typically configure:0) is recorded.
func (c *connection) onVerifyPassed(ctx context.Context) error {
	connector := c.cv.GetDefinition()

	var nextStep cschema.SetupStep
	if connector != nil && connector.SetupFlow != nil {
		var err error
		nextStep, err = connector.SetupFlow.NextSetupStep(cschema.SetupStepVerify, connector.HasProbes())
		if err != nil {
			return fmt.Errorf("failed to determine next step after verify: %w", err)
		}
	}

	if nextStep.IsZero() {
		if err := c.SetSetupStep(ctx, nil); err != nil {
			return fmt.Errorf("failed to clear setup step after verify: %w", err)
		}
		if err := c.SetState(ctx, database.ConnectionStateReady); err != nil {
			return fmt.Errorf("failed to set connection ready after verify: %w", err)
		}
		return nil
	}

	if err := c.SetSetupStep(ctx, &nextStep); err != nil {
		return fmt.Errorf("failed to advance setup step after verify: %w", err)
	}
	return nil
}

// onVerifyFailed records a verify failure on the connection: revokes any credentials acquired
// during auth so the user must re-authenticate on retry, populates setup_error with the probe
// failure message, and moves setup_step to the verify_failed terminal pseudo-step.
func (c *connection) onVerifyFailed(ctx context.Context, probeId string, invokeErr error) error {
	for _, op := range c.getRevokeCredentialsOperations() {
		if err := op(ctx); err != nil {
			// Log and continue — we still want to record the failure state even if revoke fails.
			c.logger.Error("failed to revoke credentials during verify failure", "error", err)
		}
	}

	msg := fmt.Sprintf("probe %q failed: %s", probeId, invokeErr.Error())
	if err := c.SetSetupError(ctx, &msg); err != nil {
		return err
	}

	if err := c.SetSetupStep(ctx, &cschema.SetupStepVerifyFailed); err != nil {
		return err
	}
	return nil
}
