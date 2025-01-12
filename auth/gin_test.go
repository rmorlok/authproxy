package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	jwt2 "github.com/rmorlok/authproxy/jwt"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuth_Gin(t *testing.T) {
	now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
	ctx := context.Background().WithClock(clock.NewFakeClock(now))

	testClaims := func() *jwt2.AuthProxyClaims {
		return &jwt2.AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "random id",
				Issuer:    "remark42",
				Audience:  []string{string(config.ServiceIdApi)},
				ExpiresAt: nil,
				NotBefore: nil,
				IssuedAt:  &jwt.NumericDate{ctx.Clock().Now()},
				Subject:   "id1",
			},

			Actor: &jwt2.Actor{
				ID:    "id1",
				IP:    "127.0.0.1",
				Email: "me@example.com",
			},
		}
	}

	testAdminClaims := func() *jwt2.AuthProxyClaims {
		return &jwt2.AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "random id",
				Issuer:    "remark42",
				Audience:  []string{string(config.ServiceIdApi)},
				ExpiresAt: nil,
				NotBefore: nil,
				IssuedAt:  &jwt.NumericDate{ctx.Clock().Now()},
				Subject:   "admin/aid1",
			},

			Actor: &jwt2.Actor{
				ID:    "admin/aid1",
				IP:    "127.0.0.1",
				Email: "me@example.com",
				Admin: true,
			},
		}
	}

	type TestSetup struct {
		Gin      *gin.Engine
		Cfg      config.C
		AuthUtil *AuthTestUtil
	}

	setup := func(t *testing.T, authMethod func(A) gin.HandlerFunc) *TestSetup {
		cfg := config.FromRoot(&testConfigPublicPrivateKey)
		cfg, auth, authUtil := TestAuthService(t, config.ServiceIdApi, cfg)
		r := gin.Default()
		r.GET("/", authMethod(auth), func(c *gin.Context) {
			a := GetActorInfoFromGinContext(c)
			if a == nil {
				c.String(200, "no_actor")
				return
			}
			c.String(200, a.ID)
		})

		return &TestSetup{
			Gin:      r,
			Cfg:      cfg,
			AuthUtil: authUtil,
		}
	}

	t.Run("required", func(t *testing.T) {
		authFunc := func(a A) gin.HandlerFunc { return a.Required() }
		t.Run("valid", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testClaims()

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, c.Actor.ID, w.Body.String())
		})

		t.Run("valid with admin", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testAdminClaims()

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, c.Actor.ID, w.Body.String())
		})

		t.Run("expired", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testClaims()
			c.ExpiresAt = &jwt.NumericDate{now.Add(-time.Hour)}

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not present", func(t *testing.T) {
			ts := setup(t, authFunc)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad value", func(t *testing.T) {
			ts := setup(t, authFunc)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, "bad")
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("no actor in token", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testClaims()
			c.Actor = nil

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})
	})

	t.Run("optional", func(t *testing.T) {
		authFunc := func(a A) gin.HandlerFunc { return a.Optional() }
		t.Run("valid with auth", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testClaims()

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, c.Actor.ID, w.Body.String())
		})

		t.Run("valid with admin", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testAdminClaims()

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, c.Actor.ID, w.Body.String())
		})

		t.Run("valid without auth", func(t *testing.T) {
			ts := setup(t, authFunc)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, "no_actor", w.Body.String())
		})

		t.Run("expired", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testClaims()
			c.ExpiresAt = &jwt.NumericDate{now.Add(-time.Hour)}

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad value", func(t *testing.T) {
			ts := setup(t, authFunc)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, "bad")
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("no actor in token", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testClaims()
			c.Actor = nil

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})
	})

	t.Run("admin only", func(t *testing.T) {
		authFunc := func(a A) gin.HandlerFunc { return a.AdminOnly() }
		t.Run("valid", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testAdminClaims()

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, c.Actor.ID, w.Body.String())
		})

		t.Run("not valid admin", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testAdminClaims()
			c.Actor.ID = "admin/unknown"
			c.RegisteredClaims.Subject = "admin/unknown"

			tok, err := jwt2.NewJwtTokenBuilder().
				WithClaims(c).
				WithPrivateKeyPath("../test_data/system_keys/system").
				TokenCtx(testContext)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not admin", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testClaims()

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("expired", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testAdminClaims()
			c.ExpiresAt = &jwt.NumericDate{now.Add(-time.Hour)}

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad value", func(t *testing.T) {
			ts := setup(t, authFunc)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, "bad")
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("no actor in token", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testAdminClaims()
			c.Actor = nil

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not present", func(t *testing.T) {
			ts := setup(t, authFunc)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})
	})
}
