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
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httperr"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	smeta "github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/tasks"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type OpenAPIListConnectorsResponseJson = schemaapiopenapi.ListConnectorsResponseJson
type OpenAPIConnectorJson = schemaapiopenapi.ConnectorJson
type OpenAPIConnectorPatchJson = schemaapiopenapi.ConnectorPatchJson
type OpenAPIListConnectorGenerationsResponseJson = schemaapiopenapi.ListConnectorGenerationsResponseJson
type OpenAPIConnectorLifecycleActionJson = schemaapiopenapi.ConnectorLifecycleActionJson
type OpenAPIConnectorForceStateActionJson = schemaapiopenapi.ConnectorForceStateActionJson

type ListConnectorsRequestQueryParams struct {
	Cursor        *string                            `form:"cursor"`
	LimitVal      *int32                             `form:"limit"`
	StateVal      *database.ConnectorGenerationState `form:"state"`
	NamespaceVal  *string                            `form:"namespace"`
	NameVal       *string                            `form:"name"`
	LabelSelector *string                            `form:"labelSelector"`
	OrderByVal    *string                            `form:"orderBy"`
}

type ListConnectorGenerationsRequestQueryParams struct {
	Cursor        *string                            `form:"cursor"`
	LimitVal      *int32                             `form:"limit"`
	StateVal      *database.ConnectorGenerationState `form:"state"`
	NamespaceVal  *string                            `form:"namespace"`
	NameVal       *string                            `form:"name"`
	LabelSelector *string                            `form:"labelSelector"`
	OrderByVal    *string                            `form:"orderBy"`
}

// connectorGenerationID is the composite identifier parsed from generation-level
// connector routes.
type connectorGenerationID struct {
	ConnectorID apid.ID
	Generation  uint64
}

type ConnectorsRoutes struct {
	cfg         config.C
	connectors  connIface.C
	authService auth.A
	encrypt     encrypt.E
}

const defaultConnectorLifecycleTimeout = 10 * time.Minute

func parseConnectorID(gctx *gin.Context) (apid.ID, *httperr.Error) {
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

func parseConnectorGenerationID(gctx *gin.Context) (connectorGenerationID, *httperr.Error) {
	id, herr := parseConnectorID(gctx)
	if herr != nil {
		return connectorGenerationID{}, herr
	}
	generationStr := gctx.Param("generation")
	if generationStr == "" {
		return connectorGenerationID{}, httperr.BadRequest("generation is required")
	}
	generation, err := strconv.ParseUint(generationStr, 10, 64)
	if err != nil {
		return connectorGenerationID{}, httperr.BadRequest("failed to parse generation as an integer")
	}
	return connectorGenerationID{ConnectorID: id, Generation: generation}, nil
}

// @Summary		Get connector
// @Description	Get a specific connector by its UUID
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Connector UUID"
// @Success		200	{object}	OpenAPIConnectorJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id} [get]
func (r *ConnectorsRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	connectorId, httpErr := parseConnectorID(gctx)
	if httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		val.MarkErrorReturn()
		return
	}

	c, err := r.loadConnectorByID(ctx, connectorId)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFoundf("connector '%s' not found", connectorId))
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

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, c.GetResource()); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
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
// @Param			name			query		string	false	"Filter by exact resource name"
// @Param			labelSelector	query		string	false	"Filter by label selector"
// @Param			orderBy		query		string	false	"Order by field (e.g., 'created_at:asc')"
// @Success		200				{object}	OpenAPIListConnectorsResponseJson
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

		if req.NameVal != nil {
			name := common.ResourceName(*req.NameVal)
			if err := name.Validate(); err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid connector name: %s", err.Error()))
				val.MarkErrorReturn()
				return
			}
			b = b.ForName(name)
		}

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

	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewListConnectorsResponseJson(
		util.Map(
			auth.FilterForValidatedResources(val, result.Results),
			func(c connIface.Connector) cschema.Connector {
				return *c.GetResource()
			},
		),
		result.Cursor,
	))
}

