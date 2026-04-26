package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
)

// CancelSetup abandons an in-flight reconfigure on a ready connection by clearing
// its setup_step and setup_error. The connection stays in the ready state so the
// previously stored configuration continues to apply. Only valid when the connection
// is already ready — abandoning setup mid-creation should use AbortConnection, which
// also revokes credentials and removes the connection.
func (c *connection) CancelSetup(ctx context.Context) error {
	if c.GetState() != database.ConnectionStateReady {
		return httperr.BadRequest("connection is not in a state that can cancel setup")
	}

	if c.GetSetupStep() == nil && c.GetSetupError() == nil {
		return nil
	}

	if c.GetSetupStep() != nil {
		if err := c.SetSetupStep(ctx, nil); err != nil {
			return httperr.InternalServerError(httperr.WithInternalErrorf("failed to clear setup step: %w", err))
		}
	}
	if c.GetSetupError() != nil {
		if err := c.SetSetupError(ctx, nil); err != nil {
			return httperr.InternalServerError(httperr.WithInternalErrorf("failed to clear setup error: %w", err))
		}
	}
	return nil
}
