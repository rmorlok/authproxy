package routes

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	auth2 "github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
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
			cfg = config.FromRoot(&config.Root{})
		}

		root := cfg.GetRoot()
		if root == nil {
			panic("No root in config")
		}

		if len(root.Connectors) == 0 {
			root.Connectors = []config.Connector{
				{
					Id:          "test-connector",
					DisplayName: "Test ConnectorJson",
				},
			}
		}

		cfg, auth, authUtil := auth2.TestAuthService(t, config.ServiceIdApi, cfg)
		cr := NewConnectorsRoutes(cfg, auth)
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

		t.Run("invalid id", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodGet, "/connectors/bad-connector", nil, "some-actor")
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorId(http.MethodGet, "/connectors/test-connector", nil, "some-actor")
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectorJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "test-connector", resp.Id)
			require.Equal(t, "Test ConnectorJson", resp.DisplayName)
		})
	})
}
