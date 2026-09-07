package routes

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	smeta "github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type ActorsRoutes struct {
	core   coreIface.C
	auth   auth.A
	logger *slog.Logger
}

var (
	_ = schemaapiopenapi.ActorJson{}
	_ = schemaapiopenapi.ActorPatchJson{}
	_ = schemaapiopenapi.ListActorsResponseJson{}
)

type ListActorsRequestQuery struct {
	Cursor        *string `form:"cursor"`
	LimitVal      *int32  `form:"limit"`
	ExternalId    *string `form:"externalId"`
	NameVal       *string `form:"name"`
	NamespaceVal  *string `form:"namespace"`
	LabelSelector *string `form:"labelSelector"`
	OrderByVal    *string `form:"orderBy"`
}

// @Summary		List actors
// @Description	List actors with optional filtering and pagination
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			externalId		query		string	false	"Filter by external ID"
// @Param			name			query		string	false	"Filter by exact resource name"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			labelSelector	query		string	false	"Filter by label selector"
// @Param			orderBy		query		string	false	"Order by field (e.g., 'created_at:asc')"
// @Success		200				{object}	schemaapiopenapi.ListActorsResponseJson
// @Failure		400				{object}	ErrorResponse
// @Failure		401				{object}	ErrorResponse
// @Failure		500				{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors [get]
func (r *ActorsRoutes) list(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	ctx := gctx.Request.Context()

	var req ListActorsRequestQuery
	var err error

	if err = gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	var ex coreIface.ListActorsExecutor

	if req.Cursor != nil {
		ex, err = r.core.ListActorsFromCursor(ctx, *req.Cursor)
		if err != nil {
			apgin.WriteError(gctx, r.logger, httperr.BadRequestErr(err))
			val.MarkErrorReturn()
			return
		}
	} else {
		b := r.core.ListActorsBuilder()

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.ExternalId != nil {
			b = b.ForExternalId(*req.ExternalId)
		}

		if req.NameVal != nil {
			name := scommon.ResourceName(*req.NameVal)
			if err := name.Validate(); err != nil {
				apgin.WriteError(gctx, r.logger, httperr.BadRequestf("invalid actor name: %s", err.Error()))
				val.MarkErrorReturn()
				return
			}
			b = b.ForName(name)
		}

		b = b.ForNamespaceMatchers(val.GetEffectiveNamespaceMatchers(req.NamespaceVal))

		if req.LabelSelector != nil {
			b = b.ForLabelSelector(*req.LabelSelector)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.ActorOrderByField](*req.OrderByVal)
			if err != nil {
				apgin.WriteError(gctx, r.logger, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
				val.MarkErrorReturn()
				return
			}

			if !database.IsValidActorOrderByField(field) {
				apgin.WriteError(gctx, r.logger, httperr.BadRequestf("invalid sort field '%s'", field))
				val.MarkErrorReturn()
				return
			}

			b.OrderBy(field, order)
		}

		ex = b
	}

	result := ex.FetchPage(ctx)

	if result.Error != nil {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(result.Error)))
		val.MarkErrorReturn()
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewListActorsResponseJson(
		util.Map(auth.FilterForValidatedResources(val, result.Results), func(actor coreIface.Actor) actorschema.Actor {
			return *actor.GetResource()
		}),
		result.Cursor,
	))
}

