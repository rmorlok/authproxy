package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRedirectGenerator is a test AuthRedirectUrlGenerator that builds a URL with the returnToUrl
// embedded as a query parameter so tests can assert the expected return target.
type stubRedirectGenerator struct {
	loginUrl string
}

func (s *stubRedirectGenerator) GetInitiateSessionUrl(returnToUrl string) string {
	u, _ := url.Parse(s.loginUrl)
	q := u.Query()
	q.Set("return_to", returnToUrl)
	u.RawQuery = q.Encode()
	return u.String()
}

// authRedirectTestSetup wires a gin engine, a real auth service (service id Api), and helpers
// to build authenticated requests — reused across the redirect tests.
type authRedirectTestSetup struct {
	Gin      *gin.Engine
	Cfg      config.C
	Auth     A
	AuthUtil *AuthTestUtil
	Gen      *stubRedirectGenerator
}

func newAuthRedirectTestSetup(t *testing.T, register func(g *gin.Engine, a A, gen AuthRedirectUrlGenerator)) *authRedirectTestSetup {
	cfg, db := database.MustApplyBlankTestDbConfig(t, nil)
	// GetBaseUrl — used to build the return_to URL on redirect — panics without a port.
	cfg.GetRoot().Api.PortVal = common.NewIntegerValueDirect(8081)
	cfg, auth, authUtil := TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)

	gen := &stubRedirectGenerator{loginUrl: "https://login.example.com/login"}

	r := gin.New()
	register(r, auth, gen)

	return &authRedirectTestSetup{
		Gin:      r,
		Cfg:      cfg,
		Auth:     auth,
		AuthUtil: authUtil,
		Gen:      gen,
	}
}

func okHandler(gctx *gin.Context) {
	gctx.PureJSON(http.StatusOK, gin.H{"ok": true})
}

func TestRequiredWithAuthRedirect(t *testing.T) {
	t.Setenv("AUTHPROXY_DEBUG_MODE", "true")
	ctx := context.Background()

	register := func(g *gin.Engine, a A, gen AuthRedirectUrlGenerator) {
		g.GET("/ping", a.RequiredWithAuthRedirect(gen), okHandler)
	}

	t.Run("unauthenticated redirects to login with return_to", func(t *testing.T) {
		tu := newAuthRedirectTestSetup(t, register)

		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/ping?foo=bar", nil)
		require.NoError(t, err)
		tu.Gin.ServeHTTP(w, req)

		require.Equal(t, http.StatusFound, w.Code)

		loc, err := url.Parse(w.Header().Get("Location"))
		require.NoError(t, err)
		assert.Equal(t, "login.example.com", loc.Host)
		assert.Equal(t, "/login", loc.Path)

		returnTo := loc.Query().Get("return_to")
		require.NotEmpty(t, returnTo, "Location should include return_to query param")
		assert.Contains(t, returnTo, "/ping")
		assert.Contains(t, returnTo, "foo=bar")
	})

	t.Run("unauthenticated with valid auth_token query param proceeds", func(t *testing.T) {
		tu := newAuthRedirectTestSetup(t, register)

		// The user arrives back at the endpoint after login with an auth_token query parameter
		// issued by the host application. The middleware should authenticate via the token and
		// let the handler run.
		tokenString, err := tu.AuthUtil.GenerateBearerToken(
			ctx, "some-actor", "root", aschema.AllPermissions(),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/ping?foo=bar&auth_token="+url.QueryEscape(tokenString), nil)
		require.NoError(t, err)
		tu.Gin.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Header().Get("x-authproxy-debug"))
	})

	t.Run("authenticated via header proceeds", func(t *testing.T) {
		tu := newAuthRedirectTestSetup(t, register)

		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodGet, "/ping", nil, "root", "some-actor", aschema.AllPermissions(),
		)
		require.NoError(t, err)
		tu.Gin.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Header().Get("x-authproxy-debug"))
	})

	t.Run("invalid JWT does not redirect", func(t *testing.T) {
		// A JWT with the wrong audience is a parse/validation error, not a "session lapsed" case.
		// The middleware should surface the error (401) rather than silently redirect — we don't
		// want bad creds to bounce through the login flow.
		tu := newAuthRedirectTestSetup(t, register)

		s := jwt.NewJwtTokenBuilder().
			WithActorExternalId("jimmycarter").
			WithAudience("invalid").
			MustWithConfigKey(ctx, tu.Cfg.GetRoot().SystemAuth.JwtSigningKey).
			MustSignerCtx(ctx)

		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/ping", nil)
		require.NoError(t, err)
		s.SignAuthHeader(req)
		tu.Gin.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code, w.Header().Get("x-authproxy-debug"))
	})

	t.Run("authenticated but validator fails returns 403", func(t *testing.T) {
		// RequiredWithAuthRedirect still honors validators and returns 403 when an authenticated
		// actor doesn't pass them — only unauthenticated triggers the redirect.
		rejectAll := func(gctx *gin.Context, ra *core.RequestAuth) (bool, string) {
			return false, "nope"
		}
		tu := newAuthRedirectTestSetup(t, func(g *gin.Engine, a A, gen AuthRedirectUrlGenerator) {
			g.GET("/ping", a.RequiredWithAuthRedirect(gen, rejectAll), okHandler)
		})

		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodGet, "/ping", nil, "root", "some-actor", aschema.AllPermissions(),
		)
		require.NoError(t, err)
		tu.Gin.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code, w.Header().Get("x-authproxy-debug"))
	})
}

