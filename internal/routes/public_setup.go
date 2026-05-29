package routes

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/encrypt"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/setup_token"
)

// PublicSetupRoutes exposes the /public/connections/{id}/setup/advance and
// .../abort endpoints. The endpoints are unauthenticated to the AuthProxy
// session — the setup token (carried as ?token=...) is the authorization.
//
// Flow: a connector YAML declares a redirect step whose URL template
// includes {{RETURN_ADVANCE}} / {{RETURN_ABORT}}. At redirect-step render
// time core mints a token per placeholder, embedding the connection id +
// the current setup_step id + the original _initiate return-to URL. The
// 3rd party redirects the user back through one of these endpoints when
// they're done; we verify + consume the token, advance/abort the
// connection, and bounce the user to the marketplace return URL.
type PublicSetupRoutes struct {
	cfg     config.C
	core    iface.C
	r       apredis.Client
	encrypt encrypt.E
	logger  *slog.Logger
}

// NewPublicSetupRoutes constructs the public setup-transition routes. All
// dependencies come from the DependencyManager — no per-instance state.
func NewPublicSetupRoutes(
	cfg config.C,
	core iface.C,
	r apredis.Client,
	encrypt encrypt.E,
	logger *slog.Logger,
) *PublicSetupRoutes {
	return &PublicSetupRoutes{
		cfg:     cfg,
		core:    core,
		r:       r,
		encrypt: encrypt,
		logger:  logger,
	}
}

// Register binds the two endpoints to the gin engine. Both accept GET
// (for browser navigation from a 3rd-party redirect) and POST (for
// programmatic callers). The handler is the same for either verb.
func (h *PublicSetupRoutes) Register(g *gin.Engine) {
	g.GET("/public/connections/:id/setup/advance", h.advance)
	g.POST("/public/connections/:id/setup/advance", h.advance)
	g.GET("/public/connections/:id/setup/abort", h.abort)
	g.POST("/public/connections/:id/setup/abort", h.abort)
}

func (h *PublicSetupRoutes) advance(gctx *gin.Context) {
	h.handle(gctx, setup_token.IntentAdvance, h.doAdvance)
}

func (h *PublicSetupRoutes) abort(gctx *gin.Context) {
	h.handle(gctx, setup_token.IntentAbort, h.doAbort)
}

// handle is the shared token-validation + dispatch wrapper. The per-intent
// callback runs only after the token is verified, consumed, and bound to
// the request's connection id.
func (h *PublicSetupRoutes) handle(
	gctx *gin.Context,
	endpointIntent setup_token.Intent,
	action func(ctx *gin.Context, connectionId apid.ID, claims setup_token.Claims) error,
) {
	ctx := gctx.Request.Context()

	connectionId, err := apid.Parse(gctx.Param("id"))
	if err != nil || connectionId == apid.Nil {
		h.renderError(gctx, fmt.Errorf("invalid connection id: %w", err))
		return
	}

	token := gctx.Query("token")
	if token == "" {
		// POST fallback: accept token in the form body.
		token = gctx.PostForm("token")
	}
	if token == "" {
		h.renderError(gctx, errors.New("missing setup token"))
		return
	}

	claims, err := setup_token.VerifyAndConsume(ctx, h.r, h.encrypt, token)
	if err != nil {
		// Both ErrNotFound (forged / expired / replayed) and ErrTampered
		// (modified payload) surface as a generic error page — security
		// events were already emitted by VerifyAndConsume's caller chain.
		h.logger.WarnContext(ctx, "setup token rejected",
			"connection_id", connectionId,
			"intent", endpointIntent,
			"error", err)
		h.renderError(gctx, err)
		return
	}

	// Intent gate: a token minted for advance can't be replayed at /abort
	// and vice-versa, even though both endpoints validate the same token.
	if claims.Intent != endpointIntent {
		h.logger.WarnContext(ctx, "setup token intent mismatch",
			"connection_id", connectionId,
			"endpoint_intent", endpointIntent,
			"token_intent", claims.Intent)
		h.renderError(gctx, fmt.Errorf("token intent %q does not match endpoint %q", claims.Intent, endpointIntent))
		return
	}

	// Connection-id binding: a token minted for connection A can't be used
	// against connection B.
	if claims.ConnectionId != connectionId {
		h.logger.WarnContext(ctx, "setup token connection mismatch",
			"path_connection_id", connectionId,
			"token_connection_id", claims.ConnectionId)
		h.renderError(gctx, errors.New("token does not authorize this connection"))
		return
	}

	if err := action(gctx, connectionId, claims); err != nil {
		h.logger.ErrorContext(ctx, "setup token action failed",
			"connection_id", connectionId,
			"intent", endpointIntent,
			"error", err)
		h.renderError(gctx, err)
		return
	}
}

