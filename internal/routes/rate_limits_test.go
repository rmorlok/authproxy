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
	apredismock "github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	rlschema "github.com/rmorlok/authproxy/internal/schema/rate_limit"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
)

func TestRateLimits(t *testing.T) {
	type TestSetup struct {
		Gin      *gin.Engine
		Cfg      config.C
		AuthUtil *auth2.AuthTestUtil
		Db       database.DB
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
		rs := apredismock.NewMockClient(ctrl)

		c := core.NewCoreService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
		require.NoError(t, c.Migrate(context.Background()))
		rlr := NewRateLimitsRoutes(cfg, auth, c)
		r := apgin.ForTest(nil)
		rlr.Register(r)

		return &TestSetup{
				Gin:      r,
				Cfg:      cfg,
				AuthUtil: authUtil,
				Db:       db,
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
}

// Compile-time guards: the schema package's request_type constants are the
// canonical source the routes reference. Touching these here ensures import
// hygiene if the package or constants are renamed.
var (
	_ = common.RequestTypeProxy
	_ = rlschema.ModeEnforce
	_ time.Time
)
