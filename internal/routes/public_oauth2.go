package routes

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

type PublicOauth2Routes struct {
	cfg         config.C
	authService auth.A
	db          database.DB
	r           apredis.Client
	httpf       httpf.F
	encrypt     encrypt.E
	oauthf      oauth2.Factory
	logger      *slog.Logger
}

func (r *PublicOauth2Routes) callback(gctx *gin.Context) {
	ctx := gctx.Request.Context()

	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error:       sconfig.ErrorPageUnauthorized,
			Description: "Request is not part of an authenticated session.",
		}, errors.New("auth not present on context"))
		return
	}

	if gctx.Query("state") == "" {
		r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		}, errors.New("failed to bind state param"))
		return
	}

	stateUUID, err := apid.Parse(gctx.Query("state"))
	if err != nil {
		r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		}, fmt.Errorf("failed to parse state param to UUID: %w", err))
		return
	}

	oauthState, err := r.oauthf.GetOAuth2State(ctx, ra.MustGetActor(), stateUUID) // Get the OAuth2 state
	if err != nil {
		r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		}, fmt.Errorf("failed to get oauth2 state: %w", err))
		return
	}

	if oauthState.CancelSessionAfterAuth() {
		err = r.authService.EndGinSession(gctx, ra)
		if err != nil {
			r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
				Error: sconfig.ErrorPageInternalError,
			}, fmt.Errorf("failed to end gin session: %w", err))
			return
		}
	}

	redirectUrl, err := oauthState.CallbackFrom3rdParty(ctx, gctx.Request.URL.Query())
	if err != nil {
		r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		}, fmt.Errorf("failed to handle oauth2 callback: %w", err))
		return
	}

	gctx.Redirect(http.StatusFound, redirectUrl)
}

type RedirectParams struct {
	StateId string `form:"state_id"`
}

func (r *PublicOauth2Routes) redirect(gctx *gin.Context) {
	ctx := gctx.Request.Context()

	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		}, errors.New("auth not present on context"))
		return
	}

	// If we are not in a session, we create one, but cancel it after the oauth flow completes
	shouldCancelSession := false
	if !ra.IsSession() {
		shouldCancelSession = true
		err := r.authService.EstablishGinSession(gctx, ra)
		if err != nil {
			r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
				Error: sconfig.ErrorPageInternalError,
			}, fmt.Errorf("failed to establish gin session: %w", err))
			return
		}
	}

	var req RedirectParams
	if err := gctx.ShouldBindQuery(&req); err != nil {
		r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		}, fmt.Errorf("failed to bind redirect params: %w", err))
		return
	}

	if req.StateId == "" {
		r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		}, errors.New("state_id is required"))
		return
	}

	stateId, err := apid.Parse(req.StateId)
	if err != nil {
		r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		}, fmt.Errorf("failed to parse state_id: %w", err))
		return
	}

	o2, err := r.oauthf.GetOAuth2State(ctx, ra.MustGetActor(), stateId)
	if err != nil {
		r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		}, fmt.Errorf("failed to get oauth2 state: %w", err))
		return
	}

	err = o2.RecordCancelSessionAfterAuth(ctx, shouldCancelSession)
	if err != nil {
		r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		}, fmt.Errorf("failed to record cancel session after auth: %w", err))
		return
	}

	redirectUrl, err := o2.GenerateAuthUrl(ctx, ra.MustGetActor())
	if err != nil {
		r.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
			Error: sconfig.ErrorPageInternalError,
		}, fmt.Errorf("failed to generate oauth2 redirect url: %w", err))
		return
	}

	// Redirect the user to the generated OAuth2 URL
	gctx.Redirect(http.StatusFound, redirectUrl)
}

func (r *PublicOauth2Routes) Register(g *gin.Engine) {
	g.GET("/oauth2/callback", r.authService.Required(), r.callback)
	g.GET("/oauth2/redirect", r.authService.Optional(), r.redirect) // Auth here is optional so we can handle nice redirects for unauthed requests
}

func NewPublicOauth2Routes(
	cfg config.C,
	authService auth.A,
	db database.DB,
	r apredis.Client,
	c iface.C,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) *PublicOauth2Routes {
	return &PublicOauth2Routes{
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
