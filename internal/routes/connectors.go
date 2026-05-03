package routes

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	connIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/routes/key_value"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type ConnectorJson struct {
	Id            apid.ID                        `json:"id" swaggertype:"string"`
	Version       uint64                         `json:"version"`
	Namespace     string                         `json:"namespace"`
	State         database.ConnectorVersionState `json:"state"`
	DisplayName   string                         `json:"display_name"`
	Highlight     string                         `json:"highlight,omitempty"`
	Description   string                         `json:"description"`
	StatusPageUrl string                         `json:"status_page_url,omitempty"`
	Logo          string                         `json:"logo"`
	HasConfigure  bool                           `json:"has_configure"`
	Labels        map[string]string              `json:"labels,omitempty"`
	Annotations   map[string]string              `json:"annotations,omitempty"`
	CreatedAt     time.Time                      `json:"created_at"`
	UpdatedAt     time.Time                      `json:"updated_at"`

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
		HasConfigure:  def.SetupFlow.HasConfigure(),
		Labels:        cv.GetLabels(),
		Annotations:   cv.GetAnnotations(),
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
	Id          apid.ID                        `json:"id" swaggertype:"string"`
	Version     uint64                         `json:"version"`
	Namespace   string                         `json:"namespace"`
	State       database.ConnectorVersionState `json:"state"`
	Definition  cschema.Connector              `json:"definition"`
	Labels      map[string]string              `json:"labels,omitempty"`
	Annotations map[string]string              `json:"annotations,omitempty"`
	CreatedAt   time.Time                      `json:"created_at"`
	UpdatedAt   time.Time                      `json:"updated_at"`
}

