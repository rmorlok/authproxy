package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apgin"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	asynqmock "github.com/rmorlok/authproxy/internal/apasynq/mock"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/routes/key_value"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func namespaceCreateRequest(path string) CreateNamespaceRequestJson {
	resource, err := nschema.NewNamespaceForPath(path)
	if err != nil {
		panic(err)
	}
	return *resource
}

func namespacePatchBody(labels, annotations map[string]string) string {
	patch := nschema.NewNamespacePatch()
	if labels != nil {
		patch.Metadata.Labels = &labels
	}
	if annotations != nil {
		patch.Metadata.Annotations = &annotations
	}
	data, err := json.Marshal(patch)
	if err != nil {
		panic(err)
	}
	return string(data)
}

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
		// Namespace label changes enqueue a propagation task. The route-level
		// tests are not interested in the asynq side; allow any number of
		// enqueue calls and let them succeed silently.
		ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, nil)
		rs := mock.NewMockClient(ctrl)
		c := core.NewCoreService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
		assert.NoError(t, c.Migrate(ctx))
		nr := NewNamespacesRoutes(cfg, auth, c)
		r := apgin.ForTest(nil)
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
			require.Equal(t, "root.dev", resp.Metadata.ID)
			require.Equal(t, "dev", string(resp.Metadata.Name))
			require.Equal(t, string(database.NamespaceStateActive), string(resp.Status.State))
		})

		t.Run("root omits parent namespace", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces/root",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			var resp NamespaceJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, nschema.Root, resp.Metadata.ID)
			require.Equal(t, nschema.Root, string(resp.Metadata.Name))
			require.Empty(t, resp.Metadata.Namespace)
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
			require.Equal(t, "root.dev", resp.Metadata.ID)
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
			require.Equal(t, "root.prod", resp.Metadata.ID)
		})
	})

	t.Run("create namespace", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			body := namespaceCreateRequest("root.newns")
			jsonBody, _ := json.Marshal(body)
			req, err := http.NewRequest(http.MethodPost, "/namespaces", bytes.NewReader(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			body := namespaceCreateRequest("root.newns")
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
			body := namespaceCreateRequest("root.restricted")
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
			body := namespaceCreateRequest("root.allowed")
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
			require.Equal(t, "root.allowed", resp.Metadata.ID)
			require.Equal(t, "allowed", string(resp.Metadata.Name))
			require.Equal(t, "root", resp.Metadata.Namespace)
			require.Equal(t, "allowed", resp.Metadata.Labels["apxy/ns/-/name"])
			require.Equal(t, string(database.NamespaceStateActive), string(resp.Status.State))
		})

		t.Run("valid with labels", func(t *testing.T) {
			w := httptest.NewRecorder()
			body := namespaceCreateRequest("root.withlabels")
			body.Metadata.Labels = map[string]string{"env": "test", "team": "dev"}
			body.Metadata.Annotations = map[string]string{"example.com/owner": "platform"}
			body.Spec.EncryptionKeyRef = &meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       nschema.EncryptionKeyKind,
				Namespace:  "root",
				Name:       "key_global",
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
			require.Equal(t, "root.withlabels", resp.Metadata.ID)
			require.Equal(t, "test", resp.Metadata.Labels["env"])
			require.Equal(t, "dev", resp.Metadata.Labels["team"])
			require.Equal(t, "platform", resp.Metadata.Annotations["example.com/owner"])
			require.Equal(t, database.GlobalKeyID.String(), resp.Spec.EncryptionKeyRef.ID)
		})

		t.Run("rejects apxy/-prefixed labels in request body", func(t *testing.T) {
			w := httptest.NewRecorder()
			body := namespaceCreateRequest("root.apxy-blocked")
			body.Metadata.Labels = map[string]string{"apxy/cxr/source": "config"}
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
			require.Equal(t, http.StatusBadRequest, w.Code, "API must reject apxy/-prefixed labels at the user-input boundary")
			require.Contains(t, w.Body.String(), "reserved")
		})

		t.Run("conflict when namespace already exists", func(t *testing.T) {
			// First create the namespace
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:  "root.existing",
				State: database.NamespaceStateActive,
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			body := namespaceCreateRequest("root.existing")
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
			body := nschema.NewNamespace()
			body.Metadata.Name = "invalid path with spaces"
			body.Metadata.Namespace = "root"
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

		err = tu.Db.CreateNamespace(ctx, &database.Namespace{
			Path:  "root.prod.old",
			State: database.NamespaceStateActive,
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
			require.Len(t, resp.Items, 5)
		})

		t.Run("bad request for invalid children_of namespace", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces?childrenOf=%2F%2F%2Faction%2Frefresh",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("bad request for invalid namespace matcher", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces?namespace=%2F%2F%2Faction%2Frefresh",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("permission constrained namespace dropdown", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces?limit=50&order=created_at%20asc",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.dev.**", "namespaces", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListNamespacesResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 2)
			require.Equal(t, "root.dev", resp.Items[0].Metadata.ID)
			require.Equal(t, "root.dev.old", resp.Items[1].Metadata.ID)
		})

		t.Run("filter by derived name preserves pagination and authorization", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet, "/namespaces?name=old&limit=1", nil,
				"root", "some-actor", aschema.AllPermissions(),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			var first ListNamespacesResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &first))
			require.Len(t, first.Items, 1)
			require.Equal(t, "old", string(first.Items[0].Metadata.Name))
			require.NotEmpty(t, first.Metadata.Continue)

			w = httptest.NewRecorder()
			req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet, "/namespaces?cursor="+url.QueryEscape(first.Metadata.Continue), nil,
				"root", "some-actor", aschema.AllPermissions(),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var second ListNamespacesResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &second))
			require.Len(t, second.Items, 1)
			require.Equal(t, "old", string(second.Items[0].Metadata.Name))
			require.NotEqual(t, first.Items[0].Metadata.ID, second.Items[0].Metadata.ID)

			w = httptest.NewRecorder()
			req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet, "/namespaces?name=old", nil,
				"root", "some-actor", aschema.PermissionsSingle("root.dev.**", "namespaces", "list"),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var authorized ListNamespacesResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &authorized))
			require.Len(t, authorized.Items, 1)
			require.Equal(t, "root.dev.old", authorized.Items[0].Metadata.ID)
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
			require.Equal(t, resp.Items[0].Metadata.ID, "root.dev")
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
			require.Equal(t, resp.Items[0].Metadata.ID, "root.dev")
			require.Equal(t, resp.Items[1].Metadata.ID, "root.dev.old")
		})

		t.Run("filter with label_selector", func(t *testing.T) {
			err := tu.Db.CreateNamespace(ctx, &database.Namespace{
				Path:   "root.labeled",
				State:  database.NamespaceStateActive,
				Labels: database.Labels{"env": "test-label"},
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/namespaces?labelSelector=env%3Dtest-label", nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListNamespacesResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, "root.labeled", resp.Items[0].Metadata.ID)
			require.Equal(t, "test-label", resp.Items[0].Metadata.Labels["env"])
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
			body := namespacePatchBody(map[string]string{"env": "prod"}, nil)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/namespaces/root.patchns", bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			body := namespacePatchBody(map[string]string{"env": "prod"}, nil)
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
			body := namespacePatchBody(map[string]string{"env": "prod"}, nil)
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

		t.Run("bad request - missing or null patch fields", func(t *testing.T) {
			tests := []struct {
				name string
				body string
			}{
				{
					name: "missing metadata",
					body: `{"apiVersion":"authproxy.net/v1alpha1","kind":"Namespace","spec":{}}`,
				},
				{
					name: "missing spec",
					body: `{"apiVersion":"authproxy.net/v1alpha1","kind":"Namespace","metadata":{}}`,
				},
				{
					name: "null sections",
					body: `{"apiVersion":"authproxy.net/v1alpha1","kind":"Namespace","metadata":null,"spec":null}`,
				},
				{
					name: "null encryption key reference",
					body: `{"apiVersion":"authproxy.net/v1alpha1","kind":"Namespace","metadata":{},"spec":{"encryptionKeyRef":null}}`,
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					w := httptest.NewRecorder()
					req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
						http.MethodPatch,
						"/namespaces/root.patchns",
						bytes.NewBufferString(test.body),
						"root",
						"some-actor",
						aschema.AllPermissions(),
					)
					require.NoError(t, err)
					req.Header.Set("Content-Type", "application/json")

					tu.Gin.ServeHTTP(w, req)
					require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
				})
			}
		})

		t.Run("bad request - immutable identity", func(t *testing.T) {
			otherID := "root.other"
			patch := nschema.NewNamespacePatch()
			patch.Metadata.ID = &otherID
			body, err := json.Marshal(patch)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/namespaces/root.patchns",
				bytes.NewReader(body),
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
			body := namespacePatchBody(map[string]string{"env": "production", "team": "backend"}, nil)
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
			require.Equal(t, "root.patchns", resp.Metadata.ID)
			require.Equal(t, "patchns", string(resp.Metadata.Name))
			require.Equal(t, "production", resp.Metadata.Labels["env"])
			require.Equal(t, "backend", resp.Metadata.Labels["team"])

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

			body := namespacePatchBody(map[string]string{}, nil)
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
			respUser, _ := database.SplitUserAndApxyLabels(database.Labels(resp.Metadata.Labels))
			require.Empty(t, respUser)

			// Verify in database (user portion only).
			ns, err := tu.Db.GetNamespace(context.Background(), "root.clearlabels")
			require.NoError(t, err)
			nsUser, _ := database.SplitUserAndApxyLabels(ns.Labels)
			require.Empty(t, nsUser)
		})

		t.Run("success - labels unchanged when not provided", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:   "root.unchangedlabels",
				State:  database.NamespaceStateActive,
				Labels: database.Labels{"old": "value"},
			})
			require.NoError(t, err)

			body := namespacePatchBody(nil, nil)
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
			respUser, _ := database.SplitUserAndApxyLabels(database.Labels(resp.Metadata.Labels))
			require.Equal(t, database.Labels{"old": "value"}, respUser)

			// Verify in database (user portion only).
			ns, err := tu.Db.GetNamespace(context.Background(), "root.unchangedlabels")
			require.NoError(t, err)
			nsUser, _ := database.SplitUserAndApxyLabels(ns.Labels)
			require.Equal(t, database.Labels{"old": "value"}, nsUser)
		})

		t.Run("success - replaces labels entirely", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:   "root.replacelabels",
				State:  database.NamespaceStateActive,
				Labels: database.Labels{"old-key": "old-value", "another": "label"},
			})
			require.NoError(t, err)

			body := namespacePatchBody(map[string]string{"new-key": "new-value"}, nil)
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
			respUser, _ := database.SplitUserAndApxyLabels(database.Labels(resp.Metadata.Labels))
			require.Len(t, respUser, 1)
			require.Equal(t, "new-value", respUser["new-key"])

			// Verify old labels are gone (user portion only).
			ns, err := tu.Db.GetNamespace(context.Background(), "root.replacelabels")
			require.NoError(t, err)
			nsUser, _ := database.SplitUserAndApxyLabels(ns.Labels)
			require.Len(t, nsUser, 1)
			require.Equal(t, "new-value", nsUser["new-key"])
			_, exists := nsUser["old-key"]
			require.False(t, exists)
		})
	})

	t.Run("namespace key resource", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:  "root.key-resource",
			State: database.NamespaceStateActive,
		})
		require.NoError(t, err)

		patch := nschema.NewNamespacePatch()
		patch.Spec.EncryptionKeyRef = &meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       "Key",
			Namespace:  "root",
			Name:       "key_global",
		}
		body, err := json.Marshal(patch)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPut,
			"/namespaces/root.key-resource/key",
			bytes.NewReader(body),
			"root",
			"some-actor",
			aschema.AllPermissions(),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var updated NamespaceJson
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
		require.Equal(t, meta.APIVersionV1Alpha1, updated.APIVersion)
		require.Equal(t, nschema.NamespaceKind, updated.Kind)
		require.Equal(t, "root.key-resource", updated.Metadata.ID)
		require.Equal(t, database.GlobalKeyID.String(), updated.Spec.EncryptionKeyRef.ID)

		w = httptest.NewRecorder()
		req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodGet,
			"/namespaces/root.key-resource/key",
			nil,
			"root",
			"some-actor",
			aschema.AllPermissions(),
		)
		require.NoError(t, err)
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var fetched NamespaceJson
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &fetched))
		require.Equal(t, database.GlobalKeyID.String(), fetched.Spec.EncryptionKeyRef.ID)

		otherKeyID := apid.New(apid.PrefixKey)
		err = tu.Db.CreateKey(context.Background(), &database.Key{
			Id:        otherKeyID,
			Namespace: "root",
			Name:      "other",
		})
		require.NoError(t, err)

		patch.Spec.EncryptionKeyRef = &meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       nschema.EncryptionKeyKind,
			ID:         database.GlobalKeyID.String(),
			Namespace:  "root",
			Name:       "other",
		}
		body, err = json.Marshal(patch)
		require.NoError(t, err)
		w = httptest.NewRecorder()
		req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPut,
			"/namespaces/root.key-resource/key",
			bytes.NewReader(body),
			"root",
			"some-actor",
			aschema.AllPermissions(),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
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
			respUser, _ := database.SplitUserAndApxyLabels(database.Labels(resp))
			require.Empty(t, respUser)
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

			var resp key_value.KeyValueJson
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

			var resp key_value.KeyValueJson
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

			var resp key_value.KeyValueJson
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

	t.Run("update namespace with annotations", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		t.Run("success - update annotations", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:  "root.patchannot",
				State: database.NamespaceStateActive,
			})
			require.NoError(t, err)

			body := namespacePatchBody(nil, map[string]string{"description": "my namespace", "owner": "teamA"})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/namespaces/root.patchannot",
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
			require.Equal(t, "root.patchannot", resp.Metadata.ID)
			require.Equal(t, "my namespace", resp.Metadata.Annotations["description"])
			require.Equal(t, "teamA", resp.Metadata.Annotations["owner"])

			// Verify in database
			ns, err := tu.Db.GetNamespace(context.Background(), "root.patchannot")
			require.NoError(t, err)
			require.Equal(t, "my namespace", ns.Annotations["description"])
			require.Equal(t, "teamA", ns.Annotations["owner"])
		})

		t.Run("success - annotations unchanged when not provided", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:        "root.unchangedannot",
				State:       database.NamespaceStateActive,
				Annotations: database.Annotations{"old": "value"},
			})
			require.NoError(t, err)

			body := namespacePatchBody(nil, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/namespaces/root.unchangedannot",
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
			require.Equal(t, map[string]string{"old": "value"}, resp.Metadata.Annotations)

			// Verify in database
			ns, err := tu.Db.GetNamespace(context.Background(), "root.unchangedannot")
			require.NoError(t, err)
			require.Equal(t, database.Annotations{"old": "value"}, ns.Annotations)
		})

		t.Run("success - replaces annotations entirely", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:        "root.replaceannot",
				State:       database.NamespaceStateActive,
				Annotations: database.Annotations{"old-key": "old-value", "another": "annotation"},
			})
			require.NoError(t, err)

			body := namespacePatchBody(nil, map[string]string{"new-key": "new-value"})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/namespaces/root.replaceannot",
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
			require.Len(t, resp.Metadata.Annotations, 1)
			require.Equal(t, "new-value", resp.Metadata.Annotations["new-key"])

			// Verify old annotations are gone
			ns, err := tu.Db.GetNamespace(context.Background(), "root.replaceannot")
			require.NoError(t, err)
			require.Len(t, ns.Annotations, 1)
			require.Equal(t, "new-value", ns.Annotations["new-key"])
			_, exists := ns.Annotations["old-key"]
			require.False(t, exists)
		})
	})

	t.Run("get annotations", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		// Create a namespace with annotations
		err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:        "root.annotated",
			State:       database.NamespaceStateActive,
			Annotations: database.Annotations{"description": "test ns", "owner": "teamB"},
		})
		require.NoError(t, err)

		// Create a namespace without annotations
		err = tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:  "root.noannots",
			State: database.NamespaceStateActive,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/namespaces/root.annotated/annotations", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces/root.nonexistent/annotations",
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
				"/namespaces/root.annotated/annotations",
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
			require.Equal(t, "test ns", resp["description"])
			require.Equal(t, "teamB", resp["owner"])
		})

		t.Run("success - empty annotations", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces/root.noannots/annotations",
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

	t.Run("get annotation", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		// Create a namespace with annotations
		err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:        "root.annotated",
			State:       database.NamespaceStateActive,
			Annotations: database.Annotations{"env": "staging"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/namespaces/root.annotated/annotations/env", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("namespace not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces/root.nonexistent/annotations/env",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("annotation not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/namespaces/root.annotated/annotations/nonexistent",
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
				"/namespaces/root.annotated/annotations/env",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp key_value.KeyValueJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "env", resp.Key)
			require.Equal(t, "staging", resp.Value)
		})
	})

	t.Run("put annotation", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:  "root.putannot",
			State: database.NamespaceStateActive,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/namespaces/root.putannot/annotations/env", bytes.NewBufferString(body))
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
				"/namespaces/root.putannot/annotations/env",
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
				"/namespaces/root.nonexistent/annotations/env",
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
				"/namespaces/root.putannot/annotations/env",
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

		t.Run("success - add new annotation", func(t *testing.T) {
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/namespaces/root.putannot/annotations/env",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp key_value.KeyValueJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "env", resp.Key)
			require.Equal(t, "production", resp.Value)

			// Verify in database
			ns, err := tu.Db.GetNamespace(context.Background(), "root.putannot")
			require.NoError(t, err)
			require.Equal(t, "production", ns.Annotations["env"])
		})

		t.Run("success - update existing annotation", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:        "root.updateannot",
				State:       database.NamespaceStateActive,
				Annotations: database.Annotations{"version": "v1"},
			})
			require.NoError(t, err)

			body := `{"value": "v2"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/namespaces/root.updateannot/annotations/version",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp key_value.KeyValueJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "version", resp.Key)
			require.Equal(t, "v2", resp.Value)

			// Verify in database
			ns, err := tu.Db.GetNamespace(context.Background(), "root.updateannot")
			require.NoError(t, err)
			require.Equal(t, "v2", ns.Annotations["version"])
		})

		t.Run("success - preserves other annotations", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:        "root.preserveannot",
				State:       database.NamespaceStateActive,
				Annotations: database.Annotations{"env": "dev", "team": "platform"},
			})
			require.NoError(t, err)

			body := `{"value": "staging"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/namespaces/root.preserveannot/annotations/env",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			// Verify both annotations in database
			ns, err := tu.Db.GetNamespace(context.Background(), "root.preserveannot")
			require.NoError(t, err)
			require.Equal(t, "staging", ns.Annotations["env"])
			require.Equal(t, "platform", ns.Annotations["team"])
		})
	})

	t.Run("delete annotation", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		// Create a namespace with annotations
		err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
			Path:        "root.deleteannot",
			State:       database.NamespaceStateActive,
			Annotations: database.Annotations{"env": "prod", "team": "backend"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/namespaces/root.deleteannot/annotations/env", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/namespaces/root.deleteannot/annotations/env",
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
				"/namespaces/root.nonexistent/annotations/env",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("success - delete annotation", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:        "root.deleteoneannot",
				State:       database.NamespaceStateActive,
				Annotations: database.Annotations{"to-delete": "value", "to-keep": "value2"},
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/namespaces/root.deleteoneannot/annotations/to-delete",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			// Verify the annotation is deleted but other annotations remain
			ns, err := tu.Db.GetNamespace(context.Background(), "root.deleteoneannot")
			require.NoError(t, err)
			_, exists := ns.Annotations["to-delete"]
			require.False(t, exists)
			require.Equal(t, "value2", ns.Annotations["to-keep"])
		})

		t.Run("success - delete is idempotent", func(t *testing.T) {
			err := tu.Db.CreateNamespace(context.Background(), &database.Namespace{
				Path:        "root.idempotentannot",
				State:       database.NamespaceStateActive,
				Annotations: database.Annotations{"annotation": "value"},
			})
			require.NoError(t, err)

			// Delete the annotation twice
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					"/namespaces/root.idempotentannot/annotations/annotation",
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNoContent, w.Code)
			}

			// Verify the annotation is deleted
			ns, err := tu.Db.GetNamespace(context.Background(), "root.idempotentannot")
			require.NoError(t, err)
			_, exists := ns.Annotations["annotation"]
			require.False(t, exists)
		})
	})
}