// @Summary		Get actor by UUID
// @Description	Get a specific actor by its UUID
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Actor UUID"
// @Success		200	{object}	schemaapiopenapi.ActorJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id} [get]
func (r *ActorsRoutes) get(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	ctx := gctx.Request.Context()

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("id is required", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	a, err := r.core.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(a); httpErr != nil {
		apgin.WriteError(gctx, r.logger, httpErr)
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, a.GetResource()); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Get actor by external ID
// @Description	Get a specific actor by its external ID within a namespace
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			externalId	path		string	true	"External ID of the actor"
// @Param			namespace	query		string	false	"Namespace (defaults to authenticated actor's namespace)"
// @Success		200			{object}	schemaapiopenapi.ActorJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/external-id/{externalId} [get]
func (r *ActorsRoutes) getByExternalId(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	externalId := gctx.Param("externalId")
	if externalId == "" {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("externalId is required"))
		val.MarkErrorReturn()
		return
	}

	ra := auth.MustGetAuthFromGinContext(gctx)
	namespace := ra.GetActor().GetNamespace()
	if gctx.Query("namespace") != "" {
		namespace = gctx.Query("namespace")
	}

	a, err := r.core.GetActorByExternalId(ctx, namespace, externalId)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(a); httpErr != nil {
		apgin.WriteError(gctx, r.logger, httpErr)
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, a.GetResource()); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Delete actor by UUID
// @Description	Delete a specific actor by its UUID
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			id	path	string	true	"Actor UUID"
// @Success		204	"No Content"
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		403	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id} [delete]
func (r *ActorsRoutes) delete(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("id is required", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	a, err := r.core.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			// The actor already doesn't exist
			val.MarkValidated()
			gctx.Status(http.StatusNoContent)
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(a); httpErr != nil {
		apgin.WriteError(gctx, r.logger, httpErr)
		return
	}

	r.logger.Info("deleting actor", "id", a.GetId().String(), "externalId", a.GetExternalId())

	err = r.core.DeleteActor(ctx, id)
	if err != nil {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	gctx.Status(http.StatusNoContent)
}

// @Summary		Delete actor by external ID
// @Description	Delete a specific actor by its external ID within a namespace
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			externalId	path	string	true	"External ID of the actor"
// @Param			namespace	query	string	false	"Namespace (defaults to authenticated actor's namespace)"
// @Success		204			"No Content"
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/external-id/{externalId} [delete]
func (r *ActorsRoutes) deleteByExternalId(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)
	externalId := gctx.Param("externalId")

	if externalId == "" {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("externalId is required"))
		val.MarkErrorReturn()
		return
	}

	ra := auth.MustGetAuthFromGinContext(gctx)
	namespace := ra.GetActor().GetNamespace()
	if gctx.Query("namespace") != "" {
		namespace = gctx.Query("namespace")
	}

	a, err := r.core.GetActorByExternalId(ctx, namespace, externalId)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			// The actor already doesn't exist
			gctx.Status(http.StatusNoContent)
			val.MarkValidated()
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(a); httpErr != nil {
		apgin.WriteError(gctx, r.logger, httpErr)
		return
	}

	r.logger.Info("deleting actor", "id", a.GetId().String(), "externalId", a.GetExternalId())

	err = r.core.DeleteActor(ctx, a.GetId())
	if err != nil {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	gctx.Status(http.StatusNoContent)
}

// @Summary		Create actor
// @Description	Create a new actor in a namespace
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			request	body		schemaapiopenapi.ActorJson	true	"Actor creation request"
// @Success		201		{object}	schemaapiopenapi.ActorJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors [post]
func (r *ActorsRoutes) create(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	var req actorschema.Actor
	if err := apgin.BindResourceJSON(gctx, &req, smeta.ValidationModeCreate); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := val.ValidateNamespace(req.Metadata.Namespace); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.Forbidden(err.Error(), httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	actor, err := r.core.CreateActor(ctx, &req)
	if err != nil {
		if errors.Is(err, core.ErrInvalidArgument) {
			apgin.WriteError(gctx, r.logger, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
			val.MarkErrorReturn()
			return
		}
		if errors.Is(err, database.ErrNamespaceDoesNotExist) {
			apgin.WriteError(gctx, r.logger, httperr.BadRequestf("namespace '%s' does not exist", req.Metadata.Namespace))
			val.MarkErrorReturn()
			return
		}
		if errors.Is(err, database.ErrDuplicate) {
			if conflictErr := resourceNameConflictError(err, "actor", req.Metadata.Name, req.Metadata.Namespace); conflictErr != nil && req.Metadata.Name != "" {
				apgin.WriteError(gctx, r.logger, conflictErr)
			} else {
				apgin.WriteError(gctx, r.logger, httperr.Conflictf("actor with externalId '%s' already exists in namespace '%s'", req.Spec.ExternalId, req.Metadata.Namespace))
			}
			val.MarkErrorReturn()
			return
		}
		apgin.WriteErr(gctx, r.logger, err)
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusCreated, actor.GetResource()); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Update actor by UUID
// @Description	Update a specific actor by its UUID
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			id		path		string					true	"Actor UUID"
// @Param			request	body		schemaapiopenapi.ActorPatchJson	true	"Actor update request"
// @Success		200		{object}	schemaapiopenapi.ActorJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		409		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id} [patch]
func (r *ActorsRoutes) update(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if id == apid.Nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("id is required"))
		val.MarkErrorReturn()
		return
	}

	var req actorschema.ActorPatch
	if err := apgin.BindResourceJSON(gctx, &req, smeta.ValidationModeUpdate); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Get the existing actor
	existingActor, err := r.core.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Validate authorization for the actor's namespace
	if httpErr := val.ValidateHttpStatusError(existingActor); httpErr != nil {
		apgin.WriteError(gctx, r.logger, httpErr)
		return
	}

	updatedActor, err := r.core.UpdateActor(ctx, id, &req)
	if err != nil {
		if req.Metadata.Name != nil {
			if conflictErr := resourceNameConflictError(err, "actor", *req.Metadata.Name, existingActor.GetNamespace()); conflictErr != nil {
				apgin.WriteError(gctx, r.logger, conflictErr)
				val.MarkErrorReturn()
				return
			}
		}
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found", httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
		if errors.Is(err, core.ErrInvalidArgument) {
			apgin.WriteError(gctx, r.logger, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, updatedActor.GetResource()); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

// @Summary		Update actor by external ID
// @Description	Update a specific actor by its external ID within a namespace
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			externalId	path		string					true	"External ID of the actor"
// @Param			namespace	query		string					false	"Namespace (defaults to authenticated actor's namespace)"
// @Param			request		body		schemaapiopenapi.ActorPatchJson	true	"Actor update request"
// @Success		200			{object}	schemaapiopenapi.ActorJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/external-id/{externalId} [patch]
func (r *ActorsRoutes) updateByExternalId(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	externalId := gctx.Param("externalId")
	if externalId == "" {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("externalId is required"))
		val.MarkErrorReturn()
		return
	}

	ra := auth.MustGetAuthFromGinContext(gctx)
	namespace := ra.GetActor().GetNamespace()
	if gctx.Query("namespace") != "" {
		namespace = gctx.Query("namespace")
	}

	var req actorschema.ActorPatch
	if err := apgin.BindResourceJSON(gctx, &req, smeta.ValidationModeUpdate); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	if req.Metadata.Name != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("actor names can only be changed through the ID-addressed update endpoint"))
		val.MarkErrorReturn()
		return
	}

	// Get the existing actor
	existingActor, err := r.core.GetActorByExternalId(ctx, namespace, externalId)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Validate authorization for the actor's namespace
	if httpErr := val.ValidateHttpStatusError(existingActor); httpErr != nil {
		apgin.WriteError(gctx, r.logger, httpErr)
		return
	}

	updatedActor, err := r.core.UpdateActor(ctx, existingActor.GetId(), &req)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found", httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
		if errors.Is(err, core.ErrInvalidArgument) {
			apgin.WriteError(gctx, r.logger, httperr.BadRequestErr(err, httperr.WithPublicErr(err)))
			val.MarkErrorReturn()
			return
		}
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if err := apgin.RenderResourceJSON(gctx, http.StatusOK, updatedActor.GetResource()); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
	}
}

func (r *ActorsRoutes) Register(g gin.IRouter) {
	externalIDExtractor := func(obj interface{}) string {
		return obj.(coreIface.Actor).GetExternalId()
	}
	g.GET(
		"/actors",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForVerb("list").
			Build(),
		r.list,
	)
	g.POST(
		"/actors",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForVerb("create").
			Build(),
		r.create,
	)
	g.GET(
		"/actors/external-id/:externalId",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("externalId").
			ForIdExtractor(externalIDExtractor).
			ForNamespaceQueryParam("namespace").
			ForVerb("get").
			Build(),
		r.getByExternalId,
	)
	g.DELETE(
		"/actors/external-id/:externalId",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("externalId").
			ForIdExtractor(externalIDExtractor).
			ForNamespaceQueryParam("namespace").
			ForVerb("delete").
			Build(),
		r.deleteByExternalId,
	)
	g.PATCH(
		"/actors/external-id/:externalId",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("externalId").
			ForIdExtractor(externalIDExtractor).
			ForNamespaceQueryParam("namespace").
			ForVerb("update").
			Build(),
		r.updateByExternalId,
	)
	g.GET(
		"/actors/:id",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.get,
	)
	g.DELETE(
		"/actors/:id",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("id").
			ForVerb("delete").
			Build(),
		r.delete,
	)
	g.PATCH(
		"/actors/:id",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.update,
	)
}

func NewActorsRoutes(
	authService auth.A,
	c coreIface.C,
	logger *slog.Logger,
) *ActorsRoutes {
	return &ActorsRoutes{
		auth:   authService,
		core:   c,
		logger: logger,
	}
}
