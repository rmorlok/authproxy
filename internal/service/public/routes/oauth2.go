package routes

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/pkg/errors"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

type Oauth2Routes struct {
	cfg         config.C
	authService auth.A
	db          database.DB
	r           apredis.Client
	httpf       httpf.F
	encrypt     encrypt.E
	oauthf      oauth2.Factory
	logger      *slog.Logger
}

func (r *Oauth2Routes) callback(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	logger := r.logger.With("method", "callback")

	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		logger.Warn("callback called without auth")
		api_common.AddGinDebugHeader(gctx, "auth not present on context")
		r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error:       sconfig.ErrorPageUnauthorized,
			Description: "Request is not part of an authenticated session.",
		})
		return
	}

	if gctx.Query("state") == "" {
		err := errors.New("failed to bind state param")
		logger.Error(err.Error(), "error", err)
		api_common.AddGinDebugHeaderError(gctx, err)
		gctx.Redirect(http.StatusFound, r.cfg.GetErrorPageUrl(sconfig.ErrorPageInternalError))
		r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		})
		return
	}

	stateUUID, err := apid.Parse(gctx.Query("state"))
	if err != nil {
		err = errors.Wrap(err, "failed to parse state param to UUID")
		logger.Error(err.Error(), "error", err)
		api_common.AddGinDebugHeaderError(gctx, err)
		r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		})
		return
	}

	oauthState, err := r.oauthf.GetOAuth2State(ctx, ra.MustGetActor(), stateUUID) // Get the OAuth2 state
	if err != nil {
		err = errors.Wrap(err, "failed to get oauth2 state")
		logger.Error(err.Error(), "error", err)
		api_common.AddGinDebugHeaderError(gctx, err)
		r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		})
		return
	}

	if oauthState.CancelSessionAfterAuth() {
		err = r.authService.EndGinSession(gctx, ra)
		if err != nil {
			err = errors.Wrap(err, "failed to end gin session")
			logger.Error(err.Error(), "error", err)
			api_common.AddGinDebugHeaderError(gctx, err)
			r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
				Error: sconfig.ErrorPageInternalError,
			})
			return
		}
	}

	redirectUrl, err := oauthState.CallbackFrom3rdParty(ctx, gctx.Request.URL.Query())
	if err != nil {
		err = errors.Wrap(err, "failed to handle oauth2 callback")
		logger.Error(err.Error(), "error", err)
		api_common.AddGinDebugHeaderError(gctx, err)
		r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		})
		return
	}

	gctx.Redirect(http.StatusFound, redirectUrl)
}

type RedirectParams struct {
	StateId string `form:"state_id"`
}

func (r *Oauth2Routes) redirect(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	logger := r.logger.With("method", "redirect")

	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		logger.Warn("redirect called without auth")
		api_common.AddGinDebugHeader(gctx, "auth not present on context")
		r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		})
		return
	}

	// If we are not in a session, we create one, but cancel it after the oauth flow completes
	shouldCancelSession := false
	if !ra.IsSession() {
		shouldCancelSession = true
		err := r.authService.EstablishGinSession(gctx, ra)
		if err != nil {
			err = errors.Wrap(err, "failed to establish gin session")
			logger.Error(err.Error(), "error", err)
			api_common.AddGinDebugHeaderError(gctx, err)
			r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
				Error: sconfig.ErrorPageInternalError,
			})
			return
		}
	}

	var req RedirectParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		err = errors.Wrap(err, "failed to bind redirect params")
		logger.Error(err.Error(), "error", err)
		api_common.AddGinDebugHeaderError(gctx, err)
		r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		})
		return
	}

	if req.StateId == "" {
		logger.Error("state_id is required")
		api_common.AddGinDebugHeader(gctx, "state_id is required")
		r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		})
		return
	}

	stateId, err := apid.Parse(req.StateId)
	if err != nil {
		err = errors.Wrap(err, "failed to parse state_id")
		logger.Error(err.Error(), "error", err)
		api_common.AddGinDebugHeaderError(gctx, err)
		r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		})
		return
	}

	o2, err := r.oauthf.GetOAuth2State(ctx, ra.MustGetActor(), stateId)
	if err != nil {
		err = errors.Wrap(err, "failed to get oauth2 state")
		logger.Error(err.Error(), "error", err)
		api_common.AddGinDebugHeaderError(gctx, err)
		r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		})
		return
	}

	err = o2.RecordCancelSessionAfterAuth(ctx, shouldCancelSession)
	if err != nil {
		err = errors.Wrap(err, "failed to record cancel session after auth")
		logger.Error(err.Error(), "error", err)
		api_common.AddGinDebugHeaderError(gctx, err)
		r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		})
		return
	}

	redirectUrl, err := o2.GenerateAuthUrl(ctx, ra.MustGetActor())
	if err != nil {
		err = errors.Wrap(err, "failed to generate oauth2 redirect url")
		logger.Error(err.Error(), "error", err)
		api_common.AddGinDebugHeaderError(gctx, err)
		r.cfg.GetRoot().ErrorPages.RenderRenderOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		})
		return
	}

	// Redirect the user to the generated OAuth2 URL
	gctx.Redirect(http.StatusFound, redirectUrl)
}

func (r *Oauth2Routes) Register(g *gin.Engine) {
	g.GET("/oauth2/callback", r.authService.Required(), r.callback)
	g.GET("/oauth2/redirect", r.authService.Optional(), r.redirect) // Auth here is optional so we can handle nice redirects for unauthed requests
}

func NewOauth2Routes(
	cfg config.C,
	authService auth.A,
	db database.DB,
	r apredis.Client,
	c iface.C,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) *Oauth2Routes {
	return &Oauth2Routes{
		cfg:         cfg,
		authService: authService,
		db:          db,
		r:           r,
		httpf:       httpf,
		encrypt:     encrypt,
		oauthf:      oauth2.NewFactory(cfg, db, r, c, httpf, encrypt, logger),
		logger:      logger,
	}
}