// @Summary		Get connector generation
// @Description	Get a specific generation of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string	true	"Connector UUID"
// @Param			generation	path		integer	true	"Generation number"
// @Success		200		{object}	OpenAPIConnectorJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/generations/{generation} [get]
func (r *ConnectorsRoutes) getGeneration(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	generationID, httpErr := parseConnectorGenerationID(gctx)
	if httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		val.MarkErrorReturn()
		return
	}
	connectorId := generationID.ConnectorID
	generation := generationID.Generation

	b := r.connectors.
		ListConnectorGenerationsBuilder().
		ForId(connectorId).
		Limit(1)

	b = b.ForGeneration(generation)

	// TODO: support lookup by certain states

	result := b.FetchPage(ctx)
	if result.Error != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(result.Error)))
		val.MarkErrorReturn()
		return
	}

	if len(result.Results) == 0 {
		apgin.WriteError(gctx, nil, httperr.NotFoundf("connector generation '%s:%d' not found", connectorId, generation))
		val.MarkErrorReturn()
		return
	}

	c := result.Results[0]

	if httpErr := val.ValidateHttpStatusError(c); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, c.GetResource()); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		List connector generations
// @Description	List all generations of a specific connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id				path		string	true	"Connector UUID"
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			state			query		string	false	"Filter by generation state"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			name			query		string	false	"Filter by exact resource name"
// @Param			labelSelector	query		string	false	"Filter by label selector"
// @Param			orderBy		query		string	false	"Order by field (e.g., 'generation desc')"
// @Success		200				{object}	OpenAPIListConnectorGenerationsResponseJson
// @Failure		400				{object}	ErrorResponse
// @Failure		401				{object}	ErrorResponse
// @Failure		500				{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/generations [get]
func (r *ConnectorsRoutes) listGenerations(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var err error
	var ex connIface.ListConnectorGenerationsExecutor

	connectorId, httpErr := parseConnectorID(gctx)
	if httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		val.MarkErrorReturn()
		return
	}

	var req ListConnectorGenerationsRequestQueryParams
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
		apgin.APIJSON(
			gctx,
			http.StatusOK,
			schemaapi.NewListConnectorGenerationsResponseJson(
				nil, // items
				"",  // continueToken
			),
		)

		return
	}

	if req.Cursor != nil {
		ex, err = r.connectors.ListConnectorGenerationsFromCursor(ctx, *req.Cursor)
		if err != nil {
			apgin.WriteError(
				gctx,
				nil, // logger
				httperr.InternalServerError(
					httperr.WithInternalErr(err),
					httperr.WithResponseMsg("failed to list connector generations from cursor"),
				),
			)

			val.MarkErrorReturn()

			return
		}
	} else {
		b := r.connectors.ListConnectorGenerationsBuilder().
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

		if req.NameVal != nil {
			name := common.ResourceName(*req.NameVal)
			if err := name.Validate(); err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid connector name: %s", err.Error()))
				val.MarkErrorReturn()
				return
			}
			b = b.ForName(name)
		}

		if req.LabelSelector != nil {
			b = b.ForLabelSelector(*req.LabelSelector)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.ConnectorGenerationOrderByField](*req.OrderByVal)
			if err != nil {
				apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidConnectorGenerationOrderByField(field) {
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

	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewListConnectorGenerationsResponseJson(
		util.Map(auth.FilterForValidatedResources(val, result.Results), func(c connIface.Connector) cschema.Connector {
			return *c.GetResource()
		}),
		result.Cursor,
	))
}

