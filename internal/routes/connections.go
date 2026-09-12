package routes

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	smeta "github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"log/slog"
	"net/http"
)

type ConnectionsRoutes struct {
	cfg     config.C
	auth    auth.A
	core    coreIface.C
	db      database.DB
	r       apredis.Client
	httpf   httpf.F
	encrypt encrypt.E
	oauthf  oauth2.Factory
}

type ProxyResponse = schemaapi.ProxyResponseJson

type OpenAPIConnectionJson = schemaapiopenapi.ConnectionJson
type OpenAPIConnectionPatchJson = schemaapiopenapi.ConnectionPatchJson
type OpenAPIListConnectionResponseJson = schemaapiopenapi.ListConnectionResponseJson
type OpenAPIConnectionInitiateActionJson = schemaapiopenapi.ConnectionInitiateActionJson
type OpenAPIConnectionSetupActionJson = schemaapiopenapi.ConnectionSetupActionJson
type OpenAPIConnectionSetupSubmitActionJson = schemaapiopenapi.ConnectionSetupSubmitActionJson
type OpenAPIConnectionSetupControlActionJson = schemaapiopenapi.ConnectionSetupControlActionJson
type OpenAPIEmptyConnectionActionJson = schemaapiopenapi.EmptyConnectionActionJson
type OpenAPIConnectionDisconnectActionJson = schemaapiopenapi.ConnectionDisconnectActionJson
type OpenAPIConnectionGenerationMigrationActionJson = schemaapiopenapi.ConnectionGenerationMigrationActionJson
type OpenAPIConnectionForceStateActionJson = schemaapiopenapi.ConnectionForceStateActionJson
type OpenAPIDataSourceOptionListJson = schemaapiopenapi.DataSourceOptionListJson
type OpenAPIConnectionScopeListJson = schemaapiopenapi.ConnectionScopeListJson
type ProxyRequest = schemaapiopenapi.ProxyRequestJson
type OpenAPIProxyResponseJson = schemaapiopenapi.ProxyResponseJson

func connectionSetupAction(
	resp coreIface.ConnectionSetupResponse,
) (schemaapi.ConnectionSetupAction, error) {
	status := schemaapi.ConnectionSetupActionStatus{
		Type: schemaapi.ConnectionSetupResponseType(resp.GetType()),
	}

	switch typed := resp.(type) {
	case *coreIface.ConnectionSetupRedirect:
		status.RedirectURL = typed.RedirectUrl
	case *coreIface.ConnectionSetupForm:
		status.StepID = typed.StepId
		status.StepTitle = typed.StepTitle
		status.StepDescription = typed.StepDescription
		status.JSONSchema = typed.JsonSchema
		status.UISchema = typed.UiSchema
		redactedData, err := schemaapi.RedactConnectionSetupData(typed.Data)
		if err != nil {
			return schemaapi.ConnectionSetupAction{}, err
		}
		status.Data = redactedData
	case *coreIface.ConnectionSetupComplete, *coreIface.ConnectionSetupVerifying:
	case *coreIface.ConnectionSetupError:
		status.Error = typed.Error
		status.CanRetry = typed.CanRetry
	default:
		return schemaapi.ConnectionSetupAction{}, errors.New("unknown connection setup response")
	}

	return schemaapi.NewConnectionSetupAction(
		connectionschema.NewConnectionReference(resp.GetId()),
		status,
	), nil
}

func validateConnectionActionPathTarget(
	target smeta.ObjectReference,
	connection coreIface.Connection,
) error {
	if target.ID != "" && target.ID != connection.GetId().String() {
		return errors.New("metadata.target.id does not match the connection path")
	}

	if target.HasNamespacedName() &&
		(target.Namespace != connection.GetNamespace() ||
			target.Name != connection.GetName()) {
		return errors.New("metadata.target namespace/name does not match the connection path")
	}

	return nil
}

func renderConnectionSetupAction(
	gctx *gin.Context,
	val *auth.ResourcePermissionValidator,
	resp coreIface.ConnectionSetupResponse,
) {
	action, err := connectionSetupAction(resp)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}
	if err := apgin.RenderActionJSON(
		gctx,
		http.StatusOK,
		&action,
		schemaapi.ConnectionSetupActionKind,
	); err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
	}
}

