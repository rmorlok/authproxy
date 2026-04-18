package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	asynqmock "github.com/rmorlok/authproxy/internal/apasynq/mock"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	rmock "github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSessionInitiateUrlGenerator is the test double for SessionInitiateUrlGenerator; embeds
// the returnToUrl in the Location query so the redirect assertion can verify the original
// URL round-trips through the login flow.
type stubSessionInitiateUrlGenerator struct {
	loginUrl string
}

func (s *stubSessionInitiateUrlGenerator) GetInitiateSessionUrl(returnTo string) string {
	u, _ := url.Parse(s.loginUrl)
	q := u.Query()
	q.Set("return_to", returnTo)
	u.RawQuery = q.Encode()
	return u.String()
}

// setupPublicOauth2Test wires PublicOauth2Routes against real (but in-memory) deps.
// For the scenarios exercised here — unauthenticated redirect, missing permission, and
// invalid state params — no OAuth2 factory interaction is required, so the factory is
// constructed normally.
func setupPublicOauth2Test(t *testing.T) (*gin.Engine, *auth2.AuthTestUtil, *stubSessionInitiateUrlGenerator, func()) {
	cfg := config.FromRoot(&sconfig.Root{
		Connectors: &sconfig.Connectors{
			LoadFromList: []sconfig.Connector{},
		},
	})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	// GetBaseUrl is called to build return_to on unauthenticated redirects; requires a port.
	cfg.GetRoot().Public.PortVal = common.NewIntegerValueDirect(8080)
	cfg, rds := apredis.MustApplyTestConfig(cfg)
	cfg, auth, authUtil := auth2.TestAuthServiceWithDb(sconfig.ServiceIdPublic, cfg, db)
	h := httpf2.CreateFactory(cfg, rds, nil, aplog.NewNoopLogger())
	cfg, e := encrypt.NewTestEncryptService(cfg, db)

	ctrl := gomock.NewController(t)
	ac := asynqmock.NewMockClient(ctrl)
	rs := rmock.NewMockClient(ctrl)
	c := core.NewCoreService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
	require.NoError(t, c.Migrate(context.Background()))

	gen := &stubSessionInitiateUrlGenerator{loginUrl: "https://login.example.com/login"}
	routes := NewPublicOauth2Routes(cfg, auth, gen, db, rds, c, h, e, test_utils.NewTestLogger())

	r := apgin.ForTest(nil)
	r.Use(apgin.DebugModeMiddleware(true))
	routes.Register(r)

	return r, authUtil, gen, func() { ctrl.Finish() }
}