// @Summary		Create connector
// @Description	Create a new connector with generation 1 in draft state
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			request	body		OpenAPIConnectorJson	true	"Connector creation request"
// @Success		201		{object}	OpenAPIConnectorJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors [post]
func (r *ConnectorsRoutes) createConnector(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req cschema.Connector
	if err := apgin.BindResourceJSON(gctx, &req, smeta.ValidationModeCreate); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := val.ValidateNamespace(req.Metadata.Namespace); err != nil {
		apgin.WriteError(gctx, nil, httperr.Forbidden("", httperr.WithPublicErr(err)))
		return
	}

	result, err := r.connectors.CreateConnector(ctx, &req)
	if err != nil {
		if conflictErr := resourceNameConflictError(err, "connector", req.Metadata.Name, req.Metadata.Namespace); conflictErr != nil && req.Metadata.Name != "" {
			apgin.WriteError(gctx, nil, conflictErr)
			val.MarkErrorReturn()
			return
		}
		if errors.Is(err, core.ErrInvalidArgument) {
			apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusCreated, result.GetResource()); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Update connector
// @Description	Update a connector-level name and/or its draft definition metadata
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string							true	"Connector UUID"
// @Param			request	body		OpenAPIConnectorPatchJson	true	"Connector update request"
// @Success		200		{object}	OpenAPIConnectorJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id} [patch]
func (r *ConnectorsRoutes) updateConnector(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	connectorId, httpErr := parseConnectorID(gctx)
	if httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		val.MarkErrorReturn()
		return
	}

	var req cschema.ConnectorPatch
	if err := apgin.BindResourceJSON(gctx, &req, smeta.ValidationModeUpdate); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	current, err := r.loadConnectorByID(ctx, connectorId)
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
	if httpErr := val.ValidateHttpStatusError(current); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	result, err := r.connectors.UpdateConnector(ctx, connectorId, &req)
	if err != nil {
		if req.Metadata != nil && req.Metadata.Name != nil {
			if conflictErr := resourceNameConflictError(err, "connector", *req.Metadata.Name, current.GetNamespace()); conflictErr != nil {
				apgin.WriteError(gctx, nil, conflictErr)
				val.MarkErrorReturn()
				return
			}
		}
		if errors.Is(err, core.ErrInvalidArgument) {
			apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, result.GetResource()); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Create connector generation
// @Description	Create a new draft generation for an existing connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string									true	"Connector UUID"
// @Param			request	body		OpenAPIConnectorJson	false	"Generation creation request; an empty body clones the newest generation as a draft"
// @Success		201		{object}	OpenAPIConnectorJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/generations [post]
func (r *ConnectorsRoutes) createGeneration(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	connectorId, httpErr := parseConnectorID(gctx)
	if httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		val.MarkErrorReturn()
		return
	}

	// Verify the connector exists and check auth
	connector, err := r.loadConnectorByID(ctx, connectorId)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFoundf("connector '%s' not found", connectorId))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}
	if httpErr := val.ValidateHttpStatusError(connector); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	var req *cschema.Connector
	// Support a blank post to create a new draft generation of the connector.
	if gctx.Request.ContentLength > 0 {
		req = cschema.NewConnector()
		if err := apgin.BindResourceJSON(gctx, req, smeta.ValidationModeCreate); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	result, err := r.connectors.CreateConnectorGeneration(ctx, connectorId, req)
	if err != nil {
		if errors.Is(err, core.ErrDraftAlreadyExists) {
			apgin.WriteError(gctx, nil, httperr.Conflict("a draft generation already exists for this connector"))
			val.MarkErrorReturn()
			return
		}
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFoundf("connector '%s' not found", connectorId))
			val.MarkErrorReturn()
			return
		}
		if errors.Is(err, core.ErrInvalidArgument) {
			apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusCreated, result.GetResource()); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Update connector generation
// @Description	Update a specific draft generation of a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string							true	"Connector UUID"
// @Param			generation	path		integer							true	"Generation number"
// @Param			request		body		OpenAPIConnectorPatchJson	true	"Generation update request"
// @Success		200		{object}	OpenAPIConnectorJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/generations/{generation} [patch]
func (r *ConnectorsRoutes) updateGeneration(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	generationID, httpErr := parseConnectorGenerationID(gctx)
	if httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		val.MarkErrorReturn()
		return
	}
	connectorId := generationID.ConnectorID
	generation := generationID.Generation

	var req cschema.ConnectorPatch
	if err := apgin.BindResourceJSON(gctx, &req, smeta.ValidationModeUpdate); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	existing, err := r.connectors.GetConnectorGeneration(ctx, connectorId, generation)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFoundf("connector generation '%s:%d' not found", connectorId, generation))
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

	if existing.GetState() != database.ConnectorGenerationStateDraft {
		apgin.WriteError(gctx, nil, httperr.Conflictf("connector generation '%s:%d' is not a draft", connectorId, generation))
		val.MarkErrorReturn()
		return
	}

	result, err := r.connectors.UpdateConnectorGeneration(ctx, connectorId, generation, &req)
	if err != nil {
		if errors.Is(err, core.ErrNotDraft) {
			apgin.WriteError(gctx, nil, httperr.Conflictf("connector generation '%s:%d' is not a draft", connectorId, generation))
			val.MarkErrorReturn()
			return
		}
		if errors.Is(err, core.ErrInvalidArgument) {
			apgin.WriteError(gctx, nil, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, result.GetResource()); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Force connector generation state
// @Description	Force a connector generation to a specific state (admin operation)
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string								true	"Connector UUID"
// @Param			generation	path		integer								true	"Generation number"
// @Param			request	body		OpenAPIConnectorForceStateActionJson	true	"Force-state action"
// @Success		200		{object}	OpenAPIConnectorJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/generations/{generation}/_forceState [put]
func (r *ConnectorsRoutes) forceGenerationState(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	generationID, httpErr := parseConnectorGenerationID(gctx)
	if httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		val.MarkErrorReturn()
		return
	}
	connectorId := generationID.ConnectorID
	generation := generationID.Generation

	req := schemaapi.ConnectorForceStateAction{}
	if err := apgin.BindActionJSON(gctx, &req, schemaapi.ConnectorForceStateActionKind); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return
	}
	if req.Metadata.Target.ID != connectorId.String() || req.Metadata.Target.Generation != generation {
		apgin.WriteError(gctx, nil, httperr.BadRequest("metadata.target must match the connector generation in the request path"))
		val.MarkErrorReturn()
		return
	}

	state := database.ConnectorGenerationState(req.Spec.State)
	if !database.IsValidConnectorGenerationState(state) {
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid connector generation state '%s'", req.Spec.State))
		val.MarkErrorReturn()
		return
	}

	c, err := r.connectors.GetConnectorGeneration(ctx, connectorId, generation)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFoundf("connector generation '%s:%d' not found", connectorId, generation))
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

	if c.GetState() == state {
		if err := apgin.RenderResourceJSON(gctx, http.StatusOK, c.GetResource()); err != nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
		}
		return
	}

	err = c.SetState(ctx, state)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.FromError(err))
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, c.GetResource()); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