// @Summary		Initiate connection
// @Description	Initiate a new connection to an external service through a connector
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			request	body		OpenAPIConnectionInitiateActionJson	true	"Connection initiation action"
// @Success		200		{object}	OpenAPIConnectionSetupActionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/_initiate [post]
func (r *ConnectionsRoutes) initiate(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req schemaapi.ConnectionInitiateAction
	if err := apgin.BindActionJSON(
		gctx,
		&req,
		schemaapi.ConnectionInitiateActionKind,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	// InitiateConnection also performs request validation for security
	resp, err := r.core.InitiateConnection(
		ctx,
		coreIface.InitiateConnectionRequest{
			ConnectorRef:  req.Metadata.Target,
			IntoNamespace: req.Spec.IntoNamespace,
			Name:          req.Spec.Name,
			Labels:        req.Spec.Labels,
			Annotations:   req.Spec.Annotations,
			ReturnToUrl:   req.Spec.ReturnToURL,
		},
	)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	renderConnectionSetupAction(gctx, val, resp)
}

// @Summary		Submit connection form
// @Description	Submit form data for a connection setup step
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string					true	"Connection ID"
// @Param			request	body		OpenAPIConnectionSetupSubmitActionJson	true	"Form submission action"
// @Success		200		{object}	OpenAPIConnectionSetupActionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		501		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_submit [post]
func (r *ConnectionsRoutes) submit(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	var req schemaapi.ConnectionSetupSubmitAction
	if err := apgin.BindActionJSON(
		gctx,
		&req,
		schemaapi.ConnectionSetupSubmitActionKind,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}
	if err := validateConnectionActionPathTarget(
		req.Metadata.Target,
		c,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	resp, err := c.SubmitForm(
		ctx,
		coreIface.SubmitConnectionRequest{
			StepId:      req.Spec.StepID,
			Data:        req.Spec.Data,
			ReturnToUrl: req.Spec.ReturnToURL,
		},
	)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	renderConnectionSetupAction(gctx, val, resp)
}

// @Summary		Get setup step
// @Description	Get the current setup step for a connection, used to resume an interrupted setup flow
// @Tags			connections
// @Produce		json
// @Param			id	path		string	true	"Connection ID"
// @Param			returnToUrl	query	string	false	"URL to return to after a resumed redirect step"
// @Success		200	{object}	OpenAPIConnectionSetupActionJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_setupStep [get]
func (r *ConnectionsRoutes) getSetupStep(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, coreIface.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
		} else {
			apgin.WriteErr(gctx, nil, err)
		}
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	resp, err := c.GetCurrentSetupStepResponse(ctx, gctx.Query("returnToUrl"))
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	renderConnectionSetupAction(gctx, val, resp)
}

// @Summary		Get data source options
// @Description	Fetch dynamic options for a data source defined in the current setup step
// @Tags			connections
// @Produce		json
// @Param			id			path		string	true	"Connection ID"
// @Param			sourceId	path		string	true	"Data Source ID"
// @Success		200	{object}		OpenAPIDataSourceOptionListJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_dataSource/{sourceId} [get]
func (r *ConnectionsRoutes) getDataSource(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	sourceId := gctx.Param("sourceId")
	if sourceId == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("sourceId is required"))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, coreIface.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
		} else {
			apgin.WriteErr(gctx, nil, err)
		}
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	options, err := c.GetDataSource(ctx, sourceId)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	items := make([]schemaapi.DataSourceOptionJson, len(options))
	for i, option := range options {
		items[i] = schemaapi.DataSourceOptionJson{
			Value: option.Value,
			Label: option.Label,
		}
	}
	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewDataSourceOptionList(items))
}

type ListConnectionRequestQuery struct {
	Cursor        *string                   `form:"cursor"`
	LimitVal      *int32                    `form:"limit"`
	StateVal      *database.ConnectionState `form:"state"`
	ConnectorId   *string                   `form:"connectorId"`
	NamespaceVal  *string                   `form:"namespace"`
	NameVal       *string                   `form:"name"`
	LabelSelector *string                   `form:"labelSelector"`
	OrderByVal    *string                   `form:"orderBy"`
}

