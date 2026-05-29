package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// onVerifyPassed advances the connection to the next setup step after all probes have run
// successfully. If there are no remaining steps the connection is marked configured; otherwise
// the next step (typically the first configure step) is recorded.
//
// Verify success is also the canonical "credentials are working" signal, so health_state is
// pinned to healthy here. MarkHealthState is idempotent: on initial setup this is a no-op
// (the connection was created healthy), and on reauth-driven verify it flips the connection
// back to healthy after a successful credential rotation.
func (c *connection) onVerifyPassed(ctx context.Context) error {
	if err := c.MarkHealthState(ctx, database.ConnectionHealthStateHealthy, "verify_passed"); err != nil {
		return fmt.Errorf("failed to mark connection healthy after verify: %w", err)
	}

	flow := c.s.buildManifestSetupFlow(c)
	next, hasNext := flow.NextStep(cschema.SetupStepVerify.Id())
	if !hasNext {
		if _, err := c.completeFlow(ctx); err != nil {
			return err
		}
		return nil
	}
	nextStep := cschema.MustNewSetupStep(next.Id())
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
