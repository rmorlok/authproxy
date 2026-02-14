package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	asynqmock "github.com/rmorlok/authproxy/internal/apasynq/mock"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

func TestConnectors(t *testing.T) {
	type TestSetup struct {
		Gin      *gin.Engine
		Cfg      config.C
		AuthUtil *auth2.AuthTestUtil
	}

	setup := func(t *testing.T, cfg config.C) *TestSetup {
		if cfg == nil {
			cfg = config.FromRoot(&sconfig.Root{
				Connectors: &sconfig.Connectors{
					LoadFromList: []sconfig.Connector{},
				},
			})
		}

		root := cfg.GetRoot()
		if root == nil {
			panic("No root in config")
		}

		if len(root.Connectors.LoadFromList) == 0 {
			root.Connectors.LoadFromList = []sconfig.Connector{
				{
					Id:          uuid.MustParse("10000000-0000-0000-0000-000000000001"),
					Namespace:   util.ToPtr("root"),
					Labels:      map[string]string{"type": "test-connector"},
					DisplayName: "Test ConnectorJson",
				},
				{
					Id:          uuid.MustParse("20000000-0000-0000-0000-000000000002"),
					Namespace:   util.ToPtr("root.child"),
					Labels:      map[string]string{"type": "test-connector-2"},
					DisplayName: "Test ConnectorJson 2",
				},
			}
		}

		ctrl := gomock.NewController(t)
		ac := asynqmock.NewMockClient(ctrl)
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
		cfg, e := encrypt.NewTestEncryptService(cfg, db)
		cfg, auth, authUtil := auth2.TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)
		rs := mock.NewMockClient(ctrl)
		h := httpf2.CreateFactory(cfg, rs, aplog.NewNoopLogger())
		c := core.NewCoreService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
		require.NoError(t, c.Migrate(context.Background()))

		cr := NewConnectorsRoutes(cfg, auth, c)

		r := api_common.GinForTest(nil)
		cr.Register(r)

		return &TestSetup{
			Gin:      r,
			Cfg:      cfg,
			AuthUtil: authUtil,
		}
	}

	t.Run("connectors", func(t *testing.T) {
		t.Run("get", func(t *testing.T) {
			tu := setup(t, nil)

			t.Run("unauthorized", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := http.NewRequest(http.MethodGet, "/connectors/test-connector", nil)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusUnauthorized, w.Code)
			})

			t.Run("malformed id", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/bad-connector", nil, "root", "some-actor", aschema.AllPermissions())
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusBadRequest, w.Code)
			})

			t.Run("invalid id", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/99999999-0000-0000-0000-000000000001", nil, "root", "some-actor", aschema.AllPermissions())
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNotFound, w.Code)
			})

			t.Run("forbidden", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/10000000-0000-0000-0000-000000000001",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "actors", "get"), // Wrong resource
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusForbidden, w.Code)
			})

			t.Run("allowed with matching resource id permission", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/10000000-0000-0000-0000-000000000001",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingleWithResourceIds("root.**", "connectors", "get", "10000000-0000-0000-0000-000000000001"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ConnectorJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, uuid.MustParse("10000000-0000-0000-0000-000000000001"), resp.Id)
			})

			t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/10000000-0000-0000-0000-000000000001",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingleWithResourceIds("root.**", "connectors", "get", "20000000-0000-0000-0000-000000000002"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusForbidden, w.Code)
			})

			t.Run("allowed with multiple resource ids including target", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/10000000-0000-0000-0000-000000000001",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingleWithResourceIds("root.**", "connectors", "get", "20000000-0000-0000-0000-000000000002", "10000000-0000-0000-0000-000000000001"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)
			})

			t.Run("valid", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/10000000-0000-0000-0000-000000000001",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "get"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ConnectorJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, uuid.MustParse("10000000-0000-0000-0000-000000000001"), resp.Id)
				require.Equal(t, "Test ConnectorJson", resp.DisplayName)
			})
		})

		t.Run("list", func(t *testing.T) {
			tu := setup(t, nil)

			t.Run("unauthorized", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := http.NewRequest(http.MethodGet, "/connectors", nil)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusUnauthorized, w.Code)
			})

			t.Run("forbidden", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors?order=id%20asc",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "delete"), // Wrong verb
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusForbidden, w.Code)
			})

			t.Run("valid", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors?order=id%20asc",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ListConnectorsResponseJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Len(t, resp.Items, 2)
				require.Equal(t, uuid.MustParse("10000000-0000-0000-0000-000000000001"), resp.Items[0].Id)
				require.Equal(t, "Test ConnectorJson", resp.Items[0].DisplayName)
				require.Equal(t, uuid.MustParse("20000000-0000-0000-0000-000000000002"), resp.Items[1].Id)
				require.Equal(t, "Test ConnectorJson 2", resp.Items[1].DisplayName)
			})

			t.Run("namespace filter", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors?order=id%20asc&namespace=root.child",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ListConnectorsResponseJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Len(t, resp.Items, 1)
				require.Equal(t, uuid.MustParse("20000000-0000-0000-0000-000000000002"), resp.Items[0].Id)
				require.Equal(t, "Test ConnectorJson 2", resp.Items[0].DisplayName)
			})

			t.Run("label filter", func(t *testing.T) {
				connectorId := uuid.MustParse("10000000-0000-0000-0000-000000000001")
				cfg := config.FromRoot(&sconfig.Root{
					Connectors: &sconfig.Connectors{
						LoadFromList: []sconfig.Connector{
							{
								Id:          uuid.MustParse("10000000-0000-0000-0000-000000000123"),
								Version:     1,
								Labels:      map[string]string{"type": "test-connector", "env": "dev"},
								DisplayName: "Test Connector",
							},
							{
								Id:          connectorId,
								Version:     1,
								Labels:      map[string]string{"type": "test-connector", "env": "prod"},
								DisplayName: "Test Connector",
							},
						},
					},
				})
				tu := setup(t, cfg)

				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors?label_selector=env%3Dprod",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ListConnectorsResponseJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Len(t, resp.Items, 1)
				require.Equal(t, connectorId, resp.Items[0].Id)
				require.Equal(t, "prod", resp.Items[0].Labels["env"])
			})
		})
	})

	t.Run("versions", func(t *testing.T) {
		t.Run("get", func(t *testing.T) {
			tu := setup(t, nil)

			t.Run("unauthorized", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := http.NewRequest(http.MethodGet, "/connectors/test-connector/versions/1", nil)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusUnauthorized, w.Code)
			})

			t.Run("malformed id", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/bad-connector/versions/1", nil, "root", "some-actor", aschema.AllPermissions())
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusBadRequest, w.Code)
			})

			t.Run("invalid id", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/99999999-0000-0000-0000-000000000001/versions/1", nil, "root", "some-actor", aschema.AllPermissions())
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNotFound, w.Code)
			})

			t.Run("invalid version", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/99999999-0000-0000-0000-000000000001/versions/999", nil, "root", "some-actor", aschema.AllPermissions())
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNotFound, w.Code)
			})

			t.Run("forbidden", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/10000000-0000-0000-0000-000000000001/versions/1",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "get"), // Wrong verb
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusForbidden, w.Code)
			})

			t.Run("allowed with matching resource id permission", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/10000000-0000-0000-0000-000000000001/versions/1",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingleWithResourceIds("root.**", "connectors", "list/versions", "10000000-0000-0000-0000-000000000001"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ConnectorVersionJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, uuid.MustParse("10000000-0000-0000-0000-000000000001"), resp.Id)
			})

			t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/10000000-0000-0000-0000-000000000001/versions/1",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingleWithResourceIds("root.**", "connectors", "list/versions", "20000000-0000-0000-0000-000000000002"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusForbidden, w.Code)
			})

			t.Run("valid", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/10000000-0000-0000-0000-000000000001/versions/1",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list/versions"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ConnectorVersionJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, uuid.MustParse("10000000-0000-0000-0000-000000000001"), resp.Id)
			})
		})

		t.Run("list", func(t *testing.T) {
			tu := setup(t, nil)

			t.Run("unauthorized", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := http.NewRequest(http.MethodGet, "/connectors/10000000-0000-0000-0000-000000000001/versions", nil)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusUnauthorized, w.Code)
			})

			t.Run("valid", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/10000000-0000-0000-0000-000000000001/versions?order=id%20asc",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list/versions"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ListConnectorVersionsResponseJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Len(t, resp.Items, 1)
				require.Equal(t, uuid.MustParse("10000000-0000-0000-0000-000000000001"), resp.Items[0].Id)
			})

			t.Run("namespace filter", func(t *testing.T) {
				w := httptest.NewRecorder()
				// Namespace filter doesn't actually make sense here because versions can't change namespaces
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/10000000-0000-0000-0000-000000000001/versions?order=id%20asc&namespace=root.child",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list/versions"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ListConnectorsResponseJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Len(t, resp.Items, 0)
			})
		})
	})

	t.Run("create connector", func(t *testing.T) {
		t.Run("unauthorized", func(t *testing.T) {
			tu := setup(t, nil)
			body := CreateConnectorRequestJson{
				Namespace:  "root",
				Definition: cschema.Connector{DisplayName: "New Connector"},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/connectors", bytes.NewReader(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			tu := setup(t, nil)
			body := CreateConnectorRequestJson{
				Namespace:  "root",
				Definition: cschema.Connector{DisplayName: "New Connector"},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connectors",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connectors", "list"), // Wrong verb
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("invalid namespace", func(t *testing.T) {
			tu := setup(t, nil)
			body := CreateConnectorRequestJson{
				Namespace:  "!!invalid!!",
				Definition: cschema.Connector{DisplayName: "New Connector"},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connectors",
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

		t.Run("invalid definition", func(t *testing.T) {
			tu := setup(t, nil)
			body := CreateConnectorRequestJson{
				Namespace: "root",
				Definition: cschema.Connector{
					DisplayName: "Bad Connector",
					Probes:      []cschema.Probe{{}}, // Empty probe fails validation
				},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connectors",
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

		t.Run("valid", func(t *testing.T) {
			tu := setup(t, nil)
			body := CreateConnectorRequestJson{
				Namespace: "root",
				Definition: cschema.Connector{
					DisplayName: "New Connector",
					Description: "A brand new connector",
				},
				Labels: map[string]string{"env": "test"},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connectors",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusCreated, w.Code)

			var resp ConnectorVersionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.NotEqual(t, uuid.Nil, resp.Id)
			require.Equal(t, uint64(1), resp.Version)
			require.Equal(t, "root", resp.Namespace)
			require.Equal(t, database.ConnectorVersionStateDraft, resp.State)
			require.Equal(t, "New Connector", resp.Definition.DisplayName)
			require.Equal(t, "A brand new connector", resp.Definition.Description)
			require.Equal(t, "test", resp.Labels["env"])
		})
	})

	t.Run("update connector", func(t *testing.T) {
		connectorId := uuid.MustParse("10000000-0000-0000-0000-000000000001")

		t.Run("unauthorized", func(t *testing.T) {
			tu := setup(t, nil)
			body := UpdateConnectorRequestJson{
				Definition: &cschema.Connector{DisplayName: "Updated"},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("/connectors/%s", connectorId), bytes.NewReader(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			tu := setup(t, nil)
			body := UpdateConnectorRequestJson{
				Definition: &cschema.Connector{DisplayName: "Updated"},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connectors/99999999-0000-0000-0000-000000000099",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("invalid definition", func(t *testing.T) {
			tu := setup(t, nil)
			body := UpdateConnectorRequestJson{
				Definition: &cschema.Connector{
					DisplayName: "Bad",
					Probes:      []cschema.Probe{{}},
				},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/connectors/%s", connectorId),
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

		t.Run("valid - creates draft and updates", func(t *testing.T) {
			tu := setup(t, nil)
			body := UpdateConnectorRequestJson{
				Definition: &cschema.Connector{DisplayName: "Updated Connector"},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/connectors/%s", connectorId),
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectorVersionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, connectorId, resp.Id)
			require.Equal(t, uint64(2), resp.Version) // New draft version
			require.Equal(t, database.ConnectorVersionStateDraft, resp.State)
			require.Equal(t, "Updated Connector", resp.Definition.DisplayName)
		})

		t.Run("valid - update with labels", func(t *testing.T) {
			tu := setup(t, nil)
			newLabels := map[string]string{"env": "staging"}
			body := UpdateConnectorRequestJson{
				Definition: &cschema.Connector{DisplayName: "With Labels"},
				Labels:     &newLabels,
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/connectors/%s", connectorId),
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectorVersionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "staging", resp.Labels["env"])
		})
	})

	t.Run("create version", func(t *testing.T) {
		connectorId := uuid.MustParse("10000000-0000-0000-0000-000000000001")

		t.Run("unauthorized", func(t *testing.T) {
			tu := setup(t, nil)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("/connectors/%s/versions", connectorId), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("connector not found", func(t *testing.T) {
			tu := setup(t, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connectors/99999999-0000-0000-0000-000000000099/versions",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid - clone from latest", func(t *testing.T) {
			tu := setup(t, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				fmt.Sprintf("/connectors/%s/versions", connectorId),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusCreated, w.Code)

			var resp ConnectorVersionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, connectorId, resp.Id)
			require.Equal(t, uint64(2), resp.Version)
			require.Equal(t, database.ConnectorVersionStateDraft, resp.State)
			require.Equal(t, "Test ConnectorJson", resp.Definition.DisplayName)
		})

		t.Run("conflict - draft already exists", func(t *testing.T) {
			tu := setup(t, nil)
			// First create a draft
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				fmt.Sprintf("/connectors/%s/versions", connectorId),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusCreated, w.Code)

			// Try again - should conflict
			w = httptest.NewRecorder()
			req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				fmt.Sprintf("/connectors/%s/versions", connectorId),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusConflict, w.Code)
		})

		t.Run("invalid definition", func(t *testing.T) {
			tu := setup(t, nil)
			def := cschema.Connector{
				DisplayName: "Bad",
				Probes:      []cschema.Probe{{}},
			}
			body := CreateConnectorVersionRequestJson{
				Definition: &def,
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				fmt.Sprintf("/connectors/%s/versions", connectorId),
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

		t.Run("valid - with custom definition", func(t *testing.T) {
			tu := setup(t, nil)
			def := cschema.Connector{DisplayName: "Custom Version"}
			body := CreateConnectorVersionRequestJson{
				Definition: &def,
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				fmt.Sprintf("/connectors/%s/versions", connectorId),
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusCreated, w.Code)

			var resp ConnectorVersionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "Custom Version", resp.Definition.DisplayName)
		})
	})

	t.Run("update version", func(t *testing.T) {
		connectorId := uuid.MustParse("10000000-0000-0000-0000-000000000001")

		t.Run("unauthorized", func(t *testing.T) {
			tu := setup(t, nil)
			body := UpdateConnectorRequestJson{
				Definition: &cschema.Connector{DisplayName: "Updated"},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("/connectors/%s/versions/1", connectorId), bytes.NewReader(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			tu := setup(t, nil)
			body := UpdateConnectorRequestJson{
				Definition: &cschema.Connector{DisplayName: "Updated"},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				"/connectors/99999999-0000-0000-0000-000000000099/versions/1",
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("conflict - not a draft", func(t *testing.T) {
			tu := setup(t, nil)
			// Version 1 was migrated as primary, not draft
			body := UpdateConnectorRequestJson{
				Definition: &cschema.Connector{DisplayName: "Updated"},
			}
			jsonBody, _ := json.Marshal(body)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/connectors/%s/versions/1", connectorId),
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusConflict, w.Code)
		})

		t.Run("invalid definition", func(t *testing.T) {
			tu := setup(t, nil)

			// First create a draft version
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				fmt.Sprintf("/connectors/%s/versions", connectorId),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusCreated, w.Code)

			var createResp ConnectorVersionJson
			err = json.Unmarshal(w.Body.Bytes(), &createResp)
			require.NoError(t, err)
			draftVersion := createResp.Version

			// Try to update with invalid definition
			body := UpdateConnectorRequestJson{
				Definition: &cschema.Connector{
					DisplayName: "Bad",
					Probes:      []cschema.Probe{{}},
				},
			}
			jsonBody, _ := json.Marshal(body)
			w = httptest.NewRecorder()
			req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/connectors/%s/versions/%d", connectorId, draftVersion),
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

		t.Run("valid - update draft version", func(t *testing.T) {
			tu := setup(t, nil)

			// First create a draft version
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				fmt.Sprintf("/connectors/%s/versions", connectorId),
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusCreated, w.Code)

			var createResp ConnectorVersionJson
			err = json.Unmarshal(w.Body.Bytes(), &createResp)
			require.NoError(t, err)
			draftVersion := createResp.Version

			// Now update it
			body := UpdateConnectorRequestJson{
				Definition: &cschema.Connector{DisplayName: "Updated Draft"},
			}
			jsonBody, _ := json.Marshal(body)
			w = httptest.NewRecorder()
			req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPatch,
				fmt.Sprintf("/connectors/%s/versions/%d", connectorId, draftVersion),
				bytes.NewReader(jsonBody),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectorVersionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, connectorId, resp.Id)
			require.Equal(t, draftVersion, resp.Version)
			require.Equal(t, database.ConnectorVersionStateDraft, resp.State)
			require.Equal(t, "Updated Draft", resp.Definition.DisplayName)
		})
	})
}