// @Summary		List connections
// @Description	List connections with optional filtering and pagination
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			state			query		string	false	"Filter by connection state"
// @Param			connectorId	query		string	false	"Filter by connector ID"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			name			query		string	false	"Filter by exact resource name"
// @Param			labelSelector	query		string	false	"Filter by label selector"
// @Param			orderBy		query		string	false	"Order by field (e.g., 'created_at:asc')"
// @Success		200				{object}	OpenAPIListConnectionResponseJson
// @Failure		400				{object}	ErrorResponse
// @Failure		401				{object}	ErrorResponse
// @Failure		500				{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections [get]
func (r *ConnectionsRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req ListConnectionRequestQuery
	var err error

	if err = gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	var ex coreIface.ListConnectionsExecutor

	if req.Cursor != nil {
		ex, err = r.core.ListConnectionsFromCursor(ctx, *req.Cursor)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
			val.MarkErrorReturn()
			return
		}
	} else {
		b := r.core.ListConnectionsBuilder()

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.StateVal != nil {
			b = b.ForState(*req.StateVal)
		}

		if req.ConnectorId != nil {
			connectorId, err := apid.Parse(*req.ConnectorId)
			if err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequest("invalid connectorId format", httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}
			if err := connectorId.ValidatePrefix(apid.PrefixConnector); err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequest("invalid connectorId prefix", httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}
			b = b.ForConnectorId(connectorId)
		}

		b = b.ForNamespaceMatchers(
			val.GetEffectiveNamespaceMatchers(req.NamespaceVal),
		)

		if req.NameVal != nil {
			name := scommon.ResourceName(*req.NameVal)
			if err := name.Validate(); err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid connection name: %s", err.Error()))
				val.MarkErrorReturn()
				return
			}
			b = b.ForName(name)
		}

		if req.LabelSelector != nil {
			b = b.ForLabelSelector(*req.LabelSelector)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.ConnectionOrderByField](*req.OrderByVal)
			if err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidConnectionOrderByField(field) {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid sort field '%s'", field))
				val.MarkErrorReturn()
				return
			}

			b.OrderBy(field, order)
		}

		ex = b
	}

	result := ex.FetchPage(ctx)

	if result.Error != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(result.Error)))
		val.MarkErrorReturn()
		return
	}

	connections := auth.FilterForValidatedResources(val, result.Results)
	items := make([]connectionschema.Connection, 0, len(connections))
	for _, connection := range connections {
		resource, err := connection.GetResource(ctx)
		if err != nil {
			apgin.WriteErr(gctx, nil, err)
			val.MarkErrorReturn()
			return
		}
		if err := resource.ValidateFor(
			smeta.ValidationModeResponse,
			nil, // validation context
		); err != nil {
			apgin.WriteErr(gctx, nil, err)
			val.MarkErrorReturn()
			return
		}
		items = append(items, *resource)
	}
	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewListConnectionResponseJson(items, result.Cursor))
}

// @Summary		Get connection
// @Description	Get a specific connection by its UUID
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
// @Success		200	{object}	OpenAPIConnectionJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id} [get]
func (r *ConnectionsRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if c == nil {
		apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	resource, err := c.GetResource(ctx)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}
	if err := apgin.RenderResourceJSON(
		gctx,
		http.StatusOK,
		resource,
	); err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
	}
}

