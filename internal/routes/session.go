package routes

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/auth"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
)

// SessionInitiateUrlGenerator is any object that can generate the URLs to redirect the
// user to for initiating a session.
type SessionInitiateUrlGenerator interface {
	// GetInitiateSessionUrl returns the URL to redirect the user to for initiating a session.
	// The returnToUrl is the URL the user should be redirected to after the session is established. The return
	// value is the fully encoded URL that should be used.
	GetInitiateSessionUrl(returnToUrl string) string
}

type SessionRoutes struct {
	cfg                         config.C
	sessionInitiateUrlGenerator SessionInitiateUrlGenerator
	authService                 auth.A
	db                          database.DB
	r                           apredis.Client
	httpf                       httpf.F
	encrypt                     encrypt.E
	oauthf                      oauth2.Factory
	logger                      *slog.Logger
}

type InitiateParams struct {
	ReturnToUrl string `json:"return_to_url"`
}

type InitiateFailureResponse struct {
	RedirectUrl string `json:"redirect_url"`
}

type InitiateSuccessResponse struct {
	// This should include any configuration the SPA needs
	ActorId uuid.UUID `json:"actor_id"`
}

// initiate is called when the marketplace portal loads to attempt to establish a session with the server. The session
// might already exist, or the app might have been provided with a nonce JWT to exchange for a session, which would
// have been provided as the normal auth header.
//
// If we are successful setting up a session, return any configuration information needed by the marketplace. If session
// is not successful, return a 403 error code, but have a custom response that includes a redirect URL where the
// SPA can redirect to get authenticated. This URL will redirect to the host application to get a nonce JWT, and return
// to the specified URL with a `auth_token` URL parameter, which will be used by the SPA to call this endpoint again.
func (r *SessionRoutes) initiate(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	logger := aplog.NewBuilder(r.logger).
		WithCtx(ctx).
		Build()

	logger.Debug("received initiate request")

	var req InitiateParams
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		logger.Debug("request was not authenticated, returning redirect url")
		api_common.AddGinDebugHeader(r.cfg, gctx, "auth not present on context")
		gctx.PureJSON(http.StatusUnauthorized, InitiateFailureResponse{
			RedirectUrl: r.sessionInitiateUrlGenerator.GetInitiateSessionUrl(req.ReturnToUrl),
		})
		return
	}

	logger.Debug("request was authenticated")

	if !ra.IsSession() {
		logger.Debug("existing request was not in a session, creating one")
		err := r.authService.EstablishGinSession(gctx, ra)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusInternalServerError().
				WithInternalErr(errors.Wrap(err, "failed to establish gin session")).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}
	}

	a := ra.MustGetActor()

	gctx.PureJSON(http.StatusOK, InitiateSuccessResponse{
		ActorId: a.Id,
	})
}

// terminate is called to explicitly terminate the gin session. This is called by the SPA in unload situations
// where it expects to be navigating away from the SPA so that sessions are more quickly cleaned up.
func (r *SessionRoutes) terminate(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	logger := aplog.NewBuilder(r.logger).
		WithCtx(ctx).
		Build()

	logger.Debug("received terminate session request")

	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		logger.Debug("request was already unauthenticated; ignoring")
		api_common.AddGinDebugHeader(r.cfg, gctx, "auth not present on context")
		gctx.PureJSON(http.StatusOK, gin.H{})
		return
	}

	err := r.authService.EndGinSession(gctx, ra)
	if err != nil {
		logger.Error("failed to end gin session", "error", err)
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(errors.Wrap(err, "failed to end gin session")).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	logger.Debug("successfully terminated session")
	gctx.PureJSON(http.StatusOK, gin.H{})
}

func (r *SessionRoutes) Register(g gin.IRouter) {
	g.POST("/session/_initiate", r.authService.OptionalXsrfNotRequired(), r.initiate)
	g.POST("/session/_terminate", r.authService.Optional(), r.terminate)
}

func NewSessionRoutes(
	cfg config.C,
	sessionInitiateUrlGenerator SessionInitiateUrlGenerator,
	authService auth.A,
	db database.DB,
	r apredis.Client,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
) *SessionRoutes {
	return &SessionRoutes{
		cfg:                         cfg,
		sessionInitiateUrlGenerator: sessionInitiateUrlGenerator,
		authService:                 authService,
		db:                          db,
		r:                           r,
		httpf:                       httpf,
		encrypt:                     encrypt,
		logger:                      logger,
	}
}
