package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	"github.com/rmorlok/authproxy/util"
	"github.com/rmorlok/authproxy/util/pagination"

	"log/slog"
	"net/http"
	"time"
)

type ActorsRoutes struct {
	cfg     config.C
	auth    auth.A
	db      database.DB
	redis   redis.R
	httpf   httpf.F
	encrypt encrypt.E
	logger  *slog.Logger
}

type ActorJson struct {
	ID         uuid.UUID `json:"id"`
	ExternalId string    `json:"external_id"`
	Email      string    `json:"email"`
	Admin      bool      `json:"admin"`
	SuperAdmin bool      `json:"super_admin"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func DatabaseActorToJson(a database.Actor) ActorJson {
	return ActorJson{
		ID:         a.ID,
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
	// The Auth check is duplicative of the annotation for the route, but being done since are pulling auth anyway.
	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusUnauthorized().
			WithResponseMsg("unauthorized").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if !util.ToPtr(ra.MustGetActor()).IsAdmin() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusForbidden().
			WithResponseMsg("only admins can delete actors").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

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
				return
			}

			if !database.IsValidActorOrderByField(field) {
				api_common.NewHttpStatusErrorBuilder().
					WithStatusBadRequest().
					WithResponseMsgf("invalid sort field '%s'", field).
					BuildStatusError().
					WriteGinResponse(r.cfg, gctx)
				return
			}

			b.OrderBy(database.ActorOrderByField(field), order)
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
		return
	}

	gctx.PureJSON(http.StatusOK, ListActorsResponseJson{
		Items:  util.Map(result.Results, DatabaseActorToJson),
		Cursor: result.Cursor,
	})
}

func (r *ActorsRoutes) get(gctx *gin.Context) {
	// The Auth check is duplicative of the annotation for the route, but being done since are pulling auth anyway.
	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusUnauthorized().
			WithResponseMsg("unauthorized").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if !util.ToPtr(ra.MustGetActor()).IsAdmin() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusForbidden().
			WithResponseMsg("only admins can delete actors").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	ctx := gctx.Request.Context()
	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if id == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
	}

	a, err := r.db.GetActor(ctx, id)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if a == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("actor not found").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, DatabaseActorToJson(*a))
}

func (r *ActorsRoutes) getByExternalId(gctx *gin.Context) {
	// The Auth check is duplicative of the annotation for the route, but being done since are pulling auth anyway.
	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusUnauthorized().
			WithResponseMsg("unauthorized").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if !util.ToPtr(ra.MustGetActor()).IsAdmin() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusForbidden().
			WithResponseMsg("only admins can delete actors").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	ctx := gctx.Request.Context()
	externalId := gctx.Param("externalId")

	if externalId == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("externalId is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
	}

	a, err := r.db.GetActorByExternalId(ctx, externalId)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if a == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("actor not found").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, DatabaseActorToJson(*a))
}

func (r *ActorsRoutes) delete(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	id, err := uuid.Parse(gctx.Param("id"))
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("failed to parse id as UUID").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if id == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	// The Auth check is duplicative of the annotation for the route, but being done since are pulling auth anyway.
	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusUnauthorized().
			WithResponseMsg("unauthorized").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if !util.ToPtr(ra.MustGetActor()).IsAdmin() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusForbidden().
			WithResponseMsg("only admins can delete actors").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	a, err := r.db.GetActor(ctx, id)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	// The actor already doesn't exist
	if a == nil {
		gctx.Status(http.StatusNoContent)
		return
	}

	r.logger.Info("deleting actor ", "id", a.ID.String(), "external_id", a.ExternalId)

	err = r.db.DeleteActor(ctx, id)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.Status(http.StatusNoContent)
}

func (r *ActorsRoutes) deleteByExternalId(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	externalId := gctx.Param("externalId")

	if externalId == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("externalId is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	// The Auth check is duplicative of the annotation for the route, but being done since are pulling auth anyway.
	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusUnauthorized().
			WithResponseMsg("unauthorized").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if !util.ToPtr(ra.MustGetActor()).IsAdmin() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusForbidden().
			WithResponseMsg("only admins can delete actors").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	a, err := r.db.GetActorByExternalId(ctx, externalId)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	// The actor already doesn't exist
	if a == nil {
		gctx.Status(http.StatusNoContent)
		return
	}

	r.logger.Info("deleting actor ", "id", a.ID.String(), "external_id", a.ExternalId)

	err = r.db.DeleteActor(ctx, a.ID)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.Status(http.StatusNoContent)
}

func (r *ActorsRoutes) Register(g gin.IRouter) {
	g.GET("/actors", r.auth.AdminOnly(), r.list)
	g.GET("/actors/external-id/:externalId", r.auth.AdminOnly(), r.getByExternalId)
	g.DELETE("/actors/external-id/:externalId", r.auth.AdminOnly(), r.deleteByExternalId)
	g.GET("/actors/:id", r.auth.AdminOnly(), r.get)
	g.DELETE("/actors/:id", r.auth.AdminOnly(), r.delete)
}

func NewActorsRoutes(
	cfg config.C,
	authService auth.A,
	db database.DB,
	redis redis.R,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) *ActorsRoutes {
	return &ActorsRoutes{
		cfg:     cfg,
		auth:    authService,
		db:      db,
		redis:   redis,
		httpf:   httpf,
		encrypt: encrypt,
		logger:  logger,
	}
}
