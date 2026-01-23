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
	Id         uuid.UUID `json:"id"`
	ExternalId string    `json:"external_id"`
	Email      string    `json:"email"`
	Admin      bool      `json:"admin"`
	SuperAdmin bool      `json:"super_admin"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func DatabaseActorToJson(a *database.Actor) ActorJson {
	return ActorJson{
		Id:         a.Id,
		ExternalId: a.ExternalId,
		Email:      a.Email,
		Admin:      a.Admin,
		SuperAdmin: a.SuperAdmin,
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
	}
}

type ListActorsRequestQuery struct {
	Cursor     *string `form:"cursor"`
	LimitVal   *int32  `form:"limit"`
	ExternalId *string `form:"external_id"`
	Email      *string `form:"email"`
	Admin      *bool   `form:"admin"`
	SuperAdmin *bool   `form:"super_admin"`
	OrderByVal *string `form:"order_by"`
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

		if req.Email != nil {
			b = b.ForEmail(*req.Email)
		}

		if req.Admin != nil {
			b = b.ForIsAdmin(*req.Admin)
		}

		if req.SuperAdmin != nil {
			b = b.ForIsSuperAdmin(*req.SuperAdmin)
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
	externalId := gctx.Param("external_id")
	val := auth.MustGetValidatorFromGinContext(gctx)

	if externalId == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("external_id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		val.MarkErrorReturn()
		return
	}

	a, err := r.db.GetActorByExternalId(ctx, externalId)
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

	a, err := r.db.GetActorByExternalId(ctx, externalId)
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

func (r *ActorsRoutes) Register(g gin.IRouter) {
	g.GET(
		"/actors",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForVerb("list").
			Build(),
		r.list,
	)
	g.GET(
		"/actors/external-id/:external_id",
		r.auth.NewRequiredBuilder().
			ForResource("actors").
			ForIdField("external_id").
			ForIdExtractor(func(obj interface{}) string { return obj.(*database.Actor).ExternalId }).
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
