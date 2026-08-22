package core

import (
	"context"
	"errors"
	"fmt"

	apauth "github.com/rmorlok/authproxy/internal/apauth/core"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

/*
 * This file contains the logic for initiating connections. This is in the core service because despite it being
 * heavily tied to the request/response structure, it also deeply depends on the connector configuration logic.
 */

// InitiateConnection starts the process of initiating the connection. This method provides auth validation as part of
// the logic.
func (s *service) InitiateConnection(
	ctx context.Context,
	req iface.InitiateConnectionRequest,
) (iface.ConnectionSetupResponse, error) {
	val := auth.MustGetValidatorFromContext(ctx)
	if err := req.Validate(); err != nil {
		val.MarkErrorReturn()
		return nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err))
	}

	var err error
	var c iface.Connector
	if req.HasVersion() {
		c, err = s.GetConnectorVersion(ctx, req.ConnectorId, req.ConnectorVersion)
	} else {
		c, err = s.GetConnectorVersionForState(ctx, req.ConnectorId, database.ConnectorDefinitionVersionStatePrimary)
	}

	if err != nil {
		val.MarkErrorReturn()

		if errors.Is(err, ErrNotFound) {
			return nil, httperr.NotFoundf("connector '%s' not found", req.ConnectorId)
		}

		return nil, httperr.InternalServerError(httperr.WithInternalErr(err))
	}

	targetNamespace := c.GetNamespace()
	if req.HasIntoNamespace() {
		targetNamespace = req.IntoNamespace
	} else if val.ValidateNamespaceResourceId(targetNamespace, c.GetId().String()) != nil {
		// If the `intoNamespace` option is not specified, attempt to infer where
		// the connection should be placed based on the actor's permissions.
		// Try to colocate the connection with the connector first (use the
		// connector's namespace). If that is not possible, see if the actor has
		// a single namespace they are authorized to create connections in that
		// is a child of the connector's namespace. If so, use that. If permissions
		// would be ambiguous, reject and force the client to specify. This covers
		// the common case where actors can only connect into their own namespace.
		if inferred, ok := inferConnectionNamespace(
			c.GetNamespace(),
			apauth.ActorFromContext(ctx).GetNamespace(),
			val.GetEffectiveNamespaceMatchers(nil),
		); ok {
			targetNamespace = inferred
		}
	}

	if err := namespace.ValidatePath(targetNamespace); err != nil {
		val.MarkErrorReturn()
		return nil, httperr.BadRequest(fmt.Sprintf("invalid namespace '%s'", targetNamespace), httperr.WithInternalErr(err))
	}

	if !namespace.IsSameOrChild(c.GetNamespace(), targetNamespace) {
		val.MarkErrorReturn()
		return nil, httperr.BadRequestf("target namespace '%s' is not a child of the connector's namespace '%s'", targetNamespace, c.GetNamespace())
	}

	actor := apauth.ActorFromContext(ctx)
	if !namespace.IsSameOrChild(actor.GetNamespace(), targetNamespace) {
		return nil, httperr.Forbidden("connection namespace must be child of creating actor")
	}

	// Primary validation for the request -- make sure the user can initiate
	// connections in the target namespace with the specified connector id.
	if err := val.ValidateNamespaceResourceId(targetNamespace, c.GetId().String()); err != nil {
		val.MarkErrorReturn()
		return nil, httperr.Forbidden(err.Error(), httperr.WithInternalErr(err))
	}

	_, err = s.EnsureNamespaceAncestorPath(ctx, targetNamespace, nil)
	if err != nil {
		val.MarkErrorReturn()
		return nil, httperr.InternalServerError(httperr.WithInternalErr(err))
	}

	var name scommon.ResourceName
	if req.Name != nil {
		name = *req.Name
	}
	connectionIface, err := s.CreateConnection(ctx, targetNamespace, name, c)
	if err != nil {
		val.MarkErrorReturn()
		if errors.Is(err, database.ErrDuplicate) {
			return nil, httperr.Conflictf("connection name '%s' already exists in namespace '%s'", name, targetNamespace)
		}
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
	first, err := flow.FirstStep(ctx)
	if err != nil {
		val.MarkErrorReturn()
		return nil, httperr.InternalServerError(httperr.WithInternalErrorf("failed to evaluate setup flow: %w", err))
	}
	if first == nil {
		// No setup steps to walk through — the connection is immediately
		// considered configured.
		return connection.completeFlow(ctx)
	}

	return connection.advanceToStep(ctx, first, flow, req.ReturnToUrl)
}

// inferConnectionNamespace returns an inferred connection namespace only when
// the actor has one exact, unambiguous namespace in which they can create the
// connection and that namespace is a child of both the connector and actor
// namespaces. Ambiguous, wildcard, unrelated, and invalid namespaces cannot be
// inferred, requiring the client to specify intoNamespace explicitly.
func inferConnectionNamespace(connectorNamespace, actorNamespace string, allowedNamespaces []string) (string, bool) {
	if len(allowedNamespaces) != 1 {
		return "", false
	}

	candidate := allowedNamespaces[0]
	if namespace.ValidatePath(candidate) != nil ||
		!namespace.IsSameOrChild(connectorNamespace, candidate) ||
		!namespace.IsSameOrChild(actorNamespace, candidate) {
		return "", false
	}

	return candidate, true
}
