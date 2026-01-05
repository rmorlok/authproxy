package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	asynqmock "github.com/rmorlok/authproxy/internal/apasynq/mock"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
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
					Namespace:   util.ToPtr("root"),
					Type:        "test-connector",
					DisplayName: "Test ConnectorJson",
				},
				{
					Id:          uuid.MustParse("20000000-0000-0000-0000-000000000002"),
					Namespace:   util.ToPtr("root.child"),
					Type:        "test-connector-2",
					DisplayName: "Test ConnectorJson 2",
				},
			}
		}

		ctrl := gomock.NewController(t)
		ac := asynqmock.NewMockClient(ctrl)
		cfg, db := database.MustApplyBlankTestDbConfig(t.Name(), cfg)
		cfg, e := encrypt.NewTestEncryptService(cfg, db)
		cfg, auth, authUtil := auth2.TestAuthServiceWithDb(config.ServiceIdApi, cfg, db)
		rs := mock.NewMockClient(ctrl)
		h := httpf2.CreateFactory(cfg, rs, aplog.NewNoopLogger())
		c := core.NewCoreService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
		require.NoError(t, c.Migrate(context.Background()))

		cr := NewConnectorsRoutes(cfg, auth, c)

		r := gin.Default()
		cr.Register(r)

		return &TestSetup{
			Gin:      r,
			Cfg:      cfg,
			AuthUtil: authUtil,
		}
	}

	t.Run("connectors", func(t *testing.T) {
		t.Run("get", func(t *testing.T) {
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
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/bad-connector", nil, "some-actor")
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusBadRequest, w.Code)
			})

			t.Run("invalid id", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/99999999-0000-0000-0000-000000000001", nil, "some-actor")
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNotFound, w.Code)
			})

			t.Run("valid", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/10000000-0000-0000-0000-000000000001", nil, "some-actor")
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

		t.Run("list", func(t *testing.T) {
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
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors?order=id%20asc", nil, "some-actor")
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ListConnectorsResponseJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Len(t, resp.Items, 2)
				require.Equal(t, uuid.MustParse("10000000-0000-0000-0000-000000000001"), resp.Items[0].Id)
				require.Equal(t, "Test ConnectorJson", resp.Items[0].DisplayName)
				require.Equal(t, uuid.MustParse("20000000-0000-0000-0000-000000000002"), resp.Items[1].Id)
				require.Equal(t, "Test ConnectorJson 2", resp.Items[1].DisplayName)
			})

			t.Run("namespace filter", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors?order=id%20asc&namespace=root.child", nil, "some-actor")
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ListConnectorsResponseJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Len(t, resp.Items, 1)
				require.Equal(t, uuid.MustParse("20000000-0000-0000-0000-000000000002"), resp.Items[0].Id)
				require.Equal(t, "Test ConnectorJson 2", resp.Items[0].DisplayName)
			})
		})
	})

	t.Run("versions", func(t *testing.T) {
		t.Run("get", func(t *testing.T) {
			tu := setup(t, nil)

			t.Run("unauthorized", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := http.NewRequest(http.MethodGet, "/connectors/test-connector/versions/1", nil)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusUnauthorized, w.Code)
			})

			t.Run("malformed id", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/bad-connector/versions/1", nil, "some-actor")
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusBadRequest, w.Code)
			})

			t.Run("invalid id", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/99999999-0000-0000-0000-000000000001/versions/1", nil, "some-actor")
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNotFound, w.Code)
			})

			t.Run("invalid version", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/99999999-0000-0000-0000-000000000001/versions/999", nil, "some-actor")
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNotFound, w.Code)
			})

			t.Run("valid", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/10000000-0000-0000-0000-000000000001/versions/1", nil, "some-actor")
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ConnectorVersionJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, uuid.MustParse("10000000-0000-0000-0000-000000000001"), resp.Id)
			})
		})

		t.Run("list", func(t *testing.T) {
			tu := setup(t, nil)

			t.Run("unauthorized", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := http.NewRequest(http.MethodGet, "/connectors/10000000-0000-0000-0000-000000000001/versions", nil)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusUnauthorized, w.Code)
			})

			t.Run("valid", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/10000000-0000-0000-0000-000000000001/versions?order=id%20asc", nil, "some-actor")
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ListConnectorVersionsResponseJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Len(t, resp.Items, 1)
				require.Equal(t, uuid.MustParse("10000000-0000-0000-0000-000000000001"), resp.Items[0].Id)
			})

			t.Run("namespace filter", func(t *testing.T) {
				w := httptest.NewRecorder()
				// Namespace filter doesn't actually make sense here because versions can't change namespaces
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/10000000-0000-0000-0000-000000000001/versions?order=id%20asc&namespace=root.child", nil, "some-actor")
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ListConnectorsResponseJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Len(t, resp.Items, 0)
			})
		})
	})
}
