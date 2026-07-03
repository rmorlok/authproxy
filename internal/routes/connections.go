package routes

import (
	"context"
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
	"github.com/rmorlok/authproxy/internal/routes/key_value"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"log/slog"
	"net/http"
)

type ConnectionsRoutes struct {
	cfg           config.C
	auth          auth.A
	core          coreIface.C
	db            database.DB
	r             apredis.Client
	httpf         httpf.F
	encrypt       encrypt.E
	oauthf        oauth2.Factory
	labelsAdapter key_value.Adapter[apid.ID]
	annotsAdapter key_value.Adapter[apid.ID]
}

type InitiateConnectionRequest = schemaapi.InitiateConnectionRequest
type ConnectionSetupRedirect = schemaapi.ConnectionSetupRedirect
type ConnectionSetupForm = schemaapi.ConnectionSetupForm
type ConnectionSetupComplete = schemaapi.ConnectionSetupComplete
type SubmitConnectionRequest = schemaapi.SubmitConnectionRequest
type DataSourceOptionJson = schemaapi.DataSourceOptionJson
type ConnectionState = schemaapi.ConnectionState
type ConnectionHealthState = schemaapi.ConnectionHealthState
type ConnectionJson = schemaapi.ConnectionJson
type ListConnectionResponseJson = schemaapi.ListConnectionResponseJson
type DisconnectConnectionRequestJson = schemaapi.DisconnectConnectionRequestJson
type DisconnectResponseJson = schemaapi.DisconnectResponseJson
type MigrateConnectionVersionRequestJson = schemaapi.MigrateConnectionVersionRequestJson
type MigrateConnectionVersionResponseJson = schemaapi.MigrateConnectionVersionResponseJson
type ForceStateRequestJson = schemaapi.ForceConnectionStateRequestJson
type UpdateConnectionRequestJson = schemaapi.UpdateConnectionRequestJson
type ProxyResponse = schemaapi.ProxyResponseJson

type OpenAPIConnectionJson = schemaapiopenapi.ConnectionJson
type OpenAPIListConnectionResponseJson = schemaapiopenapi.ListConnectionResponseJson
type OpenAPIDisconnectConnectionRequestJson = schemaapiopenapi.DisconnectConnectionRequestJson
type OpenAPIDisconnectResponseJson = schemaapiopenapi.DisconnectResponseJson
type OpenAPIMigrateConnectionVersionRequestJson = schemaapiopenapi.MigrateConnectionVersionRequestJson
type OpenAPIMigrateConnectionVersionResponseJson = schemaapiopenapi.MigrateConnectionVersionResponseJson
type ProxyRequest = schemaapiopenapi.ProxyRequestJson
type OpenAPIProxyResponseJson = schemaapiopenapi.ProxyResponseJson

// @Summary		Initiate connection
// @Description	Initiate a new connection to an external service through a connector
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			request	body		InitiateConnectionRequest	true	"Connection initiation request"
// @Success		200		{object}	ConnectionSetupRedirect
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/_initiate [post]
func (r *ConnectionsRoutes) initiate(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req coreIface.InitiateConnectionRequest
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	// InitiateConnection also performs request validation for security
	resp, err := r.core.InitiateConnection(ctx, req)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, resp)
}

// @Summary		Submit connection form
// @Description	Submit form data for a connection setup step
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string					true	"Connection ID"
// @Param			request	body		SubmitConnectionRequest	true	"Form submission data"
// @Success		200		{object}	ConnectionSetupComplete
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

	var req coreIface.SubmitConnectionRequest
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	resp, err := c.SubmitForm(ctx, req)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, resp)
}

// @Summary		Get setup step
// @Description	Get the current setup step for a connection, used to resume an interrupted setup flow
// @Tags			connections
// @Produce		json
// @Param			id	path		string	true	"Connection ID"
// @Param			return_to_url	query	string	false	"URL to return to after a resumed redirect step"
// @Success		200	{object}	ConnectionSetupComplete
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_setup_step [get]
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

	resp, err := c.GetCurrentSetupStepResponse(ctx, gctx.Query("return_to_url"))
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, resp)
}