// @Summary		Disconnect connection
// @Description	Disconnect an existing connection and revoke its credentials
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
// @Param			request	body		OpenAPIConnectionDisconnectActionJson	true	"Disconnect action"
// @Success		200	{object}	OpenAPIConnectionDisconnectActionJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		403	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_disconnect [post]
func (r *ConnectionsRoutes) disconnect(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	var req schemaapi.ConnectionDisconnectAction
	if err := apgin.BindActionJSON(
		gctx,
		&req,
		schemaapi.ConnectionDisconnectActionKind,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	if err := validateConnectionActionPathTarget(
		req.Metadata.Target,
		c,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}
	opts := connectionDisconnectOptions(req.Spec)

	ti, err := r.core.DisconnectConnection(ctx, id, opts)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	ra := auth.MustGetAuthFromGinContext(gctx)
	taskId, err := ti.
		BindToActor(ra.MustGetActor()).
		ToSecureEncryptedString(ctx, r.encrypt)

	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Hard code the disconnecting state to avoid race condictions with task workers
	connectionResource, err := c.GetResource(ctx)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}
	connectionResource.Status.Lifecycle.State = connectionschema.ConnectionStateDisconnecting
	response := schemaapi.NewConnectionDisconnectResponse(
		req.Metadata.Target,
		req.Spec,
		schemaapi.ConnectionDisconnectStatus{
			TaskID:     taskId,
			Connection: *connectionResource,
		},
	)
	if err := apgin.RenderActionJSON(
		gctx,
		http.StatusOK,
		&response,
		schemaapi.ConnectionDisconnectActionKind,
	); err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
	}
}

func connectionDisconnectOptions(
	spec schemaapi.ConnectionDisconnectSpec,
) coreIface.ConnectionDisconnectOptions {
	timeout := defaultConnectorLifecycleTimeout
	if spec.TimeoutSeconds != nil {
		timeout = time.Duration(*spec.TimeoutSeconds) * time.Second
	}
	return coreIface.ConnectionDisconnectOptions{Timeout: timeout}
}

// @Summary		Migrate connection connector generation
// @Description	Start a workflow that migrates an existing connection to another generation of the same connector
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string										true	"Connection UUID"
// @Param			request	body		OpenAPIConnectionGenerationMigrationActionJson	true	"Migration action"
// @Success		200		{object}	OpenAPIConnectionGenerationMigrationActionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_migrateGeneration [post]
func (r *ConnectionsRoutes) migrateGeneration(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}
	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}
	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	var req schemaapi.ConnectionGenerationMigrationAction
	if err := apgin.BindActionJSON(
		gctx,
		&req,
		schemaapi.ConnectionGenerationMigrationActionKind,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	if err := validateConnectionActionPathTarget(
		req.Metadata.Target,
		c,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	targetConnector, err := r.core.ResolveConnectorReference(
		ctx,
		req.Spec.ConnectorRef,
	)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	if targetConnector.GetId() != c.GetConnectorId() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("connectorRef must identify the connection's connector"))
		val.MarkErrorReturn()
		return
	}

	opts := connectionMigrationOptions(
		targetConnector.GetGeneration(),
		req.Spec.TimeoutSeconds,
	)

	task, err := r.core.MigrateConnectionGeneration(ctx, id, opts)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	ra := auth.MustGetAuthFromGinContext(gctx)
	taskId, err := task.TaskInfo.
		BindToActor(ra.MustGetActor()).
		ToSecureEncryptedString(ctx, r.encrypt)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	sourceRef := smeta.ObjectReference{
		APIVersion: smeta.APIVersionV1Alpha1,
		Kind:       cschema.ConnectorKind,
		ID:         c.GetConnectorId().String(),
		Name:       c.GetConnector().GetName(),
		Namespace:  c.GetConnector().GetNamespace(),
		Generation: c.GetConnectorGeneration(),
	}
	targetRef := req.Spec.ConnectorRef
	targetRef.ID = targetConnector.GetId().String()
	targetRef.Name = targetConnector.GetName()
	targetRef.Namespace = targetConnector.GetNamespace()
	targetRef.Generation = targetConnector.GetGeneration()

	response := schemaapi.NewConnectionGenerationMigrationResponse(
		req.Metadata.Target,
		req.Spec,
		schemaapi.ConnectionGenerationMigrationStatus{
			TaskID:             taskId,
			SourceConnectorRef: sourceRef,
			TargetConnectorRef: targetRef,
		},
	)

	if err := apgin.RenderActionJSON(
		gctx,
		http.StatusOK,
		&response,
		schemaapi.ConnectionGenerationMigrationActionKind,
	); err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
	}
}

func connectionMigrationOptions(
	targetGeneration uint64,
	timeoutSeconds *int64,
) coreIface.ConnectionMigrationOptions {
	timeout := defaultConnectorLifecycleTimeout
	if timeoutSeconds != nil {
		timeout = time.Duration(*timeoutSeconds) * time.Second
	}
	return coreIface.ConnectionMigrationOptions{
		TargetGeneration: targetGeneration,
		Timeout:          timeout,
	}
}

