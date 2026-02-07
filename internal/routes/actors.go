package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"log/slog"
	"net/http"
	"time"
)

type ActorsRoutes struct {
	cfg     config.C
	auth    auth.A
	db      database.DB
	r       apredis.Client
	httpf   httpf.F
	encrypt encrypt.E
	logger  *slog.Logger
}

type ActorJson struct {
	Id         uuid.UUID         `json:"id"`
	Namespace  string            `json:"namespace"`
	ExternalId string            `json:"external_id"`
	Labels     map[string]string `json:"labels,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type CreateActorRequestJson struct {
	ExternalId string            `json:"external_id"`
	Namespace  string            `json:"namespace"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type UpdateActorRequestJson struct {
	Labels map[string]string `json:"labels"`
}

type PutActorLabelRequestJson struct {
	Value string `json:"value"`
}

type ActorLabelJson struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func DatabaseActorToJson(a *database.Actor) ActorJson {
	return ActorJson{
		Id:         a.Id,
		Namespace:  a.GetNamespace(),
		Labels:     a.GetLabels(),
		ExternalId: a.ExternalId,
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
	}
}

type ListActorsRequestQuery struct {
	Cursor        *string `form:"cursor"`
	LimitVal      *int32  `form:"limit"`
	ExternalId    *string `form:"external_id"`
	NamespaceVal  *string `form:"namespace"`
	LabelSelector *string `form:"label_selector"`
	OrderByVal    *string `form:"order_by"`
}

type ListActorsResponseJson struct {
	Items  []ActorJson `json:"items"`
	Cursor string      `json:"cursor,omitempty"`
}

// @Summary		List actors
// @Description	List actors with optional filtering and pagination
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"Pagination cursor"
// @Param			limit			query		integer	false	"Maximum number of results to return"
// @Param			external_id		query		string	false	"Filter by external ID"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			label_selector	query		string	false	"Filter by label selector"
// @Param			order_by		query		string	false	"Order by field (e.g., 'created_at:asc')"
// @Success		200				{object}	ListActorsResponseJson
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
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg(err.Error()).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	var ex database.ListActorsExecutor

	if req.Cursor != nil {
		ex, err = r.db.ListActorsFromCursor(ctx, *req.Cursor)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithInternalErr(err).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}
	} else {
		b := r.db.ListActorsBuilder()

		if req.LimitVal != nil {
			b = b.Limit(*req.LimitVal)
		}

		if req.ExternalId != nil {
			b = b.ForExternalId(*req.ExternalId)
		}

		b = b.ForNamespaceMatchers(val.GetEffectiveNamespaceMatchers(req.NamespaceVal))

		if req.LabelSelector != nil {
			b = b.ForLabelSelector(*req.LabelSelector)
		}

		if req.OrderByVal != nil {
			field, order, err := pagination.SplitOrderByParam[database.ActorOrderByField](*req.OrderByVal)
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

			if !database.IsValidActorOrderByField(field) {
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
			WithStatusInternalServerError().
			WithInternalErr(result.Error).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, ListActorsResponseJson{
		Items:  util.Map(auth.FilterForValidatedResources(val, result.Results), DatabaseActorToJson),
		Cursor: result.Cursor,
	})
}

// @Summary		Get actor by UUID
// @Description	Get a specific actor by its UUID
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Actor UUID"
// @Success		200	{object}	ActorJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id} [get]
func (r *ActorsRoutes) get(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	ctx := gctx.Request.Context()

	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	a, err := r.db.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("actor not found").
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

	if a == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("actor not found").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(a); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, DatabaseActorToJson(a))
}

