package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apgin"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/redis/go-redis/v9"
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
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/routes/key_value"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	smeta "github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func connectionActionBody(kind smeta.Kind, id apid.ID, spec any) map[string]any {
	return map[string]any{
		"apiVersion": smeta.APIVersionV1Alpha1,
		"kind":       kind,
		"metadata": map[string]any{
			"target": connectionschema.NewConnectionReference(id),
		},
		"spec": spec,
	}
}

func connectorActionBody(kind smeta.Kind, id apid.ID, generation uint64, spec any) map[string]any {
	return map[string]any{
		"apiVersion": smeta.APIVersionV1Alpha1,
		"kind":       kind,
		"metadata": map[string]any{
			"target": smeta.ObjectReference{
				APIVersion: smeta.APIVersionV1Alpha1,
				Kind:       cschema.ConnectorKind,
				ID:         id.String(),
				Generation: generation,
			},
		},
		"spec": spec,
	}
}

func connectionPatchBody(metadata map[string]any) map[string]any {
	return map[string]any{
		"apiVersion": smeta.APIVersionV1Alpha1,
		"kind":       connectionschema.ConnectionKind,
		"metadata":   metadata,
		"spec":       map[string]any{},
	}
}

func connectionSubmitActionBody(id apid.ID) map[string]any {
	return connectionActionBody(schemaapi.ConnectionSetupSubmitActionKind, id, map[string]any{
		"stepId": "setup-step",
		"data":   map[string]any{"key": "value"},
	})
}

