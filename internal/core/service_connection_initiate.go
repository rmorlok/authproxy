package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apauth/core"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/httperr"
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

	connection, err := s.CreateConnection(ctx, targetNamespace, cv)
	if err != nil {
		val.MarkErrorReturn()
		return nil, httperr.InternalServerError(httperr.WithInternalErr(err))
	}

	connector := cv.GetDefinition()

	// If the connector has preconnect steps, return the first form instead of proceeding to auth
	if connector.SetupFlow.HasPreconnect() {
		firstStep := connector.SetupFlow.Preconnect.Steps[0]
		setupStep := "preconnect:0"
		if err := connection.SetSetupStep(ctx, &setupStep); err != nil {
			val.MarkErrorReturn()
			return nil, httperr.InternalServerError(httperr.WithInternalErr(err))
		}

		return &iface.InitiateConnectionForm{
			Id:              connection.GetId(),
			Type:            iface.PreconnectionResponseTypeForm,
			StepId:          firstStep.Id,
			StepTitle:       firstStep.Title,
			StepDescription: firstStep.Description,
			CurrentStep:     0,
			TotalSteps:      connector.SetupFlow.TotalSteps(),
			JsonSchema:      json.RawMessage(firstStep.JsonSchema),
			UiSchema:        json.RawMessage(firstStep.UiSchema),
		}, nil
	}

	if _, ok := connector.Auth.Inner().(*config.AuthOAuth2); ok {
		if req.ReturnToUrl == "" {
			val.MarkErrorReturn()
			return nil, httperr.BadRequest("must specify return_to_url")
		}

		ra := core.GetAuthFromContext(ctx)
		o2 := s.getOAuth2Factory().NewOAuth2(connection)
		url, err := o2.SetStateAndGeneratePublicUrl(ctx, ra.MustGetActor(), req.ReturnToUrl)
		if err != nil {
			val.MarkErrorReturn()
			return nil, httperr.InternalServerError(httperr.WithInternalErr(err))
		}

		return &iface.InitiateConnectionRedirect{
			Id:          connection.GetId(),
			Type:        iface.PreconnectionResponseTypeRedirect,
			RedirectUrl: url,
		}, nil
	}

	val.MarkErrorReturn()
	return nil, httperr.InternalServerErrorMsg("unsupported connector auth type")
}
