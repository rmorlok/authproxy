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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestNamespaces(t *testing.T) {
	type TestSetup struct {
		Gin      *gin.Engine
		Cfg      config.C
		AuthUtil *auth2.AuthTestUtil
		Db       database.DB
	}

	setup := func(t *testing.T, ctx context.Context, cfg config.C) (*TestSetup, func()) {
		cfg = config.FromRoot(&sconfig.Root{
			Connectors: &sconfig.Connectors{
				LoadFromList: []sconfig.Connector{},
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
		assert.NoError(t, c.Migrate(ctx))
		nr := NewNamespacesRoutes(cfg, auth, c)
		r := gin.New()
		nr.Register(r)

		return &TestSetup{
				Gin:      r,
				Cfg:      cfg,
				AuthUtil: authUtil,
				Db:       db,
			}, func() {
				ctrl.Finish()
			}
	}

	t.Run("get namespace", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		// Root namespace is automatically created as part of migration with config
		//err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
		//	Path:  sconfig.RootNamespace,
		//	State: database.NamespaceStateActive,
		//})
		//require.NoError(t, err)

		err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:  "root.dev",
			State: database.NamespaceStateActive,
		})
		require.NoError(t, err)

		err = tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:  "root.prod",
			State: database.NamespaceStateActive,
		})
		require.NoError(t, err)

		err = tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:  "root.old",
			State: database.NamespaceStateDestroyed,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/namespaces/root", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces/root",
				nil,
				"some-actor",
				aschema.PermissionsSingle("root.**", "namespaces", "list"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("invalid path", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/namespaces/root.does-not-exist", nil, "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/namespaces/root.dev", nil, "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp NamespaceJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "root.dev", resp.Path)
			require.Equal(t, database.NamespaceStateActive, resp.State)
		})

		t.Run("allowed with matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces/root.dev",
				nil,
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "namespaces", "get", "root.dev"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp NamespaceJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "root.dev", resp.Path)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces/root.prod",
				nil,
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "namespaces", "get", "root.dev"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("allowed with multiple resource ids including target", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces/root.prod",
				nil,
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "namespaces", "get", "root.dev", "root.prod"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp NamespaceJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "root.prod", resp.Path)
		})
	})

	t.Run("list namespaces", func(t *testing.T) {
		now := time.Now()
		c := clock.NewFakeClock(now)
		ctx := apctx.WithClock(context.Background(), c)

		tu, done := setup(t, ctx, nil)
		defer done()

		// Root namespace is automatically created as part of migration with config
		//err := tu.Db.CreateNamespace(ctx, &database.Namespace{
		//	Path:  sconfig.RootNamespace,
		//	State: database.NamespaceStateActive,
		//})
		//require.NoError(t, err)

		now = now.Add(time.Second)
		c.SetTime(now)
		err := tu.Db.CreateNamespace(ctx, &database.Namespace{
			Path:  "root.dev",
			State: database.NamespaceStateActive,
		})
		require.NoError(t, err)

		now = now.Add(time.Second)
		c.SetTime(now)
		err = tu.Db.CreateNamespace(ctx, &database.Namespace{
			Path:  "root.prod",
			State: database.NamespaceStateActive,
		})
		require.NoError(t, err)

		now = now.Add(time.Second)
		c.SetTime(now)
		err = tu.Db.CreateNamespace(ctx, &database.Namespace{
			Path:  "root.dev.old",
			State: database.NamespaceStateDestroyed,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/namespaces", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces?limit=50&order=created_at%20asc",
				nil,
				"some-actor",
				aschema.PermissionsSingle("root.**", "namespaces", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces?limit=50&order=created_at%20asc",
				nil,
				"some-actor",
				aschema.PermissionsSingle("root.**", "namespaces", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListNamespacesResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 4)
		})

		t.Run("filter to namespace", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/namespaces?limit=50&order=created_at%20asc&namespace=root.dev", nil, "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListNamespacesResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, resp.Items[0].Path, "root.dev")
		})

		t.Run("filter to namespace matcher", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/namespaces?limit=50&order=created_at%20asc&namespace=root.dev.**", nil, "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListNamespacesResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 2)
			require.Equal(t, resp.Items[0].Path, "root.dev")
			require.Equal(t, resp.Items[1].Path, "root.dev.old")
		})
	})
}