// @Summary		Abort connection setup
// @Description	Abort an in-progress connection setup, cleaning up credentials and deleting the connection
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
// @Param			request	body	OpenAPIEmptyConnectionActionJson	true	"Abort action"
// @Success		204
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_abort [post]
func (r *ConnectionsRoutes) abort(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, coreIface.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
		} else {
			apgin.WriteErr(gctx, nil, err)
		}
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}
	var req schemaapi.EmptyConnectionAction
	if err := apgin.BindActionJSON(
		gctx,
		&req,
		schemaapi.ConnectionSetupAbortActionKind,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}
	if err := validateConnectionActionPathTarget(
		req.Metadata.Target,
		c,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	err = r.core.AbortConnection(ctx, id)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	gctx.Status(http.StatusNoContent)
}

// @Summary		Reconfigure connection
// @Description	Restart the configure phase for a completed connection, allowing re-entry of post-auth settings
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
// @Param			request	body	OpenAPIEmptyConnectionActionJson	true	"Reconfigure action"
// @Success		200	{object}	OpenAPIConnectionSetupActionJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_reconfigure [post]
func (r *ConnectionsRoutes) reconfigure(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, coreIface.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
		} else {
			apgin.WriteErr(gctx, nil, err)
		}
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}
	var req schemaapi.EmptyConnectionAction
	if err := apgin.BindActionJSON(
		gctx,
		&req,
		schemaapi.ConnectionReconfigureActionKind,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}
	if err := validateConnectionActionPathTarget(
		req.Metadata.Target,
		c,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	resp, err := c.Reconfigure(ctx)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	renderConnectionSetupAction(gctx, val, resp)
}

// @Summary		Cancel in-flight setup
// @Description	Abandon a reconfigure attempt on a ready connection by clearing setup_step and setup_error. The connection remains ready and its previously stored configuration continues to apply.
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
// @Param			request	body	OpenAPIEmptyConnectionActionJson	true	"Cancel setup action"
// @Success		204
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_cancelSetup [post]
func (r *ConnectionsRoutes) cancelSetup(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, coreIface.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
		} else {
			apgin.WriteErr(gctx, nil, err)
		}
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}
	var req schemaapi.EmptyConnectionAction
	if err := apgin.BindActionJSON(
		gctx,
		&req,
		schemaapi.ConnectionSetupCancelActionKind,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}
	if err := validateConnectionActionPathTarget(
		req.Metadata.Target,
		c,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	if err := c.CancelSetup(ctx); err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	gctx.Status(http.StatusNoContent)
}

// @Summary		Retry connection setup
// @Description	Retry a connection setup that ended in a terminal failure state. Applies to any setup-phase failure: an auth-phase failure such as an OAuth token-exchange error (auth_failed) or a probe failure during verify (verify_failed). Clears the recorded error and either returns to the first preconnect step (if the connector defines one, so the user can correct any input that led to the failure) or re-initiates the auth flow from scratch.
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path	string					true	"Connection UUID"
// @Param			request	body	OpenAPIConnectionSetupControlActionJson	true	"Retry action"
// @Success		200	{object}	OpenAPIConnectionSetupActionJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_retry [post]
func (r *ConnectionsRoutes) retry(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, coreIface.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
		} else {
			apgin.WriteErr(gctx, nil, err)
		}
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	var req schemaapi.ConnectionSetupControlAction
	if err := apgin.BindActionJSON(
		gctx,
		&req,
		schemaapi.ConnectionSetupRetryActionKind,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}
	if err := validateConnectionActionPathTarget(
		req.Metadata.Target,
		c,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	resp, err := r.core.RetryConnectionSetup(ctx, id, req.Spec.ReturnToURL)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	renderConnectionSetupAction(gctx, val, resp)
}

