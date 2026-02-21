package routes

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	connIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type ConnectorJson struct {
	Id          uuid.UUID                      `json:"id"`
	Version     uint64                         `json:"version"`
	Namespace   string                         `json:"namespace"`
	State       database.ConnectorVersionState `json:"state"`
	DisplayName   string                         `json:"display_name"`
	Highlight     string                         `json:"highlight,omitempty"`
	Description   string                         `json:"description"`
	StatusPageUrl string                         `json:"status_page_url,omitempty"`
	Logo          string                         `json:"logo"`
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
		Id:            cv.GetId(),
		Version:       cv.GetVersion(),
		Namespace:     cv.GetNamespace(),
		State:         cv.GetState(),
		Highlight:     def.Highlight,
		DisplayName:   def.DisplayName,
		Description:   def.Description,
		StatusPageUrl: def.StatusPageUrl,
		Logo:          logo,
		Labels:        cv.GetLabels(),
		CreatedAt:     cv.GetCreatedAt(),
		UpdatedAt:     cv.GetUpdatedAt(),
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

// CreateConnectorRequestJson is the request body for POST /connectors
type CreateConnectorRequestJson struct {
	Namespace  string            `json:"namespace"`
	Definition cschema.Connector `json:"definition"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// UpdateConnectorRequestJson is the request body for PATCH /connectors/:id and PATCH /connectors/:id/versions/:version
type UpdateConnectorRequestJson struct {
	Definition *cschema.Connector `json:"definition,omitempty"`
	Labels     *map[string]string `json:"labels,omitempty"`
}

// CreateConnectorVersionRequestJson is the request body for POST /connectors/:id/versions
type CreateConnectorVersionRequestJson struct {
	Definition *cschema.Connector `json:"definition,omitempty"`
	Labels     *map[string]string `json:"labels,omitempty"`
}

// ConnectorLabelJson is a single label key-value pair for a connector
type ConnectorLabelJson struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PutConnectorLabelRequestJson is the request body for PUT /connectors/:id/labels/:label
type PutConnectorLabelRequestJson struct {
	Value string `json:"value"`
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

// @Summary		Create connector
// @Description	Create a new connector with version 1 in draft state
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			request	body		SwaggerCreateConnectorRequest	true	"Connector creation request"
// @Success		201		{object}	SwaggerConnectorVersionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors [post]
func (r *ConnectorsRoutes) createConnector(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req CreateConnectorRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg(err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateNamespacePath(req.Namespace); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid namespace '%s': %s", req.Namespace, err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if err := database.Labels(req.Labels).Validate(); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid labels: %s", err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if err := req.Definition.Validate(&common.ValidationContext{}); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid connector definition: %s", err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if err := val.ValidateNamespace(req.Namespace); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusForbidden().
			WithPublicErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	result, err := r.connectors.CreateConnectorVersion(ctx, req.Namespace, &req.Definition, req.Labels)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusCreated, ConnectorVersionToJson(result))
}

// @Summary		Update connector
// @Description	Update an existing connector's draft version, creating one if needed
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string							true	"Connector UUID"
// @Param			request	body		SwaggerUpdateConnectorRequest	true	"Connector update request"
// @Success		200		{object}	SwaggerConnectorVersionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id} [patch]
func (r *ConnectorsRoutes) updateConnector(gctx *gin.Context) {
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

	var req UpdateConnectorRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg(err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if req.Labels != nil {
		if err := database.Labels(*req.Labels).Validate(); err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithInternalErr(err).
				WithResponseMsgf("invalid labels: %s", err.Error()).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
	}

	draft, err := r.connectors.GetOrCreateDraftConnectorVersion(ctx, connectorId)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("connector '%s' not found", connectorId).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(draft); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	var def *cschema.Connector
	if req.Definition != nil {
		def = req.Definition
	} else {
		def = draft.GetDefinition()
	}

	if err := def.Validate(&common.ValidationContext{}); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid connector definition: %s", err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	var labels map[string]string
	if req.Labels != nil {
		labels = *req.Labels
	} else {
		labels = draft.GetLabels()
	}

	result, err := r.connectors.UpdateDraftConnectorVersion(ctx, connectorId, draft.GetVersion(), def, labels)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectorVersionToJson(result))
}

// @Summary		Create connector version
// @Description	Create a new draft version for an existing connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string									true	"Connector UUID"
// @Param			request	body		SwaggerCreateConnectorVersionRequest	false	"Version creation request"
// @Success		201		{object}	SwaggerConnectorVersionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions [post]
func (r *ConnectorsRoutes) createVersion(gctx *gin.Context) {
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

	// Verify the connector exists and check auth
	connectorResult := r.connectors.
		ListConnectorsBuilder().
		ForId(connectorId).
		Limit(1).
		FetchPage(ctx)

	if connectorResult.Error != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(connectorResult.Error).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if len(connectorResult.Results) == 0 {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsgf("connector '%s' not found", connectorId).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	connector := connectorResult.Results[0]
	if httpErr := val.ValidateHttpStatusError(connector); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	var req CreateConnectorVersionRequestJson
	// Support a blank post to create a new draft version of the connector
	if gctx.Request.ContentLength > 0 {
		if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithInternalErr(err).
				WithResponseMsg(err.Error()).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
	}

	if req.Definition != nil {
		if err := req.Definition.Validate(&common.ValidationContext{}); err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithInternalErr(err).
				WithResponseMsgf("invalid connector definition: %s", err.Error()).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
	}

	var labels map[string]string
	if req.Labels != nil {
		labels = *req.Labels
		if err := database.Labels(labels).Validate(); err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithInternalErr(err).
				WithResponseMsgf("invalid labels: %s", err.Error()).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
	}

	result, err := r.connectors.CreateDraftConnectorVersion(ctx, connectorId, req.Definition, labels)
	if err != nil {
		if errors.Is(err, core.ErrDraftAlreadyExists) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatus(http.StatusConflict).
				WithResponseMsg("a draft version already exists for this connector").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
		if errors.Is(err, core.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("connector '%s' not found", connectorId).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusCreated, ConnectorVersionToJson(result))
}

// @Summary		Update connector version
// @Description	Update a specific draft version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string							true	"Connector UUID"
// @Param			version	path		integer							true	"Version number"
// @Param			request	body		SwaggerUpdateConnectorRequest	true	"Version update request"
// @Success		200		{object}	SwaggerConnectorVersionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions/{version} [patch]
func (r *ConnectorsRoutes) updateVersion(gctx *gin.Context) {
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

	var req UpdateConnectorRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg(err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if req.Labels != nil {
		if err := database.Labels(*req.Labels).Validate(); err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithInternalErr(err).
				WithResponseMsgf("invalid labels: %s", err.Error()).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
	}

	existing, err := r.connectors.GetConnectorVersion(ctx, connectorId, version)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("connector version '%s:%d' not found", connectorId, version).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(existing); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	if existing.GetState() != database.ConnectorVersionStateDraft {
		api_common.NewHttpStatusErrorBuilder().
			WithStatus(http.StatusConflict).
			WithResponseMsgf("connector version '%s:%d' is not a draft", connectorId, version).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	var def *cschema.Connector
	if req.Definition != nil {
		def = req.Definition
	} else {
		def = existing.GetDefinition()
	}

	if err := def.Validate(&common.ValidationContext{}); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid connector definition: %s", err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	var labels map[string]string
	if req.Labels != nil {
		labels = *req.Labels
	} else {
		labels = existing.GetLabels()
	}

	result, err := r.connectors.UpdateDraftConnectorVersion(ctx, connectorId, version, def, labels)
	if err != nil {
		if errors.Is(err, core.ErrNotDraft) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatus(http.StatusConflict).
				WithResponseMsgf("connector version '%s:%d' is not a draft", connectorId, version).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectorVersionToJson(result))
}

// @Summary		Get all labels for a connector
// @Description	Get all labels associated with the primary version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Connector UUID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/labels [get]
func (r *ConnectorsRoutes) getLabels(gctx *gin.Context) {
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

	labels := c.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	gctx.PureJSON(http.StatusOK, labels)
}

// @Summary		Get a specific label for a connector
// @Description	Get a specific label value by key for the primary version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string	true	"Connector UUID"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	SwaggerConnectorLabelJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/labels/{label} [get]
func (r *ConnectorsRoutes) getLabel(gctx *gin.Context) {
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

	labelKey := gctx.Param("label")
	if labelKey == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("label key is required").
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

	labels := c.GetLabels()
	value, exists := labels[labelKey]
	if !exists {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsgf("label '%s' not found", labelKey).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectorLabelJson{
		Key:   labelKey,
		Value: value,
	})
}

// @Summary		Set a label for a connector
// @Description	Set or update a specific label on a connector's draft version, creating one if needed
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string								true	"Connector UUID"
// @Param			label	path		string								true	"Label key"
// @Param			request	body		SwaggerPutConnectorLabelRequest		true	"Label value"
// @Success		200		{object}	SwaggerConnectorLabelJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/labels/{label} [put]
func (r *ConnectorsRoutes) putLabel(gctx *gin.Context) {
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

	labelKey := gctx.Param("label")
	if labelKey == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("label key is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateLabelKey(labelKey); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid label key: %s", err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	var req PutConnectorLabelRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid request body").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateLabelValue(req.Value); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid label value: %s", err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	draft, err := r.connectors.GetOrCreateDraftConnectorVersion(ctx, connectorId)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("connector '%s' not found", connectorId).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(draft); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	labels := make(map[string]string)
	for k, v := range draft.GetLabels() {
		labels[k] = v
	}
	labels[labelKey] = req.Value

	_, err = r.connectors.UpdateDraftConnectorVersion(ctx, connectorId, draft.GetVersion(), draft.GetDefinition(), labels)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectorLabelJson{
		Key:   labelKey,
		Value: req.Value,
	})
}

// @Summary		Delete a label from a connector
// @Description	Delete a specific label from a connector's draft version, creating one if needed
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path	string	true	"Connector UUID"
// @Param			label	path	string	true	"Label key"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/labels/{label} [delete]
func (r *ConnectorsRoutes) deleteLabel(gctx *gin.Context) {
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

	labelKey := gctx.Param("label")
	if labelKey == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("label key is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	draft, err := r.connectors.GetOrCreateDraftConnectorVersion(ctx, connectorId)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			gctx.Status(http.StatusNoContent)
			val.MarkValidated()
			return
		}
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(draft); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	labels := make(map[string]string)
	for k, v := range draft.GetLabels() {
		labels[k] = v
	}
	delete(labels, labelKey)

	_, err = r.connectors.UpdateDraftConnectorVersion(ctx, connectorId, draft.GetVersion(), draft.GetDefinition(), labels)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.Status(http.StatusNoContent)
}

// @Summary		Get all labels for a connector version
// @Description	Get all labels associated with a specific version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string	true	"Connector UUID"
// @Param			version	path		integer	true	"Version number"
// @Success		200		{object}	map[string]string
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions/{version}/labels [get]
func (r *ConnectorsRoutes) getVersionLabels(gctx *gin.Context) {
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

	cv, err := r.connectors.GetConnectorVersion(ctx, connectorId, version)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("connector version '%s:%d' not found", connectorId, version).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(cv); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	labels := cv.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	gctx.PureJSON(http.StatusOK, labels)
}

// @Summary		Get a specific label for a connector version
// @Description	Get a specific label value by key for a specific version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string	true	"Connector UUID"
// @Param			version	path		integer	true	"Version number"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	SwaggerConnectorLabelJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions/{version}/labels/{label} [get]
func (r *ConnectorsRoutes) getVersionLabel(gctx *gin.Context) {
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

	labelKey := gctx.Param("label")
	if labelKey == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("label key is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	cv, err := r.connectors.GetConnectorVersion(ctx, connectorId, version)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("connector version '%s:%d' not found", connectorId, version).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(cv); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	labels := cv.GetLabels()
	value, exists := labels[labelKey]
	if !exists {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsgf("label '%s' not found", labelKey).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectorLabelJson{
		Key:   labelKey,
		Value: value,
	})
}

// @Summary		Set a label for a connector version
// @Description	Set or update a specific label on a specific draft version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string								true	"Connector UUID"
// @Param			version	path		integer								true	"Version number"
// @Param			label	path		string								true	"Label key"
// @Param			request	body		SwaggerPutConnectorLabelRequest		true	"Label value"
// @Success		200		{object}	SwaggerConnectorLabelJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions/{version}/labels/{label} [put]
func (r *ConnectorsRoutes) putVersionLabel(gctx *gin.Context) {
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

	labelKey := gctx.Param("label")
	if labelKey == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("label key is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateLabelKey(labelKey); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid label key: %s", err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	var req PutConnectorLabelRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid request body").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateLabelValue(req.Value); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsgf("invalid label value: %s", err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	existing, err := r.connectors.GetConnectorVersion(ctx, connectorId, version)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("connector version '%s:%d' not found", connectorId, version).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(existing); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	if existing.GetState() != database.ConnectorVersionStateDraft {
		api_common.NewHttpStatusErrorBuilder().
			WithStatus(http.StatusConflict).
			WithResponseMsgf("connector version '%s:%d' is not a draft", connectorId, version).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	labels := make(map[string]string)
	for k, v := range existing.GetLabels() {
		labels[k] = v
	}
	labels[labelKey] = req.Value

	_, err = r.connectors.UpdateDraftConnectorVersion(ctx, connectorId, version, existing.GetDefinition(), labels)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectorLabelJson{
		Key:   labelKey,
		Value: req.Value,
	})
}

// @Summary		Delete a label from a connector version
// @Description	Delete a specific label from a specific draft version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path	string	true	"Connector UUID"
// @Param			version	path	integer	true	"Version number"
// @Param			label	path	string	true	"Label key"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions/{version}/labels/{label} [delete]
func (r *ConnectorsRoutes) deleteVersionLabel(gctx *gin.Context) {
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

	labelKey := gctx.Param("label")
	if labelKey == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("label key is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	existing, err := r.connectors.GetConnectorVersion(ctx, connectorId, version)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			gctx.Status(http.StatusNoContent)
			val.MarkValidated()
			return
		}
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(existing); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	if existing.GetState() != database.ConnectorVersionStateDraft {
		api_common.NewHttpStatusErrorBuilder().
			WithStatus(http.StatusConflict).
			WithResponseMsgf("connector version '%s:%d' is not a draft", connectorId, version).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	labels := make(map[string]string)
	for k, v := range existing.GetLabels() {
		labels[k] = v
	}
	delete(labels, labelKey)

	_, err = r.connectors.UpdateDraftConnectorVersion(ctx, connectorId, version, existing.GetDefinition(), labels)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			DefaultStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.Status(http.StatusNoContent)
}

type ForceConnectorVersionStateRequestJson struct {
	State database.ConnectorVersionState `json:"state"`
}

// @Summary		Force connector version state
// @Description	Force a connector version to a specific state (admin operation)
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string								true	"Connector UUID"
// @Param			version	path		integer								true	"Version number"
// @Param			request	body		SwaggerForceConnectorVersionStateRequest	true	"New state"
// @Success		200		{object}	SwaggerConnectorVersionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions/{version}/_force_state [put]
func (r *ConnectorsRoutes) forceVersionState(gctx *gin.Context) {
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

	req := ForceConnectorVersionStateRequestJson{}
	err = gctx.BindJSON(&req)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if req.State == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("state is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if !database.IsValidConnectorVersionState(req.State) {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid connector version state '%s'", req.State).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	cv, err := r.connectors.GetConnectorVersion(ctx, connectorId, version)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsgf("connector version '%s:%d' not found", connectorId, version).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(cv); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	if cv.GetState() == req.State {
		gctx.PureJSON(http.StatusOK, ConnectorVersionToJson(cv))
		return
	}

	err = cv.SetState(ctx, req.State)
	if err != nil {
		api_common.HttpStatusErrorBuilderFromError(err).
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectorVersionToJson(cv))
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
	g.POST("/connectors",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForVerb("create").
			Build(),
		r.createConnector,
	)
	g.PATCH("/connectors/:id",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.updateConnector,
	)
	g.POST("/connectors/:id/versions",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("create").
			Build(),
		r.createVersion,
	)
	g.PATCH("/connectors/:id/versions/:version",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.updateVersion,
	)
	g.PUT("/connectors/:id/versions/:version/_force_state",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("force_state").
			Build(),
		r.forceVersionState,
	)
	g.GET("/connectors/:id/labels",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.getLabels,
	)
	g.GET("/connectors/:id/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.getLabel,
	)
	g.PUT("/connectors/:id/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.putLabel,
	)
	g.DELETE("/connectors/:id/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.deleteLabel,
	)
	g.GET("/connectors/:id/versions/:version/labels",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("list/versions").
			Build(),
		r.getVersionLabels,
	)
	g.GET("/connectors/:id/versions/:version/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("list/versions").
			Build(),
		r.getVersionLabel,
	)
	g.PUT("/connectors/:id/versions/:version/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.putVersionLabel,
	)
	g.DELETE("/connectors/:id/versions/:version/labels/:label",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.deleteVersionLabel,
	)
}

func NewConnectorsRoutes(cfg config.C, authService auth.A, c connIface.C) *ConnectorsRoutes {
	return &ConnectorsRoutes{
		cfg:         cfg,
		authService: authService,
		connectors:  c,
	}
}
