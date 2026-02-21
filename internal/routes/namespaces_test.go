package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/rmorlok/authproxy/internal/api_common"
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
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
		cfg, rds := apredis.MustApplyTestConfig(cfg)
		cfg, auth, authUtil := auth2.TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)
		h := httpf2.CreateFactory(cfg, rds, nil, aplog.NewNoopLogger())
		cfg, e := encrypt.NewTestEncryptService(cfg, db)
		ctrl := gomock.NewController(t)
		ac := asynqmock.NewMockClient(ctrl)
		rs := mock.NewMockClient(ctrl)
		c := core.NewCoreService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
		assert.NoError(t, c.Migrate(ctx))
		nr := NewNamespacesRoutes(cfg, auth, c)
		r := api_common.GinForTest(nil)
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
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "namespaces", "list"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("invalid path", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/namespaces/root.does-not-exist", nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/namespaces/root.dev", nil, "root", "some-actor", aschema.AllPermissions())
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
				"root",
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
				"root",
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
				"root",
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

	t.Run("create namespace", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			body := map[string]string{"path": "root.newns"}
			jsonBody, _ := json.Marshal(body)
			req, err := http.NewRequest(http.MethodPost, "/namespaces", bytes.NewReader(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			body := map[string]string{"path": "root.newns"}
			jsonBody, _ := json.Marshal(body)
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/namespaces",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "namespaces", "list"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("forbidden namespace not allowed", func(t *testing.T) {
			w := httptest.NewRecorder()
			body := map[string]string{"path": "root.restricted"}
			jsonBody, _ := json.Marshal(body)
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/namespaces",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.other.**", "namespaces", "create"), // Wrong namespace
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code) // ValidateNamespace returns bad request
		})

		t.Run("valid with create permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			body := map[string]string{"path": "root.allowed"}
			jsonBody, _ := json.Marshal(body)
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/namespaces",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "namespaces", "create"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp NamespaceJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "root.allowed", resp.Path)
			require.Equal(t, database.NamespaceStateActive, resp.State)
		})

		t.Run("valid with labels", func(t *testing.T) {
			w := httptest.NewRecorder()
			body := CreateNamespaceRequestJson{
				Path:   "root.withlabels",
				Labels: map[string]string{"env": "test", "team": "dev"},
			}
			jsonBody, _ := json.Marshal(body)
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/namespaces",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "namespaces", "create"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp NamespaceJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "root.withlabels", resp.Path)
			require.Equal(t, "test", resp.Labels["env"])
			require.Equal(t, "dev", resp.Labels["team"])
		})

		t.Run("conflict when namespace already exists", func(t *testing.T) {
			// First create the namespace
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:  "root.existing",
				State: database.NamespaceStateActive,
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			body := map[string]string{"path": "root.existing"}
			jsonBody, _ := json.Marshal(body)
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/namespaces",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusConflict, w.Code)
		})

		t.Run("bad request for invalid namespace path", func(t *testing.T) {
			w := httptest.NewRecorder()
			body := map[string]string{"path": "invalid path with spaces"}
			jsonBody, _ := json.Marshal(body)
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/namespaces",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
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
				"root",
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
				"root",
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
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/namespaces?limit=50&order=created_at%20asc&namespace=root.dev", nil, "root", "some-actor", aschema.AllPermissions())
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
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/namespaces?limit=50&order=created_at%20asc&namespace=root.dev.**", nil, "root", "some-actor", aschema.AllPermissions())
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

		t.Run("filter with label_selector", func(t *testing.T) {
			err := tu.Db.CreateNamespace(ctx, &database.Namespace{
				Path:   "root.labeled",
				State:  database.NamespaceStateActive,
				Labels: database.Labels{"env": "test-label"},
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/namespaces?label_selector=env%3Dtest-label", nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListNamespacesResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, "root.labeled", resp.Items[0].Path)
			require.Equal(t, "test-label", resp.Items[0].Labels["env"])
		})
	})

	t.Run("update namespace", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:  "root.patchns",
			State: database.NamespaceStateActive,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			body := `{"labels": {"env": "prod"}}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/namespaces/root.patchns", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			body := `{"labels": {"env": "prod"}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/namespaces/root.patchns",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "namespaces", "get"), // Wrong verb
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("namespace not found", func(t *testing.T) {
			body := `{"labels": {"env": "prod"}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/namespaces/root.nonexistent",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("bad request - invalid JSON", func(t *testing.T) {
			body := `{invalid json}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/namespaces/root.patchns",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("success - update labels", func(t *testing.T) {
			body := `{"labels": {"env": "production", "team": "backend"}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/namespaces/root.patchns",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp NamespaceJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "root.patchns", resp.Path)
			require.Equal(t, "production", resp.Labels["env"])
			require.Equal(t, "backend", resp.Labels["team"])

			// Verify in database
			ns, err := tu.Db.GetNamespace(context.Background(), "root.patchns")
			require.NoError(t, err)
			require.Equal(t, "production", ns.Labels["env"])
			require.Equal(t, "backend", ns.Labels["team"])
		})

		t.Run("success - clear labels", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:   "root.clearlabels",
				State:  database.NamespaceStateActive,
				Labels: database.Labels{"old": "value"},
			})
			require.NoError(t, err)

			body := `{"labels": {}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/namespaces/root.clearlabels",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp NamespaceJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Empty(t, resp.Labels)

			// Verify in database
			ns, err := tu.Db.GetNamespace(context.Background(), "root.clearlabels")
			require.NoError(t, err)
			require.Empty(t, ns.Labels)
		})

		t.Run("success - labels unchanged when not provided", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:   "root.unchangedlabels",
				State:  database.NamespaceStateActive,
				Labels: database.Labels{"old": "value"},
			})
			require.NoError(t, err)

			body := `{}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/namespaces/root.unchangedlabels",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp NamespaceJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, map[string]string{"old": "value"}, resp.Labels)

			// Verify in database
			ns, err := tu.Db.GetNamespace(context.Background(), "root.unchangedlabels")
			require.NoError(t, err)
			require.Equal(t, database.Labels{"old": "value"}, ns.Labels)
		})

		t.Run("success - replaces labels entirely", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:   "root.replacelabels",
				State:  database.NamespaceStateActive,
				Labels: database.Labels{"old-key": "old-value", "another": "label"},
			})
			require.NoError(t, err)

			body := `{"labels": {"new-key": "new-value"}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/namespaces/root.replacelabels",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp NamespaceJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Labels, 1)
			require.Equal(t, "new-value", resp.Labels["new-key"])

			// Verify old labels are gone
			ns, err := tu.Db.GetNamespace(context.Background(), "root.replacelabels")
			require.NoError(t, err)
			require.Len(t, ns.Labels, 1)
			require.Equal(t, "new-value", ns.Labels["new-key"])
			_, exists := ns.Labels["old-key"]
			require.False(t, exists)
		})
	})

	t.Run("get labels", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		// Create a namespace with labels
		err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:   "root.labeled",
			State:  database.NamespaceStateActive,
			Labels: database.Labels{"env": "prod", "team": "backend"},
		})
		require.NoError(t, err)

		// Create a namespace without labels
		err = tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:  "root.nolabels",
			State: database.NamespaceStateActive,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/namespaces/root.labeled/labels", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces/root.nonexistent/labels",
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
				"/namespaces/root.labeled/labels",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "prod", resp["env"])
			require.Equal(t, "backend", resp["team"])
		})

		t.Run("success - empty labels", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces/root.nolabels/labels",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Empty(t, resp)
		})
	})

	t.Run("get label", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		// Create a namespace with labels
		err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:   "root.labeled",
			State:  database.NamespaceStateActive,
			Labels: database.Labels{"env": "staging"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/namespaces/root.labeled/labels/env", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("namespace not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces/root.nonexistent/labels/env",
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
				"/namespaces/root.labeled/labels/nonexistent",
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
				"/namespaces/root.labeled/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp NamespaceLabelJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "env", resp.Key)
			require.Equal(t, "staging", resp.Value)
		})
	})

	t.Run("put label", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:  "root.putlabel",
			State: database.NamespaceStateActive,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/namespaces/root.putlabel/labels/env", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/namespaces/root.putlabel/labels/env",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "namespaces", "get"), // Wrong verb
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("namespace not found", func(t *testing.T) {
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/namespaces/root.nonexistent/labels/env",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("bad request - invalid JSON", func(t *testing.T) {
			body := `{invalid json}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/namespaces/root.putlabel/labels/env",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("success - add new label", func(t *testing.T) {
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/namespaces/root.putlabel/labels/env",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp NamespaceLabelJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "env", resp.Key)
			require.Equal(t, "production", resp.Value)

			// Verify in database
			ns, err := tu.Db.GetNamespace(context.Background(), "root.putlabel")
			require.NoError(t, err)
			require.Equal(t, "production", ns.Labels["env"])
		})

		t.Run("success - update existing label", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:   "root.updatelabel",
				State:  database.NamespaceStateActive,
				Labels: database.Labels{"version": "v1"},
			})
			require.NoError(t, err)

			body := `{"value": "v2"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/namespaces/root.updatelabel/labels/version",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp NamespaceLabelJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "version", resp.Key)
			require.Equal(t, "v2", resp.Value)

			// Verify in database
			ns, err := tu.Db.GetNamespace(context.Background(), "root.updatelabel")
			require.NoError(t, err)
			require.Equal(t, "v2", ns.Labels["version"])
		})

		t.Run("success - preserves other labels", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:   "root.preservelabels",
				State:  database.NamespaceStateActive,
				Labels: database.Labels{"env": "dev", "team": "platform"},
			})
			require.NoError(t, err)

			body := `{"value": "staging"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/namespaces/root.preservelabels/labels/env",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			// Verify both labels in database
			ns, err := tu.Db.GetNamespace(context.Background(), "root.preservelabels")
			require.NoError(t, err)
			require.Equal(t, "staging", ns.Labels["env"])
			require.Equal(t, "platform", ns.Labels["team"])
		})
	})

	t.Run("delete label", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		// Create a namespace with labels
		err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:   "root.deletelabel",
			State:  database.NamespaceStateActive,
			Labels: database.Labels{"env": "prod", "team": "backend"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/namespaces/root.deletelabel/labels/env", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/namespaces/root.deletelabel/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "namespaces", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("namespace not found returns 204", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/namespaces/root.nonexistent/labels/env",
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
				"/namespaces/root.deletelabel/labels/nonexistent",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("success - delete label", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:   "root.deleteone",
				State:  database.NamespaceStateActive,
				Labels: database.Labels{"to-delete": "value", "to-keep": "value2"},
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/namespaces/root.deleteone/labels/to-delete",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			// Verify the label is deleted but other labels remain
			ns, err := tu.Db.GetNamespace(context.Background(), "root.deleteone")
			require.NoError(t, err)
			_, exists := ns.Labels["to-delete"]
			require.False(t, exists)
			require.Equal(t, "value2", ns.Labels["to-keep"])
		})

		t.Run("success - delete is idempotent", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:   "root.idempotent",
				State:  database.NamespaceStateActive,
				Labels: database.Labels{"label": "value"},
			})
			require.NoError(t, err)

			// Delete the label twice
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					"/namespaces/root.idempotent/labels/label",
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
			ns, err := tu.Db.GetNamespace(context.Background(), "root.idempotent")
			require.NoError(t, err)
			_, exists := ns.Labels["label"]
			require.False(t, exists)
		})
	})
}
