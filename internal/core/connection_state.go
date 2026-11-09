package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/database"
)

// SetState persists a new lifecycle state for the connection and updates the
// in-memory copy on success. Because state decides whether a connection can gain
// ToolSet bindings and install their Tools, such as when setup completes, a
// successful change also requests a ToolSet membership pass for the connection.
func (c *connection) SetState(ctx context.Context, state database.ConnectionState) error {
	c.logger.Debug("setting connection state", "current_memory_state", c.Connection.State, "to_state", state)
	err := c.s.db.SetConnectionState(ctx, c.Connection.Id, state)
	if err == nil {
		c.Connection.State = state
		c.s.enqueueConnectionToolSetReconcile(ctx, c.Connection.Id)
	}

	return err
}
