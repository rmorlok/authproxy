package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/database"
)

func (c *Connector) SetState(
	ctx context.Context,
	state database.ConnectorGenerationState,
) error {
	c.l.Debug(
		"setting connector generation state",
		"current_memory_state", c.ConnectorWithDefinition.State,
		"to_state", state,
	)
	err := c.s.db.SetConnectorGenerationState(
		ctx,
		c.ConnectorWithDefinition.Id,
		c.ConnectorWithDefinition.Generation,
		state,
	)
	if err == nil {
		c.ConnectorWithDefinition.State = state
	}

	return err
}
