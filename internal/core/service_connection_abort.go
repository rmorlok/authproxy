package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
)

// AbortConnection aborts an in-progress connection setup, cleaning up any credentials and
// deleting the connection. Only valid for connections that are still in setup (setup_step is not null).
func (s *service) AbortConnection(ctx context.Context, id apid.ID) error {
	s.logger.Info("aborting connection setup", "id", id)

	conn, err := s.getConnection(ctx, id)
	if err != nil {
		return err
	}

	setupStep := conn.GetSetupStep()
	if setupStep == nil {
		return httperr.BadRequest("connection setup is already complete; use disconnect instead")
	}

	// Revoke any credentials that may have been obtained during auth phase
	revokeOps := conn.getRevokeCredentialsOperations()
	for _, op := range revokeOps {
		if err := op(ctx); err != nil {
			s.logger.Error("failed to revoke credentials during abort", "error", err, "id", id)
			// Continue cleanup even if revocation fails
		}
	}

	// Set state to disconnected and soft-delete
	if err := conn.SetState(ctx, database.ConnectionStateDisconnected); err != nil {
		return fmt.Errorf("failed to set connection state during abort: %w", err)
	}

	if err := s.db.DeleteConnection(ctx, id); err != nil {
		return fmt.Errorf("failed to delete connection during abort: %w", err)
	}

	s.logger.Info("connection setup aborted", "id", id)
	return nil
}
