package routes

import (
	"context"
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/routes/key_value"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"

	"log/slog"
	"net/http"
	"time"
)

type ActorsRoutes struct {
	cfg           config.C
	auth          auth.A
	db            database.DB
	r             apredis.Client
	httpf         httpf.F
	encrypt       encrypt.E
	logger        *slog.Logger
	labelsAdapter key_value.Adapter[apid.ID]
	annotsAdapter key_value.Adapter[apid.ID]
}

type ActorJson struct {
	Id          apid.ID           `json:"id" swaggertype:"string"`
	Namespace   string            `json:"namespace"`
	ExternalId  string            `json:"external_id"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type CreateActorRequestJson struct {
	ExternalId  string            `json:"external_id"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type UpdateActorRequestJson struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

func DatabaseActorToJson(a *database.Actor) ActorJson {
	return ActorJson{
		Id:          a.Id,
		Namespace:   a.GetNamespace(),
		Labels:      a.GetLabels(),
		Annotations: a.GetAnnotations(),
		ExternalId:  a.ExternalId,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
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
		apgin.WriteError(gctx, r.logger, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	var ex database.ListActorsExecutor

	if req.Cursor != nil {
		ex, err = r.db.ListActorsFromCursor(ctx, *req.Cursor)
		if err != nil {
			apgin.WriteError(gctx, r.logger, httperr.BadRequestErr(err))
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

	a, err := r.db.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if a == nil {
		apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found"))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(a); httpErr != nil {
		apgin.WriteError(gctx, r.logger, httpErr)
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
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("external_id is required"))
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
			apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if a == nil {
		apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found"))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(a); httpErr != nil {
		apgin.WriteError(gctx, r.logger, httpErr)
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

	a, err := r.db.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			// The actor already doesn't exist
			val.MarkValidated()
			gctx.Status(http.StatusNoContent)
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
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
		apgin.WriteError(gctx, r.logger, httpErr)
		return
	}

	r.logger.Info("deleting actor ", "id", a.Id.String(), "external_id", a.ExternalId)

	err = r.db.DeleteActor(ctx, id)
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
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("external_id is required"))
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

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
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
		apgin.WriteError(gctx, r.logger, httpErr)
		return
	}

	r.logger.Info("deleting actor ", "id", a.Id.String(), "external_id", a.ExternalId)

	err = r.db.DeleteActor(ctx, a.Id)
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
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Validate external_id is not empty
	if req.ExternalId == "" {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("external_id is required"))
		val.MarkErrorReturn()
		return
	}

	// Validate namespace path
	if err := database.ValidateNamespacePath(req.Namespace); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest(fmt.Sprintf("invalid namespace '%s': %s", req.Namespace, err.Error()), httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Validate labels if provided
	if req.Labels != nil {
		if err := database.ValidateUserLabels(req.Labels); err != nil {
			apgin.WriteError(gctx, r.logger, httperr.BadRequest(fmt.Sprintf("invalid labels: %s", err.Error()), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	// Validate authorization for the namespace
	if err := val.ValidateNamespace(req.Namespace); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.Forbidden(err.Error(), httperr.WithPublicErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Check if actor already exists with the same external_id in the namespace
	existingActor, err := r.db.GetActorByExternalId(ctx, req.Namespace, req.ExternalId)
	if err == nil && existingActor != nil {
		apgin.WriteError(gctx, r.logger, httperr.Conflictf("actor with external_id '%s' already exists in namespace '%s'", req.ExternalId, req.Namespace))
		val.MarkErrorReturn()
		return
	}

	if err != nil && !errors.Is(err, database.ErrNotFound) {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Validate annotations if provided
	if req.Annotations != nil {
		if err := database.Annotations(req.Annotations).Validate(); err != nil {
			apgin.WriteError(gctx, r.logger, httperr.BadRequest(fmt.Sprintf("invalid annotations: %s", err.Error()), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	// Create the actor
	actor := &database.Actor{
		Id:          apid.New(apid.PrefixActor),
		Namespace:   req.Namespace,
		ExternalId:  req.ExternalId,
		Labels:      req.Labels,
		Annotations: req.Annotations,
	}

	if err := r.db.CreateActor(ctx, actor); err != nil {
		// Handle specific error cases
		if errors.Is(err, database.ErrNamespaceDoesNotExist) {
			apgin.WriteError(gctx, r.logger, httperr.BadRequestf("namespace '%s' does not exist", req.Namespace))
			val.MarkErrorReturn()
			return
		}

		if errors.Is(err, database.ErrDuplicate) {
			apgin.WriteError(gctx, r.logger, httperr.Conflictf("actor with external_id '%s' already exists in namespace '%s'", req.ExternalId, req.Namespace))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Fetch the created actor to get the timestamps
	createdActor, err := r.db.GetActor(ctx, actor.Id)
	if err != nil {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
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

	var req UpdateActorRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Validate labels if provided
	if req.Labels != nil {
		if err := database.ValidateUserLabels(req.Labels); err != nil {
			apgin.WriteError(gctx, r.logger, httperr.BadRequest(fmt.Sprintf("invalid labels: %s", err.Error()), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	// Validate annotations if provided
	if req.Annotations != nil {
		if err := database.Annotations(req.Annotations).Validate(); err != nil {
			apgin.WriteError(gctx, r.logger, httperr.BadRequest(fmt.Sprintf("invalid annotations: %s", err.Error()), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	// Get the existing actor
	existingActor, err := r.db.GetActor(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if existingActor == nil {
		apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found"))
		val.MarkErrorReturn()
		return
	}

	// Validate authorization for the actor's namespace
	if httpErr := val.ValidateHttpStatusError(existingActor); httpErr != nil {
		apgin.WriteError(gctx, r.logger, httpErr)
		return
	}

	if req.Labels != nil {
		existingActor.Labels = req.Labels
	}

	if req.Annotations != nil {
		existingActor.Annotations = req.Annotations
	}

	updatedActor, err := r.db.UpsertActor(ctx, existingActor)
	if err != nil {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
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
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("external_id is required"))
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
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("invalid request body", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	// Validate labels if provided
	if req.Labels != nil {
		if err := database.ValidateUserLabels(req.Labels); err != nil {
			apgin.WriteError(gctx, r.logger, httperr.BadRequest(fmt.Sprintf("invalid labels: %s", err.Error()), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	// Validate annotations if provided
	if req.Annotations != nil {
		if err := database.Annotations(req.Annotations).Validate(); err != nil {
			apgin.WriteError(gctx, r.logger, httperr.BadRequest(fmt.Sprintf("invalid annotations: %s", err.Error()), httperr.WithInternalErr(err)))
			val.MarkErrorReturn()
			return
		}
	}

	// Get the existing actor
	existingActor, err := r.db.GetActorByExternalId(ctx, namespace, externalId)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if existingActor == nil {
		apgin.WriteError(gctx, r.logger, httperr.NotFound("actor not found"))
		val.MarkErrorReturn()
		return
	}

	// Validate authorization for the actor's namespace
	if httpErr := val.ValidateHttpStatusError(existingActor); httpErr != nil {
		apgin.WriteError(gctx, r.logger, httpErr)
		return
	}

	if req.Labels != nil {
		existingActor.Labels = req.Labels
	}

	if req.Annotations != nil {
		existingActor.Annotations = req.Annotations
	}

	updatedActor, err := r.db.UpsertActor(ctx, existingActor)
	if err != nil {
		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	gctx.PureJSON(http.StatusOK, DatabaseActorToJson(updatedActor))
}

// Label and annotation handlers for actors delegate to a shared
// generic adapter (see internal/routes/key_value). The doc comments below
// drive the OpenAPI spec; the bodies forward to the adapter.

// @Summary		Get all labels for an actor
// @Description	Get all labels associated with a specific actor
// @Tags			actors
// @Produce		json
// @Param			id	path		string	true	"Actor UUID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id}/labels [get]
func (r *ActorsRoutes) getLabels(gctx *gin.Context) { r.labelsAdapter.HandleList(gctx) }

// @Summary		Get a specific label for an actor
// @Description	Get a specific label value by key for an actor
// @Tags			actors
// @Produce		json
// @Param			id		path		string	true	"Actor UUID"
// @Param			label	path		string	true	"Label key"
// @Success		200		{object}	SwaggerKeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id}/labels/{label} [get]
func (r *ActorsRoutes) getLabel(gctx *gin.Context) { r.labelsAdapter.HandleGet(gctx) }

// @Summary		Set a label for an actor
// @Description	Set or update a specific label value by key for an actor
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			id		path		string						true	"Actor UUID"
// @Param			label	path		string						true	"Label key"
// @Param			request	body		SwaggerPutKeyValueRequest	true	"Label value"
// @Success		200		{object}	SwaggerKeyValueJson
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id}/labels/{label} [put]
func (r *ActorsRoutes) putLabel(gctx *gin.Context) { r.labelsAdapter.HandlePut(gctx) }

// @Summary		Delete a label from an actor
// @Description	Delete a specific label by key from an actor
// @Tags			actors
// @Param			id		path	string	true	"Actor UUID"
// @Param			label	path	string	true	"Label key"
// @Success		204		"No Content"
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id}/labels/{label} [delete]
func (r *ActorsRoutes) deleteLabel(gctx *gin.Context) { r.labelsAdapter.HandleDelete(gctx) }

// @Summary		Get all annotations for an actor
// @Description	Get all annotations for an actor by ID
// @Tags			actors
// @Produce		json
// @Param			id	path		string	true	"Actor UUID"
// @Success		200	{object}	map[string]string
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id}/annotations [get]
func (r *ActorsRoutes) getAnnotations(gctx *gin.Context) { r.annotsAdapter.HandleList(gctx) }

// @Summary		Get a specific annotation for an actor
// @Description	Get a specific annotation value by key for an actor
// @Tags			actors
// @Produce		json
// @Param			id			path		string	true	"Actor UUID"
// @Param			annotation	path		string	true	"Annotation key"
// @Success		200			{object}	SwaggerKeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id}/annotations/{annotation} [get]
func (r *ActorsRoutes) getAnnotation(gctx *gin.Context) { r.annotsAdapter.HandleGet(gctx) }

// @Summary		Set an annotation for an actor
// @Description	Set or update a specific annotation value by key for an actor
// @Tags			actors
// @Accept			json
// @Produce		json
// @Param			id			path		string						true	"Actor UUID"
// @Param			annotation	path		string						true	"Annotation key"
// @Param			request		body		SwaggerPutKeyValueRequest	true	"Annotation value"
// @Success		200			{object}	SwaggerKeyValueJson
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		404			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id}/annotations/{annotation} [put]
func (r *ActorsRoutes) putAnnotation(gctx *gin.Context) { r.annotsAdapter.HandlePut(gctx) }

// @Summary		Delete an annotation from an actor
// @Description	Delete a specific annotation by key from an actor
// @Tags			actors
// @Param			id			path	string	true	"Actor UUID"
// @Param			annotation	path	string	true	"Annotation key"
// @Success		204			"No Content"
// @Failure		400			{object}	ErrorResponse
// @Failure		401			{object}	ErrorResponse
// @Failure		403			{object}	ErrorResponse
// @Failure		500			{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/actors/{id}/annotations/{annotation} [delete]
func (r *ActorsRoutes) deleteAnnotation(gctx *gin.Context) { r.annotsAdapter.HandleDelete(gctx) }

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
	g.GET(
		"/actors/:id/annotations",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.getAnnotations,
	)
	g.GET(
		"/actors/:id/annotations/:annotation",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("id").
			ForVerb("get").
			Build(),
		r.getAnnotation,
	)
	g.PUT(
		"/actors/:id/annotations/:annotation",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.putAnnotation,
	)
	g.DELETE(
		"/actors/:id/annotations/:annotation",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("id").
			ForVerb("update").
			Build(),
		r.deleteAnnotation,
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
	parseActorID := func(gctx *gin.Context) (apid.ID, *httperr.Error) {
		id, err := apid.Parse(gctx.Param("id"))
		if err != nil {
			return apid.Nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err))
		}
		if id == apid.Nil {
			return apid.Nil, httperr.BadRequest("id is required")
		}
		return id, nil
	}

	getActor := func(ctx context.Context, id apid.ID) (key_value.Resource, error) {
		actor, err := db.GetActor(ctx, id)
		if err != nil {
			return nil, err
		}
		if actor == nil {
			return nil, nil
		}
		return actor, nil
	}

	authGet := authService.NewRequiredBuilder().
		ForResource("actors").
		ForIdField("id").
		ForVerb("get").
		Build()
	authMutate := authService.NewRequiredBuilder().
		ForResource("actors").
		ForIdField("id").
		ForVerb("update").
		Build()

	labelsAdapter := key_value.Adapter[apid.ID]{
		Kind:         key_value.Label,
		ResourceName: "actor",
		PathPrefix:   "/actors/:id",
		AuthGet:      authGet,
		AuthMutate:   authMutate,
		ParseID:      parseActorID,
		Get:          getActor,
		Put: func(ctx context.Context, id apid.ID, kv map[string]string) (key_value.Resource, error) {
			return db.PutActorLabels(ctx, id, kv)
		},
		Delete: func(ctx context.Context, id apid.ID, keys []string) (key_value.Resource, error) {
			return db.DeleteActorLabels(ctx, id, keys)
		},
	}

	annotsAdapter := key_value.Adapter[apid.ID]{
		Kind:         key_value.Annotation,
		ResourceName: "actor",
		PathPrefix:   "/actors/:id",
		AuthGet:      authGet,
		AuthMutate:   authMutate,
		ParseID:      parseActorID,
		Get:          getActor,
		Put: func(ctx context.Context, id apid.ID, kv map[string]string) (key_value.Resource, error) {
			return db.PutActorAnnotations(ctx, id, kv)
		},
		Delete: func(ctx context.Context, id apid.ID, keys []string) (key_value.Resource, error) {
			return db.DeleteActorAnnotations(ctx, id, keys)
		},
	}

	return &ActorsRoutes{
		cfg:           cfg,
		auth:          authService,
		db:            db,
		r:             r,
		httpf:         httpf,
		encrypt:       encrypt,
		logger:        logger,
		labelsAdapter: labelsAdapter,
		annotsAdapter: annotsAdapter,
	}
}
