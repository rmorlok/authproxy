package core

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// ReauthConnection drives the unified re-authentication flow for an existing
// Ready connection. The connection's lifecycle state is unchanged; setup_step
// is reset so the user re-runs the credential-collection portion of the flow.
//
// Auth-type dispatch:
//
//   - api-key: setup_step reset to credentials:0. The returned form is built
//     from the connector definition only — no prior credential bytes are
//     decrypted into the payload. On submit, InsertApiKeyCredential soft-
//     deletes the active row and inserts the new one in the same transaction,
//     so rotation is atomic and the audit columns (created_at, created_by_
//     actor_id) on the new row record the rotation event.
//
//   - OAuth2: same shape as the retry path — restart at preconnect:0 if the
//     connector has preconnect steps; otherwise re-initiate the auth redirect.
//     OAuth2's refresh-token replacement is internal, so the prior credential
//     is never exposed to the UI here either.
//
// Distinct from RetryConnectionSetup, which only applies to verify_failed /
// auth_failed terminal states; reauth is the user-driven rotation path against
// a Ready connection regardless of health.
func (s *service) ReauthConnection(ctx context.Context, id apid.ID, returnToUrl string) (iface.ConnectionSetupResponse, error) {
	conn, err := s.getConnection(ctx, id)
	if err != nil {
		return nil, err
	}

	if conn.GetState() != database.ConnectionStateReady {
		return nil, httperr.BadRequest("connection is not in a reauthable state")
	}

	connector := conn.cv.GetDefinition()
	if connector == nil {
		return nil, httperr.InternalServerErrorMsg("connector definition is missing")
	}
	if connector.Auth == nil {
		return nil, httperr.InternalServerErrorMsg("connector has no auth configuration")
	}
	if connector.SetupFlow == nil {
		return nil, httperr.InternalServerErrorMsg("connector has no setup flow")
	}

	if err := conn.SetSetupError(ctx, nil); err != nil {
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to clear setup error: %w", err))
	}

	switch connector.Auth.Inner().(type) {
	case *config.AuthApiKey:
		if !connector.SetupFlow.HasCredentials() {
			return nil, httperr.InternalServerErrorMsg("api-key connector has no credentials step")
		}
		first := cschema.MustNewIndexedSetupStep(cschema.SetupPhaseCredentials, 0)
		return conn.buildFormResponse(ctx, first, connector.SetupFlow)
	case *config.AuthOAuth2:
		if connector.SetupFlow.HasPreconnect() {
			first := cschema.MustNewIndexedSetupStep(cschema.SetupPhasePreconnect, 0)
			return conn.buildFormResponse(ctx, first, connector.SetupFlow)
		}
		return conn.initiateAuthStep(ctx, returnToUrl, connector)
	default:
		return nil, httperr.BadRequest("connector auth type does not support reauth")
	}
}
