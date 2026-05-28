package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
)

// RetryConnectionSetup resets a connection that is in a terminal failure
// state (apxy:auth_failed or apxy:verify_failed) so the user can retry
// setup. The reset goes back to the flow's first step — preconnect[0] if
// any, else the first auth-method step.
func (s *service) RetryConnectionSetup(ctx context.Context, id apid.ID, returnToUrl string) (iface.ConnectionSetupResponse, error) {
	conn, err := s.getConnection(ctx, id)
	if err != nil {
		return nil, err
	}

	setupStep := conn.GetSetupStep()
	if setupStep == nil || !setupStep.IsTerminalFailure() {
		return nil, httperr.BadRequest("connection is not in a retryable state")
	}

	if err := conn.SetSetupError(ctx, nil); err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to clear setup error: %w", err))
	}

	flow := s.buildManifestSetupFlow(conn)
	first := flow.FirstStep()
	if first == nil {
		return nil, httperr.BadRequest("connector has no setup flow to retry")
	}
	return conn.advanceToStep(ctx, first, flow, returnToUrl)
}
