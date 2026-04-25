package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httperr"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// RetryConnectionSetup resets a connection that is in the verify_failed terminal state so the
// user can try setup again. If the connector has preconnect steps, the retry restarts from
// preconnect:0 so the user can correct any input that led to invalid credentials. Otherwise,
// retry re-initiates the OAuth flow from scratch.
func (s *service) RetryConnectionSetup(ctx context.Context, id apid.ID, returnToUrl string) (iface.InitiateConnectionResponse, error) {
	conn, err := s.getConnection(ctx, id)
	if err != nil {
		return nil, err
	}

	setupStep := conn.GetSetupStep()
	if setupStep == nil || *setupStep != cschema.SetupStepVerifyFailed {
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
		return conn.buildFormResponse(ctx, "preconnect:0", connector.SetupFlow)
	}

	// No preconnect to return to — re-initiate OAuth from scratch.
	return conn.initiateAuthStep(ctx, returnToUrl, connector)
}
