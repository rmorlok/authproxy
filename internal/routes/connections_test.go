package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	asynqmock "github.com/rmorlok/authproxy/internal/apasynq/mock"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestConnections(t *testing.T) {
	type TestSetup struct {
		Gin      *gin.Engine
		Cfg      config.C
		AuthUtil *auth2.AuthTestUtil
		Db       database.DB
	}

	connectorId := uuid.MustParse("10000000-0000-0000-0000-000000000001")
	connectorVersion := uint64(1)

	setup := func(t *testing.T, cfg config.C) (*TestSetup, func()) {
		cfg = config.FromRoot(&sconfig.Root{
			Connectors: &sconfig.Connectors{
				LoadFromList: []sconfig.Connector{
					{
						Id:          connectorId,
						Version:     connectorVersion,
						Labels:      map[string]string{"type": "test-connector"},
						DisplayName: "Test Connector",
					},
				},
			},
		})
		cfg, db := database.MustApplyBlankTestDbConfig(t.Name(), cfg)
		cfg, rds := apredis.MustApplyTestConfig(cfg)
		cfg, auth, authUtil := auth2.TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)
		h := httpf2.CreateFactory(cfg, rds, aplog.NewNoopLogger())
		cfg, e := encrypt.NewTestEncryptService(cfg, db)
		ctrl := gomock.NewController(t)
		ac := asynqmock.NewMockClient(ctrl)
		rs := mock.NewMockClient(ctrl)
		c := core.NewCoreService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
		assert.NoError(t, c.Migrate(context.Background()))
		cr := NewConnectionsRoutes(cfg, auth, db, rds, c, h, e, test_utils.NewTestLogger())
		r := gin.New()
		cr.Register(r)

		return &TestSetup{
				Gin:      r,
				Cfg:      cfg,
				AuthUtil: authUtil,
				Db:       db,
			}, func() {
				ctrl.Finish()
			}
	}

	t.Run("get connection", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := uuid.New()
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connections/"+u.String(), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+uuid.New().String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "list"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("invalid uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connections/"+uuid.New().String(), nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connections/"+u.String(), nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u, resp.Id)
			require.Equal(t, database.ConnectionStateCreated, resp.State)
		})

		t.Run("allowed with matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+u.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "get", u.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u, resp.Id)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			otherResourceId := uuid.New()
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+u.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "get", otherResourceId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("allowed with multiple resource ids including target", func(t *testing.T) {
			otherResourceId := uuid.New()
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+u.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "get", otherResourceId.String(), u.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u, resp.Id)
		})
	})

	t.Run("list connections", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		now := time.Now()
		c := clock.NewFakeClock(now)
		ctx := apctx.WithClock(context.Background(), c)

		u := uuid.New()
		err := tu.Db.CreateConnection(ctx, &database.Connection{
			Id:               u,
			Namespace:        "root",
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
		})
		require.NoError(t, err)

		now = now.Add(time.Second)
		c.SetTime(now)
		err = tu.Db.CreateConnection(ctx, &database.Connection{
			Id:               uuid.New(),
			Namespace:        "root.child",
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
		})
		require.NoError(t, err)

		now = now.Add(time.Second)
		c.SetTime(now)
		err = tu.Db.CreateConnection(ctx, &database.Connection{
			Id:               uuid.New(),
			Namespace:        "root.child",
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
		})
		require.NoError(t, err)

		now = now.Add(time.Second)
		c.SetTime(now)
		err = tu.Db.CreateConnection(ctx, &database.Connection{
			Id:               uuid.New(),
			Namespace:        "root.child.grandchild",
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connections", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections?limit=50&order=created_at%20asc",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections?limit=50&order=created_at%20asc",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListConnectionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 4)
		})

		t.Run("filter to namespace", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connections?limit=50&order=created_at%20asc&namespace=root", nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListConnectionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, resp.Items[0].Id, u)
		})

		t.Run("filter to namespace matcher", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connections?limit=50&order=created_at%20asc&namespace=root.child.**", nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListConnectionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 3)
		})

		t.Run("filter with label_selector", func(t *testing.T) {
			connId := uuid.New()
			err := tu.Db.CreateConnection(ctx, &database.Connection{
				Id:               connId,
				Namespace:        "root",
				ConnectorId:      connectorId,
				ConnectorVersion: connectorVersion,
				State:            database.ConnectionStateCreated,
				Labels:           database.Labels{"env": "test-label-conn"},
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connections?label_selector=env%3Dtest-label-conn", nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListConnectionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, connId, resp.Items[0].Id)
			require.Equal(t, "test-label-conn", resp.Items[0].Labels["env"])
		})
	})

	t.Run("disconnect connection", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := uuid.New()
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateReady,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/connections/"+u.String()+"/_disconnect", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/"+u.String()+"/_disconnect",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			otherResourceId := uuid.New()
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/"+u.String()+"/_disconnect",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "disconnect", otherResourceId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})
	})

	t.Run("initiate connection", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/connections/_initiate", util.JsonToReader(map[string]interface{}{
				"connector_id":  connectorId.String(),
				"return_to_url": "https://example.com/callback",
			}))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/_initiate",
				util.JsonToReader(map[string]interface{}{
					"connector_id":  connectorId.String(),
					"return_to_url": "https://example.com/callback",
				}),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})
	})

	t.Run("update connection (PATCH)", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := uuid.New()
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
			Labels:           database.Labels{"existing": "value"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/connections/"+u.String(), util.JsonToReader(map[string]interface{}{
				"labels": map[string]string{"env": "prod"},
			}))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/"+u.String(),
				util.JsonToReader(map[string]interface{}{
					"labels": map[string]string{"env": "prod"},
				}),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"), // Wrong verb
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/not-a-uuid",
				util.JsonToReader(map[string]interface{}{
					"labels": map[string]string{"env": "prod"},
				}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/"+uuid.New().String(),
				util.JsonToReader(map[string]interface{}{
					"labels": map[string]string{"env": "prod"},
				}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("invalid JSON", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/"+u.String(),
				util.JsonToReader("{invalid json}"),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("success with labels", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/"+u.String(),
				util.JsonToReader(map[string]interface{}{
					"labels": map[string]string{"env": "production", "team": "backend"},
				}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u, resp.Id)
			require.Equal(t, "production", resp.Labels["env"])
			require.Equal(t, "backend", resp.Labels["team"])
			// "existing" label should be gone since this is a full replacement
			_, exists := resp.Labels["existing"]
			require.False(t, exists)
		})

		t.Run("success preserves state", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/"+u.String(),
				util.JsonToReader(map[string]interface{}{
					"labels": map[string]string{"new": "label"},
				}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u, resp.Id)
			require.Equal(t, database.ConnectionStateCreated, resp.State)
		})
	})

	t.Run("get connection labels", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := uuid.New()
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
			Labels:           database.Labels{"env": "prod", "team": "backend"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connections/"+u.String()+"/labels", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/not-a-uuid/labels",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+uuid.New().String()+"/labels",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("success with labels", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+u.String()+"/labels",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp map[string]string
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "prod", resp["env"])
			require.Equal(t, "backend", resp["team"])
		})

		t.Run("success with empty labels", func(t *testing.T) {
			noLabelsId := uuid.New()
			err := tu.Db.CreateConnection(context.Background(), &database.Connection{
				Id:               noLabelsId,
				Namespace:        sconfig.RootNamespace,
				ConnectorId:      connectorId,
				ConnectorVersion: connectorVersion,
				State:            database.ConnectionStateCreated,
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+noLabelsId.String()+"/labels",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp map[string]string
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Empty(t, resp)
		})
	})

	t.Run("get connection label", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := uuid.New()
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
			Labels:           database.Labels{"env": "staging"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connections/"+u.String()+"/labels/env", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/not-a-uuid/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("connection not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+uuid.New().String()+"/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("label not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+u.String()+"/labels/nonexistent",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+u.String()+"/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionLabelJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "env", resp.Key)
			require.Equal(t, "staging", resp.Value)
		})
	})

	t.Run("put connection label", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := uuid.New()
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/connections/"+u.String()+"/labels/env", util.JsonToReader(map[string]interface{}{"value": "production"}))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id", func(t *testing.T) {
			otherResourceId := uuid.New()
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/labels/env",
				util.JsonToReader(map[string]interface{}{"value": "production"}),
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "update", otherResourceId.String()),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/not-a-uuid/labels/env",
				util.JsonToReader(map[string]interface{}{"value": "production"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+uuid.New().String()+"/labels/env",
				util.JsonToReader(map[string]interface{}{"value": "production"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("invalid JSON", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/labels/env",
				util.JsonToReader("{invalid json}"),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("success add new label", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/labels/env",
				util.JsonToReader(map[string]interface{}{"value": "production"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionLabelJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "env", resp.Key)
			require.Equal(t, "production", resp.Value)

			// Verify in database
			conn, err := tu.Db.GetConnection(context.Background(), u)
			require.NoError(t, err)
			require.Equal(t, "production", conn.Labels["env"])
		})

		t.Run("success update existing", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/labels/env",
				util.JsonToReader(map[string]interface{}{"value": "staging"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionLabelJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "env", resp.Key)
			require.Equal(t, "staging", resp.Value)
		})

		t.Run("success preserves other labels", func(t *testing.T) {
			// Add another label first
			_, err := tu.Db.PutConnectionLabels(context.Background(), u, map[string]string{"team": "platform"})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/labels/env",
				util.JsonToReader(map[string]interface{}{"value": "dev"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			// Verify both labels in database
			conn, err := tu.Db.GetConnection(context.Background(), u)
			require.NoError(t, err)
			require.Equal(t, "dev", conn.Labels["env"])
			require.Equal(t, "platform", conn.Labels["team"])
		})
	})

	t.Run("delete connection label", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := uuid.New()
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
			Labels:           database.Labels{"env": "prod", "team": "backend"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/connections/"+u.String()+"/labels/env", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id", func(t *testing.T) {
			otherResourceId := uuid.New()
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/connections/"+u.String()+"/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "update", otherResourceId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/connections/not-a-uuid/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("connection not found returns 204", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/connections/"+uuid.New().String()+"/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("label not found returns 204", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/connections/"+u.String()+"/labels/nonexistent",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("success delete", func(t *testing.T) {
			// Create a fresh connection with labels for this test
			deleteTestId := uuid.New()
			err := tu.Db.CreateConnection(context.Background(), &database.Connection{
				Id:               deleteTestId,
				Namespace:        sconfig.RootNamespace,
				ConnectorId:      connectorId,
				ConnectorVersion: connectorVersion,
				State:            database.ConnectionStateCreated,
				Labels:           database.Labels{"to-delete": "value", "to-keep": "value2"},
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/connections/"+deleteTestId.String()+"/labels/to-delete",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			// Verify the label is deleted but other labels remain
			conn, err := tu.Db.GetConnection(context.Background(), deleteTestId)
			require.NoError(t, err)
			_, exists := conn.Labels["to-delete"]
			require.False(t, exists)
			require.Equal(t, "value2", conn.Labels["to-keep"])
		})

		t.Run("success idempotent delete", func(t *testing.T) {
			// Create a fresh connection for idempotent test
			idempotentId := uuid.New()
			err := tu.Db.CreateConnection(context.Background(), &database.Connection{
				Id:               idempotentId,
				Namespace:        sconfig.RootNamespace,
				ConnectorId:      connectorId,
				ConnectorVersion: connectorVersion,
				State:            database.ConnectionStateCreated,
				Labels:           database.Labels{"label": "value"},
			})
			require.NoError(t, err)

			// Delete the label twice
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					"/connections/"+idempotentId.String()+"/labels/label",
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNoContent, w.Code)
			}

			// Verify the label is deleted
			conn, err := tu.Db.GetConnection(context.Background(), idempotentId)
			require.NoError(t, err)
			_, exists := conn.Labels["label"]
			require.False(t, exists)
		})
	})

	t.Run("force connection state", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := uuid.New()
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/connections/"+u.String()+"/_force_state", util.JsonToReader(ForceStateRequestJson{State: database.ConnectionStateDisconnected}))
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+uuid.New().String()+"/_force_state",
				util.JsonToReader(ForceStateRequestJson{State: database.ConnectionStateDisconnected}),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("invalid uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+uuid.New().String()+"/_force_state",
				util.JsonToReader(ForceStateRequestJson{State: database.ConnectionStateDisconnected}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/_force_state",
				util.JsonToReader(ForceStateRequestJson{State: database.ConnectionStateDisconnected}),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "force_state"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u, resp.Id)
			require.Equal(t, database.ConnectionStateDisconnected, resp.State)
		})

		t.Run("allowed with matching resource id permission", func(t *testing.T) {
			// Reset state first
			err := tu.Db.SetConnectionState(context.Background(), u, database.ConnectionStateCreated)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/_force_state",
				util.JsonToReader(ForceStateRequestJson{State: database.ConnectionStateDisconnected}),
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "force_state", u.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u, resp.Id)
			require.Equal(t, database.ConnectionStateDisconnected, resp.State)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			otherResourceId := uuid.New()
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/_force_state",
				util.JsonToReader(ForceStateRequestJson{State: database.ConnectionStateReady}),
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "force_state", otherResourceId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})
	})
}
