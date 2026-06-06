package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/client"
	wfcore "github.com/cschleiden/go-workflows/core"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/rmorlok/authproxy/internal/apgin"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apasynq/mock"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/tasks"
	"github.com/stretchr/testify/require"
)

type fakeTaskWorkflowClient struct {
	state wfcore.WorkflowInstanceState
	err   error

	requestedInstance *wflib.Instance
}

func (f *fakeTaskWorkflowClient) CreateWorkflowInstance(ctx context.Context, options client.WorkflowInstanceOptions, workflow wflib.Workflow, args ...any) (*wflib.Instance, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTaskWorkflowClient) GetWorkflowInstanceState(ctx context.Context, instance *wflib.Instance) (wfcore.WorkflowInstanceState, error) {
	f.requestedInstance = instance
	return f.state, f.err
}

func TestTasks(t *testing.T) {
	type TestSetup struct {
		Gin            *gin.Engine
		Cfg            config.C
		AuthUtil       *auth2.AuthTestUtil
		MockInspector  *mock.MockInspector
		WorkflowClient *fakeTaskWorkflowClient
		EncryptService encrypt.E
	}

	setup := func(t *testing.T, cfg config.C) *TestSetup {
		if cfg == nil {
			cfg = config.FromRoot(&sconfig.Root{})
		}

		ctrl := gomock.NewController(t)
		mockInspector := mock.NewMockInspector(ctrl)
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
		// Use fake encryption service with doBase64Encode set to false
		e := encrypt.NewFakeEncryptService(false)
		cfg, auth, authUtil := auth2.TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)
		workflowClient := &fakeTaskWorkflowClient{state: wfcore.WorkflowInstanceStateActive}

		tr := NewTaskRoutes(cfg, auth, e, mockInspector, workflowClient)

		r := apgin.ForTest(nil)
		tr.Register(r)

		return &TestSetup{
			Gin:            r,
			Cfg:            cfg,
			AuthUtil:       authUtil,
			MockInspector:  mockInspector,
			WorkflowClient: workflowClient,
			EncryptService: e,
		}
	}

	t.Run("get task", func(t *testing.T) {
		tu := setup(t, nil)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/tasks/encrypted-task-info", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("invalid encrypted task info", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/tasks/invalid-encrypted-task-info", nil, "root", "some-actor", aschema.NoPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("unauthorized actor", func(t *testing.T) {
			// Create a valid TaskInfo
			taskInfo := &tasks.TaskInfo{
				TrackedVia: tasks.TrackedViaAsynq,
				AsynqId:    "test-id",
				AsynqQueue: "test-queue",
				AsynqType:  "test-type",
				ActorId:    apid.New(apid.PrefixActor),
			}

			// Encrypt the TaskInfo
			ctx := context.Background()
			encryptedTaskInfo, err := taskInfo.ToSecureEncryptedString(ctx, tu.EncryptService)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/tasks/"+encryptedTaskInfo, nil, "root", "unauthorized-actor", aschema.NoPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("task not found", func(t *testing.T) {
			// Create a valid TaskInfo
			taskInfo := &tasks.TaskInfo{
				TrackedVia: tasks.TrackedViaAsynq,
				AsynqId:    "test-id",
				AsynqQueue: "test-queue",
				AsynqType:  "test-type",
			}

			// Encrypt the TaskInfo
			ctx := context.Background()
			encryptedTaskInfo, err := taskInfo.ToSecureEncryptedString(ctx, tu.EncryptService)
			require.NoError(t, err)

			// Mock the inspector to return ErrTaskNotFound
			tu.MockInspector.EXPECT().
				GetTaskInfo(taskInfo.AsynqQueue, taskInfo.AsynqId).
				Return(nil, asynq.ErrTaskNotFound)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/tasks/"+encryptedTaskInfo, nil, "root", "some-actor", aschema.NoPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("inspector error", func(t *testing.T) {
			// Create a valid TaskInfo
			taskInfo := &tasks.TaskInfo{
				TrackedVia: tasks.TrackedViaAsynq,
				AsynqId:    "test-id",
				AsynqQueue: "test-queue",
				AsynqType:  "test-type",
			}

			// Encrypt the TaskInfo
			ctx := context.Background()
			encryptedTaskInfo, err := taskInfo.ToSecureEncryptedString(ctx, tu.EncryptService)
			require.NoError(t, err)

			// Mock the inspector to return an error
			tu.MockInspector.EXPECT().
				GetTaskInfo(taskInfo.AsynqQueue, taskInfo.AsynqId).
				Return(nil, errors.New("inspector error"))

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/tasks/"+encryptedTaskInfo, nil, "root", "some-actor", aschema.NoPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusInternalServerError, w.Code)
		})

		t.Run("invalid tracked via", func(t *testing.T) {
			// Create an invalid TaskInfo (not tracked via asynq)
			taskInfo := &tasks.TaskInfo{
				TrackedVia: "invalid",
				AsynqId:    "test-id",
				AsynqQueue: "test-queue",
				AsynqType:  "test-type",
			}

			// Encrypt the TaskInfo
			ctx := context.Background()
			encryptedTaskInfo, err := taskInfo.ToSecureEncryptedString(ctx, tu.EncryptService)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/tasks/"+encryptedTaskInfo, nil, "root", "some-actor", aschema.NoPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusInternalServerError, w.Code)
		})

		t.Run("missing asynq data", func(t *testing.T) {
			// Create a TaskInfo with missing asynq data
			taskInfo := &tasks.TaskInfo{
				TrackedVia: tasks.TrackedViaAsynq,
				// Missing AsynqId and AsynqQueue
			}

			// Encrypt the TaskInfo
			ctx := context.Background()
			encryptedTaskInfo, err := taskInfo.ToSecureEncryptedString(ctx, tu.EncryptService)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/tasks/"+encryptedTaskInfo, nil, "root", "some-actor", aschema.NoPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusInternalServerError, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			// Create a valid TaskInfo
			taskInfo := &tasks.TaskInfo{
				TrackedVia: tasks.TrackedViaAsynq,
				AsynqId:    "test-id",
				AsynqQueue: "test-queue",
				AsynqType:  "test-type",
			}

			// Encrypt the TaskInfo
			ctx := context.Background()
			encryptedTaskInfo, err := taskInfo.ToSecureEncryptedString(ctx, tu.EncryptService)
			require.NoError(t, err)

			// Create a mock asynq.TaskInfo
			now := time.Now()
			asynqTaskInfo := &asynq.TaskInfo{
				ID:           "test-id",
				Queue:        "test-queue",
				Type:         "test-type",
				State:        asynq.TaskStateCompleted,
				CompletedAt:  now,
				LastFailedAt: now.Add(-time.Hour), // Earlier than CompletedAt
			}

			// Mock the inspector to return the task info
			tu.MockInspector.EXPECT().
				GetTaskInfo(taskInfo.AsynqQueue, taskInfo.AsynqId).
				Return(asynqTaskInfo, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/tasks/"+encryptedTaskInfo, nil, "root", "some-actor", aschema.NoPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp TaskInfoJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, encryptedTaskInfo, resp.Id)
			require.Equal(t, "test-type", resp.Type)
			require.Equal(t, string(TaskStateCompleted), string(resp.State))
			require.Equal(t, now.UTC().Format(time.RFC3339), resp.UpdatedAt.UTC().Format(time.RFC3339))
		})

		t.Run("success with retry state", func(t *testing.T) {
			// Create a valid TaskInfo
			taskInfo := &tasks.TaskInfo{
				TrackedVia: tasks.TrackedViaAsynq,
				AsynqId:    "test-id",
				AsynqQueue: "test-queue",
				AsynqType:  "test-type",
			}

			// Encrypt the TaskInfo
			ctx := context.Background()
			encryptedTaskInfo, err := taskInfo.ToSecureEncryptedString(ctx, tu.EncryptService)
			require.NoError(t, err)

			// Create a mock asynq.TaskInfo with retry state
			now := time.Now()
			asynqTaskInfo := &asynq.TaskInfo{
				ID:           "test-id",
				Queue:        "test-queue",
				Type:         "test-type",
				State:        asynq.TaskStateRetry,
				LastFailedAt: now,
				CompletedAt:  now.Add(-time.Hour), // Earlier than LastFailedAt
			}

			// Mock the inspector to return the task info
			tu.MockInspector.EXPECT().
				GetTaskInfo(taskInfo.AsynqQueue, taskInfo.AsynqId).
				Return(asynqTaskInfo, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/tasks/"+encryptedTaskInfo, nil, "root", "some-actor", aschema.NoPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp TaskInfoJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, encryptedTaskInfo, resp.Id)
			require.Equal(t, "test-type", resp.Type)
			require.Equal(t, string(TaskStateRetry), string(resp.State))
			require.Equal(t, now.UTC().Format(time.RFC3339), resp.UpdatedAt.UTC().Format(time.RFC3339))
		})

		t.Run("workflow success", func(t *testing.T) {
			tu := setup(t, nil)
			taskInfo := &tasks.TaskInfo{
				TrackedVia:          tasks.TrackedViaWorkflow,
				WorkflowInstanceId:  "workflow-instance-id",
				WorkflowExecutionId: "workflow-execution-id",
				WorkflowName:        "core.connection.disconnect.v1",
				WorkflowQueue:       "default",
			}

			ctx := context.Background()
			encryptedTaskInfo, err := taskInfo.ToSecureEncryptedString(ctx, tu.EncryptService)
			require.NoError(t, err)

			tu.WorkflowClient.state = wfcore.WorkflowInstanceStateFinished

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/tasks/"+encryptedTaskInfo, nil, "root", "some-actor", aschema.NoPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp TaskInfoJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, encryptedTaskInfo, resp.Id)
			require.Equal(t, "core.connection.disconnect.v1", resp.Type)
			require.Equal(t, string(TaskStateCompleted), string(resp.State))
			require.Equal(t, "workflow-instance-id", tu.WorkflowClient.requestedInstance.InstanceID)
			require.Equal(t, "workflow-execution-id", tu.WorkflowClient.requestedInstance.ExecutionID)
		})

		t.Run("workflow not found", func(t *testing.T) {
			tu := setup(t, nil)
			taskInfo := &tasks.TaskInfo{
				TrackedVia:          tasks.TrackedViaWorkflow,
				WorkflowInstanceId:  "workflow-instance-id",
				WorkflowExecutionId: "workflow-execution-id",
				WorkflowName:        "core.connection.disconnect.v1",
			}

			ctx := context.Background()
			encryptedTaskInfo, err := taskInfo.ToSecureEncryptedString(ctx, tu.EncryptService)
			require.NoError(t, err)

			tu.WorkflowClient.err = backend.ErrInstanceNotFound

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(http.MethodGet, "/tasks/"+encryptedTaskInfo, nil, "root", "some-actor", aschema.NoPermissions())
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})
	})
}