func TestPublicOauth2Routes_Redirect(t *testing.T) {
	t.Setenv("AUTHPROXY_DEBUG_MODE", "true")
	ctx := context.Background()

	t.Run("unauthenticated redirects to login with return_to", func(t *testing.T) {
		r, _, _, done := setupPublicOauth2Test(t)
		defer done()

		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/oauth2/redirect?state_id=abc", nil)
		require.NoError(t, err)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusFound, w.Code)
		loc, err := url.Parse(w.Header().Get("Location"))
		require.NoError(t, err)
		assert.Equal(t, "login.example.com", loc.Host)

		returnTo := loc.Query().Get("return_to")
		require.NotEmpty(t, returnTo, "Location should include return_to")
		assert.Contains(t, returnTo, "/oauth2/redirect")
		assert.Contains(t, returnTo, "state_id=abc")
	})

	t.Run("authenticated without permission returns 403", func(t *testing.T) {
		r, authUtil, _, done := setupPublicOauth2Test(t)
		defer done()

		w := httptest.NewRecorder()
		req, err := authUtil.NewSignedRequestForActorExternalId(
			http.MethodGet,
			"/oauth2/redirect?state_id=abc",
			nil,
			"root",
			"some-actor",
			aschema.PermissionsSingle("root.**", "connections", "list"), // wrong verb
		)
		require.NoError(t, err)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code, w.Header().Get("x-authproxy-debug"))
	})

	t.Run("authenticated with permission but malformed state_id reaches handler", func(t *testing.T) {
		r, authUtil, _, done := setupPublicOauth2Test(t)
		defer done()

		w := httptest.NewRecorder()
		req, err := authUtil.NewSignedRequestForActorExternalId(
			http.MethodGet,
			"/oauth2/redirect?state_id=not-a-uuid",
			nil,
			"root",
			"some-actor",
			aschema.PermissionsSingle("root.**", "connections", "create"),
		)
		require.NoError(t, err)
		r.ServeHTTP(w, req)

		// Not 302 (no redirect), not 403 (permission accepted). Debug header is set by the error
		// page renderer when the handler hits the parse failure, proving the middleware allowed
		// the request to reach the handler.
		require.NotEqual(t, http.StatusFound, w.Code)
		require.NotEqual(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Header().Get("x-authproxy-debug"), "failed to parse state_id")
	})

	t.Run("unauthenticated with valid auth_token query param passes auth and reaches handler", func(t *testing.T) {
		r, authUtil, _, done := setupPublicOauth2Test(t)
		defer done()

		tokenString, err := authUtil.GenerateBearerToken(
			ctx, "some-actor", "root",
			aschema.PermissionsSingle("root.**", "connections", "create"),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		// state_id omitted so the handler hits the "state_id is required" branch — the middleware
		// must still let the request through because the auth_token authenticates the user.
		req, err := http.NewRequest(
			http.MethodGet,
			"/oauth2/redirect?auth_token="+url.QueryEscape(tokenString),
			nil,
		)
		require.NoError(t, err)
		r.ServeHTTP(w, req)

		require.NotEqual(t, http.StatusFound, w.Code)
		require.NotEqual(t, http.StatusForbidden, w.Code)
		require.NotEqual(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Header().Get("x-authproxy-debug"), "state_id is required")
	})
}

func TestPublicOauth2Routes_Callback(t *testing.T) {
	t.Setenv("AUTHPROXY_DEBUG_MODE", "true")

	t.Run("unauthenticated redirects to login with return_to", func(t *testing.T) {
		r, _, _, done := setupPublicOauth2Test(t)
		defer done()

		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/oauth2/callback?state=abc&code=xyz", nil)
		require.NoError(t, err)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusFound, w.Code)
		loc, err := url.Parse(w.Header().Get("Location"))
		require.NoError(t, err)
		assert.Equal(t, "login.example.com", loc.Host)

		returnTo := loc.Query().Get("return_to")
		require.NotEmpty(t, returnTo)
		assert.Contains(t, returnTo, "/oauth2/callback")
		assert.Contains(t, returnTo, "state=abc")
		assert.Contains(t, returnTo, "code=xyz")
	})

	t.Run("authenticated without permission returns 403", func(t *testing.T) {
		r, authUtil, _, done := setupPublicOauth2Test(t)
		defer done()

		w := httptest.NewRecorder()
		req, err := authUtil.NewSignedRequestForActorExternalId(
			http.MethodGet,
			"/oauth2/callback?state=abc",
			nil,
			"root",
			"some-actor",
			aschema.PermissionsSingle("root.**", "connections", "list"),
		)
		require.NoError(t, err)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code, w.Header().Get("x-authproxy-debug"))
	})

	t.Run("authenticated with permission but missing state reaches handler", func(t *testing.T) {
		r, authUtil, _, done := setupPublicOauth2Test(t)
		defer done()

		w := httptest.NewRecorder()
		req, err := authUtil.NewSignedRequestForActorExternalId(
			http.MethodGet,
			"/oauth2/callback", // no state
			nil,
			"root",
			"some-actor",
			aschema.PermissionsSingle("root.**", "connections", "create"),
		)
		require.NoError(t, err)
		r.ServeHTTP(w, req)

		require.NotEqual(t, http.StatusFound, w.Code)
		require.NotEqual(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Header().Get("x-authproxy-debug"), "failed to bind state param")
	})
}
