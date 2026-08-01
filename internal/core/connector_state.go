package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/database"
)

func (c *Connector) SetState(
	ctx context.Context,
	state database.ConnectorDefinitionVersionState,
) error {
	c.l.Debug(
		"setting connector version state",
		"current_memory_state", c.ConnectorWithDefinition.State,
		"to_state", state,
	)
	err := c.s.db.SetConnectorDefinitionVersionState(
		ctx,
		c.ConnectorWithDefinition.Id,
		c.ConnectorWithDefinition.Version,
		state,
	)
	if err == nil {
		c.ConnectorWithDefinition.State = state
	}

	return err
}
