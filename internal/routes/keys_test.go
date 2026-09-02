package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/redis/go-redis/v9"
	asynqmock "github.com/rmorlok/authproxy/internal/apasynq/mock"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httperr"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/routes/key_value"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func managedKeyCreateBody(
	namespace, name string,
	keyData map[string]any,
	labels map[string]string,
) map[string]any {
	metadata := map[string]any{"namespace": namespace}
	if name != "" {
		metadata["name"] = name
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	return map[string]any{
		"apiVersion": "authproxy.net/v1alpha1",
		"kind":       "Key",
		"metadata":   metadata,
		"spec":       map[string]any{"keyData": keyData},
	}
}

func managedKeyPatchBody(metadata, spec map[string]any) string {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if spec == nil {
		spec = map[string]any{}
	}
	data, err := json.Marshal(map[string]any{
		"apiVersion": "authproxy.net/v1alpha1",
		"kind":       "Key",
		"metadata":   metadata,
		"spec":       spec,
	})
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestKeys(t *testing.T) {
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

		// Allow fire-and-forget calls from EnqueueForceSyncKeysToDatabase
		rs.EXPECT().Del(gomock.Any(), gomock.Any()).Return(redis.NewIntCmd(context.Background())).AnyTimes()
		ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

		c := core.NewCoreService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
		require.NoError(t, c.Migrate(ctx))
		ekr := NewKeysRoutes(cfg, auth, c)
		r := apgin.ForTest(nil)
		ekr.Register(r)

		return &TestSetup{
				Gin:      r,
				Cfg:      cfg,
				AuthUtil: authUtil,
				Db:       db,
			}, func() {
				ctrl.Finish()
			}
	}

	// Helper to create an key via the API and return its ID
	createKey := func(t *testing.T, tu *TestSetup, namespace string, labels map[string]string) keyschema.Key {
		body := managedKeyCreateBody(namespace, "", map[string]any{
			"value": "test-key-data-value",
		}, labels)
		jsonBody, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPost,
			"/keys",
			bytes.NewReader(jsonBody),
			"root",
			"some-actor",
			aschema.AllPermissions(),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp keyschema.Key
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotContains(t, w.Body.String(), "test-key-data-value")
		return resp
	}

	t.Run("get key", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", map[string]string{"env": "test"})

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/keys/%s", created.GetId()), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s", created.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "keys", "list"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			fakeId := apid.New(apid.PrefixKey)
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s", fakeId),
				nil,
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
				http.MethodGet,
				fmt.Sprintf("/keys/%s", created.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp keyschema.Key
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, created.GetId(), resp.GetId())
			require.Equal(t, "root", resp.Metadata.Namespace)
			require.Equal(t, keyschema.KeyStateActive, resp.Status.State)
			require.NotNil(t, resp.Spec.KeyData)
			require.Equal(t, "test", resp.Metadata.Labels["env"])
		})

		t.Run("global key omits configuration-backed key data", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s", database.GlobalKeyID),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp keyschema.Key
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, database.GlobalKeyID, resp.GetId())
			require.Nil(t, resp.Spec.KeyData)
		})

		t.Run("redacts key data by default", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s", created.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "keys", "get"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, "true", w.Header().Get(apserde.RedactedHeader))

			var raw map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
			spec := raw["spec"].(map[string]any)
			keyData := spec["keyData"].(map[string]any)
			require.Equal(t, strings.Repeat("*", len("test-key-data-value")), keyData["value"])
		})

		t.Run("never replays managed key material", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s", created.GetId()),
				nil,
				"root",
				"some-actor",
				[]aschema.Permission{
					{
						Namespace: "root.**",
						Resources: []string{"keys"},
						Verbs:     []string{"get"},
					},
					{
						Namespace: "root.**",
						Resources: []string{"secrets"},
						Verbs:     []string{"replay"},
					},
				},
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.NotContains(t, w.Body.String(), "test-key-data-value")
			require.Contains(t, w.Body.String(), strings.Repeat("*", len("test-key-data-value")))
		})

		t.Run("allowed with matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s", created.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "keys", "get", string(created.GetId())),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			fakeId := apid.New(apid.PrefixKey)
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s", created.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "keys", "get", string(fakeId)),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})
	})

	t.Run("create key", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		t.Run("unauthorized", func(t *testing.T) {
			body := managedKeyCreateBody("root", "", map[string]any{"value": "test-key"}, nil)
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/keys", bytes.NewReader(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden wrong verb", func(t *testing.T) {
			body := managedKeyCreateBody("root", "", map[string]any{"value": "test-key"}, nil)
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/keys",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "keys", "list"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad request - missing namespace", func(t *testing.T) {
			body := managedKeyCreateBody("", "", map[string]any{"value": "test-key"}, nil)
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/keys",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("bad request - server-owned fields", func(t *testing.T) {
			body := managedKeyCreateBody("root", "", map[string]any{"value": "test-key"}, nil)
			body["metadata"].(map[string]any)["id"] = "key_test550e8400abcd"
			body["metadata"].(map[string]any)["createdAt"] = "2026-09-01T12:00:00Z"
			body["status"] = map[string]any{"state": "active", "keyDataConfigured": true}
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/keys",
				util.JsonToReader(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), "server-owned")
		})

		t.Run("forbidden namespace not allowed", func(t *testing.T) {
			body := managedKeyCreateBody("root.restricted", "", map[string]any{"value": "test-key"}, nil)
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/keys",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.other.**", "keys", "create"), // Wrong namespace
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code) // ValidateNamespace returns bad request
		})

		t.Run("valid with labels", func(t *testing.T) {
			created := createKey(t, tu, "root", map[string]string{"env": "prod", "team": "backend"})
			require.True(t, created.GetId().HasPrefix(apid.PrefixKey))
			require.Equal(t, "root", created.Metadata.Namespace)
			require.Equal(t, keyschema.KeyStateActive, created.Status.State)
			require.Equal(t, "prod", created.Metadata.Labels["env"])
			require.Equal(t, "backend", created.Metadata.Labels["team"])
		})

		t.Run("valid without labels", func(t *testing.T) {
			created := createKey(t, tu, "root", nil)
			require.True(t, created.GetId().HasPrefix(apid.PrefixKey))
			require.Equal(t, "root", created.Metadata.Namespace)
			require.Equal(t, keyschema.KeyStateActive, created.Status.State)
		})
	})

	t.Run("list keys", func(t *testing.T) {
		now := time.Now()
		c := clock.NewFakeClock(now)
		ctx := apctx.WithClock(context.Background(), c)

		tu, done := setup(t, ctx, nil)
		defer done()

		// Create several keys
		now = now.Add(time.Second)
		c.SetTime(now)
		key1 := createKey(t, tu, "root", map[string]string{"env": "dev"})

		now = now.Add(time.Second)
		c.SetTime(now)
		key2 := createKey(t, tu, "root", map[string]string{"env": "prod"})

		now = now.Add(time.Second)
		c.SetTime(now)
		_ = createKey(t, tu, "root", nil)

		// Note the ek_global key already exists and is created by the database migration

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/keys", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/keys",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "keys", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("valid - list all", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/keys",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "keys", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp schemaapi.ListKeysResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 4)
		})

		t.Run("filter by state", func(t *testing.T) {
			// Disable one key first
			body := managedKeyPatchBody(nil, map[string]any{"desiredState": "disabled"})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", key1.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			// Filter by active state
			w = httptest.NewRecorder()
			req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/keys?state=active",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp schemaapi.ListKeysResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 3)
			for _, item := range resp.Items {
				require.Equal(t, keyschema.KeyStateActive, item.Status.State)
			}
		})

		t.Run("reject invalid state filter", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/keys?state=bogus",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("pagination with limit", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/keys?limit=1",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp schemaapi.ListKeysResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 1)
			require.NotEmpty(t, resp.Metadata.Continue)

			// Fetch next page using cursor
			w = httptest.NewRecorder()
			req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys?cursor=%s", url.QueryEscape(resp.Metadata.Continue)),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp2 schemaapi.ListKeysResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp2))
			require.Len(t, resp2.Items, 1)
		})

		t.Run("filter with label_selector", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/keys?labelSelector=env%3Dprod",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp schemaapi.ListKeysResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 1)
			require.Equal(t, key2.GetId(), resp.Items[0].GetId())
			require.Equal(t, "prod", resp.Items[0].Metadata.Labels["env"])
		})
	})

	t.Run("update key", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", map[string]string{"env": "test"})

		t.Run("unauthorized", func(t *testing.T) {
			body := managedKeyPatchBody(nil, map[string]any{"desiredState": "disabled"})
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("/keys/%s", created.GetId()), bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			body := managedKeyPatchBody(nil, map[string]any{"desiredState": "disabled"})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "keys", "get"), // Wrong verb
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			body := managedKeyPatchBody(nil, map[string]any{"desiredState": "disabled"})
			fakeId := apid.New(apid.PrefixKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", fakeId),
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

		t.Run("bad request - invalid state", func(t *testing.T) {
			body := managedKeyPatchBody(nil, map[string]any{"desiredState": "bogus"})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
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

		t.Run("bad request - server-owned patch fields", func(t *testing.T) {
			body := managedKeyPatchBody(
				map[string]any{"updatedAt": "2026-09-01T12:00:00Z"},
				map[string]any{},
			)
			var raw map[string]any
			require.NoError(t, json.Unmarshal([]byte(body), &raw))
			raw["status"] = map[string]any{"state": "active", "keyDataConfigured": true}
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
				util.JsonToReader(raw),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), "server-owned")
		})

		t.Run("bad request - invalid JSON", func(t *testing.T) {
			body := `{invalid json}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
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

		t.Run("bad request does not echo key material", func(t *testing.T) {
			const secret = "never-echo-this-key-material"
			body := managedKeyPatchBody(nil, map[string]any{
				"keyData": map[string]any{
					"value":      secret,
					"unexpected": "field",
				},
			})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.NotContains(t, w.Body.String(), secret)
		})

		t.Run("bad request - redacted key data placeholder", func(t *testing.T) {
			body := managedKeyPatchBody(nil, map[string]any{"keyData": map[string]any{"value": "***"}})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), "redacted placeholder values")
		})

		t.Run("success - update state", func(t *testing.T) {
			body := managedKeyPatchBody(nil, map[string]any{"desiredState": "disabled"})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp keyschema.Key
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, keyschema.KeyStateDisabled, resp.Status.State)

			// Verify in database
			got, err := tu.Db.GetKey(context.Background(), created.GetId())
			require.NoError(t, err)
			require.Equal(t, database.KeyStateDisabled, got.State)
		})

		t.Run("rejects apxy/-prefixed labels in request body", func(t *testing.T) {
			body := managedKeyPatchBody(map[string]any{"labels": map[string]any{"apxy/cxr/source": "config"}}, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code, "API must reject apxy/-prefixed labels at the user-input boundary")
			require.Contains(t, w.Body.String(), "reserved")
		})

		t.Run("success - update labels", func(t *testing.T) {
			body := managedKeyPatchBody(map[string]any{"labels": map[string]any{"env": "production", "team": "backend"}}, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp keyschema.Key
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "production", resp.Metadata.Labels["env"])
			require.Equal(t, "backend", resp.Metadata.Labels["team"])
		})

		t.Run("success - update state and labels together", func(t *testing.T) {
			body := managedKeyPatchBody(
				map[string]any{"labels": map[string]any{"new-label": "value"}},
				map[string]any{"desiredState": "active"},
			)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp keyschema.Key
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, keyschema.KeyStateActive, resp.Status.State)
			respUser, _ := database.SplitUserAndApxyLabels(database.Labels(resp.Metadata.Labels))
			require.Equal(t, database.Labels{"new-label": "value"}, respUser)
		})

		t.Run("success - labels unchanged when not provided", func(t *testing.T) {
			body := managedKeyPatchBody(nil, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp keyschema.Key
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			respUser, _ := database.SplitUserAndApxyLabels(database.Labels(resp.Metadata.Labels))
			require.Equal(t, database.Labels{"new-label": "value"}, respUser)
		})
	})

	t.Run("delete key", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", nil)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/keys/%s", created.GetId()), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/keys/%s", created.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "keys", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found returns 204", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/keys/%s", fakeId),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/keys/%s", created.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			// Verify key is gone
			w = httptest.NewRecorder()
			req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s", created.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("rejects deletion of ek_global", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/keys/%s", database.GlobalKeyID),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)

			var errResp httperr.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
			require.Contains(t, errResp.Error, "global key cannot be deleted")
		})

		t.Run("delete is idempotent", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/keys/%s", created.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})
	})

	t.Run("get labels", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		withLabels := createKey(t, tu, "root", map[string]string{"env": "prod", "team": "backend"})
		withoutLabels := createKey(t, tu, "root", nil)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/keys/%s/labels", withLabels.GetId()), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s/labels", fakeId),
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
				fmt.Sprintf("/keys/%s/labels", withLabels.GetId()),
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
				fmt.Sprintf("/keys/%s/labels", withoutLabels.GetId()),
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

		created := createKey(t, tu, "root", map[string]string{"env": "staging"})

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/keys/%s/labels/env", created.GetId()), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("key not found", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s/labels/env", fakeId),
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
				fmt.Sprintf("/keys/%s/labels/nonexistent", created.GetId()),
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
				fmt.Sprintf("/keys/%s/labels/env", created.GetId()),
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

		created := createKey(t, tu, "root", nil)

		t.Run("unauthorized", func(t *testing.T) {
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("/keys/%s/labels/env", created.GetId()), bytes.NewBufferString(body))
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
				fmt.Sprintf("/keys/%s/labels/env", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "keys", "get"), // Wrong verb
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("key not found", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixKey)
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/keys/%s/labels/env", fakeId),
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
				fmt.Sprintf("/keys/%s/labels/env", created.GetId()),
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
				fmt.Sprintf("/keys/%s/labels/env", created.GetId()),
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
			got, err := tu.Db.GetKey(context.Background(), created.GetId())
			require.NoError(t, err)
			require.Equal(t, "production", got.Labels["env"])
		})

		t.Run("success - update existing label", func(t *testing.T) {
			body := `{"value": "staging"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/keys/%s/labels/env", created.GetId()),
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
			require.Equal(t, "staging", resp.Value)
		})

		t.Run("success - preserves other labels", func(t *testing.T) {
			ekWithLabels := createKey(t, tu, "root", map[string]string{"env": "dev", "team": "platform"})

			body := `{"value": "staging"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/keys/%s/labels/env", ekWithLabels.GetId()),
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
			got, err := tu.Db.GetKey(context.Background(), ekWithLabels.GetId())
			require.NoError(t, err)
			require.Equal(t, "staging", got.Labels["env"])
			require.Equal(t, "platform", got.Labels["team"])
		})
	})

	t.Run("delete label", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", map[string]string{"env": "prod", "team": "backend"})

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/keys/%s/labels/env", created.GetId()), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/keys/%s/labels/env", created.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "keys", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("key not found returns 204", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/keys/%s/labels/env", fakeId),
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
			ekToDelete := createKey(t, tu, "root", map[string]string{"to-delete": "value", "to-keep": "value2"})

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/keys/%s/labels/to-delete", ekToDelete.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			// Verify the label is deleted but other labels remain
			got, err := tu.Db.GetKey(context.Background(), ekToDelete.GetId())
			require.NoError(t, err)
			_, exists := got.Labels["to-delete"]
			require.False(t, exists)
			require.Equal(t, "value2", got.Labels["to-keep"])
		})

		t.Run("success - delete is idempotent", func(t *testing.T) {
			ekIdempotent := createKey(t, tu, "root", map[string]string{"label": "value"})

			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					fmt.Sprintf("/keys/%s/labels/label", ekIdempotent.GetId()),
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
			got, err := tu.Db.GetKey(context.Background(), ekIdempotent.GetId())
			require.NoError(t, err)
			_, exists := got.Labels["label"]
			require.False(t, exists)
		})
	})

	t.Run("update key with annotations", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", nil)

		t.Run("success - update annotations", func(t *testing.T) {
			body := managedKeyPatchBody(map[string]any{"annotations": map[string]any{"description": "primary key", "owner": "team-a"}}, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp keyschema.Key
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "primary key", resp.Metadata.Annotations["description"])
			require.Equal(t, "team-a", resp.Metadata.Annotations["owner"])
		})

		t.Run("success - annotations unchanged when not provided", func(t *testing.T) {
			body := managedKeyPatchBody(nil, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp keyschema.Key
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "primary key", resp.Metadata.Annotations["description"])
			require.Equal(t, "team-a", resp.Metadata.Annotations["owner"])
		})

		t.Run("success - update annotations replaces all", func(t *testing.T) {
			body := managedKeyPatchBody(map[string]any{"annotations": map[string]any{"new-key": "new-value"}}, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp keyschema.Key
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, map[string]string{"new-key": "new-value"}, resp.Metadata.Annotations)
		})
	})

	t.Run("get annotations", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", nil)

		// Set some annotations via PATCH
		body := managedKeyPatchBody(map[string]any{"annotations": map[string]any{"description": "test key", "owner": "backend"}}, nil)
		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPatch,
			fmt.Sprintf("/keys/%s", created.GetId()),
			bytes.NewBufferString(body),
			"root",
			"some-actor",
			aschema.AllPermissions(),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		withoutAnnotations := createKey(t, tu, "root", nil)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/keys/%s/annotations", created.GetId()), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s/annotations", fakeId),
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
				fmt.Sprintf("/keys/%s/annotations", created.GetId()),
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
			require.Equal(t, "test key", resp["description"])
			require.Equal(t, "backend", resp["owner"])
		})

		t.Run("success - empty annotations", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s/annotations", withoutAnnotations.GetId()),
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

		created := createKey(t, tu, "root", nil)

		// Set an annotation
		body := managedKeyPatchBody(map[string]any{"annotations": map[string]any{"description": "staging key"}}, nil)
		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPatch,
			fmt.Sprintf("/keys/%s", created.GetId()),
			bytes.NewBufferString(body),
			"root",
			"some-actor",
			aschema.AllPermissions(),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/keys/%s/annotations/description", created.GetId()), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("key not found", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/keys/%s/annotations/description", fakeId),
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
				fmt.Sprintf("/keys/%s/annotations/nonexistent", created.GetId()),
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
				fmt.Sprintf("/keys/%s/annotations/description", created.GetId()),
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
			require.Equal(t, "description", resp.Key)
			require.Equal(t, "staging key", resp.Value)
		})
	})

	t.Run("put annotation", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", nil)

		t.Run("unauthorized", func(t *testing.T) {
			body := `{"value": "my description"}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("/keys/%s/annotations/description", created.GetId()), bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			body := `{"value": "my description"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/keys/%s/annotations/description", created.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "keys", "get"), // Wrong verb
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("key not found", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixKey)
			body := `{"value": "my description"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/keys/%s/annotations/description", fakeId),
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
				fmt.Sprintf("/keys/%s/annotations/description", created.GetId()),
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
			body := `{"value": "primary key"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/keys/%s/annotations/description", created.GetId()),
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
			require.Equal(t, "description", resp.Key)
			require.Equal(t, "primary key", resp.Value)

			// Verify in database
			got, err := tu.Db.GetKey(context.Background(), created.GetId())
			require.NoError(t, err)
			require.Equal(t, "primary key", got.Annotations["description"])
		})

		t.Run("success - update existing annotation", func(t *testing.T) {
			body := `{"value": "updated description"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/keys/%s/annotations/description", created.GetId()),
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
			require.Equal(t, "description", resp.Key)
			require.Equal(t, "updated description", resp.Value)
		})

		t.Run("success - preserves other annotations", func(t *testing.T) {
			ekWithAnnotations := createKey(t, tu, "root", nil)

			// Set two annotations
			body := managedKeyPatchBody(map[string]any{"annotations": map[string]any{"description": "key desc", "owner": "platform"}}, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", ekWithAnnotations.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			// Update only one annotation via PUT
			body = `{"value": "updated desc"}`
			w = httptest.NewRecorder()
			req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/keys/%s/annotations/description", ekWithAnnotations.GetId()),
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
			got, err := tu.Db.GetKey(context.Background(), ekWithAnnotations.GetId())
			require.NoError(t, err)
			require.Equal(t, "updated desc", got.Annotations["description"])
			require.Equal(t, "platform", got.Annotations["owner"])
		})
	})

	t.Run("delete annotation", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", nil)

		// Set annotations
		body := managedKeyPatchBody(map[string]any{"annotations": map[string]any{"description": "prod key", "owner": "backend"}}, nil)
		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPatch,
			fmt.Sprintf("/keys/%s", created.GetId()),
			bytes.NewBufferString(body),
			"root",
			"some-actor",
			aschema.AllPermissions(),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/keys/%s/annotations/description", created.GetId()), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/keys/%s/annotations/description", created.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "keys", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("key not found returns 204", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/keys/%s/annotations/description", fakeId),
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
			ekToDelete := createKey(t, tu, "root", nil)

			// Set annotations
			body := managedKeyPatchBody(map[string]any{"annotations": map[string]any{"to-delete": "value", "to-keep": "value2"}}, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", ekToDelete.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			w = httptest.NewRecorder()
			req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/keys/%s/annotations/to-delete", ekToDelete.GetId()),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			// Verify the annotation is deleted but other annotations remain
			got, err := tu.Db.GetKey(context.Background(), ekToDelete.GetId())
			require.NoError(t, err)
			_, exists := got.Annotations["to-delete"]
			require.False(t, exists)
			require.Equal(t, "value2", got.Annotations["to-keep"])
		})

		t.Run("success - delete is idempotent", func(t *testing.T) {
			ekIdempotent := createKey(t, tu, "root", nil)

			// Set an annotation
			body := managedKeyPatchBody(map[string]any{"annotations": map[string]any{"annotation": "value"}}, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/keys/%s", ekIdempotent.GetId()),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					fmt.Sprintf("/keys/%s/annotations/annotation", ekIdempotent.GetId()),
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
			got, err := tu.Db.GetKey(context.Background(), ekIdempotent.GetId())
			require.NoError(t, err)
			_, exists := got.Annotations["annotation"]
			require.False(t, exists)
		})
	})

	t.Run("resource name API", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		createNamed := func(name string) keyschema.Key {
			body := managedKeyCreateBody("root", name, map[string]any{"value": "named-key-data"}, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost, "/keys", util.JsonToReader(body),
				"root", "some-actor", aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var key keyschema.Key
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &key))
			return key
		}

		named := createNamed("primary-key")
		require.Equal(t, "primary-key", string(named.Metadata.Name))
		require.Equal(t, "primary-key", named.Metadata.Labels["apxy/key/-/name"])
		defaulted := createKey(t, tu, "root", nil)
		require.Equal(t, defaulted.GetId().String(), string(defaulted.Metadata.Name))
		require.Equal(t, defaulted.GetId().String(), defaulted.Metadata.Labels["apxy/key/-/name"])

		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodGet, "/keys/"+named.GetId().String(), nil,
			"root", "some-actor", aschema.AllPermissions(),
		)
		require.NoError(t, err)
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var got keyschema.Key
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		require.Equal(t, "primary-key", string(got.Metadata.Name))

		w = httptest.NewRecorder()
		req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPatch, "/keys/"+named.GetId().String(), bytes.NewBufferString(managedKeyPatchBody(map[string]any{"name": "renamed-key"}, nil)),
			"root", "some-actor", aschema.AllPermissions(),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		require.Equal(t, "renamed-key", string(got.Metadata.Name))
		require.Equal(t, "renamed-key", got.Metadata.Labels["apxy/key/-/name"])

		_ = createNamed("conflicting-key")
		w = httptest.NewRecorder()
		req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPatch, "/keys/"+named.GetId().String(), bytes.NewBufferString(managedKeyPatchBody(map[string]any{"name": "conflicting-key"}, nil)),
			"root", "some-actor", aschema.AllPermissions(),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "UNIQUE")

		w = httptest.NewRecorder()
		req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodGet, "/keys?name=renamed-key", nil,
			"root", "some-actor", aschema.AllPermissions(),
		)
		require.NoError(t, err)
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var listed schemaapi.ListKeysResponseJson
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
		require.Len(t, listed.Items, 1)
		require.Equal(t, named.GetId(), listed.Items[0].GetId())
	})
}

func TestKeyOpenAPIKeepsProviderConfigurationOpaque(t *testing.T) {
	data, err := os.ReadFile("../service/api/swagger/docs.json")
	require.NoError(t, err)

	var document struct {
		Definitions map[string]json.RawMessage `json:"definitions"`
	}
	require.NoError(t, json.Unmarshal(data, &document))

	for _, definitionName := range []string{"openapi.KeySpecJson", "openapi.KeySpecPatchJson"} {
		var definition struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(document.Definitions[definitionName], &definition))
		require.JSONEq(t, `{"type":"object"}`, string(definition.Properties["keyData"]), definitionName)
	}
}
