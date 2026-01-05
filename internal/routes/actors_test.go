package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	coreAuth "github.com/rmorlok/authproxy/internal/apauth/core"
	authService "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
)

func TestActorsRoutes(t *testing.T) {
	type TestSetup struct {
		Gin      *gin.Engine
		Cfg      config.C
		AuthUtil *authService.AuthTestUtil
		Db       database.DB
	}

	setup := func(t *testing.T, cfg config.C) (*TestSetup, func()) {
		if cfg == nil {
			cfg = config.FromRoot(&config.Root{})
		}

		// Real DB for actors to simplify pagination/cursor behavior
		cfg, db := database.MustApplyBlankTestDbConfig(t.Name(), cfg)
		// Real redis config (in-memory test) for httpf factory
		cfg, rds := apredis.MustApplyTestConfig(cfg)
		// Auth service bound to this DB
		cfg, auth, authUtil := authService.TestAuthServiceWithDb(config.ServiceIdApi, cfg, db)
		// Test encrypt service and http factory
		cfg, e := encrypt.NewTestEncryptService(cfg, db)
		h := httpf.CreateFactory(cfg, rds, test_utils.NewTestLogger())

		// Build routes
		ar := NewActorsRoutes(cfg, auth, db, rds, h, e, test_utils.NewTestLogger())
		r := gin.Default()
		ar.Register(r)

		// gomock controller (only for redis mock if we needed, but kept for parity)
		ctrl := gomock.NewController(t)
		_ = mock.NewMockClient(ctrl) // not used; ensure gomock finalized

		return &TestSetup{
			Gin:      r,
			Cfg:      cfg,
			AuthUtil: authUtil,
			Db:       db,
		}, func() { ctrl.Finish() }
	}

	// Helper to create an actor in DB
	createActor := func(t *testing.T, db database.DB, externalId, email string, admin, superAdmin bool) *database.Actor {
		a := &database.Actor{
			Id:         uuid.New(),
			ExternalId: externalId,
			Email:      email,
			Admin:      admin,
			SuperAdmin: superAdmin,
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		require.NoError(t, db.CreateActor(context.Background(), a))
		return a
	}

	// Build an admin-authenticated request from a base request
	adminize := func(t *testing.T, tu *TestSetup, req *http.Request) *http.Request {
		var err error
		req, err = tu.AuthUtil.SignRequestHeaderAs(context.Background(), req, coreAuth.Actor{ExternalId: "admin/test", Admin: true})
		require.NoError(t, err)
		return req
	}

	t.Run("list", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("non-empty (admin upserted)", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors", nil)
			require.NoError(t, err)
			req = adminize(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListActorsResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.GreaterOrEqual(t, len(resp.Items), 1)
		})

		t.Run("with results and pagination", func(t *testing.T) {
			// create 3 normal actors
			a1 := createActor(t, tu.Db, "user/1", "u1@example.com", false, false)
			a2 := createActor(t, tu.Db, "user/2", "u2@example.com", false, false)
			a3 := createActor(t, tu.Db, "user/3", "u3@example.com", false, false)
			_ = a1
			_ = a2
			_ = a3

			// page 1 with limit=2
			w1 := httptest.NewRecorder()
			req1, err := http.NewRequest(http.MethodGet, "/actors?limit=2", nil)
			require.NoError(t, err)
			req1 = adminize(t, tu, req1)

			tu.Gin.ServeHTTP(w1, req1)
			require.Equal(t, http.StatusOK, w1.Code)
			var resp1 ListActorsResponseJson
			require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &resp1))
			require.Len(t, resp1.Items, 2)
			require.NotEmpty(t, resp1.Cursor)

			// page 2 using cursor
			w2 := httptest.NewRecorder()
			req2, err := http.NewRequest(http.MethodGet, "/actors?cursor="+url.QueryEscape(resp1.Cursor), nil)
			require.NoError(t, err)
			req2 = adminize(t, tu, req2)

			tu.Gin.ServeHTTP(w2, req2)
			require.Equal(t, http.StatusOK, w2.Code)
			var resp2 ListActorsResponseJson
			require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
			require.GreaterOrEqual(t, len(resp2.Items), 1)
			require.Equal(t, "", resp2.Cursor)
		})

		t.Run("invalid order_by field", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors?order_by=not_a_field%20asc", nil)
			require.NoError(t, err)
			req = adminize(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	})

	t.Run("get by id", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		a := createActor(t, tu.Db, "user/10", "u10@example.com", false, false)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+a.Id.String(), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/not-a-uuid", nil)
			require.NoError(t, err)
			req = adminize(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+uuid.New().String(), nil)
			require.NoError(t, err)
			req = adminize(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+a.Id.String(), nil)
			require.NoError(t, err)
			req = adminize(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ActorJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, a.Id, resp.Id)
			require.Equal(t, a.ExternalId, resp.ExternalId)
		})
	})

	t.Run("get by external id", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		a := createActor(t, tu.Db, "user-20", "u20@example.com", false, false)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/external-id/"+a.ExternalId, nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/external-id/does-not-exist", nil)
			require.NoError(t, err)
			req = adminize(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/external-id/"+a.ExternalId, nil)
			require.NoError(t, err)
			req = adminize(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ActorJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, a.Id, resp.Id)
			require.Equal(t, a.ExternalId, resp.ExternalId)
		})
	})

	t.Run("delete by id", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		a := createActor(t, tu.Db, "user/30", "u30@example.com", false, false)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/"+a.Id.String(), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/not-a-uuid", nil)
			require.NoError(t, err)
			req = adminize(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found returns 204", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/"+uuid.New().String(), nil)
			require.NoError(t, err)
			req = adminize(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/"+a.Id.String(), nil)
			require.NoError(t, err)
			req = adminize(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			got, err := tu.Db.GetActor(context.Background(), a.Id)
			require.ErrorIs(t, err, database.ErrNotFound)
			require.Nil(t, got)
		})
	})

	t.Run("delete by external id", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		a := createActor(t, tu.Db, "user-40", "u40@example.com", false, false)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/external-id/"+a.ExternalId, nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not found returns 204", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/external-id/does-not-exist", nil)
			require.NoError(t, err)
			req = adminize(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/external-id/"+a.ExternalId, nil)
			require.NoError(t, err)
			req = adminize(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			got, err := tu.Db.GetActorByExternalId(context.Background(), a.ExternalId)
			require.ErrorIs(t, err, database.ErrNotFound)
			require.Nil(t, got)
		})
	})
}
