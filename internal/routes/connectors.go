package routes

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/config"
	connIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"net/http"
)

type ConnectorJson struct {
	Id          uuid.UUID                      `json:"id"`
	Version     uint64                         `json:"version"`
	Namespace   string                         `json:"namespace"`
	State       database.ConnectorVersionState `json:"state"`
	DisplayName string                         `json:"display_name"`
	Highlight   string                         `json:"highlight,omitempty"`
	Description string                         `json:"description"`
	Logo        string                         `json:"logo"`
	Labels      map[string]string              `json:"labels,omitempty"`
	CreatedAt   time.Time                      `json:"created_at"`
	UpdatedAt   time.Time                      `json:"updated_at"`

	Versions int64                           `json:"versions,omitempty"`
	States   database.ConnectorVersionStates `json:"states,omitempty"`
}

func ConnectorToJson(c connIface.Connector) ConnectorJson {
	result := ConnectorVersionToConnectorJson(c)
	result.Versions = c.GetTotalVersions()
	result.States = c.GetStates()
	return result
}

func ConnectorVersionToConnectorJson(cv connIface.ConnectorVersion) ConnectorJson {
	def := cv.GetDefinition()
	logo := ""
	if def.Logo != nil {
		logo = def.Logo.GetUrl()
	}

	return ConnectorJson{
		Id:          cv.GetId(),
		Version:     cv.GetVersion(),
		Namespace:   cv.GetNamespace(),
		State:       cv.GetState(),
		Highlight:   def.Highlight,
		DisplayName: def.DisplayName,
		Description: def.Description,
		Logo:        logo,
		Labels:      cv.GetLabels(),
		CreatedAt:   cv.GetCreatedAt(),
		UpdatedAt:   cv.GetUpdatedAt(),
	}
}

type ListConnectorsRequestQueryParams struct {
	Cursor        *string                         `form:"cursor"`
	LimitVal      *int32                          `form:"limit"`
	StateVal      *database.ConnectorVersionState `form:"state"`
	NamespaceVal  *string                         `form:"namespace"`
	LabelSelector *string                         `form:"label_selector"`
	OrderByVal    *string                         `form:"order_by"`
}

type ListConnectorsResponseJson struct {
	Items  []ConnectorJson `json:"items"`
	Cursor string          `json:"cursor,omitempty"`
}

type ConnectorVersionJson struct {
	Id         uuid.UUID                      `json:"id"`
	Version    uint64                         `json:"version"`
	Namespace  string                         `json:"namespace"`
	State      database.ConnectorVersionState `json:"state"`
	Definition cschema.Connector              `json:"definition"`
	Labels     map[string]string              `json:"labels,omitempty"`
	CreatedAt  time.Time                      `json:"created_at"`
	UpdatedAt  time.Time                      `json:"updated_at"`
}

func ConnectorVersionToJson(cv connIface.ConnectorVersion) ConnectorVersionJson {
	def := cv.GetDefinition()

	return ConnectorVersionJson{
		Id:         cv.GetId(),
		Version:    cv.GetVersion(),
		Namespace:  cv.GetNamespace(),
		State:      cv.GetState(),
		Definition: *def,
		Labels:     cv.GetLabels(),
		CreatedAt:  cv.GetCreatedAt(),
		UpdatedAt:  cv.GetUpdatedAt(),
	}
}

type ListConnectorVersionsRequestQueryParams struct {
	Cursor        *string                         `form:"cursor"`
	LimitVal      *int32                          `form:"limit"`
	StateVal      *database.ConnectorVersionState `form:"state"`
	NamespaceVal  *string                         `form:"namespace"`
	LabelSelector *string                         `form:"label_selector"`
	OrderByVal    *string                         `form:"order_by"`
}

type ListConnectorVersionsResponseJson struct {
	Items  []ConnectorVersionJson `json:"items"`
	Cursor string                 `json:"cursor,omitempty"`
}

type ConnectorsRoutes struct {
	cfg         config.C
	connectors  connIface.C
	authService auth.A
}

// @Summary		Get connector
// @Description	Get a specific connector by its UUID
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Connector UUID"
// @Success		200	{object}	SwaggerConnectorJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id} [get]
func (r *ConnectorsRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	connectorIdStr := gctx.Param("id")
	if connectorIdStr == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	connectorId, err := uuid.Parse(connectorIdStr)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if connectorId == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	result := r.connectors.
		ListConnectorsBuilder().
		ForId(connectorId).
		Limit(1).
		FetchPage(ctx)

	if result.Error != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(result.Error).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if len(result.Results) == 0 {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsgf("connector '%s' not found", connectorId).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	c := result.Results[0]

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectorToJson(c))
}