// @Summary		Get data source options
// @Description	Fetch dynamic options for a data source defined in the current setup step
// @Tags			connections
// @Produce		json
// @Param			id			path		string	true	"Connection ID"
// @Param			source_id	path		string	true	"Data Source ID"
// @Success		200	{array}		DataSourceOptionJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_data_source/{source_id} [get]
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

	sourceId := gctx.Param("source_id")
	if sourceId == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("source_id is required"))
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

	apgin.APIJSON(gctx, http.StatusOK, options)
}

func ConnectionToJson(conn coreIface.Connection) ConnectionJson {
	connector := ConnectorVersionToConnectorJson(conn.GetConnectorVersionEntity())

	return ConnectionJson{
		Id:          conn.GetId(),
		Namespace:   conn.GetNamespace(),
		Labels:      conn.GetLabels(),
		Annotations: conn.GetAnnotations(),
		State:       schemaapi.ConnectionState(conn.GetState()),
		HealthState: schemaapi.ConnectionHealthState(conn.GetHealthState()),
		SetupStep:   conn.GetSetupStep(),
		SetupError:  conn.GetSetupError(),
		Connector:   connector,
		CreatedAt:   conn.GetCreatedAt(),
		UpdatedAt:   conn.GetUpdatedAt(),
	}
}

type ListConnectionRequestQuery struct {
	Cursor        *string                   `form:"cursor"`
	LimitVal      *int32                    `form:"limit"`
	StateVal      *database.ConnectionState `form:"state"`
	ConnectorId   *string                   `form:"connector_id"`
	NamespaceVal  *string                   `form:"namespace"`
	LabelSelector *string                   `form:"label_selector"`
	OrderByVal    *string                   `form:"order_by"`
}

// @Summary		List connections
// @Description	List connections with optional filtering and pagination
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			state			query		string	false	"Filter by connection state"
// @Param			connector_id	query		string	false	"Filter by connector ID"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			label_selector	query		string	false	"Filter by label selector"
// @Param			order_by		query		string	false	"Order by field (e.g., 'created_at:asc')"
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
				apgin.WriteError(gctx, nil, httperr.BadRequest("invalid connector_id format", httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}
			if err := connectorId.ValidatePrefix(apid.PrefixConnectorVersion); err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequest("invalid connector_id prefix", httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}
			b = b.ForConnectorId(connectorId)
		}

		b = b.ForNamespaceMatchers(val.GetEffectiveNamespaceMatchers(req.NamespaceVal))

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

	apgin.APIJSON(gctx, http.StatusOK, ListConnectionResponseJson{
		Items: util.Map(auth.FilterForValidatedResources(val, result.Results), func(c coreIface.Connection) ConnectionJson {
			return ConnectionToJson(c)
		}),
		Cursor: result.Cursor,
	})
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

	apgin.APIJSON(gctx, http.StatusOK, ConnectionToJson(c))
}

// @Summary		Disconnect connection
// @Description	Disconnect an existing connection and revoke its credentials
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
// @Param			request	body		OpenAPIDisconnectConnectionRequestJson	false	"Disconnect options"
// @Success		200	{object}	OpenAPIDisconnectResponseJson
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

	opts, ok := r.parseConnectionDisconnectRequest(gctx)
	if !ok {
		return
	}

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
	connJson := ConnectionToJson(c)
	connJson.State = schemaapi.ConnectionState(database.ConnectionStateDisconnecting)

	response := DisconnectResponseJson{
		TaskId:     taskId,
		Connection: connJson,
	}

	apgin.APIJSON(gctx, http.StatusOK, response)
}

