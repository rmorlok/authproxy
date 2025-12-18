package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/database"
)

func (c *connection) SetState(ctx context.Context, state database.ConnectionState) error {
	c.logger.Debug("setting connection state", "current_memory_state", c.Connection.State, "to_state", state)
	err := c.s.db.SetConnectionState(ctx, c.Connection.ID, state)
	if err == nil {
		c.Connection.State = state
	}

	return err
}
