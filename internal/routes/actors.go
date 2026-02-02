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