func TestPermissionValidatorBuilder_WithRedirectOnUnauthenticated(t *testing.T) {
	t.Setenv("AUTHPROXY_DEBUG_MODE", "true")
	ctx := context.Background()

	// A permission-gated route that also redirects on unauthenticated — the full OAuth2 routes
	// shape. Includes MarkValidated so the post-validator doesn't panic (the handler confirms
	// that the middleware-level permission check is sufficient for this endpoint).
	handler := func(gctx *gin.Context) {
		MustGetValidatorFromGinContext(gctx).MarkValidated()
		gctx.PureJSON(http.StatusOK, gin.H{"ok": true})
	}

	register := func(g *gin.Engine, a A, gen AuthRedirectUrlGenerator) {
		mw := a.NewRequiredBuilder().
			ForResource("connections").
			ForVerb("create").
			WithRedirectOnUnauthenticated(gen).
			Build()
		g.GET("/oauth2/callback", mw, handler)
	}

	t.Run("unauthenticated redirects to login", func(t *testing.T) {
		tu := newAuthRedirectTestSetup(t, register)

		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/oauth2/callback?state=abc", nil)
		require.NoError(t, err)
		tu.Gin.ServeHTTP(w, req)

		require.Equal(t, http.StatusFound, w.Code)

		loc, err := url.Parse(w.Header().Get("Location"))
		require.NoError(t, err)
		returnTo := loc.Query().Get("return_to")
		require.NotEmpty(t, returnTo)
		assert.Contains(t, returnTo, "/oauth2/callback")
		assert.Contains(t, returnTo, "state=abc")
	})

	t.Run("authenticated with permission proceeds", func(t *testing.T) {
		tu := newAuthRedirectTestSetup(t, register)

		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodGet,
			"/oauth2/callback?state=abc",
			nil,
			"root",
			"some-actor",
			aschema.PermissionsSingle("root.**", "connections", "create"),
		)
		require.NoError(t, err)
		tu.Gin.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Header().Get("x-authproxy-debug"))
	})

	t.Run("authenticated without required permission returns 403", func(t *testing.T) {
		tu := newAuthRedirectTestSetup(t, register)

		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodGet,
			"/oauth2/callback?state=abc",
			nil,
			"root",
			"some-actor",
			aschema.PermissionsSingle("root.**", "connections", "list"), // wrong verb
		)
		require.NoError(t, err)
		tu.Gin.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code, w.Header().Get("x-authproxy-debug"))
	})

	t.Run("unauthenticated with valid auth_token authenticates and proceeds", func(t *testing.T) {
		tu := newAuthRedirectTestSetup(t, register)

		// Simulates the second hit of the flow: user was redirected to login, came back with a
		// freshly minted auth_token query param carrying the right permissions.
		tokenString, err := tu.AuthUtil.GenerateBearerToken(
			ctx, "some-actor", "root",
			aschema.PermissionsSingle("root.**", "connections", "create"),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/oauth2/callback?state=abc&auth_token="+url.QueryEscape(tokenString), nil)
		require.NoError(t, err)
		tu.Gin.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Header().Get("x-authproxy-debug"))
	})
}
