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

	wfcore "github.com/cschleiden/go-workflows/core"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	asynqmock "github.com/rmorlok/authproxy/internal/apasynq/mock"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core"
	connIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	httpf2 "github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/routes/key_value"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/tasks"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/rmorlok/authproxy/internal/util"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
	"github.com/stretchr/testify/require"
)

type fakeConnectorLifecycleCore struct {
	connIface.C
	disconnectOpts []connIface.ConnectorLifecycleOptions
	archiveOpts    []connIface.ConnectorLifecycleOptions
}

type connectorRoutesTestSetup struct {
	Gin           *gin.Engine
	Cfg           config.C
	AuthUtil      *auth2.AuthTestUtil
	Encrypt       encrypt.E
	LifecycleCore *fakeConnectorLifecycleCore
	Workflow      *fakeTaskWorkflowClient
	Routes        *ConnectorsRoutes
}

func (f *fakeConnectorLifecycleCore) DisconnectConnectorConnections(
	_ context.Context,
	_ apid.ID,
	opts connIface.ConnectorLifecycleOptions,
) (*tasks.TaskInfo, error) {
	f.disconnectOpts = append(f.disconnectOpts, opts)
	return tasks.FromWorkflowInstance(&wflib.Instance{
		InstanceID:  fmt.Sprintf("workflow-disconnect-%d", len(f.disconnectOpts)),
		ExecutionID: fmt.Sprintf("workflow-execution-%d", len(f.disconnectOpts)),
	}, core.WorkflowNameDisconnectConnectorConnectionsV1, string(apworkflows.DefaultQueue)), nil
}

func (f *fakeConnectorLifecycleCore) ArchiveConnector(
	_ context.Context,
	_ apid.ID,
	opts connIface.ConnectorLifecycleOptions,
) (*tasks.TaskInfo, error) {
	f.archiveOpts = append(f.archiveOpts, opts)
	return tasks.FromWorkflowInstance(&wflib.Instance{
		InstanceID:  fmt.Sprintf("workflow-archive-%d", len(f.archiveOpts)),
		ExecutionID: fmt.Sprintf("workflow-execution-%d", len(f.archiveOpts)),
	}, core.WorkflowNameArchiveConnectorV1, string(apworkflows.DefaultQueue)), nil
}

func assertWorkflowTaskPolls(
	t *testing.T,
	tu *connectorRoutesTestSetup,
	taskID string,
	workflowName string,
	workflowInstanceID string,
	workflowExecutionID string,
) {
	t.Helper()

	taskInfo, err := tasks.FromSecureEncryptedString(context.Background(), tu.Encrypt, taskID)
	require.NoError(t, err)
	require.Equal(t, tasks.TrackedViaWorkflow, taskInfo.TrackedVia)
	require.Equal(t, workflowName, taskInfo.WorkflowName)
	require.Equal(t, workflowInstanceID, taskInfo.WorkflowInstanceId)
	require.Equal(t, workflowExecutionID, taskInfo.WorkflowExecutionId)
	require.NotEqual(t, apid.Nil, taskInfo.ActorId)

	w := httptest.NewRecorder()
	req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
		http.MethodGet,
		"/tasks/"+taskID,
		nil,
		"root",
		"some-actor",
		aschema.NoPermissions(),
	)
	require.NoError(t, err)

	tu.Gin.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp TaskInfoJson
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, taskID, resp.Id)
	require.Equal(t, workflowName, resp.Type)
	require.Equal(t, TaskStateActive, resp.State)
	require.NotNil(t, tu.Workflow.requestedInstance)
	require.Equal(t, workflowInstanceID, tu.Workflow.requestedInstance.InstanceID)
	require.Equal(t, workflowExecutionID, tu.Workflow.requestedInstance.ExecutionID)
}

func redactionTestConnector() sconfig.Connector {
	return redactionTestConnectorWithSecret("client-secret")
}

func redactionTestConnectorWithSecret(secret string) sconfig.Connector {
	return sconfig.Connector{
		Id:          apid.MustParse("cxr_test0000000000001"),
		Version:     1,
		Namespace:   util.ToPtr("root"),
		DisplayName: "Secret Connector",
		Description: "Connector with a client secret",
		Auth: &cschema.Auth{InnerVal: &cschema.AuthOAuth2{
			Type:         cschema.AuthTypeOAuth2,
			ClientId:     common.NewStringValueDirectInline("client-id"),
			ClientSecret: common.NewStringValueDirectInline(secret),
			Scopes:       []cschema.Scope{},
			Authorization: cschema.AuthOauth2Authorization{
				Endpoint: "https://auth.example.com/authorize",
			},
			Token: cschema.AuthOauth2Token{
				Endpoint: "https://auth.example.com/token",
			},
		}},
	}
}