func (r *ConnectorsRoutes) loadConnectorByID(ctx context.Context, connectorId apid.ID) (connIface.Connector, error) {
	result := r.connectors.
		ListConnectorsBuilder().
		ForId(connectorId).
		Limit(1).
		FetchPage(ctx)
	if result.Error != nil {
		return nil, result.Error
	}
	if len(result.Results) == 0 {
		return nil, core.ErrNotFound
	}
	return result.Results[0], nil
}

func connectorActionTarget(connectorID apid.ID, generation uint64) smeta.ObjectReference {
	return smeta.ObjectReference{
		APIVersion: smeta.APIVersionV1Alpha1,
		Kind:       cschema.ConnectorKind,
		ID:         connectorID.String(),
		Generation: generation,
	}
}

func (r *ConnectorsRoutes) parseLifecycleRequest(
	gctx *gin.Context,
	connectorID apid.ID,
	kind smeta.Kind,
) (schemaapi.ConnectorLifecycleSpec, connIface.ConnectorLifecycleOptions, bool) {
	val := auth.MustGetValidatorFromGinContext(gctx)

	req := schemaapi.ConnectorLifecycleAction{}
	if err := apgin.BindActionJSON(gctx, &req, kind); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		val.MarkErrorReturn()
		return schemaapi.ConnectorLifecycleSpec{}, connIface.ConnectorLifecycleOptions{}, false
	}
	if req.Metadata.Target.ID != connectorID.String() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("metadata.target must match the connector in the request path"))
		val.MarkErrorReturn()
		return schemaapi.ConnectorLifecycleSpec{}, connIface.ConnectorLifecycleOptions{}, false
	}

	timeout := defaultConnectorLifecycleTimeout
	if req.Spec.TimeoutSeconds != nil {
		timeout = time.Duration(*req.Spec.TimeoutSeconds) * time.Second
	}

	return req.Spec, connIface.ConnectorLifecycleOptions{Timeout: timeout}, true
}

