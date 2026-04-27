package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httperr"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/routes/labels"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestEncryptionKeys(t *testing.T) {
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
		ekr := NewEncryptionKeysRoutes(cfg, auth, c)
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

	// Helper to create an encryption key via the API and return its ID
	createKey := func(t *testing.T, tu *TestSetup, namespace string, labels map[string]string) EncryptionKeyJson {
		body := map[string]interface{}{
			"namespace": namespace,
			"key_data": map[string]interface{}{
				"value": "test-key-data-value",
			},
		}
		if labels != nil {
			body["labels"] = labels
		}
		jsonBody, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPost,
			"/encryption-keys",
			bytes.NewReader(jsonBody),
			"root",
			"some-actor",
			aschema.AllPermissions(),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp EncryptionKeyJson
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	t.Run("get encryption key", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", map[string]string{"env": "test"})

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/encryption-keys/%s", created.Id), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "encryption_keys", "list"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			fakeId := apid.New(apid.PrefixEncryptionKey)
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/encryption-keys/%s", fakeId),
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
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp EncryptionKeyJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, created.Id, resp.Id)
			require.Equal(t, "root", resp.Namespace)
			require.Equal(t, database.EncryptionKeyStateActive, resp.State)
			require.Equal(t, "test", resp.Labels["env"])
		})

		t.Run("allowed with matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "encryption_keys", "get", string(created.Id)),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			fakeId := apid.New(apid.PrefixEncryptionKey)
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "encryption_keys", "get", string(fakeId)),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})
	})

	t.Run("create encryption key", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		t.Run("unauthorized", func(t *testing.T) {
			body := map[string]interface{}{
				"namespace": "root",
				"key_data": map[string]interface{}{
					"type":      "raw",
					"raw_bytes": "dGVzdC1rZXktZGF0YQ==",
				},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/encryption-keys", bytes.NewReader(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden wrong verb", func(t *testing.T) {
			body := map[string]interface{}{
				"namespace": "root",
				"key_data": map[string]interface{}{
					"type":      "raw",
					"raw_bytes": "dGVzdC1rZXktZGF0YQ==",
				},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/encryption-keys",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "encryption_keys", "list"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad request - missing namespace", func(t *testing.T) {
			body := map[string]interface{}{
				"key_data": map[string]interface{}{
					"type":      "raw",
					"raw_bytes": "dGVzdC1rZXktZGF0YQ==",
				},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/encryption-keys",
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

		t.Run("forbidden namespace not allowed", func(t *testing.T) {
			body := map[string]interface{}{
				"namespace": "root.restricted",
				"key_data": map[string]interface{}{
					"type":      "raw",
					"raw_bytes": "dGVzdC1rZXktZGF0YQ==",
				},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/encryption-keys",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.other.**", "encryption_keys", "create"), // Wrong namespace
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code) // ValidateNamespace returns bad request
		})

		t.Run("valid with labels", func(t *testing.T) {
			created := createKey(t, tu, "root", map[string]string{"env": "prod", "team": "backend"})
			require.True(t, created.Id.HasPrefix(apid.PrefixEncryptionKey))
			require.Equal(t, "root", created.Namespace)
			require.Equal(t, database.EncryptionKeyStateActive, created.State)
			require.Equal(t, "prod", created.Labels["env"])
			require.Equal(t, "backend", created.Labels["team"])
		})

		t.Run("valid without labels", func(t *testing.T) {
			created := createKey(t, tu, "root", nil)
			require.True(t, created.Id.HasPrefix(apid.PrefixEncryptionKey))
			require.Equal(t, "root", created.Namespace)
			require.Equal(t, database.EncryptionKeyStateActive, created.State)
		})
	})

	t.Run("list encryption keys", func(t *testing.T) {
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
			req, err := http.NewRequest(http.MethodGet, "/encryption-keys", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/encryption-keys",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "encryption_keys", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("valid - list all", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/encryption-keys",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "encryption_keys", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListEncryptionKeysResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 4)
		})

		t.Run("filter by state", func(t *testing.T) {
			// Disable one key first
			body := `{"state": "disabled"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", key1.Id),
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
				"/encryption-keys?state=active",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListEncryptionKeysResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 3)
			for _, item := range resp.Items {
				require.Equal(t, database.EncryptionKeyStateActive, item.State)
			}
		})

		t.Run("pagination with limit", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/encryption-keys?limit=1",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListEncryptionKeysResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 1)
			require.NotEmpty(t, resp.Cursor)

			// Fetch next page using cursor
			w = httptest.NewRecorder()
			req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/encryption-keys?cursor=%s", url.QueryEscape(resp.Cursor)),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp2 ListEncryptionKeysResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp2))
			require.Len(t, resp2.Items, 1)
		})

		t.Run("filter with label_selector", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/encryption-keys?label_selector=env%3Dprod",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListEncryptionKeysResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 1)
			require.Equal(t, key2.Id, resp.Items[0].Id)
			require.Equal(t, "prod", resp.Items[0].Labels["env"])
		})
	})

	t.Run("update encryption key", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", map[string]string{"env": "test"})

		t.Run("unauthorized", func(t *testing.T) {
			body := `{"state": "disabled"}`
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("/encryption-keys/%s", created.Id), bytes.NewBufferString(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			body := `{"state": "disabled"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "encryption_keys", "get"), // Wrong verb
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			body := `{"state": "disabled"}`
			fakeId := apid.New(apid.PrefixEncryptionKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", fakeId),
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
			body := `{"state": "bogus"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
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

		t.Run("bad request - invalid JSON", func(t *testing.T) {
			body := `{invalid json}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
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

		t.Run("success - update state", func(t *testing.T) {
			body := `{"state": "disabled"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp EncryptionKeyJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, database.EncryptionKeyStateDisabled, resp.State)

			// Verify in database
			got, err := tu.Db.GetEncryptionKey(context.Background(), created.Id)
			require.NoError(t, err)
			require.Equal(t, database.EncryptionKeyStateDisabled, got.State)
		})

		t.Run("success - update labels", func(t *testing.T) {
			body := `{"labels": {"env": "production", "team": "backend"}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp EncryptionKeyJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "production", resp.Labels["env"])
			require.Equal(t, "backend", resp.Labels["team"])
		})

		t.Run("success - update state and labels together", func(t *testing.T) {
			body := `{"state": "active", "labels": {"new-label": "value"}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp EncryptionKeyJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, database.EncryptionKeyStateActive, resp.State)
			require.Equal(t, map[string]string{"new-label": "value"}, resp.Labels)
		})

		t.Run("success - labels unchanged when not provided", func(t *testing.T) {
			body := `{}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp EncryptionKeyJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, map[string]string{"new-label": "value"}, resp.Labels)
		})
	})

	t.Run("delete encryption key", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", nil)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/encryption-keys/%s", created.Id), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "encryption_keys", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found returns 204", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixEncryptionKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/encryption-keys/%s", fakeId),
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
				fmt.Sprintf("/encryption-keys/%s", created.Id),
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
				fmt.Sprintf("/encryption-keys/%s", created.Id),
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
				fmt.Sprintf("/encryption-keys/%s", database.GlobalEncryptionKeyID),
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
			require.Contains(t, errResp.Error, "global encryption key cannot be deleted")
		})

		t.Run("delete is idempotent", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
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
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/encryption-keys/%s/labels", withLabels.Id), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixEncryptionKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/encryption-keys/%s/labels", fakeId),
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
				fmt.Sprintf("/encryption-keys/%s/labels", withLabels.Id),
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
				fmt.Sprintf("/encryption-keys/%s/labels", withoutLabels.Id),
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

		created := createKey(t, tu, "root", map[string]string{"env": "staging"})

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/encryption-keys/%s/labels/env", created.Id), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("key not found", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixEncryptionKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/encryption-keys/%s/labels/env", fakeId),
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
				fmt.Sprintf("/encryption-keys/%s/labels/nonexistent", created.Id),
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
				fmt.Sprintf("/encryption-keys/%s/labels/env", created.Id),
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
			req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("/encryption-keys/%s/labels/env", created.Id), bytes.NewBufferString(body))
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
				fmt.Sprintf("/encryption-keys/%s/labels/env", created.Id),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "encryption_keys", "get"), // Wrong verb
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("key not found", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixEncryptionKey)
			body := `{"value": "production"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/encryption-keys/%s/labels/env", fakeId),
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
				fmt.Sprintf("/encryption-keys/%s/labels/env", created.Id),
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
				fmt.Sprintf("/encryption-keys/%s/labels/env", created.Id),
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
			got, err := tu.Db.GetEncryptionKey(context.Background(), created.Id)
			require.NoError(t, err)
			require.Equal(t, "production", got.Labels["env"])
		})

		t.Run("success - update existing label", func(t *testing.T) {
			body := `{"value": "staging"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/encryption-keys/%s/labels/env", created.Id),
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
				fmt.Sprintf("/encryption-keys/%s/labels/env", ekWithLabels.Id),
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
			got, err := tu.Db.GetEncryptionKey(context.Background(), ekWithLabels.Id)
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
			req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/encryption-keys/%s/labels/env", created.Id), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/encryption-keys/%s/labels/env", created.Id),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "encryption_keys", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("key not found returns 204", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixEncryptionKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/encryption-keys/%s/labels/env", fakeId),
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
				fmt.Sprintf("/encryption-keys/%s/labels/to-delete", ekToDelete.Id),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			// Verify the label is deleted but other labels remain
			got, err := tu.Db.GetEncryptionKey(context.Background(), ekToDelete.Id)
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
					fmt.Sprintf("/encryption-keys/%s/labels/label", ekIdempotent.Id),
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
			got, err := tu.Db.GetEncryptionKey(context.Background(), ekIdempotent.Id)
			require.NoError(t, err)
			_, exists := got.Labels["label"]
			require.False(t, exists)
		})
	})

	t.Run("update encryption key with annotations", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", nil)

		t.Run("success - update annotations", func(t *testing.T) {
			body := `{"annotations": {"description": "primary key", "owner": "team-a"}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp EncryptionKeyJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "primary key", resp.Annotations["description"])
			require.Equal(t, "team-a", resp.Annotations["owner"])
		})

		t.Run("success - annotations unchanged when not provided", func(t *testing.T) {
			body := `{}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp EncryptionKeyJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, "primary key", resp.Annotations["description"])
			require.Equal(t, "team-a", resp.Annotations["owner"])
		})

		t.Run("success - update annotations replaces all", func(t *testing.T) {
			body := `{"annotations": {"new-key": "new-value"}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", created.Id),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp EncryptionKeyJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, map[string]string{"new-key": "new-value"}, resp.Annotations)
		})
	})

	t.Run("get annotations", func(t *testing.T) {
		tu, done := setup(t, context.Background(), nil)
		defer done()

		created := createKey(t, tu, "root", nil)

		// Set some annotations via PATCH
		body := `{"annotations": {"description": "test key", "owner": "backend"}}`
		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPatch,
			fmt.Sprintf("/encryption-keys/%s", created.Id),
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
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/encryption-keys/%s/annotations", created.Id), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixEncryptionKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/encryption-keys/%s/annotations", fakeId),
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
				fmt.Sprintf("/encryption-keys/%s/annotations", created.Id),
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
				fmt.Sprintf("/encryption-keys/%s/annotations", withoutAnnotations.Id),
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
		body := `{"annotations": {"description": "staging key"}}`
		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPatch,
			fmt.Sprintf("/encryption-keys/%s", created.Id),
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
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/encryption-keys/%s/annotations/description", created.Id), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("key not found", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixEncryptionKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/encryption-keys/%s/annotations/description", fakeId),
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
				fmt.Sprintf("/encryption-keys/%s/annotations/nonexistent", created.Id),
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
				fmt.Sprintf("/encryption-keys/%s/annotations/description", created.Id),
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
			req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("/encryption-keys/%s/annotations/description", created.Id), bytes.NewBufferString(body))
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
				fmt.Sprintf("/encryption-keys/%s/annotations/description", created.Id),
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "encryption_keys", "get"), // Wrong verb
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("key not found", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixEncryptionKey)
			body := `{"value": "my description"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/encryption-keys/%s/annotations/description", fakeId),
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
				fmt.Sprintf("/encryption-keys/%s/annotations/description", created.Id),
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
			body := `{"value": "primary encryption key"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/encryption-keys/%s/annotations/description", created.Id),
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
			require.Equal(t, "primary encryption key", resp.Value)

			// Verify in database
			got, err := tu.Db.GetEncryptionKey(context.Background(), created.Id)
			require.NoError(t, err)
			require.Equal(t, "primary encryption key", got.Annotations["description"])
		})

		t.Run("success - update existing annotation", func(t *testing.T) {
			body := `{"value": "updated description"}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/encryption-keys/%s/annotations/description", created.Id),
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
			body := `{"annotations": {"description": "key desc", "owner": "platform"}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", ekWithAnnotations.Id),
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
				fmt.Sprintf("/encryption-keys/%s/annotations/description", ekWithAnnotations.Id),
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
			got, err := tu.Db.GetEncryptionKey(context.Background(), ekWithAnnotations.Id)
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
		body := `{"annotations": {"description": "prod key", "owner": "backend"}}`
		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPatch,
			fmt.Sprintf("/encryption-keys/%s", created.Id),
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
			req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("/encryption-keys/%s/annotations/description", created.Id), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/encryption-keys/%s/annotations/description", created.Id),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "encryption_keys", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("key not found returns 204", func(t *testing.T) {
			fakeId := apid.New(apid.PrefixEncryptionKey)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				fmt.Sprintf("/encryption-keys/%s/annotations/description", fakeId),
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
			body := `{"annotations": {"to-delete": "value", "to-keep": "value2"}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", ekToDelete.Id),
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
				fmt.Sprintf("/encryption-keys/%s/annotations/to-delete", ekToDelete.Id),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			// Verify the annotation is deleted but other annotations remain
			got, err := tu.Db.GetEncryptionKey(context.Background(), ekToDelete.Id)
			require.NoError(t, err)
			_, exists := got.Annotations["to-delete"]
			require.False(t, exists)
			require.Equal(t, "value2", got.Annotations["to-keep"])
		})

		t.Run("success - delete is idempotent", func(t *testing.T) {
			ekIdempotent := createKey(t, tu, "root", nil)

			// Set an annotation
			body := `{"annotations": {"annotation": "value"}}`
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/encryption-keys/%s", ekIdempotent.Id),
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
					fmt.Sprintf("/encryption-keys/%s/annotations/annotation", ekIdempotent.Id),
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
			got, err := tu.Db.GetEncryptionKey(context.Background(), ekIdempotent.Id)
			require.NoError(t, err)
			_, exists := got.Annotations["annotation"]
			require.False(t, exists)
		})
	})
}
