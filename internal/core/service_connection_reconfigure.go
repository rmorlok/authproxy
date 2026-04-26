package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
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

	first := cschema.MustNewIndexedSetupStep(cschema.SetupPhaseConfigure, 0)
	return c.buildFormResponse(ctx, first, connector.SetupFlow)
}
