package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/pkg/errors"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"log/slog"
	"net/http"
	"time"
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

// @Summary		Initiate connection
// @Description	Initiate a new connection to an external service through a connector
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			request	body		InitiateConnectionRequest	true	"Connection initiation request"
// @Success		200		{object}	InitiateConnectionRedirect
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
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	// InitiateConnection also performs request validation for security
	resp, err := r.core.InitiateConnection(ctx, req)
	if err != nil {
		api_common.HttpStatusErrorBuilderFromError(err).
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, resp)
}

type ConnectionJson struct {
	Id        apid.ID                `json:"id" swaggertype:"string"`
	Namespace string                   `json:"namespace"`
	Labels    map[string]string        `json:"labels,omitempty"`
	State     database.ConnectionState `json:"state"`
	Connector ConnectorJson            `json:"connector"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}

func ConnectionToJson(conn coreIface.Connection) ConnectionJson {
	connector := ConnectorVersionToConnectorJson(conn.GetConnectorVersionEntity())

	return ConnectionJson{
		Id:        conn.GetId(),
		Namespace: conn.GetNamespace(),
		Labels:    conn.GetLabels(),
		State:     conn.GetState(),
		Connector: connector,
		CreatedAt: conn.GetCreatedAt(),
		UpdatedAt: conn.GetUpdatedAt(),
	}
}

type ListConnectionRequestQuery struct {
	Cursor        *string                   `form:"cursor"`
	LimitVal      *int32                    `form:"limit"`
	StateVal      *database.ConnectionState `form:"state"`
	NamespaceVal  *string                   `form:"namespace"`
	LabelSelector *string                   `form:"label_selector"`
	OrderByVal    *string                   `form:"order_by"`
}

type ListConnectionResponseJson struct {
	Items  []ConnectionJson `json:"items"`
	Cursor string           `json:"cursor,omitempty"`
}

// @Summary		List connections
// @Description	List connections with optional filtering and pagination
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			state			query		string	false	"Filter by connection state"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			label_selector	query		string	false	"Filter by label selector"
// @Param			order_by		query		string	false	"Order by field (e.g., 'created_at:asc')"
// @Success		200				{object}	SwaggerListConnectionResponse
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
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg(err.Error()).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	var ex coreIface.ListConnectionsExecutor

	if req.Cursor != nil {
		ex, err = r.core.ListConnectionsFromCursor(ctx, *req.Cursor)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithInternalErr(err).
				BuildStatusError().
				WriteGinResponse(nil, gctx)
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

		b = b.ForNamespaceMatchers(val.GetEffectiveNamespaceMatchers(req.NamespaceVal))

		if req.LabelSelector != nil {
			b = b.ForLabelSelector(*req.LabelSelector)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.ConnectionOrderByField](*req.OrderByVal)
			if err != nil {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithInternalErr(err).
					WithResponseMsg(err.Error()).
					BuildStatusError().
					WriteGinResponse(nil, gctx)
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidConnectionOrderByField(field) {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithResponseMsgf("invalid sort field '%s'", field).
					BuildStatusError().
					WriteGinResponse(nil, gctx)
				val.MarkErrorReturn()
				return
			}

			b.OrderBy(field, order)
		}

		ex = b
	}

	result := ex.FetchPage(ctx)

	if result.Error != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(result.Error).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ListConnectionResponseJson{
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
// @Success		200	{object}	SwaggerConnectionJson
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
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid id format").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("connection not found").
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if c == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("connection not found").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectionToJson(c))
}

type DisconnectResponseJson struct {
	TaskId     string         `json:"task_id"`
	Connection ConnectionJson `json:"connection"`
}

// @Summary		Disconnect connection
// @Description	Disconnect an existing connection and revoke its credentials
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
// @Success		200	{object}	SwaggerDisconnectResponse
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
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid id format").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	ti, err := r.core.DisconnectConnection(ctx, id)
	if err != nil {
		api_common.HttpStatusErrorBuilderFromError(err).
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	ra := auth.MustGetAuthFromGinContext(gctx)
	taskId, err := ti.
		BindToActor(ra.MustGetActor()).
		ToSecureEncryptedString(ctx, r.encrypt)

	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	// Hard code the disconnecting state to avoid race condictions with task workers
	connJson := ConnectionToJson(c)
	connJson.State = database.ConnectionStateDisconnecting

	response := DisconnectResponseJson{
		TaskId:     taskId,
		Connection: connJson,
	}

	gctx.PureJSON(http.StatusOK, response)
}

type ForceStateRequestJson struct {
	State database.ConnectionState `json:"state"`
}

// @Summary		Force connection state
// @Description	Force a connection to a specific state (admin operation)
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string					true	"Connection UUID"
// @Param			request	body		SwaggerForceStateRequest	true	"New state"
// @Success		200		{object}	SwaggerConnectionJson
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
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid id format").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	req := ForceStateRequestJson{}
	err = gctx.BindJSON(&req)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if req.State == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("state is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("connection not found").
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	if c.GetState() == req.State {
		gctx.PureJSON(http.StatusOK, ConnectionToJson(c))
		return
	}

	err = c.SetState(ctx, req.State)
	if err != nil {
		api_common.HttpStatusErrorBuilderFromError(err).
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectionToJson(c))
}

type UpdateConnectionRequestJson struct {
	Labels map[string]string `json:"labels"`
}

type PutConnectionLabelRequestJson struct {
	Value string `json:"value"`
}

type ConnectionLabelJson struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// @Summary		Update connection
// @Description	Update a connection's labels
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string							true	"Connection UUID"
// @Param			request	body		SwaggerUpdateConnectionRequest	true	"Connection update request"
// @Success		200		{object}	SwaggerConnectionJson
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
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid id format").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	var req UpdateConnectionRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid request body").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if req.Labels != nil {
		if err := database.Labels(req.Labels).Validate(); err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithInternalErr(err).
				WithResponseMsgf("invalid labels: %s", err.Error()).
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("connection not found").
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	if req.Labels != nil {
		_, err = r.db.UpdateConnectionLabels(ctx, id, req.Labels)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(err).
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		// Re-fetch connection to get updated state with connector info
		c, err = r.core.GetConnection(ctx, id)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(err).
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}
	}

	gctx.PureJSON(http.StatusOK, ConnectionToJson(c))
}

// @Summary		Get all labels for a connection
// @Description	Get all labels associated with a specific connection
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Connection UUID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/labels [get]
func (r *ConnectionsRoutes) getLabels(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid id format").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("connection not found").
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if c == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("connection not found").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	labels := c.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	gctx.PureJSON(http.StatusOK, labels)
}

// @Summary		Get a specific label for a connection
// @Description	Get a specific label value by key for a connection
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string	true	"Connection UUID"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	SwaggerConnectionLabelJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/labels/{label} [get]
func (r *ConnectionsRoutes) getLabel(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid id format").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	labelKey := gctx.Param("label")
	if labelKey == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("label key is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("connection not found").
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if c == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("connection not found").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	labels := c.GetLabels()
	value, exists := labels[labelKey]
	if !exists {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsgf("label '%s' not found", labelKey).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectionLabelJson{
		Key:   labelKey,
		Value: value,
	})
}

// @Summary		Set a label for a connection
// @Description	Set or update a specific label value by key for a connection
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path		string								true	"Connection UUID"
// @Param			label	path		string								true	"Label key"
// @Param			request	body		SwaggerPutConnectionLabelRequest	true	"Label value"
// @Success		200		{object}	SwaggerConnectionLabelJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/labels/{label} [put]
func (r *ConnectionsRoutes) putLabel(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid id format").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	labelKey := gctx.Param("label")
	if labelKey == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("label key is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateLabelKey(labelKey); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid label key: %s", err.Error()).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	var req PutConnectionLabelRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid request body").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateLabelValue(req.Value); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid label value: %s", err.Error()).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("connection not found").
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if c == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("connection not found").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	updatedConn, err := r.db.PutConnectionLabels(ctx, id, map[string]string{labelKey: req.Value})
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("connection not found").
				BuildStatusError().
				WriteGinResponse(nil, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectionLabelJson{
		Key:   labelKey,
		Value: updatedConn.Labels[labelKey],
	})
}

// @Summary		Delete a label from a connection
// @Description	Delete a specific label by key from a connection
// @Tags			connections
// @Accept			json
// @Produce		json
// @Param			id		path	string	true	"Connection UUID"
// @Param			label	path	string	true	"Label key"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/labels/{label} [delete]
func (r *ConnectionsRoutes) deleteLabel(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid id format").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	labelKey := gctx.Param("label")
	if labelKey == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("label key is required").
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	c, err := r.core.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			gctx.Status(http.StatusNoContent)
			val.MarkValidated()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	if c == nil {
		gctx.Status(http.StatusNoContent)
		val.MarkValidated()
		return
	}

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		httpErr.WriteGinResponse(nil, gctx)
		return
	}

	_, err = r.db.DeleteConnectionLabels(ctx, id, []string{labelKey})
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			gctx.Status(http.StatusNoContent)
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(nil, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.Status(http.StatusNoContent)
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
		"/connections/:id/_disconnect",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("disconnect").
			ForIdField("id").
			Build(),
		r.disconnect,
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
