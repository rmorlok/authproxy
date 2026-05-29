package core

import (
	"context"
	"errors"
	"fmt"

	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

/*
 * This file contains the logic for initiating connections. This is in the core service because despite it being
 * heavily tied to the request/response structure, it also deeply depends on the connector configuration logic.
 */

// InitiateConnection starts the process of initiating the connection. This method provides auth validation as part of
// the logic.
func (s *service) InitiateConnection(ctx context.Context, req iface.InitiateConnectionRequest) (iface.ConnectionSetupResponse, error) {
	val := auth.MustGetValidatorFromContext(ctx)
	if err := req.Validate(); err != nil {
		val.MarkErrorReturn()
		return nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err))
	}

	var err error
	var cv iface.ConnectorVersion
	if req.HasVersion() {
		cv, err = s.GetConnectorVersion(ctx, req.ConnectorId, req.ConnectorVersion)
	} else {
		cv, err = s.GetConnectorVersionForState(ctx, req.ConnectorId, database.ConnectorVersionStatePrimary)
	}

	if err != nil {
		val.MarkErrorReturn()

		if errors.Is(err, ErrNotFound) {
			return nil, httperr.NotFoundf("connector '%s' not found", req.ConnectorId)
		}

		return nil, httperr.InternalServerError(httperr.WithInternalErr(err))
	}

	targetNamespace := cv.GetNamespace()
	if req.HasIntoNamespace() {
		targetNamespace = req.IntoNamespace
	}

	if err := aschema.ValidateNamespacePath(targetNamespace); err != nil {
		val.MarkErrorReturn()
		return nil, httperr.BadRequest(fmt.Sprintf("invalid namespace '%s'", targetNamespace), httperr.WithInternalErr(err))
	}

	if !aschema.NamespaceIsSameOrChild(cv.GetNamespace(), targetNamespace) {
		val.MarkErrorReturn()
		return nil, httperr.BadRequestf("target namespace '%s' is not a child of the connector's namespace '%s'", targetNamespace, cv.GetNamespace())
	}

	// Primary validation for the request -- make sure the user can initiate connections in the target namespace with
	// the specified connector id.
	if err := val.ValidateNamespaceResourceId(targetNamespace, cv.GetId().String()); err != nil {
		val.MarkErrorReturn()
		return nil, httperr.Forbidden(err.Error(), httperr.WithInternalErr(err))
	}

	_, err = s.EnsureNamespaceAncestorPath(ctx, targetNamespace, nil)
	if err != nil {
		val.MarkErrorReturn()
		return nil, httperr.InternalServerError(httperr.WithInternalErr(err))
	}

	connectionIface, err := s.CreateConnection(ctx, targetNamespace, cv)
	if err != nil {
		val.MarkErrorReturn()
		return nil, httperr.InternalServerError(httperr.WithInternalErr(err))
	}
	// CreateConnection returns the concrete *connection typed as iface; we
	// need the concrete to invoke the dispatch helpers.
	connection, ok := connectionIface.(*connection)
	if !ok {
		val.MarkErrorReturn()
		return nil, httperr.InternalServerErrorMsg("created connection is not a *connection")
	}

	flow := s.buildManifestSetupFlow(connection)
	first := flow.FirstStep()
	if first == nil {
		// No setup steps to walk through — the connection is immediately
		// considered configured.
		return connection.completeFlow(ctx)
	}

	return connection.advanceToStep(ctx, first, flow, req.ReturnToUrl)
}