func (r *ConnectorsRoutes) writeLifecycleTaskResponse(
	gctx *gin.Context,
	kind smeta.Kind,
	connectorID apid.ID,
	spec schemaapi.ConnectorLifecycleSpec,
	taskInfo *tasks.TaskInfo,
) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	if taskInfo == nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("connector lifecycle task was not started"))
		val.MarkErrorReturn()
		return
	}

	ra := auth.MustGetAuthFromGinContext(gctx)
	taskId, err := taskInfo.
		BindToActor(ra.MustGetActor()).
		ToSecureEncryptedString(ctx, r.encrypt)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	response := schemaapi.NewConnectorLifecycleResponse(
		kind,
		connectorActionTarget(connectorID, 0),
		spec,
		taskId,
	)
	if err := apgin.RenderActionJSON(gctx, http.StatusOK, &response, kind); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Disconnect all connector connections
// @Description	Start a workflow that disconnects all connections for a connector
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string							true	"Connector UUID"
// @Param			request	body		OpenAPIConnectorLifecycleActionJson	true	"Disconnect-all action"
// @Success		200		{object}	OpenAPIConnectorLifecycleActionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Failure		501		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/_disconnectAll [post]
func (r *ConnectorsRoutes) disconnectAll(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	connectorId, httpErr := parseConnectorID(gctx)
	if httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		val.MarkErrorReturn()
		return
	}

	connector, err := r.loadConnectorByID(ctx, connectorId)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFoundf("connector '%s' not found", connectorId))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(connector); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	spec, opts, ok := r.parseLifecycleRequest(gctx, connectorId, schemaapi.ConnectorDisconnectAllActionKind)
	if !ok {
		return
	}

	taskInfo, err := r.connectors.DisconnectConnectorConnections(ctx, connectorId, opts)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	r.writeLifecycleTaskResponse(gctx, schemaapi.ConnectorDisconnectAllActionKind, connectorId, spec, taskInfo)
}

// @Summary		Archive connector
// @Description	Start a workflow that archives a connector after disconnecting its connections
// @Tags			connectors
// @Accept			json
// @Produce		json
// @Param			id		path		string							true	"Connector UUID"
// @Param			request	body		OpenAPIConnectorLifecycleActionJson	true	"Archive action"
// @Success		200		{object}	OpenAPIConnectorLifecycleActionJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Failure		501		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connectors/{id}/_archive [post]
func (r *ConnectorsRoutes) archive(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	connectorId, httpErr := parseConnectorID(gctx)
	if httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		val.MarkErrorReturn()
		return
	}

	connector, err := r.loadConnectorByID(ctx, connectorId)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFoundf("connector '%s' not found", connectorId))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(connector); httpErr != nil {
		apgin.WriteError(gctx, nil, httpErr)
		return
	}

	spec, opts, ok := r.parseLifecycleRequest(gctx, connectorId, schemaapi.ConnectorArchiveActionKind)
	if !ok {
		return
	}

	taskInfo, err := r.connectors.ArchiveConnector(ctx, connectorId, opts)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		val.MarkErrorReturn()
		return
	}

	r.writeLifecycleTaskResponse(gctx, schemaapi.ConnectorArchiveActionKind, connectorId, spec, taskInfo)
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
	g.GET("/connectors/:id/generations",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("list/generations").
			Build(),
		r.listGenerations,
	)
	g.GET(
		"/connectors/:id/generations/:generation",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("list/generations").
			Build(),
		r.getGeneration,
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
	g.POST("/connectors/:id/generations",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("create").
			Build(),
		r.createGeneration,
	)
	g.POST("/connectors/:id/_disconnectAll",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("disconnect_all").
			Build(),
		r.disconnectAll,
	)
	g.POST("/connectors/:id/_archive",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("archive").
			Build(),
		r.archive,
	)
	g.PATCH("/connectors/:id/generations/:generation",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.updateGeneration,
	)
	g.PUT("/connectors/:id/generations/:generation/_forceState",
		r.authService.NewRequiredBuilder().
			ForResource("connectors").
			ForIdField("id").
			ForVerb("force_state").
			Build(),
		r.forceGenerationState,
	)
}

func NewConnectorsRoutes(cfg config.C, authService auth.A, c connIface.C, e encrypt.E) *ConnectorsRoutes {
	return &ConnectorsRoutes{
		cfg:         cfg,
		authService: authService,
		connectors:  c,
		encrypt:     e,
	}
}
