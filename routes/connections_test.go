package routes

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	asynqmock "github.com/rmorlok/authproxy/apasynq/mock"
	"github.com/rmorlok/authproxy/aplog"
	auth2 "github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	httpf2 "github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	redismock "github.com/rmorlok/authproxy/redis/mock"
	"github.com/rmorlok/authproxy/test_utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
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
		cfg, rds := redis.MustApplyTestConfig(cfg)
		cfg, auth, authUtil := auth2.TestAuthServiceWithDb(config.ServiceIdApi, cfg, db)
		h := httpf2.CreateFactory(cfg, rds, aplog.NewNoopLogger())
		cfg, e := encrypt.NewTestEncryptService(cfg, db)
		ctrl := gomock.NewController(t)
		ac := asynqmock.NewMockClient(ctrl)
		rs := redismock.NewMockR(ctrl)
		c := connectors.NewConnectorsService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
		assert.NoError(t, c.MigrateConnectors(context.Background()))
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
}
