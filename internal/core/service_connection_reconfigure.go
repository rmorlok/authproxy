package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

// Reconfigure initiates a reconfiguration of a completed connection by resetting
// its setup step to the first configure step. The connection must be in the ready
// state and its connector must have configure steps defined.
func (c *connection) Reconfigure(ctx context.Context) (iface.InitiateConnectionResponse, error) {
	if c.GetState() != database.ConnectionStateReady {
		return nil, httperr.BadRequest("connection must be in ready state to reconfigure")
	}

	connector := c.cv.GetDefinition()
	if connector == nil || connector.SetupFlow == nil || !connector.SetupFlow.HasConfigure() {
		return nil, httperr.BadRequest("connector has no configure steps to reconfigure")
	}

	return c.buildFormResponse(ctx, "configure:0", connector.SetupFlow)
}
