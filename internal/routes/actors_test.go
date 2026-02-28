package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/rmorlok/authproxy/internal/api_common"
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
	"github.com/rmorlok/authproxy/internal/apblob"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/rmorlok/authproxy/internal/util"
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
			cfg = config.FromRoot(&sconfig.Root{})
		}

		// Real DB for actors to simplify pagination/cursor behavior
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
		// Real redis config (in-memory test) for httpf factory
		cfg, rds := apredis.MustApplyTestConfig(cfg)
		// Auth service bound to this DB
		cfg, auth, authUtil := authService.TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)
		// Test encrypt service and http factory
		cfg, e := encrypt.NewTestEncryptService(cfg, db)
		h := httpf.CreateFactory(cfg, rds, apblob.NewMemoryClient(), test_utils.NewTestLogger())

		// Build routes
		ar := NewActorsRoutes(cfg, auth, db, rds, h, e, test_utils.NewTestLogger())
		r := api_common.GinForTest(nil)
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
	createActor := func(t *testing.T, db database.DB, externalId, namespace string) *database.Actor {
		require.NoError(t, db.EnsureNamespaceByPath(context.Background(), namespace))
		a := &database.Actor{
			Id:         uuid.New(),
			Namespace:  namespace,
			ExternalId: externalId,
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		require.NoError(t, db.CreateActor(context.Background(), a))
		return a
	}

	// Helper to create an actor with namespace in DB
	createActorWithNamespace := func(t *testing.T, db database.DB, externalId, namespace string) *database.Actor {
		db.EnsureNamespaceByPath(context.Background(), namespace)
		a := &database.Actor{
			Id:         uuid.New(),
			Namespace:  namespace,
			ExternalId: externalId,
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		require.NoError(t, db.CreateActor(context.Background(), a))
		return a
	}

	// Build an authenticated request from a base request
	authenticate := func(t *testing.T, tu *TestSetup, req *http.Request) *http.Request {
		var err error
		req, err = tu.AuthUtil.SignRequestHeaderAs(
			context.Background(),
			req,
			coreAuth.Actor{
				ExternalId:  "test-actor",
				Namespace:   "root",
				Permissions: aschema.AllPermissions(),
			},
		)
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

		t.Run("non-empty (actor upserted)", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListActorsResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.GreaterOrEqual(t, len(resp.Items), 1)
		})

		t.Run("with results and pagination", func(t *testing.T) {
			// create 3 actors
			a1 := createActor(t, tu.Db, "user/1", "root")
			a2 := createActor(t, tu.Db, "user/2", "root")
			a3 := createActor(t, tu.Db, "user/3", "root")
			_ = a1
			_ = a2
			_ = a3

			// page 1 with limit=2
			w1 := httptest.NewRecorder()
			req1, err := http.NewRequest(http.MethodGet, "/actors?limit=2", nil)
			require.NoError(t, err)
			req1 = authenticate(t, tu, req1)

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
			req2 = authenticate(t, tu, req2)

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
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("with label_selector", func(t *testing.T) {
			createActor(t, tu.Db, "user/l1", "root")
			// Need to update the actor because createActor helper doesn't support labels
			_, err := tu.Db.UpsertActor(context.Background(), &database.Actor{
				ExternalId: "user/l1",
				Namespace:  "root",
				Labels:     database.Labels{"app": "test-app"},
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors?label_selector=app%3Dtest-app", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListActorsResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 1)
			require.Equal(t, "user/l1", resp.Items[0].ExternalId)
			require.Equal(t, "test-app", resp.Items[0].Labels["app"])
		})
	})

	t.Run("get by id", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		a := createActor(t, tu.Db, "user/10", "root")
		otherId := uuid.New()

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+a.Id.String(), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/actors/"+a.Id.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "actors", "get", otherId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/not-a-uuid", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+uuid.New().String(), nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+a.Id.String(), nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

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

		a := createActor(t, tu.Db, "user-20", "root")

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/external-id/"+a.ExternalId, nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/actors/external-id/"+a.ExternalId,
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "actors", "get", "other-external-id"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/external-id/does-not-exist", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/external-id/"+a.ExternalId, nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

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

		a := createActor(t, tu.Db, "user/30", "root")
		otherId := uuid.New()

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/"+a.Id.String(), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/actors/"+a.Id.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "actors", "delete", otherId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/not-a-uuid", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found returns 204", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/"+uuid.New().String(), nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/"+a.Id.String(), nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

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

		a := createActor(t, tu.Db, "user-40", "root")
		a2 := createActor(t, tu.Db, "user-40", "root.some-namespace")

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/external-id/"+a.ExternalId, nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/actors/external-id/"+a.ExternalId,
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "actors", "delete", "other-external-id"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found returns 204", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/external-id/does-not-exist", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/external-id/"+a.ExternalId, nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			got, err := tu.Db.GetActorByExternalId(context.Background(), a.Namespace, a.ExternalId)
			require.ErrorIs(t, err, database.ErrNotFound)
			require.Nil(t, got)
		})

		t.Run("success in other namespace", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/external-id/"+a2.ExternalId+"?namespace="+a2.Namespace, nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			got, err := tu.Db.GetActorByExternalId(context.Background(), a2.Namespace, a2.ExternalId)
			require.ErrorIs(t, err, database.ErrNotFound)
			require.Nil(t, got)
		})
	})

	t.Run("namespace in response", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		a := createActorWithNamespace(t, tu.Db, "ns-user-1", "root.tenant1")

		t.Run("get by id includes namespace", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+a.Id.String(), nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ActorJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "root.tenant1", resp.Namespace)
		})

		t.Run("get by external id includes namespace", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/external-id/"+a.ExternalId+"?namespace=root.tenant1", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ActorJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "root.tenant1", resp.Namespace)
		})

		t.Run("list includes namespace", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors?namespace=root.tenant1", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListActorsResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 1)
			require.Equal(t, "root.tenant1", resp.Items[0].Namespace)
		})
	})

	t.Run("namespace filtering", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		// Create actors in different namespaces
		createActorWithNamespace(t, tu.Db, "filter-user-1", "root")
		createActorWithNamespace(t, tu.Db, "filter-user-2", "root.tenant1")
		createActorWithNamespace(t, tu.Db, "filter-user-3", "root.tenant1.sub")
		createActorWithNamespace(t, tu.Db, "filter-user-4", "root.tenant2")

		t.Run("exact namespace filter", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors?namespace=root.tenant1", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListActorsResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			// Should only get the actor in root.tenant1 exactly, not sub-namespaces
			require.Len(t, resp.Items, 1)
			require.Equal(t, "root.tenant1", resp.Items[0].Namespace)
		})

		t.Run("wildcard namespace filter", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors?namespace="+url.QueryEscape("root.tenant1.**"), nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListActorsResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			// Should get both root.tenant1 and root.tenant1.sub
			require.Len(t, resp.Items, 2)
		})
	})

	t.Run("create", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		t.Run("unauthorized", func(t *testing.T) {
			reqBody := CreateActorRequestJson{
				ExternalId: "new-actor",
				Namespace:  "root",
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/actors", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching namespace permission", func(t *testing.T) {
			reqBody := CreateActorRequestJson{
				ExternalId: "new-actor",
				Namespace:  "root",
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/actors",
				bytes.NewBufferString(body),
				"root.other",
				"some-actor",
				aschema.PermissionsSingle("root.other.**", "actors", "create"),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad request - missing external_id", func(t *testing.T) {
			body := `{"namespace": "root"}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/actors", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("bad request - invalid namespace", func(t *testing.T) {
			reqBody := CreateActorRequestJson{
				ExternalId: "new-actor",
				Namespace:  "invalid",
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/actors", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("bad request - namespace does not exist", func(t *testing.T) {
			reqBody := CreateActorRequestJson{
				ExternalId: "new-actor",
				Namespace:  "root.nonexistent",
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/actors", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("bad request - invalid JSON", func(t *testing.T) {
			body := `{invalid json}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/actors", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("conflict - duplicate external_id in same namespace", func(t *testing.T) {
			// Create an actor first
			createActor(t, tu.Db, "duplicate-actor", "root")

			// Try to create another actor with the same external_id
			reqBody := CreateActorRequestJson{
				ExternalId: "duplicate-actor",
				Namespace:  "root",
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/actors", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusConflict, w.Code)
		})

		t.Run("success - basic actor", func(t *testing.T) {
			reqBody := CreateActorRequestJson{
				ExternalId: "created-actor",
				Namespace:  "root",
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/actors", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusCreated, w.Code)

			var resp ActorJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.NotEqual(t, uuid.Nil, resp.Id)
			require.Equal(t, "created-actor", resp.ExternalId)
			require.Equal(t, "root", resp.Namespace)
			require.NotZero(t, resp.CreatedAt)
			require.NotZero(t, resp.UpdatedAt)

			// Verify the actor exists in the database
			actor, err := tu.Db.GetActorByExternalId(context.Background(), "root", "created-actor")
			require.NoError(t, err)
			require.NotNil(t, actor)
			require.Equal(t, resp.Id, actor.Id)
		})

		t.Run("success - actor with labels", func(t *testing.T) {
			reqBody := CreateActorRequestJson{
				ExternalId: "actor-with-labels",
				Namespace:  "root",
				Labels: map[string]string{
					"env":  "test",
					"team": "platform",
				},
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/actors", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusCreated, w.Code)

			var resp ActorJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "actor-with-labels", resp.ExternalId)
			require.Equal(t, "test", resp.Labels["env"])
			require.Equal(t, "platform", resp.Labels["team"])

			// Verify the actor exists in the database with labels
			actor, err := tu.Db.GetActorByExternalId(context.Background(), "root", "actor-with-labels")
			require.NoError(t, err)
			require.NotNil(t, actor)
			require.Equal(t, "test", actor.Labels["env"])
			require.Equal(t, "platform", actor.Labels["team"])
		})

		t.Run("success - same external_id in different namespace", func(t *testing.T) {
			// Create actor in root namespace
			createActor(t, tu.Db, "multi-namespace-actor", "root")

			// Create another actor with same external_id in different namespace
			tu.Db.EnsureNamespaceByPath(context.Background(), "root.other")
			reqBody := CreateActorRequestJson{
				ExternalId: "multi-namespace-actor",
				Namespace:  "root.other",
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/actors", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusCreated, w.Code)

			var resp ActorJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "multi-namespace-actor", resp.ExternalId)
			require.Equal(t, "root.other", resp.Namespace)

			// Verify both actors exist
			actor1, err := tu.Db.GetActorByExternalId(context.Background(), "root", "multi-namespace-actor")
			require.NoError(t, err)
			require.NotNil(t, actor1)

			actor2, err := tu.Db.GetActorByExternalId(context.Background(), "root.other", "multi-namespace-actor")
			require.NoError(t, err)
			require.NotNil(t, actor2)

			require.NotEqual(t, actor1.Id, actor2.Id)
		})
	})

	t.Run("update by id", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		a := createActor(t, tu.Db, "update-actor-1", "root")
		otherId := uuid.New()

		t.Run("unauthorized", func(t *testing.T) {
			reqBody := UpdateActorRequestJson{
				Labels: map[string]string{"env": "prod"},
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/actors/"+a.Id.String(), bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			reqBody := UpdateActorRequestJson{
				Labels: map[string]string{"env": "prod"},
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/actors/"+a.Id.String(),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "actors", "update", otherId.String()),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			reqBody := UpdateActorRequestJson{
				Labels: map[string]string{"env": "prod"},
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/actors/not-a-uuid", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			reqBody := UpdateActorRequestJson{
				Labels: map[string]string{"env": "prod"},
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/actors/"+uuid.New().String(), bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("bad request - invalid JSON", func(t *testing.T) {
			body := `{invalid json}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/actors/"+a.Id.String(), bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("success - update labels", func(t *testing.T) {
			reqBody := UpdateActorRequestJson{
				Labels: map[string]string{"env": "production", "team": "backend"},
			}
			body := util.MustPrettyJSON(reqBody)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/actors/"+a.Id.String(), bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ActorJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, a.Id, resp.Id)
			require.Equal(t, a.ExternalId, resp.ExternalId)
			require.Equal(t, a.Namespace, resp.Namespace)
			require.Equal(t, "production", resp.Labels["env"])
			require.Equal(t, "backend", resp.Labels["team"])

			// Verify the actor is updated in the database
			updatedActor, err := tu.Db.GetActor(context.Background(), a.Id)
			require.NoError(t, err)
			require.Equal(t, "production", updatedActor.Labels["env"])
			require.Equal(t, "backend", updatedActor.Labels["team"])
		})

		t.Run("success - clear labels", func(t *testing.T) {
			// First add some labels
			actorWithLabels := createActor(t, tu.Db, "actor-to-clear-labels", "root")
			_, err := tu.Db.UpsertActor(context.Background(), &database.Actor{
				Id:         actorWithLabels.Id,
				ExternalId: actorWithLabels.ExternalId,
				Namespace:  actorWithLabels.Namespace,
				Labels:     database.Labels{"old": "value"},
			})
			require.NoError(t, err)

			// Now clear the labels
			body := `{"labels": {}}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/actors/"+actorWithLabels.Id.String(), bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ActorJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Empty(t, resp.Labels)

			// Verify the labels are cleared in the database
			updatedActor, err := tu.Db.GetActor(context.Background(), actorWithLabels.Id)
			require.NoError(t, err)
			require.Empty(t, updatedActor.Labels)
		})

		t.Run("success - labels unchanged", func(t *testing.T) {
			// First add some labels
			actorWithLabels := createActor(t, tu.Db, "actor-to-leave-labels", "root")
			_, err := tu.Db.UpsertActor(context.Background(), &database.Actor{
				Id:         actorWithLabels.Id,
				ExternalId: actorWithLabels.ExternalId,
				Namespace:  actorWithLabels.Namespace,
				Labels:     database.Labels{"old": "value"},
			})
			require.NoError(t, err)

			// Do not specify the labels
			body := `{}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/actors/"+actorWithLabels.Id.String(), bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ActorJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, resp.Labels, map[string]string{"old": "value"})

			// Verify the labels are cleared in the database
			updatedActor, err := tu.Db.GetActor(context.Background(), actorWithLabels.Id)
			require.NoError(t, err)
			require.Equal(t, updatedActor.Labels, database.Labels{"old": "value"})
		})
	})

	t.Run("update by external id", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		a := createActor(t, tu.Db, "update-ext-actor", "root")

		t.Run("unauthorized", func(t *testing.T) {
			body := `{"labels": {"env": "prod"}}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/actors/external-id/"+a.ExternalId, bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			body := `{"labels": {"env": "prod"}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/actors/external-id/"+a.ExternalId,
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "actors", "update", "other-external-id"),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			body := `{"labels": {"env": "prod"}}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/actors/external-id/does-not-exist", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("success - update labels", func(t *testing.T) {
			body := `{"labels": {"env": "staging", "version": "v2"}}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/actors/external-id/"+a.ExternalId, bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ActorJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, a.Id, resp.Id)
			require.Equal(t, a.ExternalId, resp.ExternalId)
			require.Equal(t, a.Namespace, resp.Namespace)
			require.Equal(t, "staging", resp.Labels["env"])
			require.Equal(t, "v2", resp.Labels["version"])

			// Verify the actor is updated in the database
			updatedActor, err := tu.Db.GetActorByExternalId(context.Background(), a.Namespace, a.ExternalId)
			require.NoError(t, err)
			require.Equal(t, "staging", updatedActor.Labels["env"])
			require.Equal(t, "v2", updatedActor.Labels["version"])
		})

		t.Run("success - update in different namespace", func(t *testing.T) {
			a2 := createActorWithNamespace(t, tu.Db, "update-ext-actor-ns", "root.tenant1")

			body := `{"labels": {"tenant": "tenant1"}}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/actors/external-id/"+a2.ExternalId+"?namespace=root.tenant1", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ActorJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "root.tenant1", resp.Namespace)
			require.Equal(t, "tenant1", resp.Labels["tenant"])
		})
	})

	t.Run("get labels", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		// Create an actor with labels
		a := createActor(t, tu.Db, "labels-actor", "root")
		_, err := tu.Db.UpsertActor(context.Background(), &database.Actor{
			Id:         a.Id,
			ExternalId: a.ExternalId,
			Namespace:  a.Namespace,
			Labels:     database.Labels{"env": "prod", "team": "backend"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+a.Id.String()+"/labels", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/not-a-uuid/labels", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+uuid.New().String()+"/labels", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+a.Id.String()+"/labels", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "prod", resp["env"])
			require.Equal(t, "backend", resp["team"])
		})

		t.Run("success - empty labels", func(t *testing.T) {
			actorNoLabels := createActor(t, tu.Db, "no-labels-actor", "root")

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+actorNoLabels.Id.String()+"/labels", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Empty(t, resp)
		})
	})

	t.Run("get label", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		// Create an actor with labels
		a := createActor(t, tu.Db, "get-label-actor", "root")
		_, err := tu.Db.UpsertActor(context.Background(), &database.Actor{
			Id:         a.Id,
			ExternalId: a.ExternalId,
			Namespace:  a.Namespace,
			Labels:     database.Labels{"env": "staging"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+a.Id.String()+"/labels/env", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/not-a-uuid/labels/env", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("actor not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+uuid.New().String()+"/labels/env", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("label not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+a.Id.String()+"/labels/nonexistent", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/actors/"+a.Id.String()+"/labels/env", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ActorLabelJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "env", resp.Key)
			require.Equal(t, "staging", resp.Value)
		})
	})

	t.Run("put label", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		a := createActor(t, tu.Db, "put-label-actor", "root")
		otherId := uuid.New()

		t.Run("unauthorized", func(t *testing.T) {
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/actors/"+a.Id.String()+"/labels/env", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/actors/"+a.Id.String()+"/labels/env",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "actors", "update", otherId.String()),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/actors/not-a-uuid/labels/env", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("actor not found", func(t *testing.T) {
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/actors/"+uuid.New().String()+"/labels/env", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("bad request - invalid JSON", func(t *testing.T) {
			body := `{invalid json}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/actors/"+a.Id.String()+"/labels/env", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("success - add new label", func(t *testing.T) {
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/actors/"+a.Id.String()+"/labels/env", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ActorLabelJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "env", resp.Key)
			require.Equal(t, "production", resp.Value)

			// Verify in database
			updatedActor, err := tu.Db.GetActor(context.Background(), a.Id)
			require.NoError(t, err)
			require.Equal(t, "production", updatedActor.Labels["env"])
		})

		t.Run("success - update existing label", func(t *testing.T) {
			// First set a label
			actorWithLabel := createActor(t, tu.Db, "update-existing-label", "root")
			_, err := tu.Db.UpsertActor(context.Background(), &database.Actor{
				Id:         actorWithLabel.Id,
				ExternalId: actorWithLabel.ExternalId,
				Namespace:  actorWithLabel.Namespace,
				Labels:     database.Labels{"version": "v1"},
			})
			require.NoError(t, err)

			// Update the label
			body := `{"value": "v2"}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/actors/"+actorWithLabel.Id.String()+"/labels/version", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ActorLabelJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "version", resp.Key)
			require.Equal(t, "v2", resp.Value)

			// Verify in database
			updatedActor, err := tu.Db.GetActor(context.Background(), actorWithLabel.Id)
			require.NoError(t, err)
			require.Equal(t, "v2", updatedActor.Labels["version"])
		})

		t.Run("success - preserves other labels", func(t *testing.T) {
			// First set multiple labels
			actorMultiLabel := createActor(t, tu.Db, "multi-label-actor", "root")
			_, err := tu.Db.UpsertActor(context.Background(), &database.Actor{
				Id:         actorMultiLabel.Id,
				ExternalId: actorMultiLabel.ExternalId,
				Namespace:  actorMultiLabel.Namespace,
				Labels:     database.Labels{"env": "dev", "team": "platform"},
			})
			require.NoError(t, err)

			// Update one label
			body := `{"value": "staging"}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/actors/"+actorMultiLabel.Id.String()+"/labels/env", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			// Verify both labels in database
			updatedActor, err := tu.Db.GetActor(context.Background(), actorMultiLabel.Id)
			require.NoError(t, err)
			require.Equal(t, "staging", updatedActor.Labels["env"])
			require.Equal(t, "platform", updatedActor.Labels["team"])
		})
	})

	t.Run("delete label", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		// Create an actor with labels
		a := createActor(t, tu.Db, "delete-label-actor", "root")
		_, err := tu.Db.UpsertActor(context.Background(), &database.Actor{
			Id:         a.Id,
			ExternalId: a.ExternalId,
			Namespace:  a.Namespace,
			Labels:     database.Labels{"env": "prod", "team": "backend"},
		})
		require.NoError(t, err)

		otherId := uuid.New()

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/"+a.Id.String()+"/labels/env", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/actors/"+a.Id.String()+"/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "actors", "update", otherId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/not-a-uuid/labels/env", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("actor not found returns 204", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/"+uuid.New().String()+"/labels/env", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("label not found returns 204", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/"+a.Id.String()+"/labels/nonexistent", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("success - delete label", func(t *testing.T) {
			// Create actor with label to delete
			actorToDelete := createActor(t, tu.Db, "actor-delete-one-label", "root")
			_, err := tu.Db.UpsertActor(context.Background(), &database.Actor{
				Id:         actorToDelete.Id,
				ExternalId: actorToDelete.ExternalId,
				Namespace:  actorToDelete.Namespace,
				Labels:     database.Labels{"to-delete": "value", "to-keep": "value2"},
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/actors/"+actorToDelete.Id.String()+"/labels/to-delete", nil)
			require.NoError(t, err)
			req = authenticate(t, tu, req)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			// Verify the label is deleted but other labels remain
			updatedActor, err := tu.Db.GetActor(context.Background(), actorToDelete.Id)
			require.NoError(t, err)
			_, exists := updatedActor.Labels["to-delete"]
			require.False(t, exists)
			require.Equal(t, "value2", updatedActor.Labels["to-keep"])
		})

		t.Run("success - delete is idempotent", func(t *testing.T) {
			actorIdempotent := createActor(t, tu.Db, "actor-idempotent-delete", "root")
			_, err := tu.Db.UpsertActor(context.Background(), &database.Actor{
				Id:         actorIdempotent.Id,
				ExternalId: actorIdempotent.ExternalId,
				Namespace:  actorIdempotent.Namespace,
				Labels:     database.Labels{"label": "value"},
			})
			require.NoError(t, err)

			// Delete the label twice
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				req, err := http.NewRequest(http.MethodDelete, "/actors/"+actorIdempotent.Id.String()+"/labels/label", nil)
				require.NoError(t, err)
				req = authenticate(t, tu, req)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNoContent, w.Code)
			}

			// Verify the label is deleted
			updatedActor, err := tu.Db.GetActor(context.Background(), actorIdempotent.Id)
			require.NoError(t, err)
			_, exists := updatedActor.Labels["label"]
			require.False(t, exists)
		})
	})
}