func TestParseConnectorID(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		gctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		gctx.Params = gin.Params{{Key: "id", Value: "cxr_test0000000000001"}}

		id, httpErr := parseConnectorID(gctx)
		require.Nil(t, httpErr)
		require.Equal(t, apid.MustParse("cxr_test0000000000001"), id)
	})

	t.Run("missing", func(t *testing.T) {
		gctx, _ := gin.CreateTestContext(httptest.NewRecorder())

		id, httpErr := parseConnectorID(gctx)
		require.Equal(t, apid.Nil, id)
		require.NotNil(t, httpErr)
		require.Equal(t, http.StatusBadRequest, httpErr.Status)
		require.Equal(t, "id is required", httpErr.Error())
	})

	t.Run("invalid", func(t *testing.T) {
		gctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		gctx.Params = gin.Params{{Key: "id", Value: "bad-connector"}}

		id, httpErr := parseConnectorID(gctx)
		require.Equal(t, apid.Nil, id)
		require.NotNil(t, httpErr)
		require.Equal(t, http.StatusBadRequest, httpErr.Status)
		require.Equal(t, "invalid id format", httpErr.Error())
	})
}

func TestParseConnectorVersionID(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		gctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		gctx.Params = gin.Params{
			{Key: "id", Value: "cxr_test0000000000001"},
			{Key: "version", Value: "42"},
		}

		id, httpErr := parseConnectorVersionID(gctx)
		require.Nil(t, httpErr)
		require.Equal(t, connectorVersionID{
			ConnectorID: apid.MustParse("cxr_test0000000000001"),
			Version:     42,
		}, id)
	})

	t.Run("missing version", func(t *testing.T) {
		gctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		gctx.Params = gin.Params{{Key: "id", Value: "cxr_test0000000000001"}}

		id, httpErr := parseConnectorVersionID(gctx)
		require.Equal(t, connectorVersionID{}, id)
		require.NotNil(t, httpErr)
		require.Equal(t, http.StatusBadRequest, httpErr.Status)
		require.Equal(t, "version is required", httpErr.Error())
	})

	t.Run("invalid version", func(t *testing.T) {
		gctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		gctx.Params = gin.Params{
			{Key: "id", Value: "cxr_test0000000000001"},
			{Key: "version", Value: "latest"},
		}

		id, httpErr := parseConnectorVersionID(gctx)
		require.Equal(t, connectorVersionID{}, id)
		require.NotNil(t, httpErr)
		require.Equal(t, http.StatusBadRequest, httpErr.Status)
		require.Equal(t, "failed to parse version as an integer", httpErr.Error())
	})
}