func TestConnections(t *testing.T) {
	type TestSetup struct {
		Gin      *gin.Engine
		Cfg      config.C
		AuthUtil *auth2.AuthTestUtil
		Db       database.DB
		Encrypt  encrypt.E
	}

	connectorId := apid.MustParse("cxr_test0000000000001")
	connectorVersion := uint64(1)
	oauthConnectorId := apid.MustParse("cxr_test0000000000002")
	oauthConnectorVersion := uint64(1)

	setup := func(t *testing.T, cfg config.C) (*TestSetup, func()) {
		cfg = config.FromRoot(&sconfig.Root{
			Connectors: &sconfig.Connectors{
				LoadFromList: []sconfig.Connector{
					configuredConnectorResource(connectorId, connectorVersion, "root", map[string]string{"type": "test-connector"}, cschema.ConnectorDefinition{
						DisplayName: "Test Connector",
					}),
					configuredConnectorResource(oauthConnectorId, oauthConnectorVersion, "root", map[string]string{"type": "oauth2-connector"}, cschema.ConnectorDefinition{
						DisplayName: "OAuth2 Test Connector",
						Auth: &sconfig.Auth{InnerVal: &sconfig.AuthOAuth2{
							Type: sconfig.AuthTypeOAuth2,
						}},
					}),
				},
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
		rs.EXPECT().Incr(gomock.Any(), gomock.Any()).Return(redis.NewIntCmd(context.Background())).AnyTimes()
		c := core.NewCoreService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
		assert.NoError(t, c.Migrate(context.Background()))
		cr := NewConnectionsRoutes(cfg, auth, db, rds, c, h, e, test_utils.NewTestLogger())
		r := apgin.ForTest(nil)
		cr.Register(r)

		return &TestSetup{
				Gin:      r,
				Cfg:      cfg,
				AuthUtil: authUtil,
				Db:       db,
				Encrypt:  e,
			}, func() {
				ctrl.Finish()
			}
	}

	t.Run("get connection", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
		})
		require.NoError(t, err)
		encryptedConfiguration, err := tu.Encrypt.EncryptStringForNamespace(
			context.Background(),
			sconfig.RootNamespace,
			`{"apiKey":"secret-value","tenant":"acme"}`,
		)
		require.NoError(t, err)
		require.NoError(t, tu.Db.SetConnectionEncryptedConfiguration(context.Background(), u, &encryptedConfiguration))

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connections/"+u.String(), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+apid.New(apid.PrefixConnection).String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "list"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("invalid uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connections/"+apid.New(apid.PrefixConnection).String(), nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connections/"+u.String(), nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp connectionschema.Connection
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u.String(), resp.Metadata.ID)
			require.Equal(t, connectionschema.ConnectionStateSetup, resp.Status.Lifecycle.State)
			require.Equal(t, connectorId.String(), resp.Spec.ConnectorRef.ID)
			require.Equal(t, connectorVersion, resp.Spec.ConnectorRef.Generation)
			require.Equal(t, "************", resp.Spec.Configuration["apiKey"])
			require.Equal(t, "****", resp.Spec.Configuration["tenant"])
			require.Empty(t, w.Header().Get("X-AuthProxy-Data-Redacted"), "configuration is redacted before replay-aware serialization")
		})

		t.Run("allowed with matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+u.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "get", u.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp connectionschema.Connection
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u.String(), resp.Metadata.ID)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			otherResourceId := apid.New(apid.PrefixConnection)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+u.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "get", otherResourceId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("allowed with multiple resource ids including target", func(t *testing.T) {
			otherResourceId := apid.New(apid.PrefixConnection)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+u.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "get", otherResourceId.String(), u.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp connectionschema.Connection
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u.String(), resp.Metadata.ID)
		})
	})

	t.Run("list connections", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		now := time.Now()
		c := clock.NewFakeClock(now)
		ctx := apctx.WithClock(context.Background(), c)

		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(ctx, &database.Connection{
			Id:               u,
			Namespace:        "root",
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
		})
		require.NoError(t, err)

		now = now.Add(time.Second)
		c.SetTime(now)
		err = tu.Db.CreateConnection(ctx, &database.Connection{
			Id:               apid.New(apid.PrefixConnection),
			Namespace:        "root.child",
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
		})
		require.NoError(t, err)

		now = now.Add(time.Second)
		c.SetTime(now)
		err = tu.Db.CreateConnection(ctx, &database.Connection{
			Id:               apid.New(apid.PrefixConnection),
			Namespace:        "root.child",
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
		})
		require.NoError(t, err)

		now = now.Add(time.Second)
		c.SetTime(now)
		err = tu.Db.CreateConnection(ctx, &database.Connection{
			Id:               apid.New(apid.PrefixConnection),
			Namespace:        "root.child.grandchild",
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connections", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections?limit=50&order=created_at%20asc",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections?limit=50&order=created_at%20asc",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp schemaapi.ListConnectionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 4)
		})

		t.Run("filter to namespace", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connections?limit=50&order=created_at%20asc&namespace=root", nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp schemaapi.ListConnectionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, u.String(), resp.Items[0].Metadata.ID)
		})

		t.Run("filter to namespace matcher", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connections?limit=50&order=created_at%20asc&namespace=root.child.**", nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp schemaapi.ListConnectionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 3)
		})

		t.Run("permission constrained namespace dropdown", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections?limit=50&order=created_at%20asc",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.child.**", "connections", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp schemaapi.ListConnectionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 3)
			for _, item := range resp.Items {
				require.Contains(t, item.Metadata.Namespace, "root.child")
			}
		})

		t.Run("filter to connector id", func(t *testing.T) {
			oauthConnectionId := apid.New(apid.PrefixConnection)
			err := tu.Db.CreateConnection(ctx, &database.Connection{
				Id:               oauthConnectionId,
				Namespace:        "root",
				ConnectorId:      oauthConnectorId,
				ConnectorVersion: oauthConnectorVersion,
				State:            database.ConnectionStateSetup,
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connections?connectorId="+oauthConnectorId.String(), nil, "root", "some-actor", aschema.PermissionsSingle("root.**", "connections", "list"))
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp schemaapi.ListConnectionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, oauthConnectionId.String(), resp.Items[0].Metadata.ID)
			require.Equal(t, oauthConnectorId.String(), resp.Items[0].Spec.ConnectorRef.ID)
		})

		t.Run("invalid connector id filter", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connections?connectorId=not-an-id", nil, "root", "some-actor", aschema.PermissionsSingle("root.**", "connections", "list"))
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("filter with label_selector", func(t *testing.T) {
			connId := apid.New(apid.PrefixConnection)
			err := tu.Db.CreateConnection(ctx, &database.Connection{
				Id:               connId,
				Namespace:        "root",
				ConnectorId:      connectorId,
				ConnectorVersion: connectorVersion,
				State:            database.ConnectionStateSetup,
				Labels:           database.Labels{"env": "test-label-conn"},
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connections?labelSelector=env%3Dtest-label-conn", nil, "root", "some-actor", aschema.AllPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp schemaapi.ListConnectionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, connId.String(), resp.Items[0].Metadata.ID)
			require.Equal(t, "test-label-conn", resp.Items[0].Metadata.Labels["env"])
		})
	})

	t.Run("disconnect connection", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateConfigured,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/connections/"+u.String()+"/_disconnect", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/"+u.String()+"/_disconnect",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			otherResourceId := apid.New(apid.PrefixConnection)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/"+u.String()+"/_disconnect",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "disconnect", otherResourceId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("rejects invalid timeout", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/"+u.String()+"/_disconnect",
				util.JsonToReader(connectionActionBody(schemaapi.ConnectionDisconnectActionKind, u, schemaapi.ConnectionDisconnectSpec{TimeoutSeconds: util.ToPtr(int64(0))})),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "disconnect"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("rejects action target that does not match path", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/"+u.String()+"/_disconnect",
				util.JsonToReader(connectionActionBody(
					schemaapi.ConnectionDisconnectActionKind,
					apid.New(apid.PrefixConnection),
					schemaapi.ConnectionDisconnectSpec{},
				)),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "disconnect"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("rejects invalid json", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/"+u.String()+"/_disconnect",
				util.JsonToReader("{invalid json}"),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "disconnect"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	})

	t.Run("initiate connection", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/connections/_initiate", util.JsonToReader(map[string]interface{}{
				"connectorId": connectorId.String(),
				"returnToUrl": "https://example.com/callback",
			}))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden wrong verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/_initiate",
				util.JsonToReader(map[string]interface{}{
					"connectorId": connectorId.String(),
					"returnToUrl": "https://example.com/callback",
				}),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("namespace and name connector reference selects primary generation", func(t *testing.T) {
			body := map[string]any{
				"apiVersion": smeta.APIVersionV1Alpha1,
				"kind":       schemaapi.ConnectionInitiateActionKind,
				"metadata": map[string]any{
					"target": smeta.ObjectReference{
						APIVersion: smeta.APIVersionV1Alpha1,
						Kind:       cschema.ConnectorKind,
						Name:       scommon.ResourceName(connectorId.String()),
						Namespace:  sconfig.RootNamespace,
					},
				},
				"spec": map[string]any{
					"intoNamespace": sconfig.RootNamespace,
					"name":          "reference-created",
					"labels":        map[string]string{"team": "platform"},
					"annotations":   map[string]string{"owner": "integrations"},
					"returnToUrl":   "https://example.com/callback",
				},
			}
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/_initiate",
				util.JsonToReader(body),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			var setupAction schemaapi.ConnectionSetupAction
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &setupAction))
			connectionID := apid.MustParse(setupAction.Metadata.Target.ID)
			connection, err := tu.Db.GetConnection(t.Context(), connectionID)
			require.NoError(t, err)
			require.Equal(t, connectorId, connection.ConnectorId)
			require.Equal(t, connectorVersion, connection.ConnectorVersion)
			require.Equal(t, "platform", connection.Labels["team"])
			require.Equal(t, "integrations", connection.Annotations["owner"])
			require.NotNil(t, connection.ActorId)
		})
	})

	t.Run("update connection (PATCH)", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
			Labels:           database.Labels{"existing": "value"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, "/connections/"+u.String(), util.JsonToReader(connectionPatchBody(map[string]any{
				"labels": map[string]string{"env": "prod"},
			})))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/"+u.String(),
				util.JsonToReader(connectionPatchBody(map[string]any{
					"labels": map[string]string{"env": "prod"},
				})),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"), // Wrong verb
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/not-a-uuid",
				util.JsonToReader(connectionPatchBody(map[string]any{
					"labels": map[string]string{"env": "prod"},
				})),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/"+apid.New(apid.PrefixConnection).String(),
				util.JsonToReader(connectionPatchBody(map[string]any{
					"labels": map[string]string{"env": "prod"},
				})),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("rejects apxy/-prefixed labels in request body", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/"+u.String(),
				util.JsonToReader(connectionPatchBody(map[string]any{
					"labels": map[string]string{"apxy/cxr/source": "config"},
				})),
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

		t.Run("invalid JSON", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/"+u.String(),
				util.JsonToReader("{invalid json}"),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("success with labels", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/"+u.String(),
				util.JsonToReader(connectionPatchBody(map[string]any{
					"labels": map[string]string{"env": "production", "team": "backend"},
				})),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp connectionschema.Connection
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u.String(), resp.Metadata.ID)
			require.Equal(t, "production", resp.Metadata.Labels["env"])
			require.Equal(t, "backend", resp.Metadata.Labels["team"])
			// "existing" label should be gone since this is a full replacement
			_, exists := resp.Metadata.Labels["existing"]
			require.False(t, exists)
		})

		t.Run("success preserves state", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/"+u.String(),
				util.JsonToReader(connectionPatchBody(map[string]any{
					"labels": map[string]string{"new": "label"},
				})),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp connectionschema.Connection
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u.String(), resp.Metadata.ID)
			require.Equal(t, connectionschema.ConnectionStateSetup, resp.Status.Lifecycle.State)
		})

		t.Run("success with annotations", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connections/"+u.String(),
				util.JsonToReader(connectionPatchBody(map[string]any{
					"annotations": map[string]string{"owner": "platform"},
				})),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp connectionschema.Connection
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "platform", resp.Metadata.Annotations["owner"])
		})
	})

	t.Run("get connection labels", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
			Labels:           database.Labels{"env": "prod", "team": "backend"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connections/"+u.String()+"/labels", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/not-a-uuid/labels",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+apid.New(apid.PrefixConnection).String()+"/labels",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("success with labels", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+u.String()+"/labels",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp map[string]string
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "prod", resp["env"])
			require.Equal(t, "backend", resp["team"])
		})

		t.Run("success with empty labels", func(t *testing.T) {
			noLabelsId := apid.New(apid.PrefixConnection)
			err := tu.Db.CreateConnection(context.Background(), &database.Connection{
				Id:               noLabelsId,
				Namespace:        sconfig.RootNamespace,
				ConnectorId:      connectorId,
				ConnectorVersion: connectorVersion,
				State:            database.ConnectionStateSetup,
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+noLabelsId.String()+"/labels",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp map[string]string
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			respUser, _ := database.SplitUserAndApxyLabels(database.Labels(resp))
			require.Empty(t, respUser)
		})
	})

	t.Run("get connection label", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
			Labels:           database.Labels{"env": "staging"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connections/"+u.String()+"/labels/env", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/not-a-uuid/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("connection not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+apid.New(apid.PrefixConnection).String()+"/labels/env",
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
				"/connections/"+u.String()+"/labels/nonexistent",
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
				"/connections/"+u.String()+"/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp key_value.KeyValueJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "env", resp.Key)
			require.Equal(t, "staging", resp.Value)
		})
	})

	t.Run("put connection label", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/connections/"+u.String()+"/labels/env", util.JsonToReader(map[string]interface{}{"value": "production"}))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id", func(t *testing.T) {
			otherResourceId := apid.New(apid.PrefixConnection)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/labels/env",
				util.JsonToReader(map[string]interface{}{"value": "production"}),
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "update", otherResourceId.String()),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/not-a-uuid/labels/env",
				util.JsonToReader(map[string]interface{}{"value": "production"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+apid.New(apid.PrefixConnection).String()+"/labels/env",
				util.JsonToReader(map[string]interface{}{"value": "production"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("invalid JSON", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/labels/env",
				util.JsonToReader("{invalid json}"),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("success add new label", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/labels/env",
				util.JsonToReader(map[string]interface{}{"value": "production"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp key_value.KeyValueJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "env", resp.Key)
			require.Equal(t, "production", resp.Value)

			// Verify in database
			conn, err := tu.Db.GetConnection(context.Background(), u)
			require.NoError(t, err)
			require.Equal(t, "production", conn.Labels["env"])
		})

		t.Run("success update existing", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/labels/env",
				util.JsonToReader(map[string]interface{}{"value": "staging"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp key_value.KeyValueJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "env", resp.Key)
			require.Equal(t, "staging", resp.Value)
		})

		t.Run("success preserves other labels", func(t *testing.T) {
			// Add another label first
			_, err := tu.Db.PutConnectionLabels(context.Background(), u, map[string]string{"team": "platform"})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/labels/env",
				util.JsonToReader(map[string]interface{}{"value": "dev"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			// Verify both labels in database
			conn, err := tu.Db.GetConnection(context.Background(), u)
			require.NoError(t, err)
			require.Equal(t, "dev", conn.Labels["env"])
			require.Equal(t, "platform", conn.Labels["team"])
		})
	})

	t.Run("delete connection label", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
			Labels:           database.Labels{"env": "prod", "team": "backend"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/connections/"+u.String()+"/labels/env", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id", func(t *testing.T) {
			otherResourceId := apid.New(apid.PrefixConnection)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/connections/"+u.String()+"/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "update", otherResourceId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/connections/not-a-uuid/labels/env",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("connection not found returns 204", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/connections/"+apid.New(apid.PrefixConnection).String()+"/labels/env",
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
				"/connections/"+u.String()+"/labels/nonexistent",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("success delete", func(t *testing.T) {
			// Create a fresh connection with labels for this test
			deleteTestId := apid.New(apid.PrefixConnection)
			err := tu.Db.CreateConnection(context.Background(), &database.Connection{
				Id:               deleteTestId,
				Namespace:        sconfig.RootNamespace,
				ConnectorId:      connectorId,
				ConnectorVersion: connectorVersion,
				State:            database.ConnectionStateSetup,
				Labels:           database.Labels{"to-delete": "value", "to-keep": "value2"},
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/connections/"+deleteTestId.String()+"/labels/to-delete",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			// Verify the label is deleted but other labels remain
			conn, err := tu.Db.GetConnection(context.Background(), deleteTestId)
			require.NoError(t, err)
			_, exists := conn.Labels["to-delete"]
			require.False(t, exists)
			require.Equal(t, "value2", conn.Labels["to-keep"])
		})

		t.Run("success idempotent delete", func(t *testing.T) {
			// Create a fresh connection for idempotent test
			idempotentId := apid.New(apid.PrefixConnection)
			err := tu.Db.CreateConnection(context.Background(), &database.Connection{
				Id:               idempotentId,
				Namespace:        sconfig.RootNamespace,
				ConnectorId:      connectorId,
				ConnectorVersion: connectorVersion,
				State:            database.ConnectionStateSetup,
				Labels:           database.Labels{"label": "value"},
			})
			require.NoError(t, err)

			// Delete the label twice
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					"/connections/"+idempotentId.String()+"/labels/label",
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
			conn, err := tu.Db.GetConnection(context.Background(), idempotentId)
			require.NoError(t, err)
			_, exists := conn.Labels["label"]
			require.False(t, exists)
		})
	})

	t.Run("force connection state", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/connections/"+u.String()+"/_forceState", util.JsonToReader(connectionActionBody(
				schemaapi.ConnectionForceStateActionKind,
				u,
				schemaapi.ConnectionForceStateSpec{State: connectionschema.ConnectionStateDisconnected},
			)))
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+apid.New(apid.PrefixConnection).String()+"/_forceState",
				util.JsonToReader(connectionActionBody(schemaapi.ConnectionForceStateActionKind, u, schemaapi.ConnectionForceStateSpec{State: connectionschema.ConnectionStateDisconnected})),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"), // Wrong verb
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("invalid uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+apid.New(apid.PrefixConnection).String()+"/_forceState",
				util.JsonToReader(connectionActionBody(schemaapi.ConnectionForceStateActionKind, u, schemaapi.ConnectionForceStateSpec{State: connectionschema.ConnectionStateDisconnected})),
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
				http.MethodPut,
				"/connections/"+u.String()+"/_forceState",
				util.JsonToReader(connectionActionBody(schemaapi.ConnectionForceStateActionKind, u, schemaapi.ConnectionForceStateSpec{State: connectionschema.ConnectionStateDisconnected})),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "force_state"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp schemaapi.ConnectionForceStateAction
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u.String(), resp.Metadata.Target.ID)
			require.Equal(t, connectionschema.ConnectionStateDisconnected, resp.Status.Connection.Status.Lifecycle.State)
		})

		t.Run("allowed with matching resource id permission", func(t *testing.T) {
			// Reset state first
			err := tu.Db.SetConnectionState(context.Background(), u, database.ConnectionStateSetup)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/_forceState",
				util.JsonToReader(connectionActionBody(schemaapi.ConnectionForceStateActionKind, u, schemaapi.ConnectionForceStateSpec{State: connectionschema.ConnectionStateDisconnected})),
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "force_state", u.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp schemaapi.ConnectionForceStateAction
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, u.String(), resp.Metadata.Target.ID)
			require.Equal(t, connectionschema.ConnectionStateDisconnected, resp.Status.Connection.Status.Lifecycle.State)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			otherResourceId := apid.New(apid.PrefixConnection)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/_forceState",
				util.JsonToReader(connectionActionBody(schemaapi.ConnectionForceStateActionKind, u, schemaapi.ConnectionForceStateSpec{State: connectionschema.ConnectionStateConfigured})),
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "force_state", otherResourceId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})
	})

	t.Run("get connection annotations", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
			Annotations:      database.Annotations{"note": "important", "owner": "team-a"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connections/"+u.String()+"/annotations", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+apid.New(apid.PrefixConnection).String()+"/annotations",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("success with annotations", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+u.String()+"/annotations",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp map[string]string
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "important", resp["note"])
			require.Equal(t, "team-a", resp["owner"])
		})

		t.Run("success with empty annotations", func(t *testing.T) {
			noAnnotationsId := apid.New(apid.PrefixConnection)
			err := tu.Db.CreateConnection(context.Background(), &database.Connection{
				Id:               noAnnotationsId,
				Namespace:        sconfig.RootNamespace,
				ConnectorId:      connectorId,
				ConnectorVersion: connectorVersion,
				State:            database.ConnectionStateSetup,
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+noAnnotationsId.String()+"/annotations",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp map[string]string
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Empty(t, resp)
		})
	})

	t.Run("get connection annotation", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
			Annotations:      database.Annotations{"note": "important"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connections/"+u.String()+"/annotations/note", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+apid.New(apid.PrefixConnection).String()+"/annotations/note",
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
				"/connections/"+u.String()+"/annotations/nonexistent",
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
				"/connections/"+u.String()+"/annotations/note",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp key_value.KeyValueJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "note", resp.Key)
			require.Equal(t, "important", resp.Value)
		})
	})

	t.Run("put connection annotation", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPut, "/connections/"+u.String()+"/annotations/note", util.JsonToReader(map[string]interface{}{"value": "important"}))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id", func(t *testing.T) {
			otherResourceId := apid.New(apid.PrefixConnection)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/annotations/note",
				util.JsonToReader(map[string]interface{}{"value": "important"}),
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "update", otherResourceId.String()),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+apid.New(apid.PrefixConnection).String()+"/annotations/note",
				util.JsonToReader(map[string]interface{}{"value": "important"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("bad request invalid JSON", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/annotations/note",
				util.JsonToReader("{invalid json}"),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("success add new annotation", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/annotations/note",
				util.JsonToReader(map[string]interface{}{"value": "important"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp key_value.KeyValueJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "note", resp.Key)
			require.Equal(t, "important", resp.Value)

			// Verify in database
			conn, err := tu.Db.GetConnection(context.Background(), u)
			require.NoError(t, err)
			require.Equal(t, "important", conn.Annotations["note"])
		})

		t.Run("success preserves other annotations", func(t *testing.T) {
			// Add another annotation first
			_, err := tu.Db.PutConnectionAnnotations(context.Background(), u, map[string]string{"owner": "team-a"})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connections/"+u.String()+"/annotations/note",
				util.JsonToReader(map[string]interface{}{"value": "updated"}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			// Verify both annotations in database
			conn, err := tu.Db.GetConnection(context.Background(), u)
			require.NoError(t, err)
			require.Equal(t, "updated", conn.Annotations["note"])
			require.Equal(t, "team-a", conn.Annotations["owner"])
		})
	})

	t.Run("delete connection annotation", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()
		u := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               u,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
			Annotations:      database.Annotations{"note": "important", "owner": "team-a"},
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodDelete, "/connections/"+u.String()+"/annotations/note", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden with non-matching resource id", func(t *testing.T) {
			otherResourceId := apid.New(apid.PrefixConnection)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/connections/"+u.String()+"/annotations/note",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "connections", "update", otherResourceId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found returns 204", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/connections/"+apid.New(apid.PrefixConnection).String()+"/annotations/note",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)
		})

		t.Run("success delete", func(t *testing.T) {
			// Create a fresh connection with annotations for this test
			deleteTestId := apid.New(apid.PrefixConnection)
			err := tu.Db.CreateConnection(context.Background(), &database.Connection{
				Id:               deleteTestId,
				Namespace:        sconfig.RootNamespace,
				ConnectorId:      connectorId,
				ConnectorVersion: connectorVersion,
				State:            database.ConnectionStateSetup,
				Annotations:      database.Annotations{"to-delete": "value", "to-keep": "value2"},
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/connections/"+deleteTestId.String()+"/annotations/to-delete",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNoContent, w.Code)

			// Verify the annotation is deleted but other annotations remain
			conn, err := tu.Db.GetConnection(context.Background(), deleteTestId)
			require.NoError(t, err)
			_, exists := conn.Annotations["to-delete"]
			require.False(t, exists)
			require.Equal(t, "value2", conn.Annotations["to-keep"])
		})
	})

	t.Run("submit connection form", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		connId := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               connId,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/connections/"+connId.String()+"/_submit", util.JsonToReader(connectionSubmitActionBody(connId)))
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad request - invalid id", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/not-a-valid-id/_submit",
				util.JsonToReader(connectionSubmitActionBody(connId)),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("no active setup step returns 400", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/"+connId.String()+"/_submit",
				util.JsonToReader(connectionSubmitActionBody(connId)),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		// _submit accepts either "create" or "update" so the same endpoint serves both
		// initial setup (driven by a create-only actor) and reconfigure (update-only actor).
		t.Run("forbidden with unrelated verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/"+connId.String()+"/_submit",
				util.JsonToReader(connectionSubmitActionBody(connId)),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("create-only actor passes auth", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/"+connId.String()+"/_submit",
				util.JsonToReader(connectionSubmitActionBody(connId)),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "create"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			// Auth allowed (would otherwise be 403); request still 400 since no active setup step.
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("update-only actor passes auth", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/"+connId.String()+"/_submit",
				util.JsonToReader(connectionSubmitActionBody(connId)),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "update"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	})

	t.Run("get setup step", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		connId := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               connId,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
		})
		require.NoError(t, err)

		// _setupStep is part of the setup flow, so it accepts create or update —
		// not get, since reading the in-progress setup state is gated on the same
		// activity that produces it.
		for _, verb := range []string{"create", "update"} {
			t.Run(verb+"-only actor passes auth", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connections/"+connId.String()+"/_setupStep",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connections", verb),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.NotEqual(t, http.StatusForbidden, w.Code)
				require.NotEqual(t, http.StatusUnauthorized, w.Code)
			})
		}

		t.Run("forbidden with get-only", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+connId.String()+"/_setupStep",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})
	})

	t.Run("get data source", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		connId := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               connId,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
		})
		require.NoError(t, err)

		// _dataSource serves dynamic options for the active setup step, so it accepts
		// create or update — not get.
		for _, verb := range []string{"create", "update"} {
			t.Run(verb+"-only actor passes auth", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connections/"+connId.String()+"/_dataSource/some-source",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connections", verb),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.NotEqual(t, http.StatusForbidden, w.Code)
				require.NotEqual(t, http.StatusUnauthorized, w.Code)
			})
		}

		t.Run("forbidden with get-only", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+connId.String()+"/_dataSource/some-source",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})
	})

	t.Run("retry connection setup", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		connId := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               connId,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateSetup,
		})
		require.NoError(t, err)

		// _retry accepts create or update so it serves both the initial-setup retry path
		// and a reconfigure retry.
		for _, verb := range []string{"create", "update"} {
			t.Run(verb+"-only actor passes auth", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPost,
					"/connections/"+connId.String()+"/_retry",
					util.JsonToReader(connectionActionBody(schemaapi.ConnectionSetupRetryActionKind, connId, schemaapi.ConnectionSetupControlSpec{})),
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connections", verb),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.NotEqual(t, http.StatusForbidden, w.Code)
				require.NotEqual(t, http.StatusUnauthorized, w.Code)
			})
		}

		t.Run("forbidden with unrelated verb", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connections/"+connId.String()+"/_retry",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connections", "get"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})
	})

	t.Run("get connection scopes", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		// The default test connector has no auth definition, so it stands in for the
		// "non-OAuth2" case for this endpoint's contract check.
		nonOauthConnId := apid.New(apid.PrefixConnection)
		err := tu.Db.CreateConnection(context.Background(), &database.Connection{
			Id:               nonOauthConnId,
			Namespace:        sconfig.RootNamespace,
			ConnectorId:      connectorId,
			ConnectorVersion: connectorVersion,
			State:            database.ConnectionStateConfigured,
		})
		require.NoError(t, err)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/connections/"+nonOauthConnId.String()+"/scopes", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("bad uuid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/not-a-uuid/scopes",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("connection not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+apid.New(apid.PrefixConnection).String()+"/scopes",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("non-oauth2 connector returns 422", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+nonOauthConnId.String()+"/scopes",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		})

		t.Run("oauth2 connector returns requested and granted scopes", func(t *testing.T) {
			oauthConnId := apid.New(apid.PrefixConnection)
			err := tu.Db.CreateConnection(context.Background(), &database.Connection{
				Id:               oauthConnId,
				Namespace:        sconfig.RootNamespace,
				ConnectorId:      oauthConnectorId,
				ConnectorVersion: oauthConnectorVersion,
				State:            database.ConnectionStateConfigured,
			})
			require.NoError(t, err)

			_, err = tu.Db.InsertOAuth2Token(
				context.Background(),
				oauthConnId,
				nil,
				encfield.EncryptedField{ID: "dek_test", Data: "encrypted_refresh"},
				encfield.EncryptedField{ID: "dek_test", Data: "encrypted_access"},
				nil,
				"read write",
				"read write admin",
				nil,
			)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/connections/"+oauthConnId.String()+"/scopes",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectionScopesJson
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			assert.Equal(t, []string{"read", "write", "admin"}, resp.Requested)
			assert.Equal(t, []string{"read", "write"}, resp.Granted)
		})
	})

	t.Run("resource name API", func(t *testing.T) {
		tu, done := setup(t, nil)
		defer done()

		initiate := func(name *string, expectedStatus int) apid.ID {
			spec := map[string]any{
				"returnToUrl": "https://example.com/callback",
			}
			if name != nil {
				spec["name"] = *name
			}
			body := connectorActionBody(schemaapi.ConnectionInitiateActionKind, connectorId, connectorVersion, spec)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost, "/connections/_initiate", util.JsonToReader(body),
				"root", "some-actor", aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, expectedStatus, w.Code, w.Body.String())
			if expectedStatus != http.StatusOK {
				require.NotContains(t, w.Body.String(), "UNIQUE")
				return apid.Nil
			}
			var resp schemaapi.ConnectionSetupAction
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			return apid.MustParse(resp.Metadata.Target.ID)
		}

		customName := "production-crm"
		customID := initiate(&customName, http.StatusOK)
		defaultID := initiate(nil, http.StatusOK)
		initiate(&customName, http.StatusConflict)

		for id, expectedName := range map[apid.ID]string{customID: customName, defaultID: defaultID.String()} {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet, "/connections/"+id.String(), nil,
				"root", "some-actor", aschema.AllPermissions(),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var connection connectionschema.Connection
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &connection))
			require.Equal(t, expectedName, string(connection.Metadata.Name))
			require.Equal(t, expectedName, connection.Metadata.Labels["apxy/cxn/-/name"])
			require.NotNil(t, connection.Spec.ActorRef)
			require.Equal(t, smeta.Kind("Actor"), connection.Spec.ActorRef.Kind)
		}

		otherName := "other-connection"
		_ = initiate(&otherName, http.StatusOK)
		w := httptest.NewRecorder()
		req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPatch, "/connections/"+customID.String(), util.JsonToReader(connectionPatchBody(map[string]any{"name": "renamed-connection"})),
			"root", "some-actor", aschema.AllPermissions(),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var renamed connectionschema.Connection
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &renamed))
		require.Equal(t, "renamed-connection", string(renamed.Metadata.Name))
		require.Equal(t, "renamed-connection", renamed.Metadata.Labels["apxy/cxn/-/name"])

		w = httptest.NewRecorder()
		req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodPatch, "/connections/"+customID.String(), util.JsonToReader(connectionPatchBody(map[string]any{"name": otherName})),
			"root", "some-actor", aschema.AllPermissions(),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

		w = httptest.NewRecorder()
		req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
			http.MethodGet, "/connections?name=renamed-connection", nil,
			"root", "some-actor", aschema.AllPermissions(),
		)
		require.NoError(t, err)
		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var listed schemaapi.ListConnectionResponseJson
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
		require.Len(t, listed.Items, 1)
		require.Equal(t, customID.String(), listed.Items[0].Metadata.ID)
	})
}