func (r *ConnectionsRoutes) parseConnectionDisconnectRequest(gctx *gin.Context) (coreIface.ConnectionDisconnectOptions, bool) {
	val := auth.MustGetValidatorFromGinContext(gctx)

	req := DisconnectConnectionRequestJson{}
	if gctx.Request.Body != http.NoBody && gctx.Request.ContentLength != 0 {
		if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
			val.MarkErrorReturn()
			return coreIface.ConnectionDisconnectOptions{}, false
		}
	}

	timeout := defaultConnectorLifecycleTimeout
	if req.TimeoutSeconds != nil {
		if *req.TimeoutSeconds <= 0 {
			apgin.WriteError(gctx, nil, httperr.BadRequest("timeout_seconds must be greater than zero"))
			val.MarkErrorReturn()
			return coreIface.ConnectionDisconnectOptions{}, false
		}
		timeout = time.Duration(*req.TimeoutSeconds) * time.Second
	}

	return coreIface.ConnectionDisconnectOptions{Timeout: timeout}, true
}

// @Summary		Migrate connection connector version
// @Description	Start a workflow that migrates an existing connection to another version of the same connector
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string										true	"Connection UUID"
// @Param			request	body		OpenAPIMigrateConnectionVersionRequestJson	true	"Migration options"
// @Success		200		{object}	OpenAPIMigrateConnectionVersionResponseJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_migrate_version [post]
func (r *ConnectionsRoutes) migrateVersion(gctx *gin.Context) {
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

	opts, ok := r.parseConnectionMigrationRequest(gctx)
	if !ok {
		return
	}

	task, err := r.core.MigrateConnectionVersion(ctx, id, opts)
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

	apgin.APIJSON(gctx, http.StatusOK, MigrateConnectionVersionResponseJson{
		TaskId:        taskId,
		ConnectionId:  task.ConnectionID,
		SourceVersion: task.SourceVersion,
		TargetVersion: task.TargetVersion,
	})
}

func (r *ConnectionsRoutes) parseConnectionMigrationRequest(gctx *gin.Context) (coreIface.ConnectionMigrationOptions, bool) {
	val := auth.MustGetValidatorFromGinContext(gctx)

	req := MigrateConnectionVersionRequestJson{}
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return coreIface.ConnectionMigrationOptions{}, false
	}
	if req.TargetVersion == 0 {
		apgin.WriteError(gctx, nil, httperr.BadRequest("target_version is required"))
		val.MarkErrorReturn()
		return coreIface.ConnectionMigrationOptions{}, false
	}

	timeout := defaultConnectorLifecycleTimeout
	if req.TimeoutSeconds != nil {
		if *req.TimeoutSeconds <= 0 {
			apgin.WriteError(gctx, nil, httperr.BadRequest("timeout_seconds must be greater than zero"))
			val.MarkErrorReturn()
			return coreIface.ConnectionMigrationOptions{}, false
		}
		timeout = time.Duration(*req.TimeoutSeconds) * time.Second
	}

	return coreIface.ConnectionMigrationOptions{
		TargetVersion: req.TargetVersion,
		Timeout:       timeout,
	}, true
}

// @Summary		Abort connection setup
// @Description	Abort an in-progress connection setup, cleaning up credentials and deleting the connection
// @Tags			connections
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
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
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
// @Success		200	{object}	ConnectionSetupForm
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

	resp, err := c.Reconfigure(ctx)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, resp)
}

// @Summary		Cancel in-flight setup
// @Description	Abandon a reconfigure attempt on a ready connection by clearing setup_step and setup_error. The connection remains ready and its previously stored configuration continues to apply.
// @Tags			connections
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
// @Success		204
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_cancel_setup [post]
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

	if err := c.CancelSetup(ctx); err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	gctx.Status(http.StatusNoContent)
}

type RetryConnectionRequest struct {
	ReturnToUrl string `json:"return_to_url,omitempty"`
}

