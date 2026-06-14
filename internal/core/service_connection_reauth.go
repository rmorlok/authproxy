package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
)

// ReauthConnection drives the re-authentication flow for a Configured
// connection. The lifecycle state is unchanged; setup_step is reset to the
// flow's first step so the user re-runs the credential-collection portion.
//
// Today every connector type starts reauth at the flow's FirstStep:
//   - With preconnect: preconnect[0] (preconnect data may inform auth
//     setup, e.g. OAuth2 endpoint templating).
//   - Without preconnect: the first auth-method-emitted step (api-key
//     credential form, or OAuth2 authorize redirect).
//
// Distinct from RetryConnectionSetup (which only applies in terminal
// failure states) — reauth is user-driven rotation on a healthy or
// unhealthy Configured connection.
func (s *service) ReauthConnection(ctx context.Context, id apid.ID, returnToUrl string) (iface.ConnectionSetupResponse, error) {
	conn, err := s.getConnection(ctx, id)
	if err != nil {
		return nil, err
	}

	if conn.GetState() != database.ConnectionStateConfigured {
		return nil, httperr.BadRequest("connection is not in a reauthable state")
	}

	connector := conn.cv.GetDefinition()
	if connector == nil || connector.Auth == nil {
		return nil, httperr.InternalServerErrorMsg("connector has no auth configuration")
	}
	// Reauth re-establishes credentials. No-auth connectors have no
	// credentials to rotate; reject before clearing anything.
	factory := s.getAuthMethodFactory(connector)
	if factory == nil || len(factory.ManifestSetupSteps(conn, connector)) == 0 {
		return nil, httperr.BadRequest("connector does not support reauth")
	}

	if err := conn.SetSetupError(ctx, nil); err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to clear setup error: %w", err))
	}

	flow := s.buildManifestSetupFlow(conn)
	first, err := flow.FirstStep(ctx)
	if err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to evaluate setup flow: %w", err))
	}
	if first == nil {
		return nil, httperr.BadRequest("connector has no setup flow to reauth through")
	}
	return conn.advanceToStep(ctx, first, flow, returnToUrl)
}
