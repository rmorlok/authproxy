package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// RetryConnectionSetup resets a connection that is in a terminal failure state so the user
// can try setup again. Both auth_failed (auth-phase failure such as an OAuth token-exchange
// error) and verify_failed (probe failure after auth) are retryable. If the connector has
// preconnect steps, retry restarts from preconnect:0 so the user can correct any input that
// led to the failure. Otherwise, retry re-initiates the auth flow from scratch.
func (s *service) RetryConnectionSetup(ctx context.Context, id apid.ID, returnToUrl string) (iface.ConnectionSetupResponse, error) {
	conn, err := s.getConnection(ctx, id)
	if err != nil {
		return nil, err
	}

	setupStep := conn.GetSetupStep()
	if setupStep == nil {
		return nil, httperr.BadRequest("connection is not in a retryable state")
	}
	parsed, err := cschema.ParseSetupStep(*setupStep)
	if err != nil || !parsed.IsTerminalFailure() {
		return nil, httperr.BadRequest("connection is not in a retryable state")
	}

	connector := conn.cv.GetDefinition()
	if connector == nil {
		return nil, httperr.InternalServerErrorMsg("connector definition is missing")
	}

	// Clear any prior error message — retry is a fresh attempt.
	if err := conn.SetSetupError(ctx, nil); err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to clear setup error: %w", err))
	}

	if connector.SetupFlow.HasPreconnect() {
		first := cschema.MustNewIndexedSetupStep(cschema.SetupPhasePreconnect, 0)
		return conn.buildFormResponse(ctx, first, connector.SetupFlow)
	}

	// No preconnect to return to — re-initiate OAuth from scratch.
	return conn.initiateAuthStep(ctx, returnToUrl, connector)
}