// @Summary		Retry connection setup
// @Description	Retry a connection setup that ended in a terminal failure state. Applies to any setup-phase failure: an auth-phase failure such as an OAuth token-exchange error (auth_failed) or a probe failure during verify (verify_failed). Clears the recorded error and either returns to the first preconnect step (if the connector defines one, so the user can correct any input that led to the failure) or re-initiates the auth flow from scratch.
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path	string					true	"Connection UUID"
// @Param			request	body	RetryConnectionRequest	true	"Retry request"
// @Success		200	{object}	ConnectionSetupForm
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

	var req RetryConnectionRequest
	// Body is optional — return_to_url is only needed when the connector has no preconnect steps.
	if err := bindOptionalJSONBody(gctx, &req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	resp, err := r.core.RetryConnectionSetup(ctx, id, req.ReturnToUrl)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, resp)
}

type ReauthConnectionRequest struct {
	ReturnToUrl string `json:"return_to_url,omitempty"`
}

// @Summary		Re-authenticate a connection
// @Description	Re-run the credential-collection portion of setup against an existing Ready connection. Used for user-driven credential rotation and as the recovery path when a connection is unhealthy. For api-key, returns a fresh credentials form (no prior values pre-filled); on submit the existing credential row is soft-deleted and a new one inserted in the same transaction. For OAuth2, restarts at preconnect:0 if defined, otherwise re-initiates the OAuth redirect. The connection's lifecycle state stays Ready throughout.
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path	string					true	"Connection UUID"
// @Param			request	body	ReauthConnectionRequest	true	"Reauth request"
// @Success		200	{object}	ConnectionSetupForm
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

	var req ReauthConnectionRequest
	// Body is optional — return_to_url is only needed for OAuth2 connectors with no preconnect steps.
	if err := bindOptionalJSONBody(gctx, &req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	resp, err := r.core.ReauthConnection(ctx, id, req.ReturnToUrl)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, resp)
}

// @Summary		Force connection state
// @Description	Force a connection to a specific state (admin operation)
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"Connection UUID"
// @Param			request	body		ForceStateRequestJson	true	"New state"
// @Success		200		{object}	OpenAPIConnectionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_force_state [put]
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

	req := ForceStateRequestJson{}
	err = gctx.BindJSON(&req)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}

	if req.State == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("state is required"))
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

	state := database.ConnectionState(req.State)
	if c.GetState() == state {
		apgin.APIJSON(gctx, http.StatusOK, ConnectionToJson(c))
		return
	}

	err = c.SetState(ctx, state)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, ConnectionToJson(c))
}

// @Summary		Update connection
// @Description	Update a connection's labels
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string						true	"Connection UUID"
// @Param			request	body		UpdateConnectionRequestJson	true	"Connection update request"
// @Success		200		{object}	OpenAPIConnectionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
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

	var req UpdateConnectionRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if req.Labels != nil {
		if err := database.ValidateUserLabels(req.Labels); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid labels: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
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

	if req.Labels != nil {
		_, err = r.db.UpdateConnectionLabels(ctx, id, req.Labels)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}

		// Re-fetch connection to get updated state with connector info
		c, err = r.core.GetConnection(ctx, id)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	apgin.APIJSON(gctx, http.StatusOK, ConnectionToJson(c))
}

// Label and annotation handlers for connections delegate to a shared
// generic adapter (see internal/routes/key_value). The doc comments below
// drive the OpenAPI spec; the bodies forward to the adapter.

// @Summary		Get all labels for a connection
// @Description	Get all labels associated with a specific connection
// @Tags			connections
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/labels [get]
func (r *ConnectionsRoutes) getLabels(gctx *gin.Context) { r.labelsAdapter.HandleList(gctx) }

// @Summary		Get a specific label for a connection
// @Description	Get a specific label value by key for a connection
// @Tags			connections
// @Produce		json
// @Param			id		path		string	true	"Connection UUID"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	KeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/labels/{label} [get]
func (r *ConnectionsRoutes) getLabel(gctx *gin.Context) { r.labelsAdapter.HandleGet(gctx) }

// @Summary		Set a label for a connection
// @Description	Set or update a specific label value by key for a connection
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string						true	"Connection UUID"
// @Param			label	path		string						true	"Label key"
// @Param			request	body		PutKeyValueRequestJson	true	"Label value"
// @Success		200		{object}	KeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/labels/{label} [put]
func (r *ConnectionsRoutes) putLabel(gctx *gin.Context) { r.labelsAdapter.HandlePut(gctx) }

