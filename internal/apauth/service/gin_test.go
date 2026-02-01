package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	jwt2 "github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/config"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestAuth_Gin(t *testing.T) {
	now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	testClaims := func() *jwt2.AuthProxyClaims {
		return &jwt2.AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "random id",
				Issuer:    "remark42",
				Audience:  []string{string(sconfig.ServiceIdApi)},
				ExpiresAt: nil,
				NotBefore: nil,
				IssuedAt:  &jwt.NumericDate{apctx.GetClock(ctx).Now()},
				Subject:   "id1",
			},

			Namespace: "root",
			Actor: &core.Actor{
				ExternalId: "id1",
				Namespace:  "root",
			},
		}
	}

	testAltClaims := func() *jwt2.AuthProxyClaims {
		return &jwt2.AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "random id",
				Issuer:    "remark42",
				Audience:  []string{string(sconfig.ServiceIdApi)},
				ExpiresAt: nil,
				NotBefore: nil,
				IssuedAt:  &jwt.NumericDate{apctx.GetClock(ctx).Now()},
				Subject:   "aid1",
			},
			Namespace: "root",
			Actor: &core.Actor{
				ExternalId: "aid1",
				Namespace:  "root",
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
		cfg, auth, authUtil := TestAuthService(t, sconfig.ServiceIdApi, cfg)
		r := gin.Default()
		r.GET("/", authMethod(auth), func(c *gin.Context) {
			a := GetAuthFromGinContext(c)
			if !a.IsAuthenticated() {
				c.String(200, "no_actor")
				return
			}
			c.String(200, a.GetActor().ExternalId)
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
			require.Equal(t, c.Actor.ExternalId, w.Body.String())
		})

		t.Run("valid with alt claims", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testAltClaims()

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, c.Actor.ExternalId, w.Body.String())
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
			require.Equal(t, c.Actor.ExternalId, w.Body.String())
		})

		t.Run("valid with alt claims", func(t *testing.T) {
			ts := setup(t, authFunc)
			c := testAltClaims()

			tok, err := ts.AuthUtil.s.Token(testContext, c)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
			SetJwtRequestHeader(req, tok)
			ts.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, c.Actor.ExternalId, w.Body.String())
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
}
