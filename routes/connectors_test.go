package routes

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	asynqmock "github.com/rmorlok/authproxy/apasynq/mock"
	"github.com/rmorlok/authproxy/aplog"
	redismock "github.com/rmorlok/authproxy/apredis/mock"
	auth2 "github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	httpf2 "github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/test_utils"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConnectors(t *testing.T) {
	type TestSetup struct {
		Gin      *gin.Engine
		Cfg      config.C
		AuthUtil *auth2.AuthTestUtil
	}

	setup := func(t *testing.T, cfg config.C) *TestSetup {
		if cfg == nil {
			cfg = config.FromRoot(&config.Root{
				Connectors: &config.Connectors{
					LoadFromList: []config.Connector{},
				},
			})
		}

		root := cfg.GetRoot()
		if root == nil {
			panic("No root in config")
		}

		if len(root.Connectors.LoadFromList) == 0 {
			root.Connectors.LoadFromList = []config.Connector{
				{
					Id:          uuid.MustParse("10000000-0000-0000-0000-000000000001"),
					Type:        "test-connector",
					DisplayName: "Test ConnectorJson",
				},
			}
		}

		ctrl := gomock.NewController(t)
		ac := asynqmock.NewMockClient(ctrl)
		cfg, db := database.MustApplyBlankTestDbConfig(t.Name(), cfg)
		cfg, e := encrypt.NewTestEncryptService(cfg, db)
		cfg, auth, authUtil := auth2.TestAuthServiceWithDb(config.ServiceIdApi, cfg, db)
		rs := redismock.NewMockClient(ctrl)
		h := httpf2.CreateFactory(cfg, rs, aplog.NewNoopLogger())
		c := connectors.NewConnectorsService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
		require.NoError(t, c.MigrateConnectors(context.Background()))

		cr := NewConnectorsRoutes(cfg, auth, c)

		r := gin.Default()
		cr.Register(r)

		return &TestSetup{
			Gin:      r,
			Cfg:      cfg,
			AuthUtil: authUtil,
		}
	}

	t.Run("get connector", func(t *testing.T) {
		tu := setup(t, nil)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connectors/test-connector", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("malformed id", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodGet, "/connectors/bad-connector", nil, "some-actor")
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("invalid id", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodGet, "/connectors/99999999-0000-0000-0000-000000000001", nil, "some-actor")
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodGet, "/connectors/10000000-0000-0000-0000-000000000001", nil, "some-actor")
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectorJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, uuid.MustParse("10000000-0000-0000-0000-000000000001"), resp.Id)
			require.Equal(t, "Test ConnectorJson", resp.DisplayName)
		})
	})

	t.Run("list connectors", func(t *testing.T) {
		tu := setup(t, nil)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connectors", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodGet, "/connectors", nil, "some-actor")
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListConnectorsResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, uuid.MustParse("10000000-0000-0000-0000-000000000001"), resp.Items[0].Id)
			require.Equal(t, "Test ConnectorJson", resp.Items[0].DisplayName)
		})
	})
}