// doAdvance transitions the connection past its current redirect step.
// Reuses HandleCredentialsEstablished (the same path the OAuth2 callback
// takes after token exchange) — it consults the manifest to compute the
// next step (verify / configure / done) from where the connection currently
// sits.
func (h *PublicSetupRoutes) doAdvance(gctx *gin.Context, connectionId apid.ID, claims setup_token.Claims) error {
	ctx := gctx.Request.Context()

	conn, err := h.core.GetConnection(ctx, connectionId)
	if err != nil {
		return fmt.Errorf("failed to load connection: %w", err)
	}
	// Stale-token defense: the connection's setup_step must still be the
	// step the token was minted from. If the user already advanced (e.g.
	// abandoned the 3rd-party flow and resumed via reauth), the token's
	// step is no longer current and we reject.
	current := conn.GetSetupStep()
	if current == nil || current.Id() != claims.StepId {
		return fmt.Errorf("connection setup step has moved since token was issued")
	}

	outcome, err := conn.HandleCredentialsEstablished(ctx)
	if err != nil {
		return fmt.Errorf("failed to advance connection: %w", err)
	}

	gctx.Redirect(http.StatusFound, h.computeReturnUrl(claims.ReturnToUrl, connectionId, outcome.SetupPending))
	return nil
}

// doAbort cancels the in-flight setup via the standard AbortConnection
// path — deletes any acquired credentials and soft-deletes the connection.
// Idempotent against already-aborted connections (AbortConnection's
// internal checks surface as a 4xx-shaped error which we redact to the
// error page).
func (h *PublicSetupRoutes) doAbort(gctx *gin.Context, connectionId apid.ID, claims setup_token.Claims) error {
	ctx := gctx.Request.Context()

	if err := h.core.AbortConnection(ctx, connectionId); err != nil {
		return fmt.Errorf("failed to abort connection: %w", err)
	}

	gctx.Redirect(http.StatusFound, h.computeReturnUrl(claims.ReturnToUrl, connectionId, false))
	return nil
}

// computeReturnUrl assembles the final redirect URL. If the claims carry a
// marketplace return URL, augment it with connection_id and setup=pending
// when more setup steps remain. Falls back to the internal error page URL
// when no return URL is configured — better than a blank redirect.
func (h *PublicSetupRoutes) computeReturnUrl(returnTo string, connectionId apid.ID, setupPending bool) string {
	if returnTo == "" {
		return h.cfg.GetRoot().ErrorPages.InternalError
	}
	u, err := url.Parse(returnTo)
	if err != nil {
		return returnTo
	}
	q := u.Query()
	q.Set("connection_id", connectionId.String())
	if setupPending {
		q.Set("setup", "pending")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// renderError surfaces a security or system fault on the configured error
// page (or as a 400 JSON for non-browser callers). The matching log line
// was already emitted by the caller.
func (h *PublicSetupRoutes) renderError(gctx *gin.Context, err error) {
	h.cfg.GetRoot().ErrorPages.RenderErrorOrRedirect(gctx, sconfig.ErrorTemplateValues{
		Error: sconfig.ErrorPageInternalError,
	}, err)
}