// @Summary		Get actor by external ID
// @Description	Get a specific actor by its external ID within a namespace
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			external_id	path		string	true	"External ID of the actor"
// @Param			namespace	query		string	false	"Namespace (defaults to authenticated actor's namespace)"
// @Success		200			{object}	ActorJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/external-id/{external_id} [get]
func (r *ActorsRoutes) getByExternalId(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	externalId := gctx.Param("external_id")
	if externalId == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("external_id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	ra := auth.MustGetAuthFromGinContext(gctx)
	namespace := ra.GetActor().GetNamespace()
	if gctx.Query("namespace") != "" {
		namespace = gctx.Query("namespace")
	}

	a, err := r.db.GetActorByExternalId(ctx, namespace, externalId)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("actor not found").
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

	if a == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("actor not found").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(a); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, DatabaseActorToJson(a))
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

	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	a, err := r.db.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			// The actor already doesn't exist
			val.MarkValidated()
			gctx.Status(http.StatusNoContent)
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

	// The actor already doesn't exist
	if a == nil {
		gctx.Status(http.StatusNoContent)
		val.MarkValidated()
		return
	}

	if httpErr := val.ValidateHttpStatusError(a); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	r.logger.Info("deleting actor ", "id", a.Id.String(), "external_id", a.ExternalId)

	err = r.db.DeleteActor(ctx, id)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
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
// @Param			external_id	path	string	true	"External ID of the actor"
// @Param			namespace	query	string	false	"Namespace (defaults to authenticated actor's namespace)"
// @Success		204			"No Content"
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/external-id/{external_id} [delete]
func (r *ActorsRoutes) deleteByExternalId(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)
	externalId := gctx.Param("external_id")

	if externalId == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("external_id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	ra := auth.MustGetAuthFromGinContext(gctx)
	namespace := ra.GetActor().GetNamespace()
	if gctx.Query("namespace") != "" {
		namespace = gctx.Query("namespace")
	}

	a, err := r.db.GetActorByExternalId(ctx, namespace, externalId)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			// The actor already doesn't exist
			gctx.Status(http.StatusNoContent)
			val.MarkValidated()
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

	// The actor already doesn't exist
	if a == nil {
		gctx.Status(http.StatusNoContent)
		val.MarkValidated()
		return
	}

	if httpErr := val.ValidateHttpStatusError(a); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	r.logger.Info("deleting actor ", "id", a.Id.String(), "external_id", a.ExternalId)

	err = r.db.DeleteActor(ctx, a.Id)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
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
// @Param			request	body		CreateActorRequestJson	true	"Actor creation request"
// @Success		201		{object}	ActorJson
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

	var req CreateActorRequestJson
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

	// Validate external_id is not empty
	if req.ExternalId == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("external_id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	// Validate namespace path
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

	// Validate labels if provided
	if req.Labels != nil {
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
	}

	// Validate authorization for the namespace
	if err := val.ValidateNamespace(req.Namespace); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusForbidden().
			WithPublicErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	// Check if actor already exists with the same external_id in the namespace
	existingActor, err := r.db.GetActorByExternalId(ctx, req.Namespace, req.ExternalId)
	if err == nil && existingActor != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatus(http.StatusConflict).
			WithResponseMsgf("actor with external_id '%s' already exists in namespace '%s'", req.ExternalId, req.Namespace).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if err != nil && !errors.Is(err, database.ErrNotFound) {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	// Create the actor
	actor := &database.Actor{
		Id:         uuid.New(),
		Namespace:  req.Namespace,
		ExternalId: req.ExternalId,
		Labels:     req.Labels,
	}

	if err := r.db.CreateActor(ctx, actor); err != nil {
		// Handle specific error cases
		if errors.Is(err, database.ErrNamespaceDoesNotExist) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithResponseMsgf("namespace '%s' does not exist", req.Namespace).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			val.MarkErrorReturn()
			return
		}

		if errors.Is(err, database.ErrDuplicate) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatus(http.StatusConflict).
				WithResponseMsgf("actor with external_id '%s' already exists in namespace '%s'", req.ExternalId, req.Namespace).
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

	// Fetch the created actor to get the timestamps
	createdActor, err := r.db.GetActor(ctx, actor.Id)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusCreated, DatabaseActorToJson(createdActor))
}

// @Summary		Update actor by UUID
// @Description	Update a specific actor by its UUID
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			id		path		string					true	"Actor UUID"
// @Param			request	body		UpdateActorRequestJson	true	"Actor update request"
// @Success		200		{object}	ActorJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id} [patch]
func (r *ActorsRoutes) update(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	var req UpdateActorRequestJson
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

	// Validate labels if provided
	if req.Labels != nil {
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
	}

	// Get the existing actor
	existingActor, err := r.db.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("actor not found").
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

	if existingActor == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("actor not found").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	// Validate authorization for the actor's namespace
	if httpErr := val.ValidateHttpStatusError(existingActor); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	if req.Labels != nil {
		existingActor.Labels = req.Labels
	}

	// Use UpsertActor to update
	updatedActor, err := r.db.UpsertActor(ctx, existingActor)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, DatabaseActorToJson(updatedActor))
}

// @Summary		Update actor by external ID
// @Description	Update a specific actor by its external ID within a namespace
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			external_id	path		string					true	"External ID of the actor"
// @Param			namespace	query		string					false	"Namespace (defaults to authenticated actor's namespace)"
// @Param			request		body		UpdateActorRequestJson	true	"Actor update request"
// @Success		200			{object}	ActorJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/external-id/{external_id} [patch]
func (r *ActorsRoutes) updateByExternalId(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	externalId := gctx.Param("external_id")
	if externalId == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("external_id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	ra := auth.MustGetAuthFromGinContext(gctx)
	namespace := ra.GetActor().GetNamespace()
	if gctx.Query("namespace") != "" {
		namespace = gctx.Query("namespace")
	}

	var req UpdateActorRequestJson
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

	// Validate labels if provided
	if req.Labels != nil {
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
	}

	// Get the existing actor
	existingActor, err := r.db.GetActorByExternalId(ctx, namespace, externalId)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("actor not found").
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

	if existingActor == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("actor not found").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	// Validate authorization for the actor's namespace
	if httpErr := val.ValidateHttpStatusError(existingActor); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	if req.Labels != nil {
		existingActor.Labels = req.Labels
	}

	// Use UpsertActor to update
	updatedActor, err := r.db.UpsertActor(ctx, existingActor)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, DatabaseActorToJson(updatedActor))
}

// @Summary		Get all labels for an actor
// @Description	Get all labels associated with a specific actor
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"Actor UUID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id}/labels [get]
func (r *ActorsRoutes) getLabels(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	actor, err := r.db.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("actor not found").
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

	if actor == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("actor not found").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(actor); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	labels := actor.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	gctx.PureJSON(http.StatusOK, labels)
}

// @Summary		Get a specific label for an actor
// @Description	Get a specific label value by key for an actor
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			id		path		string	true	"Actor UUID"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	ActorLabelJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id}/labels/{label} [get]
func (r *ActorsRoutes) getLabel(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == uuid.Nil {
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

	actor, err := r.db.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("actor not found").
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

	if actor == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("actor not found").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(actor); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	labels := actor.GetLabels()
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

	gctx.PureJSON(http.StatusOK, ActorLabelJson{
		Key:   labelKey,
		Value: value,
	})
}

// @Summary		Set a label for an actor
// @Description	Set or update a specific label value by key for an actor
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			id		path		string						true	"Actor UUID"
// @Param			label	path		string						true	"Label key"
// @Param			request	body		PutActorLabelRequestJson	true	"Label value"
// @Success		200		{object}	ActorLabelJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id}/labels/{label} [put]
func (r *ActorsRoutes) putLabel(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == uuid.Nil {
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

	// Validate label key
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

	var req PutActorLabelRequestJson
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

	// Validate label value
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

	// Get the existing actor for authorization check
	actor, err := r.db.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("actor not found").
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

	if actor == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("actor not found").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(actor); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	// Use transactional PutActorLabels to update
	updatedActor, err := r.db.PutActorLabels(ctx, id, map[string]string{labelKey: req.Value})
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("actor not found").
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

	gctx.PureJSON(http.StatusOK, ActorLabelJson{
		Key:   labelKey,
		Value: updatedActor.Labels[labelKey],
	})
}

// @Summary		Delete a label from an actor
// @Description	Delete a specific label by key from an actor
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			id		path	string	true	"Actor UUID"
// @Param			label	path	string	true	"Label key"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id}/labels/{label} [delete]
func (r *ActorsRoutes) deleteLabel(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	if id == uuid.Nil {
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

	// Get the existing actor for authorization check
	actor, err := r.db.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			// Actor doesn't exist, return 204 (idempotent delete)
			gctx.Status(http.StatusNoContent)
			val.MarkValidated()
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

	if actor == nil {
		// Actor doesn't exist, return 204 (idempotent delete)
		gctx.Status(http.StatusNoContent)
		val.MarkValidated()
		return
	}

	if httpErr := val.ValidateHttpStatusError(actor); httpErr != nil {
		httpErr.WriteGinResponse(r.cfg, gctx)
		return
	}

	// Use transactional DeleteActorLabels to delete
	_, err = r.db.DeleteActorLabels(ctx, id, []string{labelKey})
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			// Actor was deleted between the check and the update, return 204
			gctx.Status(http.StatusNoContent)
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

	gctx.Status(http.StatusNoContent)
}

func (r *ActorsRoutes) Register(g gin.IRouter) {
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
		"/actors/external-id/:external_id",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("external_id").
			ForIdExtractor(func(obj interface{}) string { return obj.(*database.Actor).ExternalId }).
			ForNamespaceQueryParam("namespace").
			ForVerb("get").
			Build(),
		r.getByExternalId,
	)
	g.DELETE(
		"/actors/external-id/:external_id",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("external_id").
			ForIdExtractor(func(obj interface{}) string { return obj.(*database.Actor).ExternalId }).
			ForNamespaceQueryParam("namespace").
			ForVerb("delete").
			Build(),
		r.deleteByExternalId,
	)
	g.PATCH(
		"/actors/external-id/:external_id",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("external_id").
			ForIdExtractor(func(obj interface{}) string { return obj.(*database.Actor).ExternalId }).
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
	g.GET(
		"/actors/:id/labels",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.getLabels,
	)
	g.GET(
		"/actors/:id/labels/:label",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.getLabel,
	)
	g.PUT(
		"/actors/:id/labels/:label",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.putLabel,
	)
	g.DELETE(
		"/actors/:id/labels/:label",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.deleteLabel,
	)
}

func NewActorsRoutes(
	cfg config.C,
	authService auth.A,
	db database.DB,
	r apredis.Client,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) *ActorsRoutes {
	return &ActorsRoutes{
		cfg:     cfg,
		auth:    authService,
		db:      db,
		r:       r,
		httpf:   httpf,
		encrypt: encrypt,
		logger:  logger,
	}
}