// @Summary		Delete a label from a connection
// @Description	Delete a specific label by key from a connection
// @Tags			connections
// @Param			id		path	string	true	"Connection UUID"
// @Param			label	path	string	true	"Label key"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/labels/{label} [delete]
func (r *ConnectionsRoutes) deleteLabel(gctx *gin.Context) { r.labelsAdapter.HandleDelete(gctx) }

// @Summary		Get all annotations for a connection
// @Description	Get all annotations associated with a specific connection
// @Tags			connections
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/annotations [get]
func (r *ConnectionsRoutes) getAnnotations(gctx *gin.Context) { r.annotsAdapter.HandleList(gctx) }

// @Summary		Get a specific annotation for a connection
// @Description	Get a specific annotation value by key for a connection
// @Tags			connections
// @Produce		json
// @Param			id			path		string	true	"Connection UUID"
// @Param			annotation	path		string	true	"Annotation key"
// @Success		200			{object}	KeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/annotations/{annotation} [get]
func (r *ConnectionsRoutes) getAnnotation(gctx *gin.Context) { r.annotsAdapter.HandleGet(gctx) }

// @Summary		Set an annotation for a connection
// @Description	Set or update a specific annotation value by key for a connection
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id			path		string						true	"Connection UUID"
// @Param			annotation	path		string						true	"Annotation key"
// @Param			request		body		PutKeyValueRequestJson	true	"Annotation value"
// @Success		200			{object}	KeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/annotations/{annotation} [put]
func (r *ConnectionsRoutes) putAnnotation(gctx *gin.Context) { r.annotsAdapter.HandlePut(gctx) }

// @Summary		Delete an annotation from a connection
// @Description	Delete a specific annotation by key from a connection
// @Tags			connections
// @Param			id			path	string	true	"Connection UUID"
// @Param			annotation	path	string	true	"Annotation key"
// @Success		204			"No Content"
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/annotations/{annotation} [delete]
func (r *ConnectionsRoutes) deleteAnnotation(gctx *gin.Context) { r.annotsAdapter.HandleDelete(gctx) }

// ConnectionScopesJson exposes the OAuth2 scopes a connection requested at auth time and the
// scopes the provider actually granted. The two sets can diverge when the provider chooses to
// honor only a subset of the request (RFC 6749 §3.3).
type ConnectionScopesJson struct {
	Requested []string `json:"requested"`
	Granted   []string `json:"granted"`
}

