package routes

import (
	"errors"

	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apgin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
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

// @Summary		Proxy request through connection
// @Description	Proxy an HTTP request through an authenticated connection to the external service
// @Tags			proxy
// @Accept			json
// @Produce		json
// @Param			id		path		string			true	"Connection UUID"
// @Param			request	body		ProxyRequest	true	"Proxy request payload"
// @Success		200		{object}	ProxyResponse
// @Failure		400		{object}	ErrorResponse
// @Failure		401		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		404		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/connections/{id}/_proxy [post]
func (r *ConnectionsProxyRoutes) proxy(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)

	id := gctx.Param("id")
	connectionUuid, err := apid.Parse(id)
	if err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("invalid connection id", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if connectionUuid == apid.Nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("connection id is required", httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	conn, err := r.core.GetConnection(ctx, connectionUuid)
	if err != nil {
		if errors.Is(err, iface.ErrConnectionNotFound) {
			apgin.WriteError(gctx, r.logger, httperr.NotFound("connection not found"))
			val.MarkErrorReturn()
			return
		}

		apgin.WriteError(gctx, r.logger, httperr.InternalServerError(httperr.WithInternalErr(err)))
		val.MarkErrorReturn()
		return
	}

	if httpErr := val.ValidateHttpStatusError(conn); httpErr != nil {
		apgin.WriteError(gctx, r.logger, httpErr)
		return
	}

	var proxyRequest iface.ProxyRequest
	if err := gctx.ShouldBindJSON(&proxyRequest); err != nil {
		apgin.WriteError(gctx, r.logger, httperr.BadRequest("invalid proxy request payload", httperr.WithInternalErr(err)))
		return
	}

	validateErr := proxyRequest.Validate()
	if validateErr != nil {
		apgin.WriteErr(gctx, r.logger, validateErr)
		return
	}

	resp, err := conn.ProxyRequest(ctx, httpf.RequestTypeProxy, &proxyRequest)
	if err != nil {
		apgin.WriteErr(gctx, r.logger, err)
		return
	}

	gctx.PureJSON(200, resp)
}

func (r *ConnectionsProxyRoutes) Register(g gin.IRouter) {
	g.POST(
		"/connections/:id/_proxy",
		r.auth.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("proxy").
			ForIdField("id").
			Build(),
		r.proxy,
	)
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
