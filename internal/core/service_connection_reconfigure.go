package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
)

// Reconfigure starts a reconfiguration of a completed connection by
// resetting its setup step to the first configure step. The connection must
// be in the Configured state and its connector must declare configure
// steps.
func (c *connection) Reconfigure(ctx context.Context) (iface.ConnectionSetupResponse, error) {
	if c.GetState() != database.ConnectionStateConfigured {
		return nil, httperr.BadRequest("connection must be configured to reconfigure")
	}

	connector := c.cv.GetDefinition()
	if connector == nil || connector.SetupFlow == nil || !connector.SetupFlow.HasConfigure() {
		return nil, httperr.BadRequest("connector has no configure steps to reconfigure")
	}

	flow := c.s.buildManifestSetupFlow(c)
	var step iface.ManifestSetupStep
	for i := range connector.SetupFlow.Configure.Steps {
		candidateId := connector.SetupFlow.Configure.Steps[i].Id
		candidate, ok, err := flow.StepById(ctx, candidateId)
		if err != nil {
			return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to evaluate setup flow: %w", err))
		}
		if ok {
			step = candidate
			break
		}
	}
	if step == nil {
		return c.completeFlow(ctx)
	}
	// Reconfigure never feeds a return-to-url through; configure is purely
	// in-marketplace form interaction.
	return c.advanceToStep(ctx, step, flow, "")
}