func TestConnectors(t *testing.T) {
	setup := func(t *testing.T, cfg config.C) *connectorRoutesTestSetup {
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
					Id:          apid.MustParse("cxr_test0000000000001"),
					Namespace:   util.ToPtr("root"),
					Labels:      map[string]string{"type": "test-connector"},
					DisplayName: "Test ConnectorJson",
				},
				{
					Id:          apid.MustParse("cxr_test2000000000002"),
					Namespace:   util.ToPtr("root.child"),
					Labels:      map[string]string{"type": "test-connector-2"},
					DisplayName: "Test ConnectorJson 2",
				},
			}
		}

		ctrl := gomock.NewController(t)
		ac := asynqmock.NewMockClient(ctrl)
		// Connector-version label changes enqueue a propagation task. The
		// route-level tests are not interested in the asynq side; allow any
		// number of enqueue calls and let them succeed silently.
		ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, nil)
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
		cfg, e := encrypt.NewTestEncryptService(cfg, db)
		cfg, auth, authUtil := auth2.TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)
		rs := mock.NewMockClient(ctrl)
		h := httpf2.CreateFactory(cfg, rs, nil, aplog.NewNoopLogger())
		c := core.NewCoreService(cfg, db, e, rs, h, ac, test_utils.NewTestLogger())
		require.NoError(t, c.Migrate(context.Background()))
		lifecycleCore := &fakeConnectorLifecycleCore{C: c}
		workflowClient := &fakeTaskWorkflowClient{state: wfcore.WorkflowInstanceStateActive}

		cr := NewConnectorsRoutes(cfg, auth, lifecycleCore, e)
		tr := NewTaskRoutes(cfg, auth, e, asynqmock.NewMockInspector(ctrl), workflowClient)

		r := apgin.ForTest(nil)
		cr.Register(r)
		tr.Register(r)

		return &connectorRoutesTestSetup{
			Gin:           r,
			Cfg:           cfg,
			AuthUtil:      authUtil,
			Encrypt:       e,
			LifecycleCore: lifecycleCore,
			Workflow:      workflowClient,
			Routes:        cr,
		}
	}

	t.Run("route helpers", func(t *testing.T) {
		t.Run("load connector by id", func(t *testing.T) {
			tu := setup(t, nil)

			connector, err := tu.Routes.loadConnectorByID(context.Background(), apid.MustParse("cxr_test0000000000001"))
			require.NoError(t, err)
			require.Equal(t, apid.MustParse("cxr_test0000000000001"), connector.GetId())
		})

		t.Run("load connector by id not found", func(t *testing.T) {
			tu := setup(t, nil)

			connector, err := tu.Routes.loadConnectorByID(context.Background(), apid.MustParse("cxr_nonexistent00099"))
			require.Nil(t, connector)
			require.ErrorIs(t, err, core.ErrNotFound)
		})
	})

	t.Run("connector lifecycle operations", func(t *testing.T) {
		t.Run("disconnect all unauthorized", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/connectors/cxr_test0000000000001/_disconnect_all", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("archive malformed id", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connectors/bad-connector/_archive",
				nil,
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("disconnect all forbidden without lifecycle permission", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connectors/cxr_test0000000000001/_disconnect_all",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connectors", "get"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("disconnect all forbidden with archive-only permission", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connectors/cxr_test0000000000001/_disconnect_all",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connectors", "archive"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
			require.Empty(t, tu.LifecycleCore.disconnectOpts)
		})

		t.Run("archive forbidden with disconnect-all-only permission", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connectors/cxr_test0000000000001/_archive",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connectors", "disconnect_all"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
			require.Empty(t, tu.LifecycleCore.archiveOpts)
		})

		t.Run("archive rejects invalid timeout", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connectors/cxr_test0000000000001/_archive",
				util.JsonToReader(ConnectorLifecycleRequestJson{TimeoutSeconds: util.ToPtr(int64(0))}),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connectors", "archive"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Empty(t, tu.LifecycleCore.archiveOpts)
		})

		t.Run("disconnect all starts workflow task", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connectors/cxr_test0000000000001/_disconnect_all",
				util.JsonToReader(ConnectorLifecycleRequestJson{TimeoutSeconds: util.ToPtr(int64(600))}),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connectors", "disconnect_all"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectorLifecycleResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.NotEmpty(t, resp.TaskId)
			require.Equal(t, apid.MustParse("cxr_test0000000000001"), resp.ConnectorId)
			require.Len(t, tu.LifecycleCore.disconnectOpts, 1)
			require.Equal(t, 600*time.Second, tu.LifecycleCore.disconnectOpts[0].Timeout)
			assertWorkflowTaskPolls(
				t,
				tu,
				resp.TaskId,
				core.WorkflowNameDisconnectConnectorConnectionsV1,
				"workflow-disconnect-1",
				"workflow-execution-1",
			)
		})

		t.Run("archive starts workflow task with default timeout", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/connectors/cxr_test0000000000001/_archive",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connectors", "archive"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectorLifecycleResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.NotEmpty(t, resp.TaskId)
			require.Equal(t, apid.MustParse("cxr_test0000000000001"), resp.ConnectorId)
			require.Len(t, tu.LifecycleCore.archiveOpts, 1)
			require.Equal(t, 600*time.Second, tu.LifecycleCore.archiveOpts[0].Timeout)
			assertWorkflowTaskPolls(
				t,
				tu,
				resp.TaskId,
				core.WorkflowNameArchiveConnectorV1,
				"workflow-archive-1",
				"workflow-execution-1",
			)
		})
	})

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
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/bad_notavalidid", nil, "root", "some-actor", aschema.AllPermissions())
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusBadRequest, w.Code)
			})

			t.Run("forbidden", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_test0000000000001",
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
					"/connectors/cxr_test0000000000001",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingleWithResourceIds("root.**", "connectors", "get", "cxr_test0000000000001"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ConnectorJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, apid.MustParse("cxr_test0000000000001"), resp.Id)
			})

			t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_test0000000000001",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingleWithResourceIds("root.**", "connectors", "get", "cxr_test2000000000002"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusForbidden, w.Code)
			})

			t.Run("allowed with multiple resource ids including target", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_test0000000000001",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingleWithResourceIds("root.**", "connectors", "get", "cxr_test2000000000002", "cxr_test0000000000001"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)
			})

			t.Run("valid", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_test0000000000001",
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
				require.Equal(t, apid.MustParse("cxr_test0000000000001"), resp.Id)
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
				require.Equal(t, apid.MustParse("cxr_test0000000000001"), resp.Items[0].Id)
				require.Equal(t, "Test ConnectorJson", resp.Items[0].DisplayName)
				require.Equal(t, apid.MustParse("cxr_test2000000000002"), resp.Items[1].Id)
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
				require.Equal(t, apid.MustParse("cxr_test2000000000002"), resp.Items[0].Id)
				require.Equal(t, "Test ConnectorJson 2", resp.Items[0].DisplayName)
			})

			t.Run("permission constrained namespace dropdown", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors?order=id%20asc",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.child.**", "connectors", "list"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ListConnectorsResponseJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Len(t, resp.Items, 1)
				require.Equal(t, apid.MustParse("cxr_test2000000000002"), resp.Items[0].Id)
			})

			t.Run("label filter", func(t *testing.T) {
				connectorId := apid.MustParse("cxr_test0000000000001")
				cfg := config.FromRoot(&sconfig.Root{
					Connectors: &sconfig.Connectors{
						LoadFromList: []sconfig.Connector{
							{
								Id:          apid.MustParse("cxr_test0000000000123"),
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
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/bad_notavalidid/versions/1", nil, "root", "some-actor", aschema.AllPermissions())
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusBadRequest, w.Code)
			})

			t.Run("invalid version", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/connectors/bad_notavalidid/versions/999", nil, "root", "some-actor", aschema.AllPermissions())
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusBadRequest, w.Code)
			})

			t.Run("forbidden", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_test0000000000001/versions/1",
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
					"/connectors/cxr_test0000000000001/versions/1",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingleWithResourceIds("root.**", "connectors", "list/versions", "cxr_test0000000000001"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp ConnectorVersionJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, apid.MustParse("cxr_test0000000000001"), resp.Id)
			})

			t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_test0000000000001/versions/1",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingleWithResourceIds("root.**", "connectors", "list/versions", "cxr_test2000000000002"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusForbidden, w.Code)
			})

			t.Run("valid", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_test0000000000001/versions/1",
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
				require.Equal(t, apid.MustParse("cxr_test0000000000001"), resp.Id)
			})

			t.Run("redacts connector secrets by default", func(t *testing.T) {
				tu := setup(t, config.FromRoot(&sconfig.Root{
					Connectors: &sconfig.Connectors{
						LoadFromList: []sconfig.Connector{
							redactionTestConnector(),
						},
					},
				}))

				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_test0000000000001/versions/1",
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list/versions"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, "true", w.Header().Get("X-AuthProxy-Data-Redacted"))

				var raw map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
				definition := raw["definition"].(map[string]any)
				auth := definition["auth"].(map[string]any)
				require.Equal(t, "*************", auth["client_secret"])
			})

			t.Run("replays connector secrets with secret replay permission", func(t *testing.T) {
				tu := setup(t, config.FromRoot(&sconfig.Root{
					Connectors: &sconfig.Connectors{
						LoadFromList: []sconfig.Connector{
							redactionTestConnector(),
						},
					},
				}))

				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_test0000000000001/versions/1",
					nil,
					"root",
					"some-actor",
					[]aschema.Permission{
						{
							Namespace: "root.**",
							Resources: []string{"connectors"},
							Verbs:     []string{"list/versions"},
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
				require.Empty(t, w.Header().Get("X-AuthProxy-Data-Redacted"))

				var raw map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
				definition := raw["definition"].(map[string]any)
				auth := definition["auth"].(map[string]any)
				require.Equal(t, "client-secret", auth["client_secret"])
			})
		})

		t.Run("list", func(t *testing.T) {
			tu := setup(t, nil)

			t.Run("unauthorized", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := http.NewRequest(http.MethodGet, "/connectors/cxr_test0000000000001/versions", nil)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusUnauthorized, w.Code)
			})

			t.Run("valid", func(t *testing.T) {
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_test0000000000001/versions?order=id%20asc",
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
				require.Equal(t, apid.MustParse("cxr_test0000000000001"), resp.Items[0].Id)
			})

			t.Run("namespace filter", func(t *testing.T) {
				w := httptest.NewRecorder()
				// Namespace filter doesn't actually make sense here because versions can't change namespaces
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_test0000000000001/versions?order=id%20asc&namespace=root.child",
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

		t.Run("rejects redacted placeholders in secret fields", func(t *testing.T) {
			tu := setup(t, nil)
			def := redactionTestConnectorWithSecret("***")
			def.Id = apid.Nil
			def.Version = 0
			def.Namespace = nil
			body := CreateConnectorRequestJson{
				Namespace:  "root",
				Definition: def,
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
			require.Contains(t, w.Body.String(), "redacted placeholder values")
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
			require.NotEqual(t, apid.Nil, resp.Id)
			require.Equal(t, uint64(1), resp.Version)
			require.Equal(t, "root", resp.Namespace)
			require.Equal(t, string(database.ConnectorVersionStateDraft), string(resp.State))
			require.Equal(t, "New Connector", resp.Definition.DisplayName)
			require.Equal(t, "A brand new connector", resp.Definition.Description)
			require.Equal(t, "test", resp.Labels["env"])
		})

		t.Run("rejects apxy/-prefixed labels in request body", func(t *testing.T) {
			tu := setup(t, nil)
			body := CreateConnectorRequestJson{
				Namespace: "root",
				Definition: cschema.Connector{
					DisplayName: "New Connector",
					Description: "A brand new connector",
				},
				Labels: map[string]string{"apxy/cxr/source": "config"},
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
			require.Equal(t, http.StatusBadRequest, w.Code, "API must reject apxy/-prefixed labels at the user-input boundary")
			require.Contains(t, w.Body.String(), "reserved")
		})
	})

	t.Run("update connector", func(t *testing.T) {
		connectorId := apid.MustParse("cxr_test0000000000001")

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
				"/connectors/cxr_nonexistent00099",
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
			require.Equal(t, string(database.ConnectorVersionStateDraft), string(resp.State))
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
		connectorId := apid.MustParse("cxr_test0000000000001")

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
				"/connectors/cxr_nonexistent00099/versions",
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
			require.Equal(t, string(database.ConnectorVersionStateDraft), string(resp.State))
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
		connectorId := apid.MustParse("cxr_test0000000000001")

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
				"/connectors/cxr_nonexistent00099/versions/1",
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
			require.Equal(t, string(database.ConnectorVersionStateDraft), string(resp.State))
			require.Equal(t, "Updated Draft", resp.Definition.DisplayName)
		})
	})

	t.Run("labels", func(t *testing.T) {
		connectorId := apid.MustParse("cxr_test0000000000001")

		t.Run("get labels", func(t *testing.T) {
			t.Run("unauthorized", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/connectors/%s/labels", connectorId), nil)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusUnauthorized, w.Code)
			})

			t.Run("bad uuid", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/bad-uuid/labels",
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
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_nonexistent00099/labels",
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
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/labels", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "get"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp map[string]string
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, "test-connector", resp["type"])
			})
		})

		t.Run("get label", func(t *testing.T) {
			t.Run("valid", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/labels/type", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "get"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp key_value.KeyValueJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, "type", resp.Key)
				require.Equal(t, "test-connector", resp.Value)
			})

			t.Run("label not found", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/labels/nonexistent", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "get"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNotFound, w.Code)
			})
		})

		t.Run("put label", func(t *testing.T) {
			t.Run("bad uuid", func(t *testing.T) {
				tu := setup(t, nil)
				body := key_value.PutKeyValueRequestJson{Value: "val"}
				jsonBody, _ := json.Marshal(body)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					"/connectors/bad-uuid/labels/env",
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

			t.Run("invalid key", func(t *testing.T) {
				tu := setup(t, nil)
				body := key_value.PutKeyValueRequestJson{Value: "val"}
				jsonBody, _ := json.Marshal(body)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					fmt.Sprintf("/connectors/%s/labels/!!invalid!!", connectorId),
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

			t.Run("not found", func(t *testing.T) {
				tu := setup(t, nil)
				body := key_value.PutKeyValueRequestJson{Value: "val"}
				jsonBody, _ := json.Marshal(body)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					"/connectors/cxr_nonexistent00099/labels/env",
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

			t.Run("valid - creates draft and sets label", func(t *testing.T) {
				tu := setup(t, nil)
				body := key_value.PutKeyValueRequestJson{Value: "production"}
				jsonBody, _ := json.Marshal(body)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					fmt.Sprintf("/connectors/%s/labels/env", connectorId),
					bytes.NewReader(jsonBody),
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

				// Verify the draft version has both the new label and existing labels
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/2", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)
				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var versionResp ConnectorVersionJson
				err = json.Unmarshal(w.Body.Bytes(), &versionResp)
				require.NoError(t, err)
				require.Equal(t, "production", versionResp.Labels["env"])
				require.Equal(t, "test-connector", versionResp.Labels["type"])
			})
		})

		t.Run("delete label", func(t *testing.T) {
			t.Run("not found returns 204", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					"/connectors/cxr_nonexistent00099/labels/env",
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNoContent, w.Code)
			})

			t.Run("valid - creates draft and removes label", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					fmt.Sprintf("/connectors/%s/labels/type", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNoContent, w.Code)

				// Verify the draft version no longer has the label
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/2", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)
				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var versionResp ConnectorVersionJson
				err = json.Unmarshal(w.Body.Bytes(), &versionResp)
				require.NoError(t, err)
				_, exists := versionResp.Labels["type"]
				require.False(t, exists)
			})
		})
	})

	t.Run("version labels", func(t *testing.T) {
		connectorId := apid.MustParse("cxr_test0000000000001")

		t.Run("get version labels", func(t *testing.T) {
			t.Run("version not found", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/999/labels", connectorId),
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
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/1/labels", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list/versions"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp map[string]string
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, "test-connector", resp["type"])
			})
		})

		t.Run("get version label", func(t *testing.T) {
			t.Run("valid", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/1/labels/type", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list/versions"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp key_value.KeyValueJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, "type", resp.Key)
				require.Equal(t, "test-connector", resp.Value)
			})

			t.Run("label not found", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/1/labels/nonexistent", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list/versions"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNotFound, w.Code)
			})
		})

		t.Run("put version label", func(t *testing.T) {
			t.Run("conflict - not a draft", func(t *testing.T) {
				tu := setup(t, nil)
				body := key_value.PutKeyValueRequestJson{Value: "val"}
				jsonBody, _ := json.Marshal(body)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					fmt.Sprintf("/connectors/%s/versions/1/labels/env", connectorId),
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

			t.Run("valid - on draft version", func(t *testing.T) {
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

				// Put a label on the draft version
				body := key_value.PutKeyValueRequestJson{Value: "staging"}
				jsonBody, _ := json.Marshal(body)
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					fmt.Sprintf("/connectors/%s/versions/%d/labels/env", connectorId, draftVersion),
					bytes.NewReader(jsonBody),
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
		})

		t.Run("delete version label", func(t *testing.T) {
			t.Run("conflict - not a draft", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					fmt.Sprintf("/connectors/%s/versions/1/labels/type", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusConflict, w.Code)
			})

			t.Run("not found returns 204", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					"/connectors/cxr_nonexistent00099/versions/999/labels/env",
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNoContent, w.Code)
			})

			t.Run("valid - on draft version", func(t *testing.T) {
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

				// Delete a label from the draft version
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					fmt.Sprintf("/connectors/%s/versions/%d/labels/type", connectorId, draftVersion),
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNoContent, w.Code)

				// Verify the label is gone
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/%d/labels", connectorId, draftVersion),
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)
				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var labels map[string]string
				err = json.Unmarshal(w.Body.Bytes(), &labels)
				require.NoError(t, err)
				_, exists := labels["type"]
				require.False(t, exists)
			})
		})
	})

	t.Run("force connector version state", func(t *testing.T) {
		connectorId := "cxr_test0000000000001"

		t.Run("unauthorized", func(t *testing.T) {
			tu := setup(t, nil)
			w := httptest.NewRecorder()
			req, err := http.NewRequest(
				http.MethodPut,
				fmt.Sprintf("/connectors/%s/versions/1/_force_state", connectorId),
				util.JsonToReader(ForceConnectorVersionStateRequestJson{State: string(database.ConnectorVersionStateArchived)}),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("invalid uuid", func(t *testing.T) {
			tu := setup(t, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				"/connectors/bad-uuid/versions/1/_force_state",
				util.JsonToReader(ForceConnectorVersionStateRequestJson{State: string(database.ConnectorVersionStateArchived)}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			tu := setup(t, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/connectors/%s/versions/99/_force_state", connectorId),
				util.JsonToReader(ForceConnectorVersionStateRequestJson{State: string(database.ConnectorVersionStateArchived)}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid state change", func(t *testing.T) {
			tu := setup(t, nil)
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/connectors/%s/versions/1/_force_state", connectorId),
				util.JsonToReader(ForceConnectorVersionStateRequestJson{State: string(database.ConnectorVersionStateArchived)}),
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connectors", "force_state"),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectorVersionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, string(database.ConnectorVersionStateArchived), string(resp.State))
		})

		t.Run("already in desired state", func(t *testing.T) {
			tu := setup(t, nil)
			// The connector starts in primary state
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPut,
				fmt.Sprintf("/connectors/%s/versions/1/_force_state", connectorId),
				util.JsonToReader(ForceConnectorVersionStateRequestJson{State: string(database.ConnectorVersionStatePrimary)}),
				"root",
				"some-actor",
				aschema.AllPermissions(),
			)
			require.NoError(t, err)
			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ConnectorVersionJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, string(database.ConnectorVersionStatePrimary), string(resp.State))
		})
	})

	t.Run("annotations", func(t *testing.T) {
		connectorId := apid.MustParse("cxr_test0000000000001")

		t.Run("get annotations", func(t *testing.T) {
			t.Run("unauthorized", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/connectors/%s/annotations", connectorId), nil)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusUnauthorized, w.Code)
			})

			t.Run("not found", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					"/connectors/cxr_nonexistent00099/annotations",
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
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/annotations", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "get"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp map[string]string
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.NotNil(t, resp)
			})
		})

		t.Run("get annotation", func(t *testing.T) {
			t.Run("valid", func(t *testing.T) {
				tu := setup(t, nil)

				// First create a connector with annotations via POST
				createBody, _ := json.Marshal(CreateConnectorRequestJson{
					Namespace:   "root",
					Definition:  cschema.Connector{DisplayName: "Annotated Connector"},
					Annotations: map[string]string{"my-annotation": "some-value"},
				})
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPost,
					"/connectors",
					bytes.NewReader(createBody),
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusCreated, w.Code)

				var created ConnectorVersionJson
				err = json.Unmarshal(w.Body.Bytes(), &created)
				require.NoError(t, err)

				// Force the connector version to primary so it is visible via the connector-level routes
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					fmt.Sprintf("/connectors/%s/versions/%d/_force_state", created.Id, created.Version),
					util.JsonToReader(ForceConnectorVersionStateRequestJson{State: string(database.ConnectorVersionStatePrimary)}),
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)
				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				// Now get the annotation
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/annotations/my-annotation", created.Id),
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "get"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp key_value.KeyValueJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, "my-annotation", resp.Key)
				require.Equal(t, "some-value", resp.Value)
			})

			t.Run("annotation not found", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/annotations/nonexistent", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "get"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNotFound, w.Code)
			})
		})

		t.Run("put annotation", func(t *testing.T) {
			t.Run("valid", func(t *testing.T) {
				tu := setup(t, nil)
				body := key_value.PutKeyValueRequestJson{Value: "production"}
				jsonBody, _ := json.Marshal(body)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					fmt.Sprintf("/connectors/%s/annotations/env", connectorId),
					bytes.NewReader(jsonBody),
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
			})

			t.Run("valid - creates draft and sets annotation", func(t *testing.T) {
				tu := setup(t, nil)
				body := key_value.PutKeyValueRequestJson{Value: "my-description"}
				jsonBody, _ := json.Marshal(body)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					fmt.Sprintf("/connectors/%s/annotations/description", connectorId),
					bytes.NewReader(jsonBody),
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
				require.Equal(t, "description", resp.Key)
				require.Equal(t, "my-description", resp.Value)

				// Verify the draft version has the annotation
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/2", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)
				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var versionResp ConnectorVersionJson
				err = json.Unmarshal(w.Body.Bytes(), &versionResp)
				require.NoError(t, err)
				require.Equal(t, "my-description", versionResp.Annotations["description"])
			})
		})

		t.Run("delete annotation", func(t *testing.T) {
			t.Run("valid", func(t *testing.T) {
				tu := setup(t, nil)

				// First put an annotation
				body := key_value.PutKeyValueRequestJson{Value: "to-delete"}
				jsonBody, _ := json.Marshal(body)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					fmt.Sprintf("/connectors/%s/annotations/temp", connectorId),
					bytes.NewReader(jsonBody),
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				// Now delete it
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					fmt.Sprintf("/connectors/%s/annotations/temp", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNoContent, w.Code)
			})

			t.Run("valid - creates draft and removes annotation", func(t *testing.T) {
				tu := setup(t, nil)

				// First put an annotation so it exists (this creates draft version 2)
				body := key_value.PutKeyValueRequestJson{Value: "will-be-removed"}
				jsonBody, _ := json.Marshal(body)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					fmt.Sprintf("/connectors/%s/annotations/removeme", connectorId),
					bytes.NewReader(jsonBody),
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				// Delete the annotation (reuses draft version 2)
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					fmt.Sprintf("/connectors/%s/annotations/removeme", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNoContent, w.Code)

				// Verify the draft version no longer has the annotation
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/2", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)
				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var versionResp ConnectorVersionJson
				err = json.Unmarshal(w.Body.Bytes(), &versionResp)
				require.NoError(t, err)
				_, exists := versionResp.Annotations["removeme"]
				require.False(t, exists)
			})
		})
	})

	t.Run("version annotations", func(t *testing.T) {
		connectorId := apid.MustParse("cxr_test0000000000001")

		t.Run("get version annotations", func(t *testing.T) {
			t.Run("valid", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/1/annotations", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list/versions"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp map[string]string
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.NotNil(t, resp)
			})
		})

		t.Run("get version annotation", func(t *testing.T) {
			t.Run("valid", func(t *testing.T) {
				tu := setup(t, nil)

				// First create a draft version and put an annotation on it
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

				// Put an annotation on the draft
				body := key_value.PutKeyValueRequestJson{Value: "draft-value"}
				jsonBody, _ := json.Marshal(body)
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					fmt.Sprintf("/connectors/%s/versions/%d/annotations/info", connectorId, draftVersion),
					bytes.NewReader(jsonBody),
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				// Now get the annotation
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/%d/annotations/info", connectorId, draftVersion),
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list/versions"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var resp key_value.KeyValueJson
				err = json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				require.Equal(t, "info", resp.Key)
				require.Equal(t, "draft-value", resp.Value)
			})

			t.Run("annotation not found", func(t *testing.T) {
				tu := setup(t, nil)
				w := httptest.NewRecorder()
				req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/1/annotations/nonexistent", connectorId),
					nil,
					"root",
					"some-actor",
					aschema.PermissionsSingle("root.**", "connectors", "list/versions"),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNotFound, w.Code)
			})
		})

		t.Run("put version annotation", func(t *testing.T) {
			t.Run("valid", func(t *testing.T) {
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

				// Put an annotation on the draft version
				body := key_value.PutKeyValueRequestJson{Value: "staging"}
				jsonBody, _ := json.Marshal(body)
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					fmt.Sprintf("/connectors/%s/versions/%d/annotations/env", connectorId, draftVersion),
					bytes.NewReader(jsonBody),
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
		})

		t.Run("delete version annotation", func(t *testing.T) {
			t.Run("valid", func(t *testing.T) {
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

				// Put an annotation on the draft
				body := key_value.PutKeyValueRequestJson{Value: "to-delete"}
				jsonBody, _ := json.Marshal(body)
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodPut,
					fmt.Sprintf("/connectors/%s/versions/%d/annotations/temp", connectorId, draftVersion),
					bytes.NewReader(jsonBody),
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				// Delete the annotation from the draft version
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodDelete,
					fmt.Sprintf("/connectors/%s/versions/%d/annotations/temp", connectorId, draftVersion),
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusNoContent, w.Code)

				// Verify the annotation is gone
				w = httptest.NewRecorder()
				req, err = tu.AuthUtil.NewSignedRequestForActorExternalId(
					http.MethodGet,
					fmt.Sprintf("/connectors/%s/versions/%d/annotations", connectorId, draftVersion),
					nil,
					"root",
					"some-actor",
					aschema.AllPermissions(),
				)
				require.NoError(t, err)
				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)

				var annotations map[string]string
				err = json.Unmarshal(w.Body.Bytes(), &annotations)
				require.NoError(t, err)
				_, exists := annotations["temp"]
				require.False(t, exists)
			})
		})
	})
}
