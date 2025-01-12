package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/oauth2"
	"github.com/rmorlok/authproxy/redis"
	"net/http"
)

type Oauth2Routes struct {
	cfg         config.C
	authService auth.A
	db          database.DB
	redis       *redis.Wrapper
}

type Oauth2QueryParams struct {
	Code  string `form:"code"`
	State string `form:"state"`
}

func (r *Oauth2Routes) callback(ctx *gin.Context) {
	var req Oauth2QueryParams
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.Redirect(http.StatusFound, r.cfg.GetRoot().ErrorPages.Fallback)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type RedirectParams struct {
	StateId string `form:"state_id"`
}

func (r *Oauth2Routes) redirect(gctx *gin.Context) {
	ctx := context.AsContext(gctx.Request.Context())

	actor := auth.GetActorInfoFromGinContext(gctx)
	if actor == nil {
		api_common.AddDebugHeader(r.cfg, gctx, "actor not present on context")
		gctx.Redirect(http.StatusFound, r.cfg.GetRoot().ErrorPages.GetUnauthorized())
		return
	}

	// TODO Cookie the user to make sure we have a session

	var req RedirectParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		api_common.AddDebugHeaderError(r.cfg, gctx, errors.Wrap(err, "failed to bind redirect params"))
		gctx.Redirect(http.StatusFound, r.cfg.GetRoot().ErrorPages.Fallback)
		return
	}

	if req.StateId == "" {
		api_common.AddDebugHeader(r.cfg, gctx, "state_id is required")
		gctx.Redirect(http.StatusFound, r.cfg.GetRoot().ErrorPages.Fallback)
		return
	}

	stateId, err := uuid.Parse(req.StateId)
	if err != nil {
		api_common.AddDebugHeaderError(r.cfg, gctx, errors.Wrap(err, "failed to parse state_id"))
		gctx.Redirect(http.StatusFound, r.cfg.GetRoot().ErrorPages.Fallback)
	}

	o2, err := oauth2.GetOAuth2State(ctx, r.cfg, r.db, r.redis, *actor, stateId)
	if err != nil {
		api_common.AddDebugHeaderError(r.cfg, gctx, errors.Wrap(err, "failed to get oauth2 state"))
		gctx.Redirect(http.StatusFound, r.cfg.GetRoot().ErrorPages.Fallback)
		return
	}

	redirectUrl, err := o2.GenerateAuthUrl(ctx, *actor)
	if err != nil {
		api_common.AddDebugHeaderError(r.cfg, gctx, errors.Wrap(err, "failed to generate oauth2 redirect url"))
		gctx.Redirect(http.StatusFound, r.cfg.GetRoot().ErrorPages.Fallback)
		return
	}

	// Redirect the user to the generated OAuth2 URL
	gctx.Redirect(http.StatusFound, redirectUrl)
}

func (r *Oauth2Routes) Register(g *gin.Engine) {
	g.GET("/oauth2/callback", r.authService.Required(), r.callback)
	g.GET("/oauth2/redirect", r.authService.Optional(), r.redirect) // Auth here is optional so we can handle nice redirects for uauthed requests
}

func NewOauth2Routes(
	cfg config.C,
	authService auth.A,
	db database.DB,
	redis *redis.Wrapper,
) *Oauth2Routes {
	return &Oauth2Routes{
		cfg:         cfg,
		authService: authService,
		db:          db,
		redis:       redis,
	}
}
