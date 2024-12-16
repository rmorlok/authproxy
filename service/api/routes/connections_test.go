package routes

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	auth2 "github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
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

	setup := func(t *testing.T, cfg config.C) *TestSetup {
		cfg, db := database.MustApplyBlankTestDbConfig(t.Name(), cfg)
		cfg, auth, authUtil := auth2.TestAuthService(config.ServiceIdApi, cfg)
		cr := NewConnectionsRoutes(cfg, auth, db)
		r := gin.Default()
		cr.Register(r)

		return &TestSetup{
			Gin:      r,
			Cfg:      cfg,
			AuthUtil: authUtil,
			Db:       db,
		}
	}

	t.Run("get connection", func(t *testing.T) {
		tu := setup(t, nil)
		u := uuid.New()
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{ID: u, State: database.ConnectionStateCreated})
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
