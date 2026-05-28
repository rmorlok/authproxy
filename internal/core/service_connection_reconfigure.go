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
	firstConfigureId := connector.SetupFlow.Configure.Steps[0].Id
	step, ok := flow.StepById(firstConfigureId)
	if !ok {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("first configure step %q missing from manifest", firstConfigureId))
	}
	// Reconfigure never feeds a return-to-url through; configure is purely
	// in-marketplace form interaction.
	return c.advanceToStep(ctx, step, flow, "")
}
