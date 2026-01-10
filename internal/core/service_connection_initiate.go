package core

import (
	"context"
	"errors"

	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

/*
 * This file contains the logic for initiating connections. This is in the core service because despite it being
 * heavily tied to the request/response structure, it also deeply depends on the connector configuration logic.
 */

func (s *service) InitiateConnection(ctx context.Context, req iface.InitiateConnectionRequest) (iface.InitiateConnectionResponse, error) {
	if err := req.Validate(); err != nil {
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

	if err := common.ValidateNamespacePath(targetNamespace); err != nil {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid namespace '%s'", targetNamespace).
			WithInternalErr(err).
			BuildStatusError()
	}

	if !common.NamespaceIsSameOrChild(cv.GetNamespace(), targetNamespace) {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("target namespace '%s' is not a child of the connector's namespace '%s'", targetNamespace, cv.GetNamespace()).
			BuildStatusError()
	}

	_, err = s.EnsureNamespaceAncestorPath(ctx, targetNamespace)
	if err != nil {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError()
	}

	connection, err := s.CreateConnection(ctx, targetNamespace, cv)
	if err != nil {
		return nil, api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError()
	}

	connector := cv.GetDefinition()
	if _, ok := connector.Auth.Inner().(*config.AuthOAuth2); ok {
		if req.ReturnToUrl == "" {
			return nil, api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithResponseMsg("must specify return_to_url").
				BuildStatusError()
		}

		ra := core.GetAuthFromContext(ctx)
		o2 := s.getOAuth2Factory().NewOAuth2(connection)
		url, err := o2.SetStateAndGeneratePublicUrl(ctx, ra.MustGetActor(), req.ReturnToUrl)
		if err != nil {
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

	return nil, api_common.NewHttpStatusErrorBuilder().
		WithStatusInternalServerError().
		WithResponseMsg("unsupported connector auth type").
		BuildStatusError()
}