// @Summary		Re-authenticate a connection
// @Description	Re-run the credential-collection portion of setup against an existing Ready connection. Used for user-driven credential rotation and as the recovery path when a connection is unhealthy. For api-key, returns a fresh credentials form (no prior values pre-filled); on submit the existing credential row is soft-deleted and a new one inserted in the same transaction. For OAuth2, restarts at preconnect:0 if defined, otherwise re-initiates the OAuth redirect. The connection's lifecycle state stays Ready throughout.
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path	string					true	"Connection UUID"
// @Param			request	body	OpenAPIConnectionSetupControlActionJson	true	"Reauthentication action"
// @Success		200	{object}	OpenAPIConnectionSetupActionJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_reauth [post]
func (r *ConnectionsRoutes) reauth(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, coreIface.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
		} else {
			apgin.WriteErr(gctx, nil, err)
		}
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	var req schemaapi.ConnectionSetupControlAction
	if err := apgin.BindActionJSON(
		gctx,
		&req,
		schemaapi.ConnectionReauthActionKind,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}
	if err := validateConnectionActionPathTarget(
		req.Metadata.Target,
		c,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	resp, err := r.core.ReauthConnection(ctx, id, req.Spec.ReturnToURL)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	renderConnectionSetupAction(gctx, val, resp)
}

// @Summary		Force connection state
// @Description	Force a connection to a specific state (admin operation)
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"Connection UUID"
// @Param			request	body		OpenAPIConnectionForceStateActionJson	true	"Force-state action"
// @Success		200		{object}	OpenAPIConnectionForceStateActionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_forceState [put]
func (r *ConnectionsRoutes) forceState(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	var req schemaapi.ConnectionForceStateAction
	if err := apgin.BindActionJSON(
		gctx,
		&req,
		schemaapi.ConnectionForceStateActionKind,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}
	if err := validateConnectionActionPathTarget(
		req.Metadata.Target,
		c,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	state := database.ConnectionState(req.Spec.State)
	if c.GetState() == state {
		resource, err := c.GetResource(ctx)
		if err != nil {
			apgin.WriteErr(gctx, nil, err)
			val.MarkErrorReturn()
			return
		}
		response := schemaapi.NewConnectionForceStateResponse(
			req.Metadata.Target,
			req.Spec,
			*resource,
		)
		if err := apgin.RenderActionJSON(gctx, http.StatusOK, &response, schemaapi.ConnectionForceStateActionKind); err != nil {
			apgin.WriteErr(gctx, nil, err)
			val.MarkErrorReturn()
		}
		return
	}

	err = c.SetState(ctx, state)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	resource, err := c.GetResource(ctx)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}
	response := schemaapi.NewConnectionForceStateResponse(
		req.Metadata.Target,
		req.Spec,
		*resource,
	)
	if err := apgin.RenderActionJSON(gctx, http.StatusOK, &response, schemaapi.ConnectionForceStateActionKind); err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
	}
}

// @Summary		Update connection
// @Description	Update a connection's name, labels, or annotations
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string						true	"Connection UUID"
// @Param			request	body		OpenAPIConnectionPatchJson	true	"Connection update request"
// @Success		200		{object}	OpenAPIConnectionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id} [patch]
func (r *ConnectionsRoutes) update(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	var req connectionschema.ConnectionPatch
	if err := apgin.BindResourceJSON(
		gctx,
		&req,
		smeta.ValidationModeUpdate,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	originalNamespace := c.GetNamespace()
	updated, err := r.core.UpdateConnection(ctx, id, &req)
	if err != nil {
		name := c.GetName()
		if req.Metadata != nil && req.Metadata.Name != nil {
			name = *req.Metadata.Name
		}
		if conflictErr := resourceNameConflictError(
			err,
			"connection",
			name,
			originalNamespace,
		); conflictErr != nil {
			apgin.WriteError(gctx, nil, conflictErr)
			val.MarkErrorReturn()
			return
		}
		apgin.WriteError(
			gctx,
			nil, // logger
			httperr.InternalServerError(httperr.WithInternalErr(err)),
		)
		val.MarkErrorReturn()
		return
	}

	resource, err := updated.GetResource(ctx)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, resource); err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
	}
}

