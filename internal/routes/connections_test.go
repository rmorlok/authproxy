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
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/apredis/mock"
	auth2 "github.com/rmorlok/authproxy/internal/auth"
	"github.com/rmorlok/authproxy/internal/config"
	cfg "github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
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
		cfg = config.FromRoot(&config.Root{
			Connectors: &config.Connectors{
				LoadFromList: []config.Connector{
					{
						Id:          connectorId,
						Version:     connectorVersion,
						Type:        "test-connector",
						DisplayName: "Test Connector",
					},
				},
			},
		})
		cfg, db := database.MustApplyBlankTestDbConfig(t.Name(), cfg)
		cfg, rds := apredis.MustApplyTestConfig(cfg)
		cfg, auth, authUtil := auth2.TestAuthServiceWithDb(config.ServiceIdApi, cfg, db)
		h := httpf2.CreateFactory(cfg, rds, aplog.NewNoopLogger())
		cfg, e := encrypt.NewTestEncryptService(cfg, db)
		ctrl := gomock.NewController(t)
		ac := asynqmock.NewMockClient(ctrl)
		rs := mock.NewMockClient(ctrl)
		c := core.NewCoreService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
		assert.NoError(t, c.Migrate(context.Background()))
		cr := NewConnectionsRoutes(cfg, auth, db, rds, c, h, e, test_utils.NewTestLogger())
		r := gin.Default()
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
			ID:               u,
			Namespace:        cfg.RootNamespace,
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

		t.Run("invalid uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodGet, "/connections/"+uuid.New().String(), nil, "some-actor")
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodGet, "/connections/"+u.String(), nil, "some-actor")
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u, resp.ID)
			require.Equal(t, database.ConnectionStateCreated, resp.State)
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
			ID:               u,
			Namespace:        "root",
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
		})
		require.NoError(t, err)

		now = now.Add(time.Second)
		c.SetTime(now)
		err = tu.Db.CreateConnection(context.Background(), &database.Connection{
			ID:               uuid.New(),
			Namespace:        "root.child",
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
		})
		require.NoError(t, err)

		now = now.Add(time.Second)
		c.SetTime(now)
		err = tu.Db.CreateConnection(context.Background(), &database.Connection{
			ID:               uuid.New(),
			Namespace:        "root.child",
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateCreated,
		})
		require.NoError(t, err)

		now = now.Add(time.Second)
		c.SetTime(now)
		err = tu.Db.CreateConnection(context.Background(), &database.Connection{
			ID:               uuid.New(),
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

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodGet, "/connections?limit=50&order=created_at%20asc", nil, "some-actor")
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
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodGet, "/connections?limit=50&order=created_at%20asc&namespace=root", nil, "some-actor")
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListConnectionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, resp.Items[0].ID, u)
		})

		t.Run("filter to namespace matcher", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodGet, "/connections?limit=50&order=created_at%20asc&namespace=root.child.**", nil, "some-actor")
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListConnectionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 3)
		})
	})

	t.Run("force connection state", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := uuid.New()
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			ID:               u,
			Namespace:        cfg.RootNamespace,
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

		t.Run("invalid uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodPut, "/connections/"+uuid.New().String()+"/_force_state", util.JsonToReader(ForceStateRequestJson{State: database.ConnectionStateDisconnected}), "some-actor")
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodPut, "/connections/"+u.String()+"/_force_state", util.JsonToReader(ForceStateRequestJson{State: database.ConnectionStateDisconnected}), "some-actor")
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u, resp.ID)
			require.Equal(t, database.ConnectionStateDisconnected, resp.State)
		})
	})
}