// @Summary		Get OAuth2 scopes for a connection
// @Description	Returns the requested and granted OAuth2 scopes for the connection's current token. Only valid for OAuth2 connections.
// @Tags			connections
// @Produce		json
// @Param			id	path		string	true	"Connection ID"
// @Success		200	{object}	ConnectionScopesJson
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

	connector := c.GetConnectorVersionEntity().GetDefinition()
	if connector.Auth == nil {
		apgin.WriteError(gctx, nil, httperr.New(http.StatusUnprocessableEntity, "scopes are only available for OAuth2 connections"))
		val.MarkErrorReturn()
		return
	}
	if _, ok := connector.Auth.Inner().(*cschema.AuthOAuth2); !ok {
		apgin.WriteError(gctx, nil, httperr.New(http.StatusUnprocessableEntity, "scopes are only available for OAuth2 connections"))
		val.MarkErrorReturn()
		return
	}

	token, err := r.db.GetOAuth2Token(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("no oauth2 token exists for this connection"))
		} else {
			apgin.WriteErr(gctx, nil, err)
		}
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, ConnectionScopesJson{
		Requested: oauth2.SplitScopes(token.RequestedScopes),
		Granted:   oauth2.SplitScopes(token.Scopes),
	})
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
		"/connections/:id/_setup_step",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerbs("create", "update").
			ForIdField("id").
			Build(),
		r.getSetupStep,
	)
	g.GET(
		"/connections/:id/_data_source/:source_id",
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
		"/connections/:id/_migrate_version",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("update").
			ForIdField("id").
			Build(),
		r.migrateVersion,
	)
	g.POST(
		"/connections/:id/_cancel_setup",
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
		"/connections/:id/_force_state",
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
		"/connections/:id/labels",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("get").
			ForIdField("id").
			Build(),
		r.getLabels,
	)
	g.GET(
		"/connections/:id/labels/:label",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("get").
			ForIdField("id").
			Build(),
		r.getLabel,
	)
	g.PUT(
		"/connections/:id/labels/:label",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("update").
			ForIdField("id").
			Build(),
		r.putLabel,
	)
	g.DELETE(
		"/connections/:id/labels/:label",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("update").
			ForIdField("id").
			Build(),
		r.deleteLabel,
	)
	g.GET(
		"/connections/:id/annotations",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("get").
			ForIdField("id").
			Build(),
		r.getAnnotations,
	)
	g.GET(
		"/connections/:id/annotations/:annotation",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("get").
			ForIdField("id").
			Build(),
		r.getAnnotation,
	)
	g.PUT(
		"/connections/:id/annotations/:annotation",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("update").
			ForIdField("id").
			Build(),
		r.putAnnotation,
	)
	g.DELETE(
		"/connections/:id/annotations/:annotation",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("update").
			ForIdField("id").
			Build(),
		r.deleteAnnotation,
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
	parseConnID := func(gctx *gin.Context) (apid.ID, *httperr.Error) {
		id, err := apid.Parse(gctx.Param("id"))
		if err != nil {
			return apid.Nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err))
		}
		if id == apid.Nil {
			return apid.Nil, httperr.BadRequest("id is required")
		}
		return id, nil
	}

	getConn := func(ctx context.Context, id apid.ID) (key_value.Resource, error) {
		conn, err := c.GetConnection(ctx, id)
		if err != nil {
			return nil, err
		}
		if conn == nil {
			return nil, nil
		}
		return conn, nil
	}

	authGet := authService.NewRequiredBuilder().
		ForResource("connections").
		ForVerb("get").
		ForIdField("id").
		Build()
	authMutate := authService.NewRequiredBuilder().
		ForResource("connections").
		ForVerb("update").
		ForIdField("id").
		Build()

	labelsAdapter := key_value.Adapter[apid.ID]{
		Kind:         key_value.Label,
		ResourceName: "connection",
		PathPrefix:   "/connections/:id",
		AuthGet:      authGet,
		AuthMutate:   authMutate,
		ParseID:      parseConnID,
		Get:          getConn,
		Put: func(ctx context.Context, id apid.ID, kv map[string]string) (key_value.Resource, error) {
			return db.PutConnectionLabels(ctx, id, kv)
		},
		Delete: func(ctx context.Context, id apid.ID, keys []string) (key_value.Resource, error) {
			return db.DeleteConnectionLabels(ctx, id, keys)
		},
	}

	annotsAdapter := key_value.Adapter[apid.ID]{
		Kind:         key_value.Annotation,
		ResourceName: "connection",
		PathPrefix:   "/connections/:id",
		AuthGet:      authGet,
		AuthMutate:   authMutate,
		ParseID:      parseConnID,
		Get:          getConn,
		Put: func(ctx context.Context, id apid.ID, kv map[string]string) (key_value.Resource, error) {
			return db.PutConnectionAnnotations(ctx, id, kv)
		},
		Delete: func(ctx context.Context, id apid.ID, keys []string) (key_value.Resource, error) {
			return db.DeleteConnectionAnnotations(ctx, id, keys)
		},
	}

	return &ConnectionsRoutes{
		cfg:           cfg,
		auth:          authService,
		core:          c,
		db:            db,
		r:             r,
		httpf:         httpf,
		encrypt:       encrypt,
		oauthf:        oauth2.NewFactory(cfg, db, r, c, httpf, encrypt, logger),
		labelsAdapter: labelsAdapter,
		annotsAdapter: annotsAdapter,
	}
}