// @Summary		Get OAuth2 scopes for a connection
// @Description	Returns the requested and granted OAuth2 scopes for the connection's current token. Only valid for OAuth2 connections.
// @Tags			connections
// @Produce		json
// @Param			id	path		string	true	"Connection ID"
// @Success		200	{object}	OpenAPIConnectionScopeListJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		422	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/scopes [get]
func (r *ConnectionsRoutes) getScopes(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, coreIface.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
		} else {
			apgin.WriteErr(gctx, nil, err)
		}
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	connector := c.GetConnector().GetDefinition()
	if connector.Auth == nil {
		apgin.WriteError(
			gctx,
			nil, // logger
			httperr.New(http.StatusUnprocessableEntity, "scopes are only available for OAuth2 connections"),
		)
		val.MarkErrorReturn()
		return
	}
	if _, ok := connector.Auth.Inner().(*cschema.AuthOAuth2); !ok {
		apgin.WriteError(
			gctx,
			nil, // logger
			httperr.New(http.StatusUnprocessableEntity, "scopes are only available for OAuth2 connections"),
		)
		val.MarkErrorReturn()
		return
	}

	token, err := r.db.GetOAuth2Token(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			apgin.WriteError(
				gctx,
				nil, // logger
				httperr.NotFound("no oauth2 token exists for this connection"),
			)
		} else {
			apgin.WriteErr(gctx, nil, err)
		}
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewConnectionScopeList(
		oauth2.SplitScopes(token.RequestedScopes),
		oauth2.SplitScopes(token.Scopes),
	))
}

func (r *ConnectionsRoutes) Register(g gin.IRouter) {
	g.POST(
		"/connections/_initiate",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("create").
			Build(),
		r.initiate,
	)
	g.GET(
		"/connections",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("list").
			Build(),
		r.list,
	)
	g.GET(
		"/connections/:id",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("get").
			ForIdField("id").
			Build(),
		r.get,
	)
	g.POST(
		"/connections/:id/_submit",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerbs("create", "update").
			ForIdField("id").
			Build(),
		r.submit,
	)
	g.GET(
		"/connections/:id/_setupStep",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerbs("create", "update").
			ForIdField("id").
			Build(),
		r.getSetupStep,
	)
	g.GET(
		"/connections/:id/_dataSource/:sourceId",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerbs("create", "update").
			ForIdField("id").
			Build(),
		r.getDataSource,
	)
	g.POST(
		"/connections/:id/_disconnect",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("disconnect").
			ForIdField("id").
			Build(),
		r.disconnect,
	)
	g.POST(
		"/connections/:id/_abort",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("create").
			ForIdField("id").
			Build(),
		r.abort,
	)
	g.POST(
		"/connections/:id/_reconfigure",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("update").
			ForIdField("id").
			Build(),
		r.reconfigure,
	)
	g.POST(
		"/connections/:id/_migrateGeneration",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("update").
			ForIdField("id").
			Build(),
		r.migrateGeneration,
	)
	g.POST(
		"/connections/:id/_cancelSetup",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("update").
			ForIdField("id").
			Build(),
		r.cancelSetup,
	)
	g.POST(
		"/connections/:id/_retry",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerbs("create", "update").
			ForIdField("id").
			Build(),
		r.retry,
	)
	g.POST(
		"/connections/:id/_reauth",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerbs("create", "update").
			ForIdField("id").
			Build(),
		r.reauth,
	)
	g.PUT(
		"/connections/:id/_forceState",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("force_state").
			ForIdField("id").
			Build(),
		r.forceState,
	)
	g.PATCH(
		"/connections/:id",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("update").
			ForIdField("id").
			Build(),
		r.update,
	)
	g.GET(
		"/connections/:id/scopes",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("get").
			ForIdField("id").
			Build(),
		r.getScopes,
	)
}

func NewConnectionsRoutes(
	cfg config.C,
	authService auth.A,
	db database.DB,
	r apredis.Client,
	c coreIface.C,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) *ConnectionsRoutes {
	return &ConnectionsRoutes{
		cfg:     cfg,
		auth:    authService,
		core:    c,
		db:      db,
		r:       r,
		httpf:   httpf,
		encrypt: encrypt,
		oauthf:  oauth2.NewFactory(cfg, db, r, c, httpf, encrypt, logger),
	}
}
