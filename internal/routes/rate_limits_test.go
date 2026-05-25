package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	asynqmock "github.com/rmorlok/authproxy/internal/apasynq/mock"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/ratelimit"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
)

func TestRateLimits(t *testing.T) {
	type TestSetup struct {
		Gin      *gin.Engine
		Cfg      config.C
		AuthUtil *auth2.AuthTestUtil
		Db       database.DB
		Core     coreIface.C
		RlCache  ratelimit.MutableCache
	}

	setup := func(t *testing.T) (*TestSetup, func()) {
		cfg := config.FromRoot(&sconfig.Root{
			Connectors: &sconfig.Connectors{LoadFromList: []sconfig.Connector{}},
		})
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
		cfg, rds := apredis.MustApplyTestConfig(cfg)
		cfg, auth, authUtil := auth2.TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)
		h := httpf2.CreateFactory(cfg, rds, nil, aplog.NewNoopLogger())
		cfg, e := encrypt.NewTestEncryptService(cfg, db)
		ctrl := gomock.NewController(t)
		ac := asynqmock.NewMockClient(ctrl)

		rlCache := ratelimit.NewCache()
		// The dry-run subtree needs a real redis (for Limiter.Peek) and
		// the cache, so we hand the core service the real apredis
		// client instead of the per-test mock. The core's other paths
		// don't talk to Redis in these tests.
		c := core.NewCoreService(cfg, db, e, rds, h, ac, test_utils.NewTestLogger(),
			core.WithRateLimitCache(rlCache))
		require.NoError(t, c.Migrate(context.Background()))
		rlr := NewRateLimitsRoutes(cfg, auth, c)
		r := apgin.ForTest(nil)
		rlr.Register(r)

		return &TestSetup{
				Gin:      r,
				Cfg:      cfg,
				AuthUtil: authUtil,
				Db:       db,
				Core:     c,
				RlCache:  rlCache,
			}, func() {
				ctrl.Finish()
			}
	}

	// validDef returns a JSON-serialisable rate-limit definition that passes
	// schema validation. Use map[string]interface{} so the test crafts the
	// wire format directly rather than relying on Go struct marshaling.
	validDef := func() map[string]interface{} {
		return map[string]interface{}{
			"selector": map[string]interface{}{
				"methods":       []string{"GET"},
				"request_types": []string{"proxy"},
			},
			"bucket": map[string]interface{}{
				"dimensions": []string{"actor"},
			},
			"algorithm": map[string]interface{}{
				"token_bucket": map[string]interface{}{
					"capacity":    60,
					"refill_rate": 1.0,
				},
			},
		}
	}

	createRateLimit := func(t *testing.T, tu *TestSetup, namespace string, labels map[string]string) RateLimitJson {
		body := map[string]interface{}{
			"namespace":  namespace,
			"definition": validDef(),
		}
		if labels != nil {
			body["labels"] = labels
		}
		jsonBody, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPost,
			"/rate-limits",
			bytes.NewReader(jsonBody),
			"root",
			"some-actor",
			aschema.AllPermissions(),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "create body: %s", w.Body.String())

		var resp RateLimitJson
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	t.Run("get rate limit", func(t *testing.T) {
		tu, done := setup(t)
		defer done()

		created := createRateLimit(t, tu, "root", map[string]string{"env": "test"})

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/rate-limits/%s", created.Id), nil)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/rate-limits/%s", created.Id),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "rate_limits", "create"), // Wrong verb
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			fakeId := apid.New(apid.PrefixRateLimit)
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				fmt.Sprintf("/rate-limits/%s", fakeId),
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
				fmt.Sprintf("/rate-limits/%s", created.Id),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp RateLimitJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, created.Id, resp.Id)
			require.Equal(t, "root", resp.Namespace)
			require.Equal(t, "test", resp.Labels["env"])
			require.NotNil(t, resp.Definition.Algorithm.TokenBucket)
			require.Equal(t, 60, resp.Definition.Algorithm.TokenBucket.Capacity)
		})
	})

	t.Run("create rate limit", func(t *testing.T) {
		tu, done := setup(t)
		defer done()

		t.Run("unauthorized", func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{"namespace": "root", "definition": validDef()})
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/rate-limits", bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden wrong verb", func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{"namespace": "root", "definition": validDef()})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/rate-limits",
				bytes.NewReader(body),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "rate_limits", "list"),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("missing namespace", func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{"definition": validDef()})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost, "/rate-limits", bytes.NewReader(body),
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("forbidden namespace not allowed", func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{"namespace": "root.restricted", "definition": validDef()})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost, "/rate-limits", bytes.NewReader(body),
				"root", "some-actor",
				aschema.PermissionsSingle("root.other.**", "rate_limits", "create"))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("invalid definition - empty algorithm", func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"namespace": "root",
				"definition": map[string]interface{}{
					"selector":  map[string]interface{}{},
					"bucket":    map[string]interface{}{},
					"algorithm": map[string]interface{}{},
				},
			})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost, "/rate-limits", bytes.NewReader(body),
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), "exactly one of")
		})

		t.Run("invalid definition - request_types empty list", func(t *testing.T) {
			def := validDef()
			def["selector"].(map[string]interface{})["request_types"] = []string{}
			body, _ := json.Marshal(map[string]interface{}{"namespace": "root", "definition": def})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost, "/rate-limits", bytes.NewReader(body),
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("invalid labels - reserved apxy/ prefix", func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"namespace":  "root",
				"definition": validDef(),
				"labels":     map[string]string{"apxy/foo": "bar"},
			})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost, "/rate-limits", bytes.NewReader(body),
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), "labels")
		})

		t.Run("valid with labels and annotations", func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"namespace":   "root",
				"definition":  validDef(),
				"labels":      map[string]string{"env": "prod", "team": "platform"},
				"annotations": map[string]string{"description": "throttle"},
			})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost, "/rate-limits", bytes.NewReader(body),
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			var resp RateLimitJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.True(t, resp.Id.HasPrefix(apid.PrefixRateLimit))
			require.Equal(t, "prod", resp.Labels["env"])
			require.Equal(t, "platform", resp.Labels["team"])
			require.Equal(t, "throttle", resp.Annotations["description"])
			// Implicit identifier labels are present.
			require.Equal(t, string(resp.Id), resp.Labels["apxy/rl/-/id"])
		})
	})

	t.Run("update rate limit", func(t *testing.T) {
		tu, done := setup(t)
		defer done()

		created := createRateLimit(t, tu, "root", map[string]string{"env": "test"})

		t.Run("update definition", func(t *testing.T) {
			newDef := validDef()
			newDef["mode"] = "observe"
			body, _ := json.Marshal(map[string]interface{}{"definition": newDef})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch, fmt.Sprintf("/rate-limits/%s", created.Id), bytes.NewReader(body),
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			var resp RateLimitJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, rlschema.ModeObserve, resp.Definition.Mode)
		})

		t.Run("update with invalid definition", func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"definition": map[string]interface{}{
					"selector":  map[string]interface{}{},
					"bucket":    map[string]interface{}{},
					"algorithm": map[string]interface{}{},
				},
			})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch, fmt.Sprintf("/rate-limits/%s", created.Id), bytes.NewReader(body),
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("update labels (full replace)", func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{"labels": map[string]string{"region": "us-east"}})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch, fmt.Sprintf("/rate-limits/%s", created.Id), bytes.NewReader(body),
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			var resp RateLimitJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			// PATCH with labels = full user-label replace.
			require.Equal(t, "us-east", resp.Labels["region"])
			_, hadEnv := resp.Labels["env"]
			require.False(t, hadEnv, "old env label should be replaced")
			// apxy/* survive replace.
			require.Equal(t, string(created.Id), resp.Labels["apxy/rl/-/id"])
		})

		t.Run("not found", func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{"labels": map[string]string{"env": "test"}})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch, fmt.Sprintf("/rate-limits/%s", apid.New(apid.PrefixRateLimit)),
				bytes.NewReader(body),
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})
	})

	t.Run("delete rate limit", func(t *testing.T) {
		tu, done := setup(t)
		defer done()

		created := createRateLimit(t, tu, "root", nil)

		t.Run("forbidden wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete, fmt.Sprintf("/rate-limits/%s", created.Id), nil,
				"root", "some-actor",
				aschema.PermissionsSingle("root.**", "rate_limits", "get"))
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete, fmt.Sprintf("/rate-limits/%s", created.Id), nil,
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			// Subsequent get returns 404.
			w = httptest.NewRecorder()
			req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet, fmt.Sprintf("/rate-limits/%s", created.Id), nil,
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("delete already-deleted returns 204", func(t *testing.T) {
			// The encryption-keys delete returns 204 even when the row is
			// already gone; rate limits should match.
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete, fmt.Sprintf("/rate-limits/%s", apid.New(apid.PrefixRateLimit)), nil,
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})
	})

	t.Run("list rate limits", func(t *testing.T) {
		tu, done := setup(t)
		defer done()

		// Seed: 3 in root, 2 in root (one with team=alpha, one with team=beta).
		_ = createRateLimit(t, tu, "root", map[string]string{"team": "alpha"})
		_ = createRateLimit(t, tu, "root", map[string]string{"team": "beta"})
		_ = createRateLimit(t, tu, "root", nil)

		t.Run("list all", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet, "/rate-limits", nil,
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListRateLimitsResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 3)
		})

		t.Run("filter by namespace", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet, "/rate-limits?namespace=root", nil,
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListRateLimitsResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 3)
		})

		t.Run("filter by namespace - none match", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet, "/rate-limits?namespace=root.nope", nil,
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListRateLimitsResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Empty(t, resp.Items)
		})

		t.Run("filter by label_selector", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet, "/rate-limits?label_selector=team%3Dalpha", nil,
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			var resp ListRateLimitsResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 1)
			require.Equal(t, "alpha", resp.Items[0].Labels["team"])
		})

		t.Run("invalid order_by", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet, "/rate-limits?order_by=banana%3Aasc", nil,
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	})

	t.Run("label sub-resources", func(t *testing.T) {
		tu, done := setup(t)
		defer done()

		created := createRateLimit(t, tu, "root", map[string]string{"env": "test"})

		t.Run("get all labels", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet, fmt.Sprintf("/rate-limits/%s/labels", created.Id), nil,
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			var labels map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &labels))
			require.Equal(t, "test", labels["env"])
		})

		t.Run("put one label", func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"value": "us-east"})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut, fmt.Sprintf("/rate-limits/%s/labels/region", created.Id),
				bytes.NewReader(body),
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			// Re-read to confirm.
			rl, err := tu.Db.GetRateLimit(context.Background(), created.Id)
			require.NoError(t, err)
			require.Equal(t, "us-east", rl.Labels["region"])
		})

		t.Run("delete one label", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete, fmt.Sprintf("/rate-limits/%s/labels/env", created.Id), nil,
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

			rl, err := tu.Db.GetRateLimit(context.Background(), created.Id)
			require.NoError(t, err)
			_, exists := rl.Labels["env"]
			require.False(t, exists)
		})
	})

	t.Run("annotation sub-resources", func(t *testing.T) {
		tu, done := setup(t)
		defer done()

		created := createRateLimit(t, tu, "root", nil)

		t.Run("put one annotation", func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"value": "platform@example.com"})
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut, fmt.Sprintf("/rate-limits/%s/annotations/owner", created.Id),
				bytes.NewReader(body),
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			rl, err := tu.Db.GetRateLimit(context.Background(), created.Id)
			require.NoError(t, err)
			require.Equal(t, "platform@example.com", rl.Annotations["owner"])
		})

		t.Run("get all annotations", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet, fmt.Sprintf("/rate-limits/%s/annotations", created.Id), nil,
				"root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var annots map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &annots))
			require.Equal(t, "platform@example.com", annots["owner"])
		})
	})

	// --- _dry_run ---
	//
	// installRules takes raw schema definitions, persists them via
	// core.CreateRateLimit (so the namespace / id assignment lifecycle
	// matches production), then refreshes the in-memory cache the way
	// the refresher would. This is the production-shaped path; the
	// alternative of hand-rolling *database.RateLimit values misses
	// label population that the dry-run depends on.
	installRules := func(t *testing.T, tu *TestSetup, namespace string, defs ...rlschema.RateLimit) []*database.RateLimit {
		t.Helper()
		ctx := context.Background()
		ids := make([]apid.ID, 0, len(defs))
		for _, def := range defs {
			rl, err := tu.Core.CreateRateLimit(ctx, namespace, def, nil, nil)
			require.NoError(t, err)
			ids = append(ids, rl.GetId())
		}
		out := make([]*database.RateLimit, 0, len(ids))
		for _, id := range ids {
			row, err := tu.Db.GetRateLimit(ctx, id)
			require.NoError(t, err)
			out = append(out, row)
		}
		tu.RlCache.Replace(out, time.Now())
		return out
	}

	postDryRun := func(t *testing.T, tu *TestSetup, body interface{}, perms []aschema.Permission) (int, []byte) {
		t.Helper()
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPost,
			"/rate-limits/_dry_run",
			bytes.NewReader(raw),
			"root",
			"dry-run-actor",
			perms,
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		tu.Gin.ServeHTTP(w, req)
		return w.Code, w.Body.Bytes()
	}

	// tokenBucketRule is a minimal rate-limit definition the dry-run
	// tests reuse. POST/PATCH/PUT, request_type=proxy, per-actor
	// bucket. Cheap to evaluate and easy to peek.
	tokenBucketRule := func() rlschema.RateLimit {
		return rlschema.RateLimit{
			Selector: rlschema.Selector{
				Methods:      []string{"POST", "PATCH", "PUT"},
				RequestTypes: []common.RequestType{common.RequestTypeProxy},
			},
			Bucket: rlschema.Bucket{Dimensions: []string{rlschema.DimensionActor}},
			Algorithm: rlschema.Algorithm{
				TokenBucket: &rlschema.TokenBucket{Capacity: 2, RefillRate: 1.0},
			},
		}
	}

	t.Run("dry run", func(t *testing.T) {
		// Default request body — fields supply just enough for the
		// dry-run to be valid (URL, method, request_type). Tests
		// override individual fields rather than reshape the body
		// each time.
		baseBody := func() map[string]interface{} {
			return map[string]interface{}{
				"request": map[string]interface{}{
					"method": "POST",
					"url":    "https://api.example.com/v1/things",
				},
				"request_type": "proxy",
				"context": map[string]interface{}{
					"namespace": "root",
					"actor_id":  "act_test",
				},
			}
		}

		t.Run("rejects when method missing", func(t *testing.T) {
			tu, done := setup(t)
			defer done()

			body := baseBody()
			delete(body["request"].(map[string]interface{}), "method")
			code, raw := postDryRun(t, tu, body, aschema.AllPermissions())
			require.Equal(t, http.StatusBadRequest, code, string(raw))
		})

		t.Run("rejects when url missing", func(t *testing.T) {
			tu, done := setup(t)
			defer done()

			body := baseBody()
			delete(body["request"].(map[string]interface{}), "url")
			code, raw := postDryRun(t, tu, body, aschema.AllPermissions())
			require.Equal(t, http.StatusBadRequest, code, string(raw))
		})

		t.Run("rejects when neither connection_id nor namespace given", func(t *testing.T) {
			tu, done := setup(t)
			defer done()

			body := baseBody()
			body["context"] = map[string]interface{}{}
			code, raw := postDryRun(t, tu, body, aschema.AllPermissions())
			require.Equal(t, http.StatusBadRequest, code, string(raw))
		})

		t.Run("matched rule reports peek result without consuming counter", func(t *testing.T) {
			tu, done := setup(t)
			defer done()

			rules := installRules(t, tu, "root", tokenBucketRule())
			require.Len(t, rules, 1)
			ruleId := rules[0].Id

			body := baseBody()

			// First call: would_allow=true on a fresh bucket; remaining=1.
			code, raw := postDryRun(t, tu, body, aschema.AllPermissions())
			require.Equal(t, http.StatusOK, code, string(raw))
			var resp DryRunResponseJson
			require.NoError(t, json.Unmarshal(raw, &resp))
			require.Len(t, resp.Matched, 1)
			require.Equal(t, ruleId, resp.Matched[0].RateLimitId)
			require.True(t, resp.Matched[0].WouldAllow)
			require.Equal(t, 1, resp.Matched[0].Remaining)
			require.Equal(t, "token bucket 2 @ 1/s", resp.Matched[0].AlgorithmSummary)

			// Second call: identical response — Peek must not have
			// consumed a token between the two calls.
			code2, raw2 := postDryRun(t, tu, body, aschema.AllPermissions())
			require.Equal(t, http.StatusOK, code2)
			var resp2 DryRunResponseJson
			require.NoError(t, json.Unmarshal(raw2, &resp2))
			require.Len(t, resp2.Matched, 1)
			require.True(t, resp2.Matched[0].WouldAllow)
			require.Equal(t, 1, resp2.Matched[0].Remaining, "Peek must not consume tokens between calls")
		})

		t.Run("not-matched rule emits a miss reason per clause", func(t *testing.T) {
			tu, done := setup(t)
			defer done()

			// Two rules: methods-only mismatch, request-type mismatch.
			rules := installRules(t, tu, "root",
				rlschema.RateLimit{
					Selector: rlschema.Selector{
						Methods:      []string{"DELETE"},
						RequestTypes: []common.RequestType{common.RequestTypeProxy},
					},
					Bucket:    rlschema.Bucket{},
					Algorithm: rlschema.Algorithm{TokenBucket: &rlschema.TokenBucket{Capacity: 1, RefillRate: 1}},
				},
				rlschema.RateLimit{
					Selector: rlschema.Selector{
						RequestTypes: []common.RequestType{common.RequestTypeProbe},
					},
					Bucket:    rlschema.Bucket{},
					Algorithm: rlschema.Algorithm{TokenBucket: &rlschema.TokenBucket{Capacity: 1, RefillRate: 1}},
				},
			)
			require.Len(t, rules, 2)

			code, raw := postDryRun(t, tu, baseBody(), aschema.AllPermissions())
			require.Equal(t, http.StatusOK, code, string(raw))
			var resp DryRunResponseJson
			require.NoError(t, json.Unmarshal(raw, &resp))
			require.Empty(t, resp.Matched)
			require.Len(t, resp.NotMatched, 2)

			reasonsById := map[apid.ID]string{}
			for _, nm := range resp.NotMatched {
				reasonsById[nm.RateLimitId] = nm.Reason
			}
			require.Contains(t, reasonsById[rules[0].Id], "method")
			require.Contains(t, reasonsById[rules[1].Id], "request_type")
		})

		t.Run("path-match rule reports prefix miss reason", func(t *testing.T) {
			tu, done := setup(t)
			defer done()
			rules := installRules(t, tu, "root", rlschema.RateLimit{
				Selector: rlschema.Selector{
					Methods:      []string{"POST"},
					RequestTypes: []common.RequestType{common.RequestTypeProxy},
					PathMatch: &rlschema.PathMatch{
						Kind:  rlschema.PathMatchKindPrefix,
						Value: "/services/data/",
					},
				},
				Bucket:    rlschema.Bucket{},
				Algorithm: rlschema.Algorithm{TokenBucket: &rlschema.TokenBucket{Capacity: 1, RefillRate: 1}},
			})

			body := baseBody()
			body["request"].(map[string]interface{})["url"] = "https://api.example.com/wrong/path"
			code, raw := postDryRun(t, tu, body, aschema.AllPermissions())
			require.Equal(t, http.StatusOK, code, string(raw))

			var resp DryRunResponseJson
			require.NoError(t, json.Unmarshal(raw, &resp))
			require.Len(t, resp.NotMatched, 1)
			require.Equal(t, rules[0].Id, resp.NotMatched[0].RateLimitId)
			require.Contains(t, resp.NotMatched[0].Reason, "prefix")
			require.Contains(t, resp.NotMatched[0].Reason, "/services/data/")
		})

		t.Run("labels on request flow into snapshot and label selector match", func(t *testing.T) {
			tu, done := setup(t)
			defer done()

			rules := installRules(t, tu, "root", rlschema.RateLimit{
				Selector: rlschema.Selector{
					LabelSelector: "team=acme",
					Methods:       []string{"POST"},
					RequestTypes:  []common.RequestType{common.RequestTypeProxy},
				},
				Bucket:    rlschema.Bucket{Dimensions: []string{rlschema.DimensionActor}},
				Algorithm: rlschema.Algorithm{TokenBucket: &rlschema.TokenBucket{Capacity: 5, RefillRate: 1}},
			})

			body := baseBody()
			body["request"].(map[string]interface{})["labels"] = map[string]string{"team": "acme"}
			code, raw := postDryRun(t, tu, body, aschema.AllPermissions())
			require.Equal(t, http.StatusOK, code, string(raw))

			var resp DryRunResponseJson
			require.NoError(t, json.Unmarshal(raw, &resp))
			require.Len(t, resp.Matched, 1)
			require.Equal(t, rules[0].Id, resp.Matched[0].RateLimitId)
			require.Equal(t, "acme", resp.RequestLabelSnapshot["team"])
		})

		t.Run("observe-mode match is included", func(t *testing.T) {
			tu, done := setup(t)
			defer done()
			def := tokenBucketRule()
			def.Mode = rlschema.ModeObserve
			rules := installRules(t, tu, "root", def)

			code, raw := postDryRun(t, tu, baseBody(), aschema.AllPermissions())
			require.Equal(t, http.StatusOK, code, string(raw))

			var resp DryRunResponseJson
			require.NoError(t, json.Unmarshal(raw, &resp))
			require.Len(t, resp.Matched, 1)
			require.Equal(t, rules[0].Id, resp.Matched[0].RateLimitId)
			require.Equal(t, string(rlschema.ModeObserve), resp.Matched[0].EffectiveMode)
		})

		t.Run("namespace forbidden when caller lacks permission", func(t *testing.T) {
			tu, done := setup(t)
			defer done()
			_ = installRules(t, tu, "root", tokenBucketRule())

			code, _ := postDryRun(t, tu, baseBody(), aschema.PermissionsSingle("root.other", "rate_limits", "get"))
			require.Equal(t, http.StatusForbidden, code)
		})

		t.Run("rules outside the request namespace are filtered out", func(t *testing.T) {
			tu, done := setup(t)
			defer done()

			// Create a namespace + a rule in it that should not appear
			// in the dry-run for a sibling namespace.
			ctx := context.Background()
			_, err := tu.Core.CreateNamespace(ctx, "root.sibling", nil)
			require.NoError(t, err)
			_ = installRules(t, tu, "root.sibling", tokenBucketRule())

			code, raw := postDryRun(t, tu, baseBody(), aschema.AllPermissions())
			require.Equal(t, http.StatusOK, code, string(raw))

			var resp DryRunResponseJson
			require.NoError(t, json.Unmarshal(raw, &resp))
			// Cache held one rule in root.sibling — out of scope from
			// root, so neither matched nor not-matched.
			require.Empty(t, resp.Matched)
			require.Empty(t, resp.NotMatched)
		})

		t.Run("counter state unchanged after dry-run", func(t *testing.T) {
			tu, done := setup(t)
			defer done()

			rules := installRules(t, tu, "root", tokenBucketRule())
			require.Len(t, rules, 1)

			req := baseBody()

			// 5 dry-runs in a row should each report the same fresh
			// bucket (capacity=2, remaining=1). If Peek were
			// accidentally consuming, the 2nd-3rd run would flip to
			// would_allow=false.
			for i := 0; i < 5; i++ {
				code, raw := postDryRun(t, tu, req, aschema.AllPermissions())
				require.Equal(t, http.StatusOK, code)
				var resp DryRunResponseJson
				require.NoError(t, json.Unmarshal(raw, &resp))
				require.Len(t, resp.Matched, 1, "iteration %d", i+1)
				require.True(t, resp.Matched[0].WouldAllow, "iteration %d: bucket should remain fresh", i+1)
				require.Equal(t, 1, resp.Matched[0].Remaining, "iteration %d", i+1)
			}
		})
	})
}

// Compile-time guards: the schema package's request_type constants are the
// canonical source the routes reference. Touching these here ensures import
// hygiene if the package or constants are renamed.
var (
	_ = common.RequestTypeProxy
	_ = rlschema.ModeEnforce
	_ time.Time
)