func ConnectorVersionToJson(cv connIface.ConnectorVersion) ConnectorVersionJson {
	def := cv.GetDefinition()

	return ConnectorVersionJson{
		Id:          cv.GetId(),
		Version:     cv.GetVersion(),
		Namespace:   cv.GetNamespace(),
		State:       cv.GetState(),
		Definition:  *def,
		Labels:      cv.GetLabels(),
		Annotations: cv.GetAnnotations(),
		CreatedAt:   cv.GetCreatedAt(),
		UpdatedAt:   cv.GetUpdatedAt(),
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
	Namespace   string            `json:"namespace"`
	Definition  cschema.Connector `json:"definition"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// UpdateConnectorRequestJson is the request body for PATCH /connectors/:id and PATCH /connectors/:id/versions/:version
type UpdateConnectorRequestJson struct {
	Definition  *cschema.Connector `json:"definition,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty"`
}

// CreateConnectorVersionRequestJson is the request body for POST /connectors/:id/versions
type CreateConnectorVersionRequestJson struct {
	Definition  *cschema.Connector `json:"definition,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty"`
}

// connectorVersionID is the composite identifier used by the version-level
// label/annotation adapters.
type connectorVersionID struct {
	ConnectorID apid.ID
	Version     uint64
}

type ConnectorsRoutes struct {
	cfg                  config.C
	connectors           connIface.C
	authService          auth.A
	labelsAdapter        key_value.Adapter[apid.ID]
	annotsAdapter        key_value.Adapter[apid.ID]
	versionLabelsAdapter key_value.Adapter[connectorVersionID]
	versionAnnotsAdapter key_value.Adapter[connectorVersionID]
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
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	connectorId, err := apid.Parse(connectorIdStr)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format"))
		val.MarkErrorReturn()
		return
	}

	if connectorId == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	result := r.connectors.
		ListConnectorsBuilder().
		ForId(connectorId).
		Limit(1).
		FetchPage(ctx)

	if result.Error != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(result.Error)))
		val.MarkErrorReturn()
		return
	}

	if len(result.Results) == 0 {
		apgin.WriteError(gctx, nil, httperr.NotFoundf("connector '%s' not found", connectorId))
		val.MarkErrorReturn()
		return
	}

	c := result.Results[0]

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
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
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	var err error
	var ex connIface.ListConnectorsExecutor

	if req.Cursor != nil {
		ex, err = r.connectors.ListConnectorsFromCursor(ctx, *req.Cursor)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err), httperr.WithResponseMsg("failed to list core from cursor")))
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
				apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidConnectorOrderByField(field) {
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
		apgin.WriteErr(gctx, nil, result.Error)
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
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	connectorId, err := apid.Parse(connectorIdStr)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format"))
		val.MarkErrorReturn()
		return
	}

	if connectorId == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	b := r.connectors.
		ListConnectorVersionsBuilder().
		ForId(connectorId).
		Limit(1)

	versionStr := gctx.Param("version")

	if versionStr == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("version is required"))
		val.MarkErrorReturn()
		return
	}

	version, err := strconv.ParseUint(versionStr, 10, 64)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("failed to parse version as an integer"))
		val.MarkErrorReturn()
		return
	}

	b = b.ForVersion(version)

	// TODO: support lookup by certain states

	result := b.FetchPage(ctx)
	if result.Error != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(result.Error)))
		val.MarkErrorReturn()
		return
	}

	if len(result.Results) == 0 {
		apgin.WriteError(gctx, nil, httperr.NotFoundf("connector version '%s:%d' not found", connectorId, version))
		val.MarkErrorReturn()
		return
	}

	cv := result.Results[0]

	if httpErr := val.ValidateHttpStatusError(cv); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
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
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	connectorId, err := apid.Parse(connectorIdStr)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format"))
		val.MarkErrorReturn()
		return
	}

	if connectorId == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	var req ListConnectorVersionsRequestQueryParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
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
			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err), httperr.WithResponseMsg("failed to list connector versions from cursor")))
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
				apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidConnectorVersionOrderByField(field) {
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
		apgin.WriteErr(gctx, nil, result.Error)
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
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateNamespacePath(req.Namespace); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid namespace '%s': %s", req.Namespace, err.Error()))
		val.MarkErrorReturn()
		return
	}

	if err := database.ValidateUserLabels(req.Labels); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid labels: %s", err.Error()))
		val.MarkErrorReturn()
		return
	}

	if err := database.Annotations(req.Annotations).Validate(); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid annotations: %s", err.Error()))
		val.MarkErrorReturn()
		return
	}

	if err := req.Definition.Validate(&common.ValidationContext{}); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid connector definition: %s", err.Error()))
		val.MarkErrorReturn()
		return
	}

	if err := val.ValidateNamespace(req.Namespace); err != nil {
		apgin.WriteError(gctx, nil, httperr.Forbidden("", httperr.WithPublicErr(err)))
		return
	}

	result, err := r.connectors.CreateConnectorVersion(ctx, req.Namespace, &req.Definition, req.Labels, req.Annotations)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
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
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	connectorId, err := apid.Parse(connectorIdStr)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format"))
		val.MarkErrorReturn()
		return
	}

	if connectorId == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	var req UpdateConnectorRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if req.Labels != nil {
		if err := database.ValidateUserLabels(*req.Labels); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid labels: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	if req.Annotations != nil {
		if err := database.Annotations(*req.Annotations).Validate(); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid annotations: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	draft, err := r.connectors.GetOrCreateDraftConnectorVersion(ctx, connectorId)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFoundf("connector '%s' not found", connectorId))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(draft); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	var def *cschema.Connector
	if req.Definition != nil {
		def = req.Definition
	} else {
		def = draft.GetDefinition()
	}

	if err := def.Validate(&common.ValidationContext{}); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid connector definition: %s", err.Error()))
		val.MarkErrorReturn()
		return
	}

	var labels map[string]string
	if req.Labels != nil {
		labels = *req.Labels
	} else {
		labels = draft.GetLabels()
	}

	var annotations map[string]string
	if req.Annotations != nil {
		annotations = *req.Annotations
	} else {
		annotations = draft.GetAnnotations()
	}

	result, err := r.connectors.UpdateDraftConnectorVersion(ctx, connectorId, draft.GetVersion(), def, labels, annotations)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
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
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	connectorId, err := apid.Parse(connectorIdStr)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format"))
		val.MarkErrorReturn()
		return
	}

	if connectorId == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
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
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(connectorResult.Error)))
		val.MarkErrorReturn()
		return
	}

	if len(connectorResult.Results) == 0 {
		apgin.WriteError(gctx, nil, httperr.NotFoundf("connector '%s' not found", connectorId))
		val.MarkErrorReturn()
		return
	}

	connector := connectorResult.Results[0]
	if httpErr := val.ValidateHttpStatusError(connector); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	var req CreateConnectorVersionRequestJson
	// Support a blank post to create a new draft version of the connector
	if gctx.Request.ContentLength > 0 {
		if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	if req.Definition != nil {
		if err := req.Definition.Validate(&common.ValidationContext{}); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid connector definition: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	var labels map[string]string
	if req.Labels != nil {
		labels = *req.Labels
		if err := database.ValidateUserLabels(labels); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid labels: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	var annotations map[string]string
	if req.Annotations != nil {
		annotations = *req.Annotations
		if err := database.Annotations(annotations).Validate(); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid annotations: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	result, err := r.connectors.CreateDraftConnectorVersion(ctx, connectorId, req.Definition, labels, annotations)
	if err != nil {
		if errors.Is(err, core.ErrDraftAlreadyExists) {
			apgin.WriteError(gctx, nil, httperr.Conflict("a draft version already exists for this connector"))
			val.MarkErrorReturn()
			return
		}
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFoundf("connector '%s' not found", connectorId))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteErr(gctx, nil, err)
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
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	connectorId, err := apid.Parse(connectorIdStr)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format"))
		val.MarkErrorReturn()
		return
	}

	if connectorId == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	versionStr := gctx.Param("version")
	if versionStr == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("version is required"))
		val.MarkErrorReturn()
		return
	}

	version, err := strconv.ParseUint(versionStr, 10, 64)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("failed to parse version as an integer"))
		val.MarkErrorReturn()
		return
	}

	var req UpdateConnectorRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if req.Labels != nil {
		if err := database.ValidateUserLabels(*req.Labels); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid labels: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	if req.Annotations != nil {
		if err := database.Annotations(*req.Annotations).Validate(); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid annotations: %s", err.Error()))
			val.MarkErrorReturn()
			return
		}
	}

	existing, err := r.connectors.GetConnectorVersion(ctx, connectorId, version)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFoundf("connector version '%s:%d' not found", connectorId, version))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(existing); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	if existing.GetState() != database.ConnectorVersionStateDraft {
		apgin.WriteError(gctx, nil, httperr.Conflictf("connector version '%s:%d' is not a draft", connectorId, version))
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
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid connector definition: %s", err.Error()))
		val.MarkErrorReturn()
		return
	}

	var labels map[string]string
	if req.Labels != nil {
		labels = *req.Labels
	} else {
		labels = existing.GetLabels()
	}

	var annotations map[string]string
	if req.Annotations != nil {
		annotations = *req.Annotations
	} else {
		annotations = existing.GetAnnotations()
	}

	result, err := r.connectors.UpdateDraftConnectorVersion(ctx, connectorId, version, def, labels, annotations)
	if err != nil {
		if errors.Is(err, core.ErrNotDraft) {
			apgin.WriteError(gctx, nil, httperr.Conflictf("connector version '%s:%d' is not a draft", connectorId, version))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteErr(gctx, nil, err)
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
func (r *ConnectorsRoutes) getLabels(gctx *gin.Context) { r.labelsAdapter.HandleList(gctx) }

// @Summary		Get a specific label for a connector
// @Description	Get a specific label value by key for the primary version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string	true	"Connector UUID"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	SwaggerKeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/labels/{label} [get]
func (r *ConnectorsRoutes) getLabel(gctx *gin.Context) { r.labelsAdapter.HandleGet(gctx) }

// @Summary		Set a label for a connector
// @Description	Set or update a specific label on a connector's draft version, creating one if needed
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string						true	"Connector UUID"
// @Param			label	path		string						true	"Label key"
// @Param			request	body		SwaggerPutKeyValueRequest	true	"Label value"
// @Success		200		{object}	SwaggerKeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/labels/{label} [put]
func (r *ConnectorsRoutes) putLabel(gctx *gin.Context) { r.labelsAdapter.HandlePut(gctx) }

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
func (r *ConnectorsRoutes) deleteLabel(gctx *gin.Context) { r.labelsAdapter.HandleDelete(gctx) }

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
	r.versionLabelsAdapter.HandleList(gctx)
}

// @Summary		Get a specific label for a connector version
// @Description	Get a specific label value by key for a specific version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string	true	"Connector UUID"
// @Param			version	path		integer	true	"Version number"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	SwaggerKeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions/{version}/labels/{label} [get]
func (r *ConnectorsRoutes) getVersionLabel(gctx *gin.Context) { r.versionLabelsAdapter.HandleGet(gctx) }

// @Summary		Set a label for a connector version
// @Description	Set or update a specific label on a specific draft version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string						true	"Connector UUID"
// @Param			version	path		integer						true	"Version number"
// @Param			label	path		string						true	"Label key"
// @Param			request	body		SwaggerPutKeyValueRequest	true	"Label value"
// @Success		200		{object}	SwaggerKeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions/{version}/labels/{label} [put]
func (r *ConnectorsRoutes) putVersionLabel(gctx *gin.Context) { r.versionLabelsAdapter.HandlePut(gctx) }

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
	r.versionLabelsAdapter.HandleDelete(gctx)
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
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	connectorId, err := apid.Parse(connectorIdStr)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format"))
		val.MarkErrorReturn()
		return
	}

	if connectorId == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	versionStr := gctx.Param("version")
	if versionStr == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("version is required"))
		val.MarkErrorReturn()
		return
	}

	version, err := strconv.ParseUint(versionStr, 10, 64)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("failed to parse version as an integer"))
		val.MarkErrorReturn()
		return
	}

	req := ForceConnectorVersionStateRequestJson{}
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

	if !database.IsValidConnectorVersionState(req.State) {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid connector version state '%s'", req.State))
		val.MarkErrorReturn()
		return
	}

	cv, err := r.connectors.GetConnectorVersion(ctx, connectorId, version)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFoundf("connector version '%s:%d' not found", connectorId, version))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(cv); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	if cv.GetState() == req.State {
		gctx.PureJSON(http.StatusOK, ConnectorVersionToJson(cv))
		return
	}

	err = cv.SetState(ctx, req.State)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.FromError(err))
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ConnectorVersionToJson(cv))
}

// @Summary		Get all annotations for a connector
// @Description	Get all annotations associated with a connector's latest version
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
// @Router			/connectors/{id}/annotations [get]
func (r *ConnectorsRoutes) getAnnotations(gctx *gin.Context) { r.annotsAdapter.HandleList(gctx) }

// @Summary		Get a specific annotation for a connector
// @Description	Get a specific annotation value by key for a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id			path		string	true	"Connector UUID"
// @Param			annotation	path		string	true	"Annotation key"
// @Success		200			{object}	SwaggerKeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/annotations/{annotation} [get]
func (r *ConnectorsRoutes) getAnnotation(gctx *gin.Context) { r.annotsAdapter.HandleGet(gctx) }

// @Summary		Set an annotation for a connector
// @Description	Set or update a specific annotation value by key for a connector's draft version, creating one if needed
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id			path		string						true	"Connector UUID"
// @Param			annotation	path		string						true	"Annotation key"
// @Param			request		body		SwaggerPutKeyValueRequest	true	"Annotation value"
// @Success		200			{object}	SwaggerKeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/annotations/{annotation} [put]
func (r *ConnectorsRoutes) putAnnotation(gctx *gin.Context) { r.annotsAdapter.HandlePut(gctx) }

// @Summary		Delete an annotation from a connector
// @Description	Delete a specific annotation from a connector's draft version, creating one if needed
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id			path	string	true	"Connector UUID"
// @Param			annotation	path	string	true	"Annotation key"
// @Success		204			"No Content"
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/annotations/{annotation} [delete]
func (r *ConnectorsRoutes) deleteAnnotation(gctx *gin.Context) { r.annotsAdapter.HandleDelete(gctx) }

// @Summary		Get all annotations for a connector version
// @Description	Get all annotations associated with a specific version of a connector
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
// @Router			/connectors/{id}/versions/{version}/annotations [get]
func (r *ConnectorsRoutes) getVersionAnnotations(gctx *gin.Context) {
	r.versionAnnotsAdapter.HandleList(gctx)
}

// @Summary		Get a specific annotation for a connector version
// @Description	Get a specific annotation value by key for a specific version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id			path		string	true	"Connector UUID"
// @Param			version		path		integer	true	"Version number"
// @Param			annotation	path		string	true	"Annotation key"
// @Success		200			{object}	SwaggerKeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions/{version}/annotations/{annotation} [get]
func (r *ConnectorsRoutes) getVersionAnnotation(gctx *gin.Context) {
	r.versionAnnotsAdapter.HandleGet(gctx)
}

// @Summary		Set an annotation for a connector version
// @Description	Set or update a specific annotation value by key for a specific draft version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id			path		string						true	"Connector UUID"
// @Param			version		path		integer						true	"Version number"
// @Param			annotation	path		string						true	"Annotation key"
// @Param			request		body		SwaggerPutKeyValueRequest	true	"Annotation value"
// @Success		200			{object}	SwaggerKeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		409			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions/{version}/annotations/{annotation} [put]
func (r *ConnectorsRoutes) putVersionAnnotation(gctx *gin.Context) {
	r.versionAnnotsAdapter.HandlePut(gctx)
}

// @Summary		Delete an annotation from a connector version
// @Description	Delete a specific annotation from a specific draft version of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id			path	string	true	"Connector UUID"
// @Param			version		path	integer	true	"Version number"
// @Param			annotation	path	string	true	"Annotation key"
// @Success		204			"No Content"
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		409			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/versions/{version}/annotations/{annotation} [delete]
func (r *ConnectorsRoutes) deleteVersionAnnotation(gctx *gin.Context) {
	r.versionAnnotsAdapter.HandleDelete(gctx)
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
	g.GET("/connectors/:id/annotations",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.getAnnotations,
	)
	g.GET("/connectors/:id/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.getAnnotation,
	)
	g.PUT("/connectors/:id/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.putAnnotation,
	)
	g.DELETE("/connectors/:id/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.deleteAnnotation,
	)
	g.GET("/connectors/:id/versions/:version/annotations",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("list/versions").
			Build(),
		r.getVersionAnnotations,
	)
	g.GET("/connectors/:id/versions/:version/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("list/versions").
			Build(),
		r.getVersionAnnotation,
	)
	g.PUT("/connectors/:id/versions/:version/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.putVersionAnnotation,
	)
	g.DELETE("/connectors/:id/versions/:version/annotations/:annotation",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.deleteVersionAnnotation,
	)
}

func NewConnectorsRoutes(cfg config.C, authService auth.A, c connIface.C) *ConnectorsRoutes {
	parseConnectorID := func(gctx *gin.Context) (apid.ID, *httperr.Error) {
		idStr := gctx.Param("id")
		if idStr == "" {
			return apid.Nil, httperr.BadRequest("id is required")
		}
		id, err := apid.Parse(idStr)
		if err != nil {
			return apid.Nil, httperr.BadRequest("invalid id format")
		}
		if id == apid.Nil {
			return apid.Nil, httperr.BadRequest("id is required")
		}
		return id, nil
	}

	parseConnectorVersionID := func(gctx *gin.Context) (connectorVersionID, *httperr.Error) {
		id, herr := parseConnectorID(gctx)
		if herr != nil {
			return connectorVersionID{}, herr
		}
		versionStr := gctx.Param("version")
		if versionStr == "" {
			return connectorVersionID{}, httperr.BadRequest("version is required")
		}
		version, err := strconv.ParseUint(versionStr, 10, 64)
		if err != nil {
			return connectorVersionID{}, httperr.BadRequest("failed to parse version as an integer")
		}
		return connectorVersionID{ConnectorID: id, Version: version}, nil
	}

	getConnector := func(ctx context.Context, id apid.ID) (key_value.Resource, error) {
		result := c.ListConnectorsBuilder().
			ForId(id).
			Limit(1).
			FetchPage(ctx)
		if result.Error != nil {
			return nil, result.Error
		}
		if len(result.Results) == 0 {
			return nil, database.ErrNotFound
		}
		return result.Results[0], nil
	}

	getConnectorVersion := func(ctx context.Context, id connectorVersionID) (key_value.Resource, error) {
		cv, err := c.GetConnectorVersion(ctx, id.ConnectorID, id.Version)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, database.ErrNotFound
			}
			return nil, err
		}
		if cv == nil {
			return nil, nil
		}
		return cv, nil
	}

	connectorIDExtractor := func(v interface{}) string {
		return string(v.(connIface.ConnectorVersion).GetId())
	}

	connectorAuthGet := authService.NewRequiredBuilder().
		ForResource("connectors").
		ForIdField("id").
		ForIdExtractor(connectorIDExtractor).
		ForVerb("get").
		Build()
	connectorAuthMutate := authService.NewRequiredBuilder().
		ForResource("connectors").
		ForIdField("id").
		ForIdExtractor(connectorIDExtractor).
		ForVerb("update").
		Build()
	versionAuthGet := authService.NewRequiredBuilder().
		ForResource("connectors").
		ForIdField("id").
		ForIdExtractor(connectorIDExtractor).
		ForVerb("list/versions").
		Build()
	versionAuthMutate := authService.NewRequiredBuilder().
		ForResource("connectors").
		ForIdField("id").
		ForIdExtractor(connectorIDExtractor).
		ForVerb("update").
		Build()

	putConnectorLabels := func(ctx context.Context, id apid.ID, kv map[string]string) (key_value.Resource, error) {
		draft, err := c.GetOrCreateDraftConnectorVersion(ctx, id)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, database.ErrNotFound
			}
			return nil, err
		}
		merged := make(map[string]string)
		for k, v := range draft.GetLabels() {
			merged[k] = v
		}
		for k, v := range kv {
			merged[k] = v
		}
		return c.UpdateDraftConnectorVersion(ctx, id, draft.GetVersion(), draft.GetDefinition(), merged, draft.GetAnnotations())
	}

	deleteConnectorLabels := func(ctx context.Context, id apid.ID, keys []string) (key_value.Resource, error) {
		draft, err := c.GetOrCreateDraftConnectorVersion(ctx, id)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, database.ErrNotFound
			}
			return nil, err
		}
		merged := make(map[string]string)
		for k, v := range draft.GetLabels() {
			merged[k] = v
		}
		for _, k := range keys {
			delete(merged, k)
		}
		return c.UpdateDraftConnectorVersion(ctx, id, draft.GetVersion(), draft.GetDefinition(), merged, draft.GetAnnotations())
	}

	putConnectorAnnotations := func(ctx context.Context, id apid.ID, kv map[string]string) (key_value.Resource, error) {
		draft, err := c.GetOrCreateDraftConnectorVersion(ctx, id)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, database.ErrNotFound
			}
			return nil, err
		}
		merged := make(map[string]string)
		for k, v := range draft.GetAnnotations() {
			merged[k] = v
		}
		for k, v := range kv {
			merged[k] = v
		}
		return c.UpdateDraftConnectorVersion(ctx, id, draft.GetVersion(), draft.GetDefinition(), draft.GetLabels(), merged)
	}

	deleteConnectorAnnotations := func(ctx context.Context, id apid.ID, keys []string) (key_value.Resource, error) {
		draft, err := c.GetOrCreateDraftConnectorVersion(ctx, id)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, database.ErrNotFound
			}
			return nil, err
		}
		merged := make(map[string]string)
		for k, v := range draft.GetAnnotations() {
			merged[k] = v
		}
		for _, k := range keys {
			delete(merged, k)
		}
		return c.UpdateDraftConnectorVersion(ctx, id, draft.GetVersion(), draft.GetDefinition(), draft.GetLabels(), merged)
	}

	putVersionLabels := func(ctx context.Context, id connectorVersionID, kv map[string]string) (key_value.Resource, error) {
		cv, err := c.GetConnectorVersion(ctx, id.ConnectorID, id.Version)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, database.ErrNotFound
			}
			return nil, err
		}
		if cv.GetState() != database.ConnectorVersionStateDraft {
			return nil, httperr.Conflictf("connector version '%s:%d' is not a draft", id.ConnectorID, id.Version)
		}
		merged := make(map[string]string)
		for k, v := range cv.GetLabels() {
			merged[k] = v
		}
		for k, v := range kv {
			merged[k] = v
		}
		return c.UpdateDraftConnectorVersion(ctx, id.ConnectorID, id.Version, cv.GetDefinition(), merged, cv.GetAnnotations())
	}

	deleteVersionLabels := func(ctx context.Context, id connectorVersionID, keys []string) (key_value.Resource, error) {
		cv, err := c.GetConnectorVersion(ctx, id.ConnectorID, id.Version)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, database.ErrNotFound
			}
			return nil, err
		}
		if cv.GetState() != database.ConnectorVersionStateDraft {
			return nil, httperr.Conflictf("connector version '%s:%d' is not a draft", id.ConnectorID, id.Version)
		}
		merged := make(map[string]string)
		for k, v := range cv.GetLabels() {
			merged[k] = v
		}
		for _, k := range keys {
			delete(merged, k)
		}
		return c.UpdateDraftConnectorVersion(ctx, id.ConnectorID, id.Version, cv.GetDefinition(), merged, cv.GetAnnotations())
	}

	putVersionAnnotations := func(ctx context.Context, id connectorVersionID, kv map[string]string) (key_value.Resource, error) {
		cv, err := c.GetConnectorVersion(ctx, id.ConnectorID, id.Version)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, database.ErrNotFound
			}
			return nil, err
		}
		if cv.GetState() != database.ConnectorVersionStateDraft {
			return nil, httperr.Conflictf("connector version '%s:%d' is not a draft", id.ConnectorID, id.Version)
		}
		merged := make(map[string]string)
		for k, v := range cv.GetAnnotations() {
			merged[k] = v
		}
		for k, v := range kv {
			merged[k] = v
		}
		return c.UpdateDraftConnectorVersion(ctx, id.ConnectorID, id.Version, cv.GetDefinition(), cv.GetLabels(), merged)
	}

	deleteVersionAnnotations := func(ctx context.Context, id connectorVersionID, keys []string) (key_value.Resource, error) {
		cv, err := c.GetConnectorVersion(ctx, id.ConnectorID, id.Version)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, database.ErrNotFound
			}
			return nil, err
		}
		if cv.GetState() != database.ConnectorVersionStateDraft {
			return nil, httperr.Conflictf("connector version '%s:%d' is not a draft", id.ConnectorID, id.Version)
		}
		merged := make(map[string]string)
		for k, v := range cv.GetAnnotations() {
			merged[k] = v
		}
		for _, k := range keys {
			delete(merged, k)
		}
		return c.UpdateDraftConnectorVersion(ctx, id.ConnectorID, id.Version, cv.GetDefinition(), cv.GetLabels(), merged)
	}

	labelsAdapter := key_value.Adapter[apid.ID]{
		Kind:         key_value.Label,
		ResourceName: "connector",
		PathPrefix:   "/connectors/:id",
		AuthGet:      connectorAuthGet,
		AuthMutate:   connectorAuthMutate,
		ParseID:      parseConnectorID,
		Get:          getConnector,
		Put:          putConnectorLabels,
		Delete:       deleteConnectorLabels,
	}

	annotsAdapter := key_value.Adapter[apid.ID]{
		Kind:         key_value.Annotation,
		ResourceName: "connector",
		PathPrefix:   "/connectors/:id",
		AuthGet:      connectorAuthGet,
		AuthMutate:   connectorAuthMutate,
		ParseID:      parseConnectorID,
		Get:          getConnector,
		Put:          putConnectorAnnotations,
		Delete:       deleteConnectorAnnotations,
	}

	versionLabelsAdapter := key_value.Adapter[connectorVersionID]{
		Kind:         key_value.Label,
		ResourceName: "connector version",
		PathPrefix:   "/connectors/:id/versions/:version",
		AuthGet:      versionAuthGet,
		AuthMutate:   versionAuthMutate,
		ParseID:      parseConnectorVersionID,
		Get:          getConnectorVersion,
		Put:          putVersionLabels,
		Delete:       deleteVersionLabels,
	}

	versionAnnotsAdapter := key_value.Adapter[connectorVersionID]{
		Kind:         key_value.Annotation,
		ResourceName: "connector version",
		PathPrefix:   "/connectors/:id/versions/:version",
		AuthGet:      versionAuthGet,
		AuthMutate:   versionAuthMutate,
		ParseID:      parseConnectorVersionID,
		Get:          getConnectorVersion,
		Put:          putVersionAnnotations,
		Delete:       deleteVersionAnnotations,
	}

	return &ConnectorsRoutes{
		cfg:                  cfg,
		authService:          authService,
		connectors:           c,
		labelsAdapter:        labelsAdapter,
		annotsAdapter:        annotsAdapter,
		versionLabelsAdapter: versionLabelsAdapter,
		versionAnnotsAdapter: versionAnnotsAdapter,
	}
}
