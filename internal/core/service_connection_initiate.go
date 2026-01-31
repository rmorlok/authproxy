package core

import (
	"context"
	"errors"

	"github.com/rmorlok/authproxy/internal/apauth/core"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

/*
 * This file contains the logic for initiating connections. This is in the core service because despite it being
 * heavily tied to the request/response structure, it also deeply depends on the connector configuration logic.
 */

// InitiateConnection starts the process of initiating the connection. This method provides auth validation as part of
// the logic.
func (s *service) InitiateConnection(ctx context.Context, req iface.InitiateConnectionRequest) (iface.InitiateConnectionResponse, error) {
	val := auth.MustGetValidatorFromContext(ctx)
	if err := req.Validate(); err != nil {
		val.MarkErrorReturn()
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithPublicErr(err).
			BuildStatusError()
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
			return nil, api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("connector '%s' not found", req.ConnectorId).
				BuildStatusError()
		}

		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError()
	}

	targetNamespace := cv.GetNamespace()
	if req.HasIntoNamespace() {
		targetNamespace = req.IntoNamespace
	}

	if err := aschema.ValidateNamespacePath(targetNamespace); err != nil {
		val.MarkErrorReturn()
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid namespace '%s'", targetNamespace).
			WithInternalErr(err).
			BuildStatusError()
	}

	if !aschema.NamespaceIsSameOrChild(cv.GetNamespace(), targetNamespace) {
		val.MarkErrorReturn()
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("target namespace '%s' is not a child of the connector's namespace '%s'", targetNamespace, cv.GetNamespace()).
			BuildStatusError()
	}

	// Primary validation for the request -- make sure the user can initiate connections in the target namespace with
	// the specified connector id.
	if err := val.ValidateNamespaceResourceId(targetNamespace, cv.GetId().String()); err != nil {
		val.MarkErrorReturn()
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusForbidden().
			WithPublicErr(err).
			BuildStatusError()
	}

	_, err = s.EnsureNamespaceAncestorPath(ctx, targetNamespace, nil)
	if err != nil {
		val.MarkErrorReturn()
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError()
	}

	connection, err := s.CreateConnection(ctx, targetNamespace, cv)
	if err != nil {
		val.MarkErrorReturn()
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError()
	}

	connector := cv.GetDefinition()
	if _, ok := connector.Auth.Inner().(*config.AuthOAuth2); ok {
		if req.ReturnToUrl == "" {
			val.MarkErrorReturn()
			return nil, api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithResponseMsg("must specify return_to_url").
				BuildStatusError()
		}

		ra := core.GetAuthFromContext(ctx)
		o2 := s.getOAuth2Factory().NewOAuth2(connection)
		url, err := o2.SetStateAndGeneratePublicUrl(ctx, ra.MustGetActor(), req.ReturnToUrl)
		if err != nil {
			val.MarkErrorReturn()
			return nil, api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(err).
				BuildStatusError()
		}

		return &iface.InitiateConnectionRedirect{
			Id:          connection.GetId(),
			Type:        iface.PreconnectionResponseTypeRedirect,
			RedirectUrl: url,
		}, nil
	}

	val.MarkErrorReturn()
	return nil, api_common.NewHttpStatusErrorBuilder().
		WithStatusInternalServerError().
		WithResponseMsg("unsupported connector auth type").
		BuildStatusError()
}
