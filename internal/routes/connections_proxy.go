package routes

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/request_log"

	"log/slog"
)

type ConnectionsProxyRoutes struct {
	cfg     config.C
	auth    auth.A
	core    iface.C
	db      database.DB
	r       apredis.Client
	httpf   httpf.F
	encrypt encrypt.E
	logger  *slog.Logger
}

func (r *ConnectionsProxyRoutes) proxy(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusUnauthorized().
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	id := gctx.Param("id")
	connectionUuid, err := uuid.Parse(id)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("invalid connection id").
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if connectionUuid == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("connection id is required").
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	// TODO: add security checking for ownership

	conn, err := r.core.GetConnection(ctx, connectionUuid)
	if err != nil {
		if errors.Is(err, iface.ErrConnectionNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("connection not found").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	var proxyRequest iface.ProxyRequest
	if err := gctx.ShouldBindJSON(&proxyRequest); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("invalid proxy request payload").
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	validateErr := proxyRequest.Validate()
	if validateErr != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithInternalErr(validateErr).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	resp, err := conn.ProxyRequest(ctx, request_log.RequestTypeProxy, &proxyRequest)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(200, resp)
}

func (r *ConnectionsProxyRoutes) Register(g gin.IRouter) {
	g.POST("/connections/:id/_proxy", r.auth.Required(), r.proxy)
}

func NewConnectionsProxyRoutes(
	cfg config.C,
	authService auth.A,
	db database.DB,
	r apredis.Client,
	c iface.C,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) *ConnectionsProxyRoutes {
	return &ConnectionsProxyRoutes{
		cfg:     cfg,
		auth:    authService,
		core:    c,
		db:      db,
		r:       r,
		httpf:   httpf,
		encrypt: encrypt,
		logger:  logger,
	}
}