// @Summary		List connectors
// @Description	List connectors with optional filtering and pagination
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			state			query		string	false	"Filter by connector state"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			label_selector	query		string	false	"Filter by label selector"
// @Param			order_by		query		string	false	"Order by field (e.g., 'created_at:asc')"
// @Success		200				{object}	SwaggerListConnectorsResponse
// @Failure		400				{object}	ErrorResponse
// @Failure		401				{object}	ErrorResponse
// @Failure		500				{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors [get]
func (r *ConnectorsRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req ListConnectorsRequestQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg(err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	var err error
	var ex connIface.ListConnectorsExecutor

	if req.Cursor != nil {
		ex, err = r.connectors.ListConnectorsFromCursor(ctx, *req.Cursor)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(err).
				WithResponseMsg("failed to list core from cursor").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
	} else {
		b := r.connectors.ListConnectorsBuilder()

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
			field, order, err := pagination.SplitOrderByParam[database.ConnectorOrderByField](*req.OrderByVal)
			if err != nil {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithInternalErr(err).
					WithResponseMsg(err.Error()).
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidConnectorOrderByField(field) {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithResponseMsgf("invalid sort field '%s'", field).
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
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
			DefaultStatusInternalServerError().
			WithInternalErr(result.Error).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ListConnectorsResponseJson{
		Items:  util.Map(auth.FilterForValidatedResources(val, result.Results), ConnectorToJson),
		Cursor: result.Cursor,
	})
}

// @Summary		Get connector version
// @Description	Get a specific version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string	true	"Connector UUID"
// @Param			version	path		integer	true	"Version number"
// @Success		200		{object}	SwaggerConnectorVersionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions/{version} [get]
func (r *ConnectorsRoutes) getVersion(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	connectorIdStr := gctx.Param("id")

	if connectorIdStr == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	connectorId, err := uuid.Parse(connectorIdStr)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if connectorId == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	b := r.connectors.
		ListConnectorVersionsBuilder().
		ForId(connectorId).
		Limit(1)

	versionStr := gctx.Param("version")

	if versionStr == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("version is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	version, err := strconv.ParseUint(versionStr, 10, 64)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("failed to parse version as an integer").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	b = b.ForVersion(version)

	// TODO: support lookup by certain states

	result := b.FetchPage(ctx)
	if result.Error != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(result.Error).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if len(result.Results) == 0 {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsgf("connector version '%s:%d' not found", connectorId, version).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	cv := result.Results[0]

	if httpErr := val.ValidateHttpStatusError(cv); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectorVersionToJson(cv))
}

// @Summary		List connector versions
// @Description	List all versions of a specific connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id				path		string	true	"Connector UUID"
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			state			query		string	false	"Filter by version state"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			label_selector	query		string	false	"Filter by label selector"
// @Param			order_by		query		string	false	"Order by field (e.g., 'version:desc')"
// @Success		200				{object}	SwaggerListConnectorVersionsResponse
// @Failure		400				{object}	ErrorResponse
// @Failure		401				{object}	ErrorResponse
// @Failure		500				{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions [get]
func (r *ConnectorsRoutes) listVersions(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var err error
	var ex connIface.ListConnectorVersionsExecutor

	connectorIdStr := gctx.Param("id")

	if connectorIdStr == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	connectorId, err := uuid.Parse(connectorIdStr)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if connectorId == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	var req ListConnectorVersionsRequestQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg(err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	// Compute effective namespace matchers for permission-based filtering at query level
	effectiveMatchers := val.GetEffectiveNamespaceMatchers(req.NamespaceVal)
	if effectiveMatchers != nil && len(effectiveMatchers) == 0 {
		// No access to any namespaces for this resource/verb
		val.MarkValidated()
		gctx.PureJSON(http.StatusOK, ListConnectorVersionsResponseJson{Items: []ConnectorVersionJson{}})
		return
	}

	if req.Cursor != nil {
		ex, err = r.connectors.ListConnectorVersionsFromCursor(ctx, *req.Cursor)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(err).
				WithResponseMsg("failed to list connector versions from cursor").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
	} else {
		b := r.connectors.ListConnectorVersionsBuilder().
			ForId(connectorId)

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.StateVal != nil {
			b = b.ForState(*req.StateVal)
		}

		// Apply namespace restrictions at query level
		if effectiveMatchers != nil {
			b = b.ForNamespaceMatchers(effectiveMatchers)
		} else if req.NamespaceVal != nil {
			// Admin users with a query filter
			b = b.ForNamespaceMatcher(*req.NamespaceVal)
		}

		if req.LabelSelector != nil {
			b = b.ForLabelSelector(*req.LabelSelector)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.ConnectorVersionOrderByField](*req.OrderByVal)
			if err != nil {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithInternalErr(err).
					WithResponseMsg(err.Error()).
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidConnectorVersionOrderByField(field) {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithResponseMsgf("invalid sort field '%s'", field).
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
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
			DefaultStatusInternalServerError().
			WithInternalErr(result.Error).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ListConnectorVersionsResponseJson{
		Items:  util.Map(auth.FilterForValidatedResources(val, result.Results), ConnectorVersionToJson),
		Cursor: result.Cursor,
	})
}

func (r *ConnectorsRoutes) Register(g gin.IRouter) {
	g.GET(
		"/connectors",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForVerb("list").
			Build(),
		r.list,
	)
	g.GET(
		"/connectors/:id",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.get,
	)
	g.GET("/connectors/:id/versions",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("list/versions").
			Build(),
		r.listVersions,
	)
	g.GET(
		"/connectors/:id/versions/:version",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("list/versions").
			Build(),
		r.getVersion,
	)
}

func NewConnectorsRoutes(cfg config.C, authService auth.A, c connIface.C) *ConnectorsRoutes {
	return &ConnectorsRoutes{
		cfg:         cfg,
		authService: authService,
		connectors:  c,
	}
}
